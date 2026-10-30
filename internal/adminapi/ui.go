package adminapi

import (
	"bytes"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const placeholder = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>grapi admin</title><link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='14' fill='%234F46E5'/%3E%3Cpath fill='none' stroke='%23fff' stroke-width='8' stroke-linecap='round' stroke-linejoin='round' d='M44.26 21.72A16 16 0 1 0 48 32H34'/%3E%3C/svg%3E"></head>
<body style="font-family:system-ui,sans-serif;margin:2rem"><h1>grapi admin</h1><p>This binary was built without the admin interface. Build it with <code>make build</code>.</p></body></html>`

// serveUI serves the SPA: files as they are, every other path gets
// index.html so that the client router takes over.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.UI == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder))
		return
	}
	name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, s.cfg.Path), "/")
	name = path.Clean("/" + name)[1:]
	if name != "" {
		if st, err := fs.Stat(s.cfg.UI, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.cfg.UI, name) //nolint:gosec // name is cleaned and fs.FS rejects paths outside its root
			return
		}
		// Client routes can end with a dot (api::article.article): only a
		// missing build asset is a 404.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
	}
	index, err := fs.ReadFile(s.cfg.UI, "index.html")
	if err != nil {
		http.Error(w, "admin build is missing index.html", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(withBase(index, s.cfg.Path+"/"))
}

// withBase points the relative URLs of the build at the admin path, so one
// build serves any admin.url.
func withBase(index []byte, base string) []byte {
	tag := []byte(`<head><base href="` + html.EscapeString(base) + `">`)
	return bytes.Replace(index, []byte("<head>"), tag, 1)
}
