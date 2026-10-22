package grapi

import "context"

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
