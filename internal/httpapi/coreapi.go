package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// CoreRouterConfig is createCoreRouter's only and except, by content type.
type CoreRouterConfig struct{ Only, Except []string }

func (c CoreRouterConfig) keeps(action string) bool {
	for _, a := range c.Except {
		if a == action {
			return false
		}
	}
	if len(c.Only) == 0 {
		return true
	}
	for _, a := range c.Only {
		if a == action {
			return true
		}
	}
	return false
}

// CoreAction overrides a core API action as createCoreController does with
// a call to super: next runs the core action and returns its decoded body
// ({data, meta}, nil for an empty answer), which the override may change.
type CoreAction func(w http.ResponseWriter, r *http.Request, req *Request, next func() (map[string]any, error)) (map[string]any, error)

type bodyRecorder struct {
	header http.Header
	status int
	buf    bytes.Buffer
}

func (b *bodyRecorder) Header() http.Header         { return b.header }
func (b *bodyRecorder) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bodyRecorder) WriteHeader(status int)      { b.status = status }

func (s *Server) overridden(h func(http.ResponseWriter, *http.Request, *requestContext) error, action CoreAction) func(http.ResponseWriter, *http.Request, *requestContext) error {
	return func(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
		status := http.StatusOK
		next := func() (map[string]any, error) {
			rec := &bodyRecorder{header: http.Header{}}
			if err := h(rec, r, rc); err != nil {
				return nil, err
			}
			for k, v := range rec.header {
				w.Header()[k] = v
			}
			if rec.status != 0 {
				status = rec.status
			}
			if rec.buf.Len() == 0 {
				return nil, nil
			}
			dec := json.NewDecoder(&rec.buf)
			dec.UseNumber()
			var body map[string]any
			if err := dec.Decode(&body); err != nil {
				return nil, err
			}
			return body, nil
		}
		req := &Request{Params: rc.params, Query: rc.query}
		if rc.auth != nil {
			req.User = rc.auth.User
		}
		body, err := action(w, r, req, next)
		if err != nil {
			return err
		}
		if body == nil {
			w.WriteHeader(status)
			return nil
		}
		writeJSON(w, status, body)
		return nil
	}
}
