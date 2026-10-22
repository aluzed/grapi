package httpapi

import (
	"net/http"
	"strings"
	"sync"

	"github.com/aluzed/grapi/internal/openapi"
)

// OpenAPIConfig is server.openapi['content-api'] with access "public".
type OpenAPIConfig struct {
	// Path is relative to the API prefix, "/openapi.json" by default.
	Path string
	Info openapi.Info
	// Mutate are the project's changes to the document.
	Mutate []func(doc map[string]any)
}

func (s *Server) addOpenAPI() {
	cfg := s.cfg.OpenAPI
	if cfg == nil {
		return
	}
	p := cfg.Path
	if p == "" {
		p = "/openapi.json"
	}
	prefix := strings.TrimRight(s.cfg.Prefix, "/")
	p = strings.TrimPrefix(p, prefix)
	path := splitPath(prefix + "/" + strings.TrimLeft(p, "/"))
	var once sync.Once
	var doc []byte
	h := func(w http.ResponseWriter, _ *http.Request, _ *requestContext) error {
		once.Do(func() {
			doc, _ = openapi.JSON(openapi.Generate(s.set, openapi.Options{Info: cfg.Info, Served: s.Serves, Custom: s.customOps, Mutate: cfg.Mutate}))
		})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(doc)
		return nil
	}
	s.routes = append(s.routes, route{"GET", path, "", "openapi", h, "", true, false, nil})
}

// Serves tells whether a route answers method on an OpenAPI path of the
// content API ("/users/{id}"): parameters match parameters, literal
// segments literal ones.
func (s *Server) Serves(method, p string) bool {
	segs := []string{strings.TrimPrefix(s.cfg.Prefix, "/")}
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg != "" {
			segs = append(segs, seg)
		}
	}
	for i := range s.routes {
		rt := &s.routes[i]
		if rt.method != method || len(rt.path) != len(segs) {
			continue
		}
		ok := true
		for j, seg := range segs {
			param := strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")
			if param != strings.HasPrefix(rt.path[j], ":") || !param && seg != rt.path[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
