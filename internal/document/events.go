package document

import (
	"context"
	"sync"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// emit is the document service's event manager: once the transaction is
// committed, the entry is read back populated (but for delete and
// unpublish), sanitized, then emitted as {model, uid, entry}.
func (s *Service) emit(ctx context.Context, name, uid string, entry query.Row) {
	if s.Events == nil || entry == nil {
		return
	}
	id, _ := entry.Get("id")
	db.OnCommit(ctx, func() {
		s.emits.push(func() {
			e := entry
			if name != "entry.delete" && name != "entry.unpublish" {
				row, err := s.E.FindOne(context.Background(), uid, query.Params{Where: map[string]any{"id": id}, Populate: s.deepPopulate(uid, nil)})
				if err != nil {
					s.Events.Log.Error("could not emit "+name+" for "+uid, "error", err)
					return
				}
				e = row
			}
			model := uid
			if ct, ok := s.Set.ContentType(schema.UID(uid)); ok {
				model = ct.Info.SingularName
			}
			s.Events.Emit(name, map[string]any{"model": model, "uid": uid, "entry": s.sanitizeEntry(uid, e)})
		})
	})
}

// emitter runs the event emissions one at a time, in commit order: a
// document created and published gives entry.create then entry.publish,
// as in Strapi. The worker goroutine ends when the queue is empty.
type emitter struct {
	mu      sync.Mutex
	queue   []func()
	running bool
}

func (e *emitter) push(fn func()) {
	e.mu.Lock()
	e.queue = append(e.queue, fn)
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()
	go e.drain()
}

func (e *emitter) drain() {
	for {
		e.mu.Lock()
		if len(e.queue) == 0 {
			e.running = false
			e.mu.Unlock()
			return
		}
		fn := e.queue[0]
		e.queue = e.queue[1:]
		e.mu.Unlock()
		fn()
	}
}

// sanitizeEntry is defaultSanitizeOutput without auth: passwords and
// private attributes removed, down the relations and components.
func (s *Service) sanitizeEntry(uid string, v any) any {
	m := s.model(uid)
	switch x := v.(type) {
	case query.Row:
		if x == nil {
			return nil
		}
		out := map[string]any{}
		for k, val := range x.All() {
			if m != nil {
				if a, ok := m.attrs.Get(k); ok {
					if a.Type == schema.TypePassword || a.IsPrivate() {
						continue
					}
					switch a.Type {
					case schema.TypeRelation:
						val = s.sanitizeEntry(a.Target, val)
					case schema.TypeComponent:
						val = s.sanitizeEntry(a.Component, val)
					case schema.TypeMedia:
						val = s.sanitizeEntry("plugin::upload.file", val)
					case schema.TypeDynamicZone:
						val = s.sanitizeZone(val)
					}
				}
			}
			out[k] = val
		}
		return out
	case []query.Row:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = s.sanitizeEntry(uid, e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = s.sanitizeEntry(uid, e)
		}
		return out
	}
	return v
}

func (s *Service) sanitizeZone(v any) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	out := make([]any, len(list))
	for i, e := range list {
		r, _ := e.(query.Row)
		comp := ""
		if r != nil {
			c, _ := r.Get("__component")
			comp, _ = c.(string)
		}
		out[i] = s.sanitizeEntry(comp, e)
	}
	return out
}
