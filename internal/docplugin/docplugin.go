// Package docplugin is Strapi's documentation plugin: Swagger UI on
// /documentation, behind a password when restrictedAccess is on (the
// plugin's config in the core store, a koa-session cookie once logged in).
package docplugin

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aluzed/grapi/internal/koasession"
)

//go:embed assets
var assets embed.FS

// Access is plugin_documentation_config: whether a password is asked, and
// its bcrypt hash.
type Access struct {
	RestrictedAccess bool   `json:"restrictedAccess"`
	Password         string `json:"password"`
}

// Config wires the plugin.
type Config struct {
	// BackendURL is server.url ("" serves relative links, as Strapi).
	BackendURL string
	// Keys are app.keys, which sign the session.
	Keys []string
	// Secure marks the session cookie Secure.
	Secure bool
	// Spec is the OpenAPI document shown.
	Spec func() ([]byte, error)
	// Access reads the plugin's config.
	Access func(ctx context.Context) (Access, error)
}

// Plugin serves the documentation pages and Swagger UI's files.
type Plugin struct{ cfg Config }

// New is the plugin.
func New(cfg Config) *Plugin { return &Plugin{cfg: cfg} }

var version = regexp.MustCompile(`^/documentation/v\d+\.\d+\.\d+/?$`)

// Handles reports whether the plugin answers path.
func (p *Plugin) Handles(path string) bool {
	return path == "/documentation" || path == "/documentation/" || version.MatchString(path) ||
		path == "/documentation/login" || strings.HasPrefix(path, "/plugins/documentation/")
}

func (p *Plugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/plugins/documentation/"):
		sub, _ := fs.Sub(assets, "assets")
		http.StripPrefix("/plugins/documentation/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
	case r.URL.Path == "/documentation/login" && r.Method == http.MethodPost:
		p.login(w, r)
	case r.URL.Path == "/documentation/login" && r.Method == http.MethodGet:
		p.render(w, loginPage, map[string]any{"ActionURL": p.cfg.BackendURL + "/documentation/login", "Error": r.URL.Query().Get("error") != ""})
	case r.Method == http.MethodGet && r.URL.Path != "/documentation/login":
		p.index(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Plugin) index(w http.ResponseWriter, r *http.Request) {
	access, err := p.cfg.Access(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if access.RestrictedAccess {
		doc, _ := koasession.Read(r, p.cfg.Keys)["documentation"].(map[string]any)
		if logged, _ := doc["logged"].(bool); !logged {
			target := p.cfg.BackendURL + "/documentation/login"
			if q := r.URL.Query().Encode(); q != "" {
				target += "?" + q
			}
			http.Redirect(w, r, target, http.StatusFound) //nolint:gosec // the configured server URL and a fixed path: the query cannot change the host
			return
		}
	}
	spec, err := p.cfg.Spec()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	p.render(w, indexPage, map[string]any{"BackendURL": p.cfg.BackendURL, "Spec": template.JS(spec)}) //nolint:gosec // the document grapi generates, JSON
}

func (p *Plugin) login(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		password = body.Password
	}
	access, err := p.cfg.Access(r.Context())
	query := "?error=password"
	if err == nil && access.Password != "" && bcrypt.CompareHashAndPassword([]byte(access.Password), []byte(password)) == nil {
		if err := koasession.Write(w, map[string]any{"documentation": map[string]any{"logged": true}}, 24*time.Hour, p.cfg.Keys, p.cfg.Secure); err == nil {
			query = ""
		}
	}
	http.Redirect(w, r, p.cfg.BackendURL+"/documentation"+query, http.StatusFound)
}

func (p *Plugin) render(w http.ResponseWriter, t *template.Template, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, data)
}

var indexPage = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <title>Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="{{.BackendURL}}/plugins/documentation/swagger-ui.css">
    <link rel="icon" type="image/png" href="{{.BackendURL}}/plugins/documentation/favicon-32x32.png" sizes="32x32">
    <link rel="icon" type="image/png" href="{{.BackendURL}}/plugins/documentation/favicon-16x16.png" sizes="16x16">
    <style>html { box-sizing: border-box; overflow-y: scroll; } *, *:before, *:after { box-sizing: inherit; } body { margin: 0; background: #fafafa; }</style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script>
      window.onload = function() {
        window.ui = SwaggerUIBundle({
          spec: {{.Spec}},
          dom_id: '#swagger-ui',
          docExpansion: "none",
          deepLinking: true,
          presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
          plugins: [SwaggerUIBundle.plugins.DownloadUrl],
          layout: "StandaloneLayout",
        });
      }
    </script>
    <script src="{{.BackendURL}}/plugins/documentation/swagger-ui-bundle.js"></script>
    <script src="{{.BackendURL}}/plugins/documentation/swagger-ui-standalone-preset.js"></script>
  </body>
</html>
`))

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Login - Documentation</title>
    <style>
      body { margin: 0; min-height: 100vh; background: #f6f9fc; font: 16px system-ui, sans-serif; display: flex; align-items: flex-start; justify-content: center; }
      form { margin: 12vh 16px 0; width: 100%; max-width: 420px; background: #fff; border-top: 2px solid #4945ff; border-radius: 4px; box-shadow: 0 2px 4px rgba(91, 107, 174, .15); padding: 24px; display: flex; flex-direction: column; gap: 12px; }
      h1 { font-size: 18px; margin: 0 0 8px; }
      .error { color: #d02b20; }
      input { font: inherit; padding: 10px; border: 1px solid #dcdce4; border-radius: 4px; }
      input[type=submit] { background: #4945ff; border-color: #4945ff; color: #fff; cursor: pointer; }
    </style>
  </head>
  <body>
    <form method="post" action="{{.ActionURL}}">
      <h1>Enter the password to access the documentation.</h1>
      <span class="error">{{if .Error}}Wrong password...{{end}}</span>
      <label for="password">Password</label>
      <input id="password" type="password" name="password" autocomplete="current-password">
      <input type="submit" value="Login">
    </form>
  </body>
</html>
`))
