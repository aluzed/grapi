// Package project is a loaded Strapi project for the CLI: its config,
// schemas and database. The grapi package fills in the loader and the
// helpers that need the App's wiring.
package project

import (
	"io"
	"log/slog"

	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/registry"
)

// Project is a loaded Strapi project.
type Project struct {
	Dir     string
	Conf    *config.Config
	Res     *registry.Result
	MD      *metadata.Metadata
	Desired *metadata.Schema
	DB      *db.DB
	Log     *slog.Logger
}

// Hooks are set by the grapi package.
var (
	Loader    func(dir, configDir string, stderr io.Writer) (*Project, error)
	OpenAPIOf func(p *Project) ([]byte, error)
	GraphQLOf func(p *Project) bool
	RoutesOf  func(p *Project) []httpapi.RouteInfo
)

// Load reads the config and the schemas of dir and opens its database,
// without writing anything.
func Load(dir, configDir string, stderr io.Writer) (*Project, error) {
	return Loader(dir, configDir, stderr)
}

// OpenAPI is the project's content API document.
func (p *Project) OpenAPI() ([]byte, error) { return OpenAPIOf(p) }

// GraphQLEnabled reports whether the graphql plugin is on.
func (p *Project) GraphQLEnabled() bool { return GraphQLOf(p) }

// Routes is the project's route table, without starting anything.
func (p *Project) Routes() []httpapi.RouteInfo { return RoutesOf(p) }
