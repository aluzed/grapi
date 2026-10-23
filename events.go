package grapi

// EventHub is strapi.eventHub: the events the entries, the media and the
// admin emit (entry.create, media.update...), the ones webhooks send.
type EventHub struct{ app *App }

// Events is the event hub, available once Setup ran.
func (a *App) Events() *EventHub { return &EventHub{a} }

// On calls fn on every name event; the returned function unsubscribes.
func (h *EventHub) On(name string, fn func(payload map[string]any)) func() {
	return h.app.docs.Events.On(name, fn)
}

// Subscribe calls fn on every event; the returned function unsubscribes.
func (h *EventHub) Subscribe(fn func(name string, payload map[string]any)) func() {
	return h.app.docs.Events.Subscribe(fn)
}

// Emit sends an event to the listeners and the webhooks.
func (h *EventHub) Emit(name string, payload map[string]any) {
	h.app.docs.Events.Emit(name, payload)
}
