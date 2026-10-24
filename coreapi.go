package grapi

import (
	"net/http"

	"github.com/aluzed/grapi/internal/httpapi"
)

// CoreRouterRef is a content type's core router (createCoreRouter).
type CoreRouterRef struct{ c *httpapi.CoreRouterConfig }

// CoreRouter configures the core routes of a content type, as
// createCoreRouter's options do.
func (a *App) CoreRouter(uid string) *CoreRouterRef {
	ref := &CoreRouterRef{c: &httpapi.CoreRouterConfig{}}
	a.coreRouterRefs = append(a.coreRouterRefs, coreRouterRef{uid: uid, ref: ref})
	return ref
}

type coreRouterRef struct {
	uid string
	ref *CoreRouterRef
}

// Only keeps these actions (find, findOne, create, update, delete).
func (r *CoreRouterRef) Only(actions ...string) *CoreRouterRef {
	r.c.Only = append(r.c.Only, actions...)
	return r
}

// Except removes these actions.
func (r *CoreRouterRef) Except(actions ...string) *CoreRouterRef {
	r.c.Except = append(r.c.Except, actions...)
	return r
}

// CoreAction overrides a core action as createCoreController does with a
// call to super: next runs the core action and returns its body ({data,
// meta}, nil for an empty answer), which fn may change or replace. The
// answer keeps the core action's status.
type CoreAction func(c *Request, next func() (map[string]any, error)) (map[string]any, error)

// OverrideCoreAction overrides the action (find, findOne, create, update,
// delete) of uid's core controller, or a plugin's route by its controller
// action ("plugin::users-permissions.user", "me"), as a Strapi extension
// wraps the plugin's controller.
func (a *App) OverrideCoreAction(uid, action string, fn CoreAction) {
	if a.coreActions == nil {
		a.coreActions = map[string]httpapi.CoreAction{}
	}
	a.coreActions[uid+"."+action] = func(w http.ResponseWriter, r *http.Request, req *httpapi.Request, next func() (map[string]any, error)) (map[string]any, error) {
		c := &Request{W: w, R: r, Params: req.Params, Query: req.Query}
		if req.User != nil {
			c.User = map[string]any{}
			for k, v := range req.User.All() {
				c.User[k] = v
			}
		}
		return fn(c, next)
	}
}

func (a *App) coreAPIConfig(cfg *httpapi.Config) {
	cfg.CoreRouters = map[string]httpapi.CoreRouterConfig{}
	for _, r := range a.coreRouterRefs {
		c := cfg.CoreRouters[r.uid]
		c.Only = append(c.Only, r.ref.c.Only...)
		c.Except = append(c.Except, r.ref.c.Except...)
		cfg.CoreRouters[r.uid] = c
	}
	cfg.CoreActions = a.coreActions
}
