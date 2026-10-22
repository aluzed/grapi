package httpapi

import (
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/openapi"

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

// Policy runs before a route handler: false answers Strapi's PolicyError.
type Policy func(r *http.Request, req *Request) (bool, error)

var errPolicyFailed = newError(http.StatusForbidden, "PolicyError", "Policy Failed")

// CustomRoute is a route added by the project (config/routes or Go). Path
// is relative to the API prefix ("/account/anonymize"); Scope is the
// permission it needs ("api::account.account.anonymize"), Public disables
// authentication like config.auth: false.
type CustomRoute struct {
	Method   string
	Path     string
	Scope    string
	Public   bool
	Handler  Handler
	Policies []Policy
	// OpenAPI is the route's operation in the OpenAPI document, if any.
	OpenAPI map[string]any
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
		method: strings.ToUpper(cr.Method), path: path, scope: cr.Scope, public: cr.Public, policies: cr.Policies,
		handler: func(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
			return h(w, r, &Request{Params: rc.params, Query: rc.query, User: rc.auth.User})
		},
	}
	s.routes = append([]route{rt}, s.routes...)
	if cr.OpenAPI != nil {
		segs := strings.Split(strings.Trim(cr.Path, "/"), "/")
		for i, seg := range segs {
			if strings.HasPrefix(seg, ":") {
				segs[i] = "{" + seg[1:] + "}"
			}
		}
		s.customOps = append(s.customOps, openapi.Operation{Method: strings.ToUpper(cr.Method), Path: "/" + strings.Join(segs, "/"), Spec: cr.OpenAPI})
	}
}

// AddPolicies adds policies to the generated routes of action
// ("api::post.post.find"); it reports whether a route matched.
func (s *Server) AddPolicies(action string, ps ...Policy) bool {
	found := false
	for i := range s.routes {
		rt := &s.routes[i]
		scope := rt.uid + "." + rt.action
		if rt.scope != "" {
			scope = rt.scope
		}
		if scope == action {
			rt.policies = append(rt.policies, ps...)
			found = true
		}
	}
	return found
}

func (s *Server) runPolicies(r *http.Request, rt *route, rc *requestContext) error {
	for _, p := range rt.policies {
		ok, err := p(r, &Request{Params: rc.params, Query: rc.query, User: rc.auth.User})
		if err != nil {
			return err
		}
		if !ok {
			return errPolicyFailed
		}
	}
	return nil
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
