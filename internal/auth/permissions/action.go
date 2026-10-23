// Package permissions is the RBAC engine shared by the admin, the API tokens
// and users-permissions: permissions are loaded by the caller, the engine
// never touches the database.
package permissions

import (
	"sort"
	"sync"
)

// Sections of the admin permissions screen.
const (
	SectionContentTypes = "contentTypes"
	SectionPlugins      = "plugins"
	SectionSettings     = "settings"
	SectionInternal     = "internal"
)

// Action is a registered action, plugin::content-manager.explorer.read or
// admin::webhooks.create.
type Action struct {
	UID         string
	DisplayName string
	PluginName  string
	Section     string
	Category    string
	SubCategory string
	// Subjects are the content types the action applies to (contentTypes
	// section); empty for subject-less actions.
	Subjects []string
	// ApplyToProperties are "fields" and "locales" when the action takes them.
	ApplyToProperties []string
}

// ActionID is the uid prefixed by its owner: admin::users.read,
// plugin::content-manager.explorer.read.
func (a Action) ActionID() string {
	if a.PluginName == "" || a.PluginName == "admin" {
		return "admin::" + a.UID
	}
	return "plugin::" + a.PluginName + "." + a.UID
}

// Actions is the action registry.
type Actions struct {
	mu   sync.RWMutex
	byID map[string]Action
}

// Register adds or replaces actions.
func (r *Actions) Register(actions ...Action) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[string]Action{}
	}
	for _, a := range actions {
		r.byID[a.ActionID()] = a
	}
}

// Get returns the action id.
func (r *Actions) Get(id string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byID[id]
	return a, ok
}

// All returns the actions sorted by id.
func (r *Actions) All() []Action {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Action, 0, len(r.byID))
	for _, a := range r.byID {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ActionID() < out[j].ActionID() })
	return out
}
