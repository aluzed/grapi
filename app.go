package grapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"strings"
	"syscall"
	"time"

	"github.com/aluzed/grapi/internal/bootstrap"
	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/cron"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/events"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/logging"
	"github.com/aluzed/grapi/internal/media/provider"
	"github.com/aluzed/grapi/internal/openapi"
	"github.com/aluzed/grapi/internal/plugins/email"
	"github.com/aluzed/grapi/internal/registry"
	"github.com/aluzed/grapi/internal/webhooks"
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
	cfg.Users.JWTManagement, _ = c.String(up + "jwtManagement")
	if m := cfg.Users.JWTManagement; m != "" && m != "legacy-support" {
		slog.Default().Warn("plugins.users-permissions.config.jwtManagement " + m + " is not supported by grapi yet: legacy-support is used")
	}
	cfg.Users.RateLimit = rateLimitConfig(c, up+"ratelimit")
	switch p := c.Get("server.proxy").(type) {
	case bool:
		cfg.TrustProxy = p
	case map[string]any:
		cfg.TrustProxy = p["koa"] == true
	}
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
	if name, _ := c.String("plugins.upload.config.provider"); name != "" && name != "local" {
		prov, err := uploadProvider(c, name)
		if err != nil {
			slog.Default().Error("upload provider", "error", err)
		} else {
			cfg.Upload.Provider, cfg.Upload.ProviderName = prov, name
		}
	}
	if bps, ok := c.Get("plugins.upload.config.breakpoints").(map[string]any); ok {
		for name, v := range bps {
			if n, ok := v.(int); ok {
				cfg.Upload.Breakpoints = append(cfg.Upload.Breakpoints, httpapi.Breakpoint{Name: name, Size: n})
			}
		}
		sort.Slice(cfg.Upload.Breakpoints, func(i, j int) bool { return cfg.Upload.Breakpoints[i].Size > cfg.Upload.Breakpoints[j].Size })
	}
	cfg.ServerURL, cfg.AdminURL = absoluteURLs(c)
	if e, err := emailService(c); err == nil {
		cfg.Email = e
	}
	cfg.GraphQL = graphQLConfig(c, dir)
	if fields, ok := c.Get(up + "register.allowedFields").([]any); ok {
		for _, f := range fields {
			if s, ok := f.(string); ok {
				cfg.Users.AllowedFields = append(cfg.Users.AllowedFields, s)
			}
		}
	}
	return cfg
}

type packageJSON struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

func readPackage(dir string) packageJSON {
	var pkg packageJSON
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil { //nolint:gosec // the project directory
		_ = json.Unmarshal(b, &pkg)
	}
	return pkg
}

// openAPIConfig reads server.openapi['content-api']: access "public"
// serves the document, "disabled" (the default) does not.
func openAPIConfig(c *config.Config, dir string) (*httpapi.OpenAPIConfig, error) {
	base := "server.openapi.content-api."
	access, _ := c.String(base + "access")
	switch access {
	case "", "disabled":
		return nil, nil
	case "public":
	default:
		return nil, fmt.Errorf("invalid OpenAPI access %q for \"content-api\". Expected one of: disabled, public", access)
	}
	pkg := readPackage(dir)
	cfg := &httpapi.OpenAPIConfig{Info: openapi.Info{Name: pkg.Name, Version: pkg.Version}}
	cfg.Path, _ = c.String(base + "route.path")
	return cfg, nil
}

// graphQLConfig enables the graphql plugin when the project depends on
// @strapi/plugin-graphql (Strapi enables installed plugins) or when
// plugins.graphql.enabled is true.
func graphQLConfig(c *config.Config, dir string) *httpapi.GraphQLConfig {
	enabled, set := c.Bool("plugins.graphql.enabled")
	if !set {
		_, enabled = readPackage(dir).Dependencies["@strapi/plugin-graphql"]
	}
	if !enabled {
		return nil
	}
	base := "plugins.graphql.config."
	g := &httpapi.GraphQLConfig{Introspection: c.Environment != "production"}
	g.Endpoint, _ = c.String(base + "endpoint")
	g.DefaultLimit, _ = c.Int(base + "defaultLimit")
	g.MaxLimit, _ = c.Int(base + "maxLimit")
	g.Options.DepthLimit, _ = c.Int(base + "depthLimit")
	if v, ok := c.Bool(base + "shadowCRUD"); ok && !v {
		g.Options.NoShadowCRUD = true
	}
	if v, ok := c.Bool(base + "apolloServer.introspection"); ok {
		g.Introspection = v
	}
	return g
}

// uploadProvider builds the provider of plugins.upload.config: aws-s3
// takes @strapi/provider-upload-aws-s3's providerOptions.
func uploadProvider(c *config.Config, name string) (provider.Provider, error) {
	if name != "aws-s3" {
		return nil, fmt.Errorf("upload provider %q is not supported yet", name)
	}
	base := "plugins.upload.config.providerOptions."
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := c.String(base + k); ok && v != "" {
				return v
			}
		}
		return ""
	}
	p := &provider.S3{
		BaseURL:      str("baseUrl"),
		RootPath:     str("rootPath"),
		Region:       str("s3Options.region", "region"),
		Endpoint:     str("s3Options.endpoint", "endpoint"),
		AccessKey:    str("s3Options.credentials.accessKeyId", "accessKeyId"),
		Secret:       str("s3Options.credentials.secretAccessKey", "secretAccessKey"),
		SessionToken: str("s3Options.credentials.sessionToken"),
		Bucket:       str("s3Options.params.Bucket", "params.Bucket"),
		ACL:          "public-read",
	}
	p.ForcePathStyle, _ = c.Bool(base + "s3Options.forcePathStyle")
	for _, k := range []string{"s3Options.params", "params"} {
		if params, ok := c.Get(base + k).(map[string]any); ok {
			if acl, has := params["ACL"]; has {
				p.ACL, _ = acl.(string)
			}
			if exp, ok := params["signedUrlExpires"].(int); ok {
				p.SignedURLExpires = time.Duration(exp) * time.Second
			}
		}
	}
	if p.Bucket == "" {
		return nil, fmt.Errorf("upload AWS S3 provider: `params` are required in the config object")
	}
	return p, nil
}

// rateLimitConfig reads a koa2-ratelimit config: interval in ms or as
// {min: 5}, users-permissions' defaults (1 minute, 10 requests).
func rateLimitConfig(c *config.Config, key string) httpapi.RateLimit {
	rl := httpapi.RateLimit{Interval: time.Minute, Max: 10}
	if v, ok := c.Bool(key + ".enabled"); ok && !v {
		rl.Disabled = true
	}
	if n, ok := c.Int(key + ".max"); ok {
		rl.Max = n
	}
	switch v := c.Get(key + ".interval").(type) {
	case int:
		rl.Interval = time.Duration(v) * time.Millisecond
	case float64:
		rl.Interval = time.Duration(v * float64(time.Millisecond))
	case map[string]any:
		units := map[string]time.Duration{"ms": time.Millisecond, "sec": time.Second, "min": time.Minute, "hour": time.Hour, "day": 24 * time.Hour, "week": 7 * 24 * time.Hour}
		var d time.Duration
		for unit, n := range v {
			if f, ok := n.(int); ok {
				d += time.Duration(f) * units[unit]
			}
		}
		if d > 0 {
			rl.Interval = d
		}
	}
	return rl
}

// absoluteURLs is Strapi's getAbsoluteServerUrl and getAbsoluteAdminUrl.
func absoluteURLs(c *config.Config) (string, string) {
	serverURL, _ := c.String("server.url")
	adminURL, ok := c.String("admin.url")
	if !ok {
		adminURL = "/admin"
	}
	host, _ := c.String("server.host")
	port, _ := c.Int("server.port")
	if c.Environment == "development" && (host == "127.0.0.1" || host == "0.0.0.0") {
		host = "localhost"
	}
	abs := func(u string) string {
		if strings.HasPrefix(u, "http") {
			return u
		}
		return "http://" + host + ":" + strconv.Itoa(port) + u
	}
	server := abs(serverURL)
	if strings.HasPrefix(adminURL, "http") {
		return server, adminURL
	}
	return server, strings.TrimRight(server, "/") + "/" + strings.TrimLeft(adminURL, "/")
}

// emailService reads plugins.email.config: nodemailer (smtp) or sendmail,
// Strapi's default.
func emailService(c *config.Config) (*email.Service, error) {
	base := "plugins.email.config."
	provider, _ := c.String(base + "provider")
	opt := func(k string) string {
		switch v := c.Get(base + "providerOptions." + k).(type) {
		case string:
			return v
		case nil:
			return ""
		default:
			return fmt.Sprint(v)
		}
	}
	svc := &email.Service{}
	svc.Settings.DefaultFrom, _ = c.String(base + "settings.defaultFrom")
	svc.Settings.DefaultReplyTo, _ = c.String(base + "settings.defaultReplyTo")
	switch provider {
	case "nodemailer", "smtp":
		port, _ := strconv.Atoi(opt("port"))
		p := &email.SMTP{Host: opt("host"), Port: port, Secure: opt("secure") == "true", User: opt("auth.user"), Pass: opt("auth.pass")}
		if key := opt("dkim.privateKey"); key != "" {
			p.DKIM = &email.DKIM{DomainName: opt("dkim.domainName"), KeySelector: opt("dkim.keySelector"), PrivateKey: key}
		}
		svc.Provider = p
	case "", "sendmail":
		svc.Provider = &email.Sendmail{}
	case "sendgrid":
		svc.Provider = &email.SendGrid{APIKey: opt("apiKey")}
	case "mailgun":
		svc.Provider = &email.Mailgun{Key: opt("key"), Username: opt("username"), URL: opt("url"), Domain: opt("domain")}
	case "amazon-ses":
		svc.Provider = &email.SES{Key: opt("key"), Secret: opt("secret"), Endpoint: opt("amazon")}
	default:
		return nil, fmt.Errorf("email provider %q is not supported yet", provider)
	}
	return svc, nil
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

// Project is a loaded Strapi project: its config, schemas and database.
type Project struct {
	Dir     string
	Conf    *config.Config
	Res     *registry.Result
	MD      *metadata.Metadata
	Desired *metadata.Schema
	DB      *db.DB
	Log     *slog.Logger
}

// LoadProject reads the config and the schemas of dir and opens its
// database, without writing anything.
func LoadProject(dir, configDir string, stderr io.Writer) (*Project, error) {
	level := new(slog.LevelVar)
	level.Set(logging.DefaultLevel)
	log := logging.New(stderr, logging.Text, level)
	conf, err := config.Load(config.Options{Dir: dir, ConfigDir: configDir, Log: log})
	if err != nil {
		return nil, err
	}
	if l, ok := conf.Env.Lookup("GRAPI_LOG_LEVEL", log); ok && l != "" {
		lv, err := logging.ParseLevel(l)
		if err != nil {
			return nil, fmt.Errorf("GRAPI_LOG_LEVEL: %w", err)
		}
		level.Set(lv)
	}
	f, _ := conf.Env.Lookup("GRAPI_LOG_FORMAT", log)
	if f == "" {
		f, _ = conf.String("logger.format")
	}
	format, err := logging.ParseFormat(f, logging.DefaultFormat(conf.Environment))
	if err != nil {
		return nil, fmt.Errorf("log format: %w", err)
	}
	log = logging.New(stderr, format, level)
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
	return &Project{Dir: dir, Conf: conf, Res: res, MD: md, Desired: desired, DB: d, Log: log}, nil
}

// OpenAPI is the project's content API document, with the routes grapi
// serves. It writes nothing to the database.
func (p *Project) OpenAPI() ([]byte, error) {
	cfg := apiConfig(p.Conf, p.Dir)
	pkg := readPackage(p.Dir)
	engine := &query.Engine{DB: p.DB, MD: p.MD}
	api := httpapi.New(cfg, p.DB, p.Res.Set, document.New(engine, p.Res.Set), p.Log)
	return openapi.JSON(openapi.Generate(p.Res.Set, openapi.Options{Info: openapi.Info{Name: pkg.Name, Version: pkg.Version}, Served: api.Serves}))
}

// Routes is the project's route table, without starting anything.
func (p *Project) Routes() []httpapi.RouteInfo {
	engine := &query.Engine{DB: p.DB, MD: p.MD}
	cfg := apiConfig(p.Conf, p.Dir)
	return httpapi.New(cfg, p.DB, p.Res.Set, document.New(engine, p.Res.Set), p.Log).Routes()
}

// Server is a set-up App: the API handler and the address it is
// configured to listen on.
type Server struct {
	Handler http.Handler
	Addr    string
	project *Project
	stop    func()
}

// Close stops the webhooks and releases the database.
func (s *Server) Close() error {
	if s.stop != nil {
		s.stop()
	}
	return s.project.DB.Close()
}

// Setup loads the project, syncs its database and runs the bootstrap; the
// returned handler serves the API (to embed grapi in another server).
func (a *App) Setup() (*Server, error) {
	p, err := LoadProject(a.opts.Dir, a.opts.ConfigDir, a.stderr())
	if err != nil {
		return nil, err
	}
	srv, err := a.setup(p)
	if err != nil {
		_ = p.DB.Close()
		return nil, err
	}
	return srv, nil
}

func (a *App) setup(p *Project) (*Server, error) {
	dir, conf, res, md, desired, d, log := &p.Dir, p.Conf, p.Res, p.MD, p.Desired, p.DB, p.Log
	ctx := context.Background()
	err := migrate.WithLock(ctx, d, func(ctx context.Context) error {
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
		return nil, err
	}
	engine := &query.Engine{DB: d, MD: md, Lifecycles: a.lifecycles}
	initLocale, _ := conf.Env.Lookup("GRAPI_PLUGIN_I18N_INIT_LOCALE_CODE", log)
	if err := bootstrap.Run(ctx, engine, res.Set, bootstrap.Options{InitLocale: initLocale, Log: log}); err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	apiCfg := apiConfig(conf, *dir)
	if list, ok := conf.Get("middlewares").([]any); ok {
		mw, err := httpapi.ParseMiddlewares(list)
		if err != nil {
			return nil, fmt.Errorf("config middlewares: %w", err)
		}
		apiCfg.Middlewares = &mw
	}
	if err := ensureJWTSecret(&apiCfg, conf, *dir, log); err != nil {
		return nil, err
	}
	oas, err := openAPIConfig(conf, *dir)
	if err != nil {
		return nil, err
	}
	apiCfg.OpenAPI = oas
	if apiCfg.GraphQL != nil {
		fromConfig := apiCfg.GraphQL.Options
		apiCfg.GraphQL.Options = a.graphql
		apiCfg.GraphQL.Options.DepthLimit, apiCfg.GraphQL.Options.NoShadowCRUD = fromConfig.DepthLimit, fromConfig.NoShadowCRUD
	}
	docs := document.New(engine, res.Set)
	switch v := conf.Get("api.documents.strictRelations").(type) {
	case nil:
	case bool:
		docs.StrictRelations = v
	default:
		return nil, fmt.Errorf("invalid config.api.documents.strictRelations value: %q, expected boolean (true or false)", fmt.Sprint(v))
	}
	for _, m := range a.documentMiddlewares {
		docs.Use(m)
	}
	docs.Events = events.New(log)
	retries := 3
	if n, ok := conf.Int("server.webhooks.retries"); ok {
		retries = n
	}
	hookCfg := webhooks.Config{Retries: retries, DefaultHeaders: map[string]string{}}
	if h, ok := conf.Get("server.webhooks.defaultHeaders").(map[string]any); ok {
		for k, v := range h {
			hookCfg.DefaultHeaders[k] = fmt.Sprint(v)
		}
	}
	runner := &webhooks.Runner{DB: d, Hub: docs.Events, Config: hookCfg, Log: log}
	stopWebhooks := runner.Start(context.Background())
	scheduler := cron.New(log)
	cronEnabled, _ := conf.Bool("server.cron.enabled")
	if err := a.startCron(cronEnabled, scheduler); err != nil {
		stopWebhooks()
		return nil, err
	}
	a.stopWebhooks = func() {
		scheduler.Stop()
		stopWebhooks()
	}
	a.docs = docs
	api := httpapi.New(apiCfg, d, res.Set, docs, log)
	if err := a.addRoutes(api); err != nil {
		return nil, err
	}
	host, _ := conf.String("server.host")
	port, _ := conf.Int("server.port")
	return &Server{Handler: api, Addr: net.JoinHostPort(host, strconv.Itoa(port)), project: p, stop: a.stopWebhooks}, nil
}

// Start sets the App up and serves it until SIGINT or SIGTERM.
func (a *App) Start() error {
	s, err := a.Setup()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	log := s.project.Log
	srv := &http.Server{Addr: s.Addr, Handler: s.Handler, ReadHeaderTimeout: 10 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	conf := s.project.Conf
	server, admin := absoluteURLs(conf)
	log.Info(fmt.Sprintf("grapi %s listening on %s", a.opts.Version, s.Addr),
		"environment", conf.Environment, "server", server, "admin", admin, "database", s.project.DB.Dialect.Name())
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-stop:
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
	return nil
}
