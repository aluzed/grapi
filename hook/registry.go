package hook

import (
	"context"
	"reflect"
	"sync"
)

// Handler is one handler of a hook.
type Handler[T any] func(ctx context.Context, v T) (T, error)

type entry[T any] struct {
	id int
	fn Handler[T]
}

type handlers[T any] struct {
	mu   sync.RWMutex
	list []entry[T]
	next int
}

// Register adds fn and returns the function that removes it.
func (h *handlers[T]) Register(fn Handler[T]) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	id := h.next
	h.list = append(h.list, entry[T]{id, fn})
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, e := range h.list {
			if e.id == id {
				h.list = append(h.list[:i:i], h.list[i+1:]...)
				return
			}
		}
	}
}

func (h *handlers[T]) snapshot() []Handler[T] {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Handler[T], len(h.list))
	for i, e := range h.list {
		out[i] = e.fn
	}
	return out
}

// Series runs the handlers in order on the same value; an error stops.
type Series[T any] struct{ handlers[T] }

// Call runs the handlers and returns their results.
func (h *Series[T]) Call(ctx context.Context, v T) ([]T, error) {
	var out []T
	for _, fn := range h.snapshot() {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		r, err := fn(ctx, v)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Waterfall gives each handler the result of the previous one.
type Waterfall[T any] struct{ handlers[T] }

// Call returns the last result.
func (h *Waterfall[T]) Call(ctx context.Context, v T) (T, error) {
	for _, fn := range h.snapshot() {
		if err := ctx.Err(); err != nil {
			return v, err
		}
		var err error
		if v, err = fn(ctx, v); err != nil {
			return v, err
		}
	}
	return v, nil
}

// Parallel runs the handlers concurrently and waits for all of them; the
// first error in registration order is returned.
type Parallel[T any] struct{ handlers[T] }

// Call returns the results in registration order.
func (h *Parallel[T]) Call(ctx context.Context, v T) ([]T, error) {
	fns := h.snapshot()
	out := make([]T, len(fns))
	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				errs[i] = err
				return
			}
			out[i], errs[i] = fn(ctx, v)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// Bail stops at the first handler that returns a non-zero value.
type Bail[T any] struct{ handlers[T] }

// Call returns that value, and false when no handler returned one.
func (h *Bail[T]) Call(ctx context.Context, v T) (T, bool, error) {
	var zero T
	for _, fn := range h.snapshot() {
		if err := ctx.Err(); err != nil {
			return zero, false, err
		}
		r, err := fn(ctx, v)
		if err != nil {
			return zero, false, err
		}
		if rv := reflect.ValueOf(r); rv.IsValid() && !rv.IsZero() {
			return r, true, nil
		}
	}
	return zero, false, nil
}

// Registry is the namespaced hook registry (strapi.hook(name)).
type Registry struct {
	mu    sync.RWMutex
	hooks map[string]any
}

// Register adds a hook under name ("plugin::my-plugin.seeded").
func (r *Registry) Register(name string, h any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hooks == nil {
		r.hooks = map[string]any{}
	}
	r.hooks[name] = h
}

// Delete removes the hook name.
func (r *Registry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.hooks, name)
}

// Get returns the hook name, nil when there is none.
func (r *Registry) Get(name string) any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hooks[name]
}

// SyncEvent is the value of the content-types sync hooks.
type SyncEvent struct {
	// ContentTypes are the uids of the content types being synced.
	ContentTypes []string
}

// Names of the built-in hooks, both Parallel[SyncEvent] as in Strapi.
const (
	BeforeSync = "strapi::content-types.beforeSync"
	AfterSync  = "strapi::content-types.afterSync"
)
