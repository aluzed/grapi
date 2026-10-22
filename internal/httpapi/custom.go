package httpapi

import (
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
)

// Request is what a custom route handler gets.
type Request struct {
	Params map[string]string
	Query  map[string]any
	// User is the authenticated end user (users-permissions), nil otherwise.
	User query.Row
}

// Handler is a custom route handler.
type Handler func(w http.ResponseWriter, r *http.Request, req *Request) error

// CustomRoute is a route added by the project (config/routes or Go). Path
// is relative to the API prefix ("/account/anonymize"); Scope is the
// permission it needs ("api::account.account.anonymize"), Public disables
// authentication like config.auth: false.
type CustomRoute struct {
	Method  string
	Path    string
	Scope   string
	Public  bool
	Handler Handler
}

// AddRoute registers r ahead of the core routes.
func (s *Server) AddRoute(cr CustomRoute) {
	path := []string{strings.TrimPrefix(s.cfg.Prefix, "/")}
	for _, seg := range strings.Split(strings.Trim(cr.Path, "/"), "/") {
		if seg != "" {
			path = append(path, seg)
		}
	}
	h := cr.Handler
	rt := route{
		method: strings.ToUpper(cr.Method), path: path, scope: cr.Scope, public: cr.Public,
		handler: func(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
			return h(w, r, &Request{Params: rc.params, Query: rc.query, User: rc.auth.User})
		},
	}
	s.routes = append([]route{rt}, s.routes...)
}

// HTTPError is Strapi's ctx.badRequest, ctx.unauthorized... : the
// http-errors name of the status ("BadRequestError").
func HTTPError(status int, message string, details any) error {
	name := strings.ReplaceAll(http.StatusText(status), " ", "") + "Error"
	switch status {
	case http.StatusInternalServerError:
		name = "InternalServerError"
	case http.StatusRequestEntityTooLarge:
		name = "PayloadTooLargeError"
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &APIError{Status: status, Name: name, Message: message, Details: details}
}

// WriteJSON writes v as JSON, as ctx.body does.
func WriteJSON(w http.ResponseWriter, status int, v any) { writeJSON(w, status, v) }
