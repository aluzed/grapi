// Package grapi is the entry point for embedding grapi as a Go framework:
// build an App, add hooks, then Start it. The grapi binary is such a main
// without hooks.
package grapi

import (
	"io"
	"os"

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
}

// App is a grapi application.
type App struct {
	opts                Options
	lifecycles          *query.Lifecycles
	documentMiddlewares []document.Middleware
	routes              []customRoute
	docs                *document.Service
}

// New returns an App for the project in opts.Dir.
func New(opts Options) *App {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	return &App{opts: opts, lifecycles: &query.Lifecycles{}}
}

// Documents is the Document Service, available once Setup ran (in route
// handlers and hooks).
func (a *App) Documents() *document.Service { return a.docs }

func (a *App) stderr() io.Writer {
	if a.opts.Stderr != nil {
		return a.opts.Stderr
	}
	return os.Stderr
}
