package grapi

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/aluzed/grapi/internal/clitable"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/httpapi"
)

// RouteInfo is a route of the content API, the permission action it
// checks and the names of its policies.
type RouteInfo struct {
	Method, Path, Action string
	Policies             []string
}

func policyName(ref any) string {
	switch r := ref.(type) {
	case string:
		return r
	case PolicyRef:
		return r.Name
	case *PolicyRef:
		return r.Name
	}
	return "inline"
}

func policyNames(refs []any) []string {
	var out []string
	for _, r := range refs {
		out = append(out, policyName(r))
	}
	return out
}

// Routes is the content API route table the App serves, its Go routes and
// core router settings included, without starting anything.
func (a *App) Routes() ([]RouteInfo, error) {
	dir := a.opts.Dir
	if dir == "" {
		dir = "."
	}
	p, err := loadProject(dir, a.opts.ConfigDir, io.Discard)
	if err != nil {
		return nil, err
	}
	defer func() { _ = p.DB.Close() }()
	cfg := apiConfig(p.Conf, p.Dir)
	a.coreAPIConfig(&cfg)
	engine := &query.Engine{DB: p.DB, MD: p.MD}
	api := httpapi.New(cfg, p.DB, p.Res.Set, document.New(engine, p.Res.Set), p.Log)
	if err := a.addRoutes(api); err != nil {
		return nil, err
	}
	byAction := map[string][]string{}
	for _, rp := range a.routePolicies {
		byAction[rp.action] = append(byAction[rp.action], policyNames(rp.refs)...)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "/api"
	}
	byRoute := map[string][]string{}
	for _, cr := range a.routes {
		byRoute[cr.method+" "+prefix+cr.path] = policyNames(cr.policies)
	}
	var out []RouteInfo
	for _, r := range api.Routes() {
		ps := append(append([]string{}, byRoute[r.Method+" "+r.Path]...), byAction[r.Action]...)
		out = append(out, RouteInfo{Method: r.Method, Path: r.Path, Action: r.Action, Policies: ps})
	}
	return out, nil
}

// listRoutes is `routes:list` run by a framework project's own binary, so
// that its Go routes are listed too.
func (a *App) listRoutes(w io.Writer) error {
	routes, err := a.Routes()
	if err != nil {
		return err
	}
	rows := make([]clitable.Route, 0, len(routes))
	for _, r := range routes {
		rows = append(rows, clitable.Route{Method: r.Method, Path: r.Path, Handler: r.Action, Policies: r.Policies})
	}
	_, err = fmt.Fprint(w, clitable.Routes(rows, slices.Contains(os.Args[2:], "--details")))
	return err
}

func commandArg() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	return ""
}
