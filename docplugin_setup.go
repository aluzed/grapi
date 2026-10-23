package grapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/docplugin"
	"github.com/aluzed/grapi/internal/httpapi"
	"github.com/aluzed/grapi/internal/openapi"
)

// documentationPlugin is Strapi's documentation plugin when the project
// enables it (plugins.documentation.enabled, or the dependency), else nil.
func (a *App) documentationPlugin(c *config.Config, dir string, d *db.DB, api *httpapi.Server, keys []string) *docplugin.Plugin {
	enabled, set := c.Bool("plugins.documentation.enabled")
	if !set {
		_, enabled = readPackage(dir).Dependencies["@strapi/plugin-documentation"]
	}
	if !enabled {
		return nil
	}
	pkg := readPackage(dir)
	info := openapi.Info{Name: pkg.Name, Version: pkg.Version}
	// server.url as configured, empty by default: Strapi's links are then relative.
	server, _ := c.String("server.url")
	return docplugin.New(docplugin.Config{
		BackendURL: strings.TrimRight(server, "/"),
		Keys:       keys,
		Secure:     strings.HasPrefix(server, "https://"),
		Spec:       func() ([]byte, error) { return api.OpenAPIJSON(info, a.openapiMutate) },
		Access: func(ctx context.Context) (docplugin.Access, error) {
			var access docplugin.Access
			_, err := (store.Store{DB: d}).Get(ctx, store.Scope{Type: "plugin", Name: "documentation"}, "config", &access)
			return access, err
		},
	})
}

func withDocumentation(p *docplugin.Plugin, next http.Handler) http.Handler {
	if p == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.Handles(r.URL.Path) {
			p.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
