package adminapi

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/webhooks"
)

func (s *Server) webhookRoutes() {
	if s.cfg.Webhooks == nil {
		return
	}
	s.allowed("GET /webhooks", "admin::webhooks.read", s.listWebhooks)
	s.allowed("GET /webhooks/events", "admin::webhooks.read", s.webhookEvents)
	s.allowed("GET /webhooks/{id}", "admin::webhooks.read", s.getWebhook)
	s.allowed("POST /webhooks", "admin::webhooks.create", s.createWebhook)
	s.allowed("PUT /webhooks/{id}", "admin::webhooks.update", s.updateWebhook)
	s.allowed("DELETE /webhooks/{id}", "admin::webhooks.delete", s.deleteWebhook)
	s.allowed("POST /webhooks/{id}/trigger", "admin::webhooks.update", s.triggerWebhook)
}

func webhookID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errNotFound
	}
	return id, nil
}

func webhookErr(err error) error {
	if errors.Is(err, webhooks.ErrNotFound) {
		return &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "webhook.notFound"}
	}
	return err
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	list, err := s.cfg.Webhooks.All(r.Context())
	if err != nil {
		return err
	}
	return data(w, list)
}

func (s *Server) webhookEvents(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	return data(w, webhooks.Events)
}

func (s *Server) getWebhook(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	id, err := webhookID(r)
	if err != nil {
		return err
	}
	wh, err := s.cfg.Webhooks.Get(r.Context(), id)
	if err != nil {
		return webhookErr(err)
	}
	return data(w, wh)
}

type webhookBody struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Events  []string          `json:"events"`
	Enabled *bool             `json:"isEnabled"`
}

func (b *webhookBody) webhook() (webhooks.Webhook, error) {
	var errs []FieldError
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		errs = append(errs, FieldError{Path: []string{"name"}, Message: "name is a required field"})
	}
	if u, err := url.Parse(b.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, FieldError{Path: []string{"url"}, Message: "url must be a valid http or https URL"})
	}
	for i, e := range b.Events {
		if !slices.Contains(webhooks.Events, e) {
			errs = append(errs, FieldError{Path: []string{"events", strconv.Itoa(i)}, Message: "Unknown event " + e})
		}
	}
	for k := range b.Headers {
		if strings.TrimSpace(k) == "" {
			errs = append(errs, FieldError{Path: []string{"headers"}, Message: "header names cannot be empty"})
			break
		}
	}
	if len(errs) > 0 {
		return webhooks.Webhook{}, validation(errs)
	}
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	events := b.Events
	if events == nil {
		events = []string{}
	}
	return webhooks.Webhook{Name: b.Name, URL: b.URL, Headers: b.Headers, Events: events, Enabled: enabled}, nil
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	var b webhookBody
	if err := decode(r, &b); err != nil {
		return err
	}
	wh, err := b.webhook()
	if err != nil {
		return err
	}
	if wh, err = s.cfg.Webhooks.Create(r.Context(), wh); err != nil {
		return err
	}
	s.audit(r, u, "webhook.create", map[string]any{"id": wh.ID, "name": wh.Name, "url": urlHost(wh.URL), "events": wh.Events})
	writeJSON(w, http.StatusCreated, map[string]any{"data": wh})
	return nil
}

func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	id, err := webhookID(r)
	if err != nil {
		return err
	}
	var b webhookBody
	if err := decode(r, &b); err != nil {
		return err
	}
	wh, err := b.webhook()
	if err != nil {
		return err
	}
	wh.ID = id
	if wh, err = s.cfg.Webhooks.Update(r.Context(), wh); err != nil {
		return webhookErr(err)
	}
	s.audit(r, u, "webhook.update", map[string]any{"id": wh.ID, "name": wh.Name, "url": urlHost(wh.URL), "events": wh.Events, "isEnabled": wh.Enabled})
	return data(w, wh)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	id, err := webhookID(r)
	if err != nil {
		return err
	}
	if err := s.cfg.Webhooks.Delete(r.Context(), id); err != nil {
		return webhookErr(err)
	}
	s.audit(r, u, "webhook.delete", map[string]any{"id": id})
	return data(w, map[string]any{"id": id})
}

// triggerWebhook posts the trigger-test event and reports the answer.
func (s *Server) triggerWebhook(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	id, err := webhookID(r)
	if err != nil {
		return err
	}
	wh, err := s.cfg.Webhooks.Get(r.Context(), id)
	if err != nil {
		return webhookErr(err)
	}
	status, err := s.cfg.Webhooks.Test(r.Context(), wh)
	out := map[string]any{"statusCode": status}
	if err != nil {
		out["message"] = err.Error()
	}
	return data(w, out)
}
