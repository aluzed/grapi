package grapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/aluzed/grapi/internal/httpapi"
)

// Request is a custom route call.
type Request struct {
	W http.ResponseWriter
	R *http.Request
	// Params are the path parameters (":id").
	Params map[string]string
	// Query is the query string parsed like Strapi (qs).
	Query map[string]any
	// User is the authenticated end user's row, nil for anonymous calls
	// and public routes.
	User map[string]any
	body map[string]any
}

// Body decodes the JSON body once.
func (c *Request) Body() map[string]any {
	if c.body == nil {
		c.body = map[string]any{}
		b, _ := io.ReadAll(io.LimitReader(c.R.Body, 1<<20))
		_ = json.Unmarshal(b, &c.body)
	}
	return c.body
}

// JSON writes v (ctx.send).
func (c *Request) JSON(status int, v any) error {
	httpapi.WriteJSON(c.W, status, v)
	return nil
}

// Error returns an error answered in Strapi's format, with the http-errors
// name of status: Error(400, "Invalid current password") is ctx.badRequest.
func Error(status int, message string) error { return httpapi.HTTPError(status, message, nil) }

// RouteHandler handles a custom route.
type RouteHandler func(c *Request) error

type customRoute struct {
	method, path, scope string
	public              bool
	handler             RouteHandler
	policies            []any
	openapi             map[string]any
}

// Route adds a content API route under the API prefix. scope is the
// permission a role needs, by Strapi's naming ("api::account.account.anonymize").
func (a *App) Route(method, path, scope string, h RouteHandler) *RouteRef {
	a.routes = append(a.routes, &customRoute{method: method, path: path, scope: scope, handler: h})
	return &RouteRef{r: a.routes[len(a.routes)-1]}
}

// PublicRoute adds a route without authentication (config.auth: false).
func (a *App) PublicRoute(method, path string, h RouteHandler) *RouteRef {
	a.routes = append(a.routes, &customRoute{method: method, path: path, public: true, handler: h})
	return &RouteRef{r: a.routes[len(a.routes)-1]}
}

func (a *App) addRoutes(api *httpapi.Server) error {
	for _, rp := range a.routePolicies {
		ps, err := a.policiesOf(rp.refs, "", rp.action, rp.action)
		if err != nil {
			return err
		}
		if !api.AddPolicies(rp.action, ps...) {
			return fmt.Errorf("policies: no route for the action %q", rp.action)
		}
	}
	for _, cr := range a.routes {
		h := cr.handler
		ps, err := a.policiesOf(cr.policies, cr.method, cr.path, cr.scope)
		if err != nil {
			return err
		}
		api.AddRoute(httpapi.CustomRoute{
			Method: cr.method, Path: cr.path, Scope: cr.scope, Public: cr.public, Policies: ps, OpenAPI: cr.openapi,
			Handler: func(w http.ResponseWriter, r *http.Request, req *httpapi.Request) error {
				c := &Request{W: w, R: r, Params: req.Params, Query: req.Query}
				if req.User != nil {
					c.User = map[string]any{}
					for k, v := range req.User.All() {
						c.User[k] = v
					}
				}
				return h(c)
			},
		})
	}
	return nil
}
