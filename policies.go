package grapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/httpapi"
)

// PolicyContext is what a policy gets: the request and the route it is
// about to run.
type PolicyContext struct {
	R *http.Request
	// Params are the path parameters, Query the parsed query string.
	Params map[string]string
	Query  map[string]any
	// User is the authenticated end user's row, nil for anonymous calls.
	User map[string]any
	// Method, Path and Scope describe the route.
	Method, Path, Scope string
}

// Policy decides whether a route runs. Returning false answers Strapi's
// 403 PolicyError; an error (grapi.Error) is answered as is. config is the
// value given with the policy on the route.
type Policy func(c *PolicyContext, config any) (bool, error)

// PolicyRef names a policy on a route, with its config. A plain string is
// a PolicyRef without config.
type PolicyRef struct {
	Name   string
	Config any
}

type routePolicies struct {
	action string
	refs   []any
}

// Policy registers a policy under its uid: "global::is-owner",
// "api::post.is-owner" or "plugin::my-plugin.is-owner".
func (a *App) Policy(uid string, p Policy) {
	if a.policies == nil {
		a.policies = map[string]Policy{}
	}
	a.policies[uid] = p
}

// RouteRef is a custom route, to add policies to.
type RouteRef struct{ r *customRoute }

// Policies adds policies to the route, by uid or relative to the route's
// API or plugin (its scope's namespace), then global.
func (r *RouteRef) Policies(refs ...any) *RouteRef {
	r.r.policies = append(r.r.policies, refs...)
	return r
}

// OpenAPI documents the route in the OpenAPI document (server.openapi):
// an operation object, with its parameters, requestBody and responses.
func (r *RouteRef) OpenAPI(operation map[string]any) *RouteRef {
	r.r.openapi = operation
	return r
}

// RoutePolicies adds policies to a generated route, named by its action
// ("api::post.post.find", "plugin::users-permissions.user.me").
func (a *App) RoutePolicies(action string, refs ...any) {
	a.routePolicies = append(a.routePolicies, routePolicies{action: action, refs: refs})
}

// policyNamespace is the api or plugin prefix of a scope: "api::post" for
// "api::post.post.find".
func policyNamespace(scope string) string {
	ns, rest, ok := strings.Cut(scope, "::")
	if !ok || (ns != "api" && ns != "plugin") {
		return ""
	}
	name, _, _ := strings.Cut(rest, ".")
	return ns + "::" + name
}

func (a *App) resolvePolicy(ref any, scope, route string) (Policy, any, error) {
	var name string
	var config any
	switch r := ref.(type) {
	case string:
		name = r
	case PolicyRef:
		name, config = r.Name, r.Config
	default:
		return nil, nil, fmt.Errorf("route %s: a policy is a name or a grapi.PolicyRef, not %T", route, ref)
	}
	candidates := []string{name}
	if !strings.Contains(name, "::") {
		candidates = nil
		if ns := policyNamespace(scope); ns != "" {
			candidates = append(candidates, ns+"."+name)
		}
		candidates = append(candidates, "global::"+name)
	}
	for _, uid := range candidates {
		if p, ok := a.policies[uid]; ok {
			return p, config, nil
		}
	}
	return nil, nil, fmt.Errorf("route %s: policy %q not found (looked for %s)", route, name, strings.Join(candidates, ", "))
}

func (a *App) policiesOf(refs []any, method, path, scope string) ([]httpapi.Policy, error) {
	var out []httpapi.Policy
	for _, ref := range refs {
		p, config, err := a.resolvePolicy(ref, scope, strings.TrimSpace(method+" "+path))
		if err != nil {
			return nil, err
		}
		out = append(out, func(r *http.Request, req *httpapi.Request) (bool, error) {
			return p(&PolicyContext{R: r, Params: req.Params, Query: req.Query, User: userMap(req), Method: method, Path: path, Scope: scope}, config)
		})
	}
	return out, nil
}

func userMap(req *httpapi.Request) map[string]any {
	if req.User == nil {
		return nil
	}
	m := map[string]any{}
	for k, v := range req.User.All() {
		m[k] = v
	}
	return m
}
