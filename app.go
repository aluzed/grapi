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

	"github.com/aluzed/grapi/hook"

	"github.com/aluzed/grapi/internal/adminapi"
	"github.com/aluzed/grapi/internal/bootstrap"
	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/cron"
	"github.com/aluzed/grapi/internal/db"
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
	"github.com/aluzed/grapi/internal/project"
	"github.com/aluzed/grapi/internal/ratelimit"
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
	if m := cfg.Users.JWTManagement; m != "" && m != "legacy-support" && m != "refresh" {
		slog.Default().Warn("plugins.users-permissions.config.jwtManagement " + m + " is unknown: legacy-support is used")
	}
	seconds := func(key string) time.Duration {
		n, _ := c.Int(up + "sessions." + key)
		return time.Duration(n) * time.Second
	}
	cfg.Users.Sessions = httpapi.UPSessions{
		AccessTokenLifespan: seconds("accessTokenLifespan"), MaxRefreshTokenLifespan: seconds("maxRefreshTokenLifespan"),
		IdleRefreshTokenLifespan: seconds("idleRefreshTokenLifespan"), MaxSessionLifespan: seconds("maxSessionLifespan"),
		IdleSessionLifespan: seconds("idleSessionLifespan"),
	}
	cfg.Users.Sessions.HTTPOnly, _ = c.Bool(up + "sessions.httpOnly")
	cookie := up + "sessions.cookie."
	cfg.Users.Sessions.Cookie.Name, _ = c.String(cookie + "name")
	cfg.Users.Sessions.Cookie.SameSite, _ = c.String(cookie + "sameSite")
	cfg.Users.Sessions.Cookie.Path, _ = c.String(cookie + "path")
	cfg.Users.Sessions.Cookie.Domain, _ = c.String(cookie + "domain")
	if b, ok := c.Bool(cookie + "secure"); ok {
		cfg.Users.Sessions.Cookie.Secure = &b
	}
	// koa's maxAge is in milliseconds.
	if n, ok := c.Int(cookie + "maxAge"); ok {
		cfg.Users.Sessions.Cookie.MaxAge = time.Duration(n) * time.Millisecond
	}
	cfg.Users.Production = c.Environment == "production"
	switch keys := c.Get("server.app.keys").(type) {
	case []any:
		for _, k := range keys {
			if k, ok := k.(string); ok && k != "" {
				cfg.Users.AppKeys = append(cfg.Users.AppKeys, k)
			}
		}
	case string:
		for _, k := range strings.Split(keys, ",") {
			if k = strings.TrimSpace(k); k != "" {
				cfg.Users.AppKeys = append(cfg.Users.AppKeys, k)
			}
		}
	}
	cfg.Users.RateLimit = rateLimitConfig(c, up+"ratelimit", ratelimit.Config{Interval: time.Minute, Max: 10})
	// Strapi 5 reads server.proxy.koa only: a boolean proxy trusts nothing.
	switch p := c.Get("server.proxy").(type) {
	case bool:
		if p {
			slog.Default().Warn("server.proxy: true is ignored, as in Strapi 5: set server.proxy.koa to trust the proxy headers")
		}
	case map[string]any:
		cfg.Proxy.Koa = p["koa"] == true
		cfg.Proxy.IPHeader, _ = p["ipHeader"].(string)
		cfg.Proxy.MaxIPs, _ = c.Int("server.proxy.maxIpsCount")
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
	cfg.Upload.Security = uploadSecurity(c.Get("plugins.upload.config.security"))
	if name, _ := c.String("plugins.upload.config.provider"); name != "" && name != "local" {
		// An unknown name may be a provider of the Go project: setup reports it.
		if prov, err := uploadProvider(c, name); err == nil {
			cfg.Upload.Provider, cfg.Upload.ProviderName = prov, name
		} else if !errors.Is(err, errUnknownUploadProvider) {
			slog.Default().Error("upload provider", "error", err)
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

var errUnknownUploadProvider = errors.New("unknown upload provider")

// uploadProvider builds the provider of plugins.upload.config: aws-s3
// takes @strapi/provider-upload-aws-s3's providerOptions.
// uploadSecurity reads plugins.upload.config.security; a list that holds
// anything but strings makes every upload fail, as in Strapi.
func uploadSecurity(v any) httpapi.UploadSecurity {
	var sec httpapi.UploadSecurity
	m, _ := v.(map[string]any)
	read := func(key string) ([]string, bool) {
		raw, set := m[key]
		if !set || raw == nil {
			return nil, true
		}
		list, ok := raw.([]any)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(list))
		for _, x := range list {
			s, ok := x.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	var ok bool
	if sec.AllowedTypes, ok = read("allowedTypes"); !ok {
		sec.Invalid = "Invalid configuration: allowedTypes must be an array of strings."
	}
	if sec.DeniedTypes, ok = read("deniedTypes"); !ok && sec.Invalid == "" {
		sec.Invalid = "Invalid configuration: deniedTypes must be an array of strings."
	}
	return sec
}

func uploadProvider(c *config.Config, name string) (provider.Provider, error) {
	if name == "cloudinary" {
		base := "plugins.upload.config."
		opts := func(k string) map[string]any {
			m, _ := c.Get(base + "actionOptions." + k).(map[string]any)
			return m
		}
		p := &provider.Cloudinary{UploadOptions: opts("upload"), DeleteOptions: opts("delete")}
		p.CloudName, _ = c.String(base + "providerOptions.cloud_name")
		p.APIKey, _ = c.String(base + "providerOptions.api_key")
		p.APISecret, _ = c.String(base + "providerOptions.api_secret")
		return p, nil
	}
	if name != "aws-s3" {
		return nil, errUnknownUploadProvider
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
			if h := provider.HeadersFromParams(params); len(h) > 0 {
				p.Headers = h
			}
		}
	}
	if p.Bucket == "" {
		return nil, fmt.Errorf("upload AWS S3 provider: `params` are required in the config object")
	}
	return p, nil
}

// rateLimitConfig reads a koa2-ratelimit config over defaults: interval
// in ms or as {min: 5}.
func rateLimitConfig(c *config.Config, key string, rl ratelimit.Config) ratelimit.Config {
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
	secret, err := generateSecret(c, dir, "JWT_SECRET")
	if err != nil {
		return err
	}
	cfg.Users.JWTSecret = secret
	log.Info("the users-permissions JWT secret has been generated in the .env file under JWT_SECRET")
	return nil
}

// generateSecret returns a random secret and appends it to .env under key,
// unless key is already set (to an empty value).
func generateSecret(c *config.Config, dir, key string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	secret := base64.StdEncoding.EncodeToString(b)
	if _, set := c.Env[key]; set {
		return secret, nil
	}
	f, err := os.OpenFile(filepath.Join(dir, ".env"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the project's own .env
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "%s=%s\n", key, secret); err != nil {
		return "", err
	}
	return secret, nil
}

func init() {
	project.Loader = loadProject
	project.OpenAPIOf = projectOpenAPI
	project.GraphQLOf = func(p *project.Project) bool { return graphQLConfig(p.Conf, p.Dir) != nil }
	project.RoutesOf = projectRoutes
}

// loadProject reads the config and the schemas of dir and opens its
// database, without writing anything.
func loadProject(dir, configDir string, stderr io.Writer) (*project.Project, error) {
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
	return &project.Project{Dir: dir, Conf: conf, Res: res, MD: md, Desired: desired, DB: d, Log: log}, nil
}

// projectOpenAPI is the project's content API document, with the routes
// grapi serves. It writes nothing to the database.
func projectOpenAPI(p *project.Project) ([]byte, error) {
	cfg := apiConfig(p.Conf, p.Dir)
	pkg := readPackage(p.Dir)
	engine := &query.Engine{DB: p.DB, MD: p.MD}
	api := httpapi.New(cfg, p.DB, p.Res.Set, document.New(engine, p.Res.Set), p.Log)
	return openapi.JSON(openapi.Generate(p.Res.Set, openapi.Options{Info: openapi.Info{Name: pkg.Name, Version: pkg.Version}, Served: api.Serves}))
}

func projectRoutes(p *project.Project) []httpapi.RouteInfo {
	engine := &query.Engine{DB: p.DB, MD: p.MD}
	cfg := apiConfig(p.Conf, p.Dir)
	return httpapi.New(cfg, p.DB, p.Res.Set, document.New(engine, p.Res.Set), p.Log).Routes()
}

// Server is a set-up App: the API handler and the address it is
// configured to listen on.
type Server struct {
	Handler http.Handler
	Addr    string
	project *project.Project
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
	p, err := loadProject(a.opts.Dir, a.opts.ConfigDir, a.stderr())
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

func (a *App) setup(p *project.Project) (*Server, error) {
	dir, conf, res, md, desired, d, log := &p.Dir, p.Conf, p.Res, p.MD, p.Desired, p.DB, p.Log
	ctx := context.Background()
	a.log.Store(log)
	a.conf.Store(conf)
	for _, fn := range a.onRegister {
		if err := fn(ctx); err != nil {
			return nil, fmt.Errorf("register: %w", err)
		}
	}
	if err := a.disableMigrations(ctx, p); err != nil {
		return nil, fmt.Errorf("content type migrations: %w", err)
	}
	if err := a.syncHook(ctx, hook.BeforeSync, res.Set, d); err != nil {
		return nil, fmt.Errorf("%s: %w", hook.BeforeSync, err)
	}
	err := migrate.WithLock(ctx, d, func(ctx context.Context) error {
		ran, err := migrate.Run(ctx, d, migrate.Options{Dir: os.DirFS(filepath.Join(*dir, "database", "migrations")), RunMigrations: true, Log: log})
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
	apiCfg.Sanitizers = a.sanitizers
	a.coreAPIConfig(&apiCfg)
	if apiCfg.Upload.LimitInputPixels, err = limitInputPixels(conf); err != nil {
		return nil, err
	}
	if list, ok := conf.Get("middlewares").([]any); ok {
		if err := httpapi.CheckRequiredMiddlewares(list); err != nil {
			return nil, fmt.Errorf("config middlewares: %w", err)
		}
		mw, err := httpapi.ParseMiddlewares(list)
		if err != nil {
			return nil, fmt.Errorf("config middlewares: %w", err)
		}
		apiCfg.Middlewares = &mw
	}
	if err := ensureJWTSecret(&apiCfg, conf, *dir, log); err != nil {
		return nil, err
	}
	if name, _ := conf.String("plugins.upload.config.provider"); name != "" && name != "local" {
		if p, ok := a.uploadProviders[name]; ok {
			apiCfg.Upload.Provider, apiCfg.Upload.ProviderName = p, name
		} else if apiCfg.Upload.Provider == nil {
			log.Error("upload provider", "error", fmt.Errorf("upload provider %q is not supported yet", name))
		}
	}
	if name, _ := conf.String("plugins.email.config.provider"); a.emailProviders[name] != nil {
		svc := &email.Service{Provider: a.emailProviders[name]}
		svc.Settings.DefaultFrom, _ = conf.String("plugins.email.config.settings.defaultFrom")
		svc.Settings.DefaultReplyTo, _ = conf.String("plugins.email.config.settings.defaultReplyTo")
		apiCfg.Email = svc
	}
	oas, err := openAPIConfig(conf, *dir)
	if err != nil {
		return nil, err
	}
	if oas != nil {
		oas.Mutate = a.openapiMutate
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
	hookCfg.BlockPrivateNetworks, _ = conf.Bool("server.webhooks.blockPrivateNetworks")
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
	adminCfg, err := adminConfig(conf, *dir, apiCfg.Email, apiCfg.Proxy, log)
	if err != nil {
		a.stopWebhooks()
		return nil, err
	}
	apiCfg.PreviewSecret = adminCfg.Preview.Secret
	a.email = apiCfg.Email
	adminCfg.Preview.Funcs = a.previewFuncs()
	api := httpapi.New(apiCfg, d, res.Set, docs, log)
	if err := a.addRoutes(api); err != nil {
		return nil, err
	}
	adminCfg.Version, adminCfg.Environment = a.opts.Version, conf.Environment
	adminCfg.Develop, adminCfg.ProjectDir = a.opts.AutoReload, *dir
	adminCfg.Media, adminCfg.Webhooks = api, runner
	adminCfg.APITokenSalt, adminCfg.ContentActions = apiCfg.APITokenSalt, api.ContentAPIActions
	adminCfg.Routes = api.Routes
	adminCfg.EncryptionKey, _ = conf.String("admin.secrets.encryptionKey")
	if adminCfg.EmailProvider, _ = conf.String("plugins.email.config.provider"); adminCfg.EmailProvider == "" {
		adminCfg.EmailProvider = "sendmail"
	}
	adm := adminapi.New(adminCfg, docs, log)
	if err := adm.Bootstrap(ctx); err != nil {
		a.stopWebhooks()
		return nil, fmt.Errorf("admin bootstrap: %w", err)
	}
	if err := a.enableMigrations(ctx, p, docs); err != nil {
		a.stopWebhooks()
		return nil, fmt.Errorf("content type migrations: %w", err)
	}
	if err := storeFlags(ctx, d, res.Set); err != nil {
		a.stopWebhooks()
		return nil, err
	}
	// After the sync and once the services exist, so that a handler can seed.
	if err := a.syncHook(ctx, hook.AfterSync, res.Set, d); err != nil {
		a.stopWebhooks()
		return nil, fmt.Errorf("%s: %w", hook.AfterSync, err)
	}
	for _, fn := range a.onBootstrap {
		if err := fn(ctx); err != nil {
			a.stopWebhooks()
			return nil, fmt.Errorf("bootstrap: %w", err)
		}
	}
	host, _ := conf.String("server.host")
	port, _ := conf.Int("server.port")
	stop := a.stopWebhooks
	destroy := a.onDestroy
	releases, stopReleases := context.WithCancel(context.Background())
	adm.StartScheduler(releases, 30*time.Second)
	documentation := a.documentationPlugin(conf, *dir, d, api, apiCfg.Users.AppKeys)
	return &Server{Handler: withDocumentation(documentation, withAdmin(adm, api)), Addr: net.JoinHostPort(host, strconv.Itoa(port)), project: p, stop: func() {
		stopReleases()
		for _, fn := range destroy {
			if err := fn(context.Background()); err != nil {
				log.Error("destroy", "error", err)
			}
		}
		stop()
	}}, nil
}

// Start sets the App up and serves it until SIGINT or SIGTERM. Under
// `grapi develop` (GRAPI_DEVELOP=1) it develops instead: reloads in process.
// Run as `<binary> routes:list`, it prints the route table and exits.
func (a *App) Start() error {
	if commandArg() == "routes:list" {
		return a.listRoutes(os.Stdout)
	}
	if os.Getenv("GRAPI_DEVELOP") == "1" {
		return a.Develop(nil)
	}
	s, err := a.Setup()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	log := s.project.Log
	srv := s.httpServer(s.Handler)
	ln, addr, err := s.listener()
	if err != nil {
		return err
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	conf := s.project.Conf
	server, admin := absoluteURLs(conf)
	log.Info(fmt.Sprintf("grapi %s listening on %s", a.opts.Version, addr),
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

// sharpDefaultPixels is sharp's default limitInputPixels, 0x3FFF squared.
const sharpDefaultPixels = 0x3FFF * 0x3FFF

// limitInputPixels reads plugins.upload.config.sharp.limitInputPixels as
// Strapi checks it: a boolean (true is sharp's default, false no limit) or
// an integer >= 0 (0 is no limit).
func limitInputPixels(c *config.Config) (int64, error) {
	switch v := c.Get("plugins.upload.config.sharp.limitInputPixels").(type) {
	case nil:
		return sharpDefaultPixels, nil
	case bool:
		if v {
			return sharpDefaultPixels, nil
		}
		return 0, nil
	case int:
		if v >= 0 {
			return int64(v), nil
		}
	case float64:
		if v >= 0 && v == float64(int64(v)) {
			return int64(v), nil
		}
	}
	return 0, errors.New(`upload plugin config: "sharp.limitInputPixels" must be a boolean or an integer greater than or equal to 0 (use env.int() to read it from the environment)`)
}
