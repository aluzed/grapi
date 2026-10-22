// Package events is Strapi's event hub: named events, listeners per name
// and subscribers to every event.
package events

import (
	"fmt"
	"log/slog"
	"sync"
)

// Listener gets an event's payload.
type Listener func(payload map[string]any)

// Subscriber gets every event.
type Subscriber func(name string, payload map[string]any)

// Hub dispatches events. A listener that panics is logged and does not stop
// the others.
type Hub struct {
	mu          sync.RWMutex
	listeners   map[string][]*Listener
	subscribers []*Subscriber
	Log         *slog.Logger
}

// New returns an empty hub.
func New(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{listeners: map[string][]*Listener{}, Log: log}
}

// On adds a listener and returns the function that removes it.
func (h *Hub) On(name string, l Listener) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := &l
	h.listeners[name] = append(h.listeners[name], p)
	return func() { h.off(name, p) }
}

// Once adds a listener removed after its first call.
func (h *Hub) Once(name string, l Listener) func() {
	var off func()
	var once sync.Once
	off = h.On(name, func(p map[string]any) {
		once.Do(func() {
			off()
			l(p)
		})
	})
	return off
}

func (h *Hub) off(name string, p *Listener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.listeners[name]
	for i, e := range list {
		if e == p {
			h.listeners[name] = append(list[:i:i], list[i+1:]...)
			return
		}
	}
}

// Subscribe adds a subscriber to every event.
func (h *Hub) Subscribe(s Subscriber) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := &s
	h.subscribers = append(h.subscribers, p)
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, e := range h.subscribers {
			if e == p {
				h.subscribers = append(h.subscribers[:i:i], h.subscribers[i+1:]...)
				return
			}
		}
	}
}

// Destroy removes every listener and subscriber.
func (h *Hub) Destroy() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners = map[string][]*Listener{}
	h.subscribers = nil
}

func (h *Hub) safe(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			h.Log.Error("event listener panicked", "event", name, "panic", fmt.Sprint(r))
		}
	}()
	fn()
}

// Emit calls the listeners of name, then the subscribers, in order.
func (h *Hub) Emit(name string, payload map[string]any) {
	if h == nil {
		return
	}
	h.mu.RLock()
	listeners := append([]*Listener(nil), h.listeners[name]...)
	subscribers := append([]*Subscriber(nil), h.subscribers...)
	h.mu.RUnlock()
	for _, l := range listeners {
		h.safe(name, func() { (*l)(payload) })
	}
	for _, s := range subscribers {
		h.safe(name, func() { (*s)(name, payload) })
	}
}
