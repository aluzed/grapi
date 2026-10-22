package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/registry"
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
func apiConfig(c *config.Config, dir string) httpapi.Config {
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
	up := "plugins.users-permissions.config."
	cfg.Users.JWTSecret, _ = c.String(up + "jwtSecret")
	if cfg.Users.JWTSecret == "" {
		cfg.Users.JWTSecret = c.Env["JWT_SECRET"]
	}
	cfg.Users.JWTExpiresIn, _ = c.String(up + "jwt.expiresIn")
	if n, ok := c.Int(up + "jwt.expiresIn"); ok {
		cfg.Users.JWTExpiresIn = strconv.Itoa(n)
	}
	cfg.Users.JWTAlgorithm, _ = c.String(up + "jwt.algorithm")
	public, _ := c.String("server.dirs.public")
	if public == "" {
		public = "./public"
	}
	if !filepath.IsAbs(public) {
		public = filepath.Join(dir, public)
	}
	cfg.Upload.PublicDir = public
	if n, ok := c.Int("plugins.upload.config.sizeLimit"); ok {
		cfg.Upload.SizeLimit = int64(n)
	}
	if bps, ok := c.Get("plugins.upload.config.breakpoints").(map[string]any); ok {
		for name, v := range bps {
			if n, ok := v.(int); ok {
				cfg.Upload.Breakpoints = append(cfg.Upload.Breakpoints, httpapi.Breakpoint{Name: name, Size: n})
			}
		}
		sort.Slice(cfg.Upload.Breakpoints, func(i, j int) bool { return cfg.Upload.Breakpoints[i].Size > cfg.Upload.Breakpoints[j].Size })
	}
	if fields, ok := c.Get(up + "register.allowedFields").([]any); ok {
		for _, f := range fields {
			if s, ok := f.(string); ok {
				cfg.Users.AllowedFields = append(cfg.Users.AllowedFields, s)
			}
		}
	}
	return cfg
}

// ensureJWTSecret is the users-permissions bootstrap: without a secret, one
// is generated and appended to .env so the tokens survive a restart.
func ensureJWTSecret(cfg *httpapi.Config, c *config.Config, dir string, log *slog.Logger) error {
	if cfg.Users.JWTSecret != "" {
		return nil
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	cfg.Users.JWTSecret = base64.StdEncoding.EncodeToString(b)
	if _, set := c.Env["JWT_SECRET"]; set {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, ".env"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the project's own .env
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "JWT_SECRET=%s\n", cfg.Users.JWTSecret); err != nil {
		return err
	}
	log.Info("the users-permissions JWT secret has been generated in the .env file under JWT_SECRET")
	return nil
}

// project is a loaded Strapi project: its config, schemas and database.
type project struct {
	dir     string
	conf    *config.Config
	res     *registry.Result
	md      *metadata.Metadata
	desired *metadata.Schema
	db      *db.DB
	log     *slog.Logger
}

func loadProject(dir, configDir string, stderr io.Writer) (*project, error) {
	level := new(slog.LevelVar)
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))
	conf, err := config.Load(config.Options{Dir: dir, ConfigDir: configDir, Log: log})
	if err != nil {
		return nil, err
	}
	if l, ok := conf.Env.Lookup("GRAPI_LOG_LEVEL", log); ok && l != "" {
		if err := level.UnmarshalText([]byte(l)); err != nil {
			return nil, fmt.Errorf("GRAPI_LOG_LEVEL: %w", err)
		}
	}
	res, md, err := builtin.Load(os.DirFS(dir))
	if err != nil {
		return nil, err
	}
	for _, w := range res.Warnings {
		log.Warn(w)
	}
	desired, err := md.Schema()
	if err != nil {
		return nil, err
	}
	d, err := db.Open(context.Background(), dbConfig(conf, dir), log)
	if err != nil {
		return nil, err
	}
	return &project{dir: dir, conf: conf, res: res, md: md, desired: desired, db: d, log: log}, nil
}

func projectFlags(name string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory (the one holding src/)")
	configDir := fs.String("config", "", "configuration directory (default <dir>/config)")
	return fs, dir, configDir
}

func start(args []string, stderr io.Writer) error {
	fs, dirFlag, configDir := projectFlags("start")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := loadProject(*dirFlag, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.db.Close() }()
	dir, conf, res, md, desired, d, log := &p.dir, p.conf, p.res, p.md, p.desired, p.db, p.log
	ctx := context.Background()
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
	apiCfg := apiConfig(conf, *dir)
	if list, ok := conf.Get("middlewares").([]any); ok {
		mw, err := httpapi.ParseMiddlewares(list)
		if err != nil {
			return fmt.Errorf("config middlewares: %w", err)
		}
		apiCfg.Middlewares = &mw
	}
	if err := ensureJWTSecret(&apiCfg, conf, *dir, log); err != nil {
		return err
	}
	api := httpapi.New(apiCfg, d, res.Set, document.New(engine, res.Set), log)
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
