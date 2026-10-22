package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/httpapi"
)

// dbConfig reads database.connection, or the DATABASE_* variables of a
// generated Strapi project when there is no database config.
func dbConfig(c *config.Config, dir string) db.Config {
	if c.Get("database.connection") == nil {
		return dbFromEnv(c.Env)
	}
	str := func(k string) string { v, _ := c.String("database.connection." + k); return v }
	num := func(k string) int { v, _ := c.Int("database.connection." + k); return v }
	cfg := db.Config{
		Client:           str("client"),
		ConnectionString: str("connection.connectionString"),
		Host:             str("connection.host"),
		Port:             num("connection.port"),
		Database:         str("connection.database"),
		User:             str("connection.user"),
		Password:         str("connection.password"),
		Schema:           str("connection.schema"),
		Filename:         str("connection.filename"),
		PoolMin:          num("pool.min"),
		PoolMax:          num("pool.max"),
		SSLMode:          "disable",
	}
	switch ssl := c.Get("database.connection.connection.ssl").(type) {
	case bool:
		if ssl {
			cfg.SSLMode = "require"
		}
	case map[string]any:
		cfg.SSLMode = "require"
	}
	if ms := num("acquireConnectionTimeout"); ms > 0 {
		cfg.AcquireTimeout = time.Duration(ms) * time.Millisecond
	}
	cfg.Debug, _ = c.Bool("database.connection.debug")
	if cfg.Client == "sqlite" && cfg.Filename != "" && !filepath.IsAbs(cfg.Filename) {
		cfg.Filename = filepath.Join(dir, cfg.Filename)
	}
	return cfg
}

func dbFromEnv(e config.Env) db.Config {
	env := func(key, def string) string {
		if v := e[key]; v != "" {
			return v
		}
		return def
	}
	cfg := db.Config{Client: env("DATABASE_CLIENT", "sqlite")}
	if cfg.Client == "sqlite" {
		cfg.Filename = env("DATABASE_FILENAME", ".tmp/data.db")
		return cfg
	}
	if url := e["DATABASE_URL"]; url != "" {
		cfg.ConnectionString = url
		return cfg
	}
	cfg.Host = env("DATABASE_HOST", "127.0.0.1")
	cfg.Port, _ = strconv.Atoi(env("DATABASE_PORT", "5432"))
	cfg.Database = env("DATABASE_NAME", "strapi")
	cfg.User = env("DATABASE_USERNAME", "strapi")
	cfg.Password = e["DATABASE_PASSWORD"]
	cfg.SSLMode = env("DATABASE_SSL_MODE", "disable")
	cfg.Schema = e["DATABASE_SCHEMA"]
	return cfg
}

// apiConfig reads api.rest and admin.apiToken.salt.
func apiConfig(c *config.Config) httpapi.Config {
	cfg := httpapi.Config{}
	cfg.Prefix, _ = c.String("api.rest.prefix")
	cfg.DefaultLimit, _ = c.Int("api.rest.defaultLimit")
	cfg.MaxLimit, _ = c.Int("api.rest.maxLimit")
	if wc, ok := c.Bool("api.rest.withCount"); ok {
		cfg.WithCount = &wc
	}
	cfg.APITokenSalt, _ = c.String("admin.apiToken.salt")
	if cfg.APITokenSalt == "" {
		cfg.APITokenSalt = c.Env["API_TOKEN_SALT"]
	}
	return cfg
}

func start(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory (the one holding src/)")
	configDir := fs.String("config", "", "configuration directory (default <dir>/config)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	conf, err := config.Load(config.Options{Dir: *dir, ConfigDir: *configDir, Log: log})
	if err != nil {
		return err
	}
	res, md, err := builtin.Load(os.DirFS(*dir))
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		log.Warn(w)
	}
	desired, err := md.Schema()
	if err != nil {
		return err
	}
	ctx := context.Background()
	d, err := db.Open(ctx, dbConfig(conf, *dir), log)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	err = migrate.WithLock(ctx, d, func(ctx context.Context) error {
		ran, err := migrate.Run(ctx, d, migrate.Options{RunMigrations: true, Log: log})
		if err != nil {
			return err
		}
		status, err := schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: ran, Log: log})
		if err != nil {
			return err
		}
		if status == schemasync.Changed {
			for _, pivot := range []string{schemasync.ComponentTypePivot, schemasync.RelatedTypePivot} {
				if err := schemasync.RemoveOrphanMorphTypes(ctx, d, md, pivot, log); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	engine := &query.Engine{DB: d, MD: md}
	api := httpapi.New(apiConfig(conf), d, res.Set, document.New(engine, res.Set), log)
	host, _ := conf.String("server.host")
	port, _ := conf.Int("server.port")
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	srv := &http.Server{Addr: addr, Handler: api, ReadHeaderTimeout: 10 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info(fmt.Sprintf("grapi %s listening on %s", version, addr))
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-stop:
		shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
	return nil
}
