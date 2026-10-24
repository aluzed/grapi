// Package grapi is the entry point for embedding grapi as a Go framework:
// build an App, add hooks, then Start it. The grapi binary is such a main
// without hooks.
package grapi

import (
	"context"
	"io"
	"os"

	"github.com/aluzed/grapi/hook"

	"github.com/aluzed/grapi/internal/graphql"
	"github.com/aluzed/grapi/internal/httpapi"

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
	onRegister          []func(context.Context) error
	onBootstrap         []func(context.Context) error
	onDestroy           []func(context.Context) error
	uploadProviders     map[string]UploadProvider
	emailProviders      map[string]EmailProvider
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
