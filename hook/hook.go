// Package hook defines the public hook types: row lifecycles, document
// middlewares and policies.
package hook

import "context"

// RowEvent is a row lifecycle event (Strapi's db.lifecycles). In a before*
// event, Data and Where may be changed; returning an error cancels the
// operation and its transaction.
type RowEvent struct {
	Ctx context.Context
	// UID is the model, "api::post.post".
	UID string
	// Action is beforeCreate, afterCreate, beforeUpdate, ... (Strapi's names).
	Action string
	// Data is the written row (create and update), nil otherwise.
	Data map[string]any
	// Where selects the rows of update, delete and the finds.
	Where any
	// State is shared by the before and the after of one operation.
	State map[string]any
	// Result is set in the after* events.
	Result any
}

// String returns Data[key] when it is a string.
func (e *RowEvent) String(key string) string {
	s, _ := e.Data[key].(string)
	return s
}

// RowHandler handles row events.
type RowHandler func(*RowEvent) error
