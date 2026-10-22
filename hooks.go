package grapi

import (
	"github.com/aluzed/grapi/document"
	"github.com/aluzed/grapi/hook"
	"github.com/aluzed/grapi/internal/db/query"
)

// UseDocuments adds a Document Service middleware (strapi.documents.use).
// Register it before Start.
func (a *App) UseDocuments(m document.Middleware) {
	a.documentMiddlewares = append(a.documentMiddlewares, m)
}

// RowHook binds handlers to row lifecycle actions of some models.
type RowHook struct {
	app     *App
	uids    []string
	actions []string
}

// OnRow selects one lifecycle action ("beforeCreate", "afterUpdate", ...)
// of the given models (all models when none).
func (a *App) OnRow(action string, uids ...string) *RowHook {
	return &RowHook{app: a, uids: uids, actions: []string{action}}
}

// OnRowCreate is beforeCreate: every row written by a create, the rows
// publish writes included.
func (a *App) OnRowCreate(uids ...string) *RowHook {
	return &RowHook{app: a, uids: uids, actions: []string{"beforeCreate"}}
}

// OnRowUpdate is beforeUpdate.
func (a *App) OnRowUpdate(uids ...string) *RowHook {
	return &RowHook{app: a, uids: uids, actions: []string{"beforeUpdate"}}
}

// OnRowWrite is beforeCreate and beforeUpdate.
func (a *App) OnRowWrite(uids ...string) *RowHook {
	return &RowHook{app: a, uids: uids, actions: []string{"beforeCreate", "beforeUpdate"}}
}

// OnRowDelete is beforeDelete.
func (a *App) OnRowDelete(uids ...string) *RowHook {
	return &RowHook{app: a, uids: uids, actions: []string{"beforeDelete"}}
}

// Bind registers fn and returns the function that removes it.
func (h *RowHook) Bind(fn hook.RowHandler) func() {
	handler := func(e *query.Event) error {
		re := &hook.RowEvent{Ctx: e.Ctx, UID: e.Model.UID, Action: e.Action, State: e.State, Result: e.Result}
		if e.Params != nil {
			re.Where = e.Params.Where
			if d, ok := e.Params.Data.(map[string]any); ok {
				re.Data = d
			}
		}
		if err := fn(re); err != nil {
			return err
		}
		if e.Params != nil {
			e.Params.Where = re.Where
			if re.Data != nil {
				e.Params.Data = re.Data
			}
		}
		return nil
	}
	hooks := map[string]func(*query.Event) error{}
	for _, action := range h.actions {
		hooks[action] = handler
	}
	return h.app.lifecycles.Subscribe(query.Subscriber{Models: h.uids, Hooks: hooks})
}
