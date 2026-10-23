package grapi

import (
	"context"
	"sort"

	"github.com/aluzed/grapi/internal/db"

	"github.com/aluzed/grapi/hook"
	"github.com/aluzed/grapi/schema"
)

// OnRegister runs fn when the App sets up, before the database is opened
// for writing (src/index.js register).
func (a *App) OnRegister(fn func(ctx context.Context) error) {
	a.onRegister = append(a.onRegister, fn)
}

// OnBootstrap runs fn once the database is synced and the services are
// ready, before the server answers (src/index.js bootstrap): Documents()
// is available. An error stops the start.
func (a *App) OnBootstrap(fn func(ctx context.Context) error) {
	a.onBootstrap = append(a.onBootstrap, fn)
}

// OnDestroy runs fn when the server closes (src/index.js destroy).
func (a *App) OnDestroy(fn func(ctx context.Context) error) {
	a.onDestroy = append(a.onDestroy, fn)
}

// Hooks is the hook registry (strapi.hook): the built-in
// hook.BeforeSync and hook.AfterSync, and the project's own.
func (a *App) Hooks() *hook.Registry { return a.hooks }

func (a *App) syncHook(ctx context.Context, name string, set *schema.Set, d *db.DB) error {
	h, ok := a.hooks.Get(name).(*hook.Parallel[hook.SyncEvent])
	if !ok {
		return nil
	}
	ev := hook.SyncEvent{}
	for uid := range set.ContentTypes.All() {
		ev.ContentTypes = append(ev.ContentTypes, uid)
	}
	if old, err := readStoredSchema(ctx, d); err == nil && old != nil {
		for uid := range old {
			ev.OldContentTypes = append(ev.OldContentTypes, uid)
		}
		sort.Strings(ev.OldContentTypes)
	}
	_, err := h.Call(ctx, ev)
	return err
}
