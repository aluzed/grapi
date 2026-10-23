package permissions

import (
	"sort"
	"sync"
)

// User is what a condition handler sees of the user.
type User struct {
	ID      any
	RoleIDs []any
}

// Condition turns a user into a filter (a where tree of the query engine)
// or a boolean: true lifts the restriction, false drops the permission.
type Condition struct {
	ID          string
	DisplayName string
	Category    string
	Handler     func(u User) any
}

// Built-in admin conditions.
const (
	IsCreator            = "admin::is-creator"
	HasSameRoleAsCreator = "admin::has-same-role-as-creator"
)

// Conditions is the condition registry.
type Conditions struct {
	mu   sync.RWMutex
	byID map[string]Condition
}

// NewConditions returns a registry with the built-in conditions.
func NewConditions() *Conditions {
	c := &Conditions{}
	c.Register(
		Condition{ID: IsCreator, DisplayName: "Is creator", Category: "default", Handler: func(u User) any {
			return map[string]any{"createdBy": map[string]any{"id": map[string]any{"$eq": u.ID}}}
		}},
		Condition{ID: HasSameRoleAsCreator, DisplayName: "Has same role as creator", Category: "default", Handler: func(u User) any {
			return map[string]any{"createdBy": map[string]any{"roles": map[string]any{"id": map[string]any{"$in": u.RoleIDs}}}}
		}},
	)
	return c
}

// Register adds or replaces conditions.
func (r *Conditions) Register(conds ...Condition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[string]Condition{}
	}
	for _, c := range conds {
		r.byID[c.ID] = c
	}
}

// Get returns the condition id.
func (r *Conditions) Get(id string) (Condition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[id]
	return c, ok
}

// All returns the conditions sorted by id.
func (r *Conditions) All() []Condition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Condition, 0, len(r.byID))
	for _, c := range r.byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
