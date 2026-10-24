// Package grapi is the entry point for embedding grapi as a Go framework:
// build an App, add hooks, then Start it. The grapi binary is such a main
// without hooks.
package grapi

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/aluzed/grapi/hook"

	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/graphql"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/plugins/email"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
)

// Options of an App.
type Options struct {
	// Dir is the project directory, the one holding src/ ("." by default).
	Dir string
	// ConfigDir defaults to Dir/config.
	ConfigDir string
	// Version is logged at start.
	Version string
	// Stderr receives the logs (os.Stderr by default).
	Stderr io.Writer
	// AutoReload is set by `grapi develop`, which reloads the app when src/
	// changes: the admin's Content-Type Builder writes schemas only then.
	AutoReload bool
}

// App is a grapi application.
type App struct {
	opts                Options
	lifecycles          *query.Lifecycles
	documentMiddlewares []document.Middleware
	routes              []*customRoute
	policies            map[string]Policy
	routePolicies       []routePolicies
	docs                *document.Service
	stopWebhooks        func()
	cronTasks           []cronTask
	graphql             graphql.Options
	hooks               *hook.Registry
	openapiMutate       []func(map[string]any)
	sanitizers          httpapi.Sanitizers
	coreRouterRefs      []coreRouterRef
	coreActions         map[string]httpapi.CoreAction
	preview             map[string]PreviewHandler
	email               *email.Service
	onRegister          []func(context.Context) error
	onBootstrap         []func(context.Context) error
	onDestroy           []func(context.Context) error
	uploadProviders     map[string]UploadProvider
	emailProviders      map[string]EmailProvider
	log                 atomic.Pointer[slog.Logger]
	conf                atomic.Pointer[config.Config]
}

// Config reads the project's config by dotted path, as strapi.config.get
// does: "server.port", "plugins.upload.config.sizeLimit". It is empty until
// the project is loaded, from OnRegister on.
type Config struct{ c *config.Config }

// Config is the loaded project's config.
func (a *App) Config() Config { return Config{a.conf.Load()} }

// Get is the value at path, nil when absent.
func (c Config) Get(path string) any {
	if c.c == nil {
		return nil
	}
	return c.c.Get(path)
}

// String is the string at path.
func (c Config) String(path string) (string, bool) {
	if c.c == nil {
		return "", false
	}
	return c.c.String(path)
}

// Int is the number at path.
func (c Config) Int(path string) (int, bool) {
	if c.c == nil {
		return 0, false
	}
	return c.c.Int(path)
}

// Bool is the boolean at path.
func (c Config) Bool(path string) (bool, bool) {
	if c.c == nil {
		return false, false
	}
	return c.c.Bool(path)
}

// Log is the project's logger, with the level and format of its config
// (logger.format, GRAPI_LOG_LEVEL, GRAPI_LOG_FORMAT), once the project is
// loaded: from OnRegister on. Before that, slog.Default().
func (a *App) Log() *slog.Logger {
	if l := a.log.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// New returns an App for the project in opts.Dir.
func New(opts Options) *App {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	hooks := &hook.Registry{}
	hooks.Register(hook.BeforeSync, &hook.Parallel[hook.SyncEvent]{})
	hooks.Register(hook.AfterSync, &hook.Parallel[hook.SyncEvent]{})
	return &App{opts: opts, lifecycles: &query.Lifecycles{}, hooks: hooks}
}

// Documents is the Document Service, available once Setup ran (in route
// handlers and hooks).
func (a *App) Documents() *DocumentService {
	if a.docs == nil {
		return nil
	}
	return &DocumentService{a.docs}
}

func (a *App) stderr() io.Writer {
	if a.opts.Stderr != nil {
		return a.opts.Stderr
	}
	return os.Stderr
}
