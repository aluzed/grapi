package adminapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const placeholder = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>grapi admin</title></head>
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
		if path.Ext(name) != "" {
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
	_, _ = w.Write(index)
}
