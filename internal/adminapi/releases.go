package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

// Releases (a grapi design, Strapi's are Enterprise): documents published
// or unpublished together, by hand or at a scheduled time, all or nothing.
const (
	releasesTable = "grapi_releases"
	actionsTable  = "grapi_release_actions"
)

func (s *Server) ensureReleaseTables(ctx context.Context) error {
	id := "id integer primary key autoincrement"
	if s.e.DB.Dialect.Name() == "postgres" {
		id = "id serial primary key"
	}
	q := s.e.DB.Dialect.Quote
	for _, ddl := range []string{
		fmt.Sprintf(`create table if not exists %s (%s, name varchar(255) not null, scheduled_at varchar(64), timezone varchar(64),
			status varchar(32) not null, released_at varchar(64), error text, created_at varchar(64) not null, updated_at varchar(64) not null, created_by_id integer)`, q(releasesTable), id),
		fmt.Sprintf(`create table if not exists %s (%s, release_id integer not null, type varchar(16) not null, uid varchar(255) not null,
			document_id varchar(255) not null, locale varchar(255), created_at varchar(64) not null)`, q(actionsTable), id),
		fmt.Sprintf(`create index if not exists %s on %s (uid, document_id, locale)`, q("grapi_release_actions_document_idx"), q(actionsTable)),
	} {
		if _, err := s.e.DB.Exec(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) releaseRoutes() {
	s.allowed("GET /releases", "admin::releases.read", s.listReleases)
	s.allowed("POST /releases", "admin::releases.create", s.createRelease)
	s.allowed("GET /releases/document/{uid}/{documentId}", "admin::releases.read", s.documentReleases)
	s.allowed("GET /releases/{id}", "admin::releases.read", s.getRelease)
	s.allowed("PUT /releases/{id}", "admin::releases.update", s.updateRelease)
	s.allowed("DELETE /releases/{id}", "admin::releases.delete", s.deleteRelease)
	s.allowed("POST /releases/{id}/actions", "admin::releases.actions.create", s.addReleaseAction)
	s.allowed("DELETE /releases/{id}/actions/{actionId}", "admin::releases.actions.delete", s.removeReleaseAction)
	s.allowed("POST /releases/{id}/publish", "admin::releases.publish", s.publishReleaseHandler)
}

// Release is a release with its actions.
type Release struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	ScheduledAt *string         `json:"scheduledAt"`
	Timezone    *string         `json:"timezone"`
	Status      string          `json:"status"`
	ReleasedAt  *string         `json:"releasedAt"`
	Error       *string         `json:"error"`
	CreatedAt   string          `json:"createdAt"`
	Actions     []ReleaseAction `json:"actions,omitempty"`
	Count       int             `json:"count"`
}

// ReleaseAction is one document of a release.
type ReleaseAction struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"`
	UID         string   `json:"uid"`
	DocumentID  string   `json:"documentId"`
	Locale      *string  `json:"locale"`
	Title       string   `json:"title,omitempty"`
	ContentType string   `json:"contentType,omitempty"`
	State       string   `json:"state,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

func (s *Server) q(t string) string { return s.e.DB.Dialect.Quote(t) }

const releaseColumns = "id, name, scheduled_at, timezone, status, released_at, error, created_at"

func scanRelease(scan func(...any) error) (*Release, error) {
	r := &Release{}
	err := scan(&r.ID, &r.Name, &r.ScheduledAt, &r.Timezone, &r.Status, &r.ReleasedAt, &r.Error, &r.CreatedAt)
	return r, err
}

func (s *Server) loadRelease(ctx context.Context, id int64) (*Release, error) {
	r, err := scanRelease(s.e.DB.QueryRow(ctx, fmt.Sprintf(`select %s from %s where id = %s`, releaseColumns, s.q(releasesTable), s.ph(1)), id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Release not found"}
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.e.DB.Query(ctx, fmt.Sprintf(`select id, type, uid, document_id, locale from %s where release_id = %s order by id`, s.q(actionsTable), s.ph(1)), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	r.Actions = []ReleaseAction{}
	for rows.Next() {
		var a ReleaseAction
		if err := rows.Scan(&a.ID, &a.Type, &a.UID, &a.DocumentID, &a.Locale); err != nil {
			return nil, err
		}
		r.Actions = append(r.Actions, a)
	}
	r.Count = len(r.Actions)
	return r, rows.Err()
}

func releaseID(r *http.Request, key string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil {
		return 0, errNotFound
	}
	return id, nil
}

var errDryRun = errors.New("dry run")

func canPublish(a *permissions.Ability, uid string, locale any) bool {
	if l, ok := locale.(string); ok && l != "" {
		return a.CanLocale(actionPublish, uid, l)
	}
	return a.Can(actionPublish, uid, "")
}

func actionParams(ct *schema.ContentType, a ReleaseAction) document.Params {
	p := document.Params{}
	if ct.Localized() && a.Locale != nil {
		p["locale"] = *a.Locale
	}
	return p
}

// assess sets the state of each action: already-published or
// already-unpublished, ready, or not-ready with the publication errors
// (found by publishing in a transaction that is rolled back).
func (s *Server) assess(ctx context.Context, r *Release) {
	for i := range r.Actions {
		a := &r.Actions[i]
		ct, ok := s.set.ContentType(schema.UID(a.UID))
		if !ok {
			a.State, a.Errors = "not-ready", []string{"unknown content type"}
			continue
		}
		a.ContentType = ct.Info.DisplayName
		p := actionParams(ct, *a)
		row, err := s.docs.FindOne(ctx, a.UID, a.DocumentID, document.Params{"locale": p["locale"]})
		if err != nil || row == nil {
			a.State, a.Errors = "not-ready", []string{"the document no longer exists"}
			continue
		}
		cfg, _ := s.effectiveConfig(ctx, ct)
		if cfg != nil {
			if v := toMap(row)[cfg.Settings.MainField]; v != nil {
				a.Title = fmt.Sprint(v)
			}
		}
		res := []map[string]any{{schema.AttrDocumentID: a.DocumentID, "updatedAt": toMap(row)["updatedAt"]}}
		_ = s.withStatus(ctx, a.UID, p["locale"], "draft", res, []any{a.DocumentID})
		status, _ := res[0]["status"].(string)
		switch {
		case a.Type == "unpublish" && status == "draft":
			a.State = "already-unpublished"
		case a.Type == "unpublish":
			a.State = "ready"
		case status == "published":
			a.State = "already-published"
		default:
			err := s.e.DB.Tx(ctx, func(ctx context.Context) error {
				if _, err := s.docs.Publish(ctx, a.UID, a.DocumentID, p); err != nil {
					return err
				}
				return errDryRun
			})
			if errors.Is(err, errDryRun) {
				a.State = "ready"
			} else {
				a.State, a.Errors = "not-ready", []string{err.Error()}
			}
		}
	}
	switch {
	case r.Status == "done" || r.Status == "failed":
	case len(r.Actions) == 0:
		r.Status = "empty"
	default:
		r.Status = "ready"
		for _, a := range r.Actions {
			if a.State == "not-ready" {
				r.Status = "blocked"
			}
		}
	}
}

func (s *Server) saveStatus(ctx context.Context, r *Release) {
	_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`update %s set status = %s, updated_at = %s where id = %s and released_at is null and status <> 'failed'`, s.q(releasesTable), s.ph(1), s.ph(2), s.ph(3)),
		r.Status, s.now().UTC().Format(time.RFC3339Nano), r.ID)
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	where := " where released_at is null"
	if r.URL.Query().Get("status") == "done" {
		where = " where released_at is not null"
	}
	rows, err := s.e.DB.Query(r.Context(), fmt.Sprintf(`select %s, (select count(*) from %s a where a.release_id = %s.id) from %s%s order by created_at desc`,
		releaseColumns, s.q(actionsTable), s.q(releasesTable), s.q(releasesTable), where))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	out := []*Release{}
	for rows.Next() {
		var rel Release
		if err := rows.Scan(&rel.ID, &rel.Name, &rel.ScheduledAt, &rel.Timezone, &rel.Status, &rel.ReleasedAt, &rel.Error, &rel.CreatedAt, &rel.Count); err != nil {
			return err
		}
		out = append(out, &rel)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return data(w, out)
}

type releaseBody struct {
	Name        *string `json:"name"`
	ScheduledAt *string `json:"scheduledAt"`
	Timezone    *string `json:"timezone"`
}

// scheduled checks a schedule: an RFC 3339 instant, and an IANA timezone
// kept for display.
func scheduled(b releaseBody) (any, any, error) {
	if b.ScheduledAt == nil || *b.ScheduledAt == "" {
		return nil, nil, nil
	}
	at, err := time.Parse(time.RFC3339, *b.ScheduledAt)
	if err != nil {
		return nil, nil, validation([]FieldError{{Path: []string{"scheduledAt"}, Message: "scheduledAt must be an RFC 3339 date"}})
	}
	tz := "UTC"
	if b.Timezone != nil && *b.Timezone != "" {
		if _, err := time.LoadLocation(*b.Timezone); err != nil {
			return nil, nil, validation([]FieldError{{Path: []string{"timezone"}, Message: "unknown timezone"}})
		}
		tz = *b.Timezone
	}
	return at.UTC().Format(time.RFC3339), tz, nil
}

func (s *Server) createRelease(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	var b releaseBody
	if err := decode(r, &b); err != nil {
		return err
	}
	if b.Name == nil || strings.TrimSpace(*b.Name) == "" {
		return validation([]FieldError{{Path: []string{"name"}, Message: "name is a required field"}})
	}
	at, tz, err := scheduled(b)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	var id int64
	q := fmt.Sprintf(`insert into %s (name, scheduled_at, timezone, status, created_at, updated_at, created_by_id) values (%s, %s, %s, 'empty', %s, %s, %s) returning id`,
		s.q(releasesTable), s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6))
	if err := s.e.DB.QueryRow(r.Context(), q, strings.TrimSpace(*b.Name), at, tz, now, now, u.ID).Scan(&id); err != nil {
		return err
	}
	s.audit(r, u, "release.create", map[string]any{"id": id, "name": *b.Name, "scheduledAt": at})
	s.wakeScheduler(at)
	rel, err := s.loadRelease(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": rel})
	return nil
}

func (s *Server) getRelease(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	id, err := releaseID(r, "id")
	if err != nil {
		return err
	}
	rel, err := s.loadRelease(r.Context(), id)
	if err != nil {
		return err
	}
	if rel.ReleasedAt == nil {
		s.assess(r.Context(), rel)
		s.saveStatus(r.Context(), rel)
	}
	return data(w, rel)
}

var errReleased = &Error{Status: http.StatusConflict, Name: "ConflictError", Message: "This release is published and cannot change"}

func (s *Server) updateRelease(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	id, err := releaseID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	rel, err := s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	if rel.ReleasedAt != nil {
		return errReleased
	}
	var b releaseBody
	if err := decode(r, &b); err != nil {
		return err
	}
	name := rel.Name
	if b.Name != nil && strings.TrimSpace(*b.Name) != "" {
		name = strings.TrimSpace(*b.Name)
	}
	at, tz, err := scheduled(b)
	if err != nil {
		return err
	}
	// A new schedule gives a failed release another chance.
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`update %s set name = %s, scheduled_at = %s, timezone = %s, error = null, status = case when status = 'failed' then 'ready' else status end, updated_at = %s where id = %s`,
		s.q(releasesTable), s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5)), name, at, tz, s.now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	s.audit(r, u, "release.update", map[string]any{"id": id, "name": name, "scheduledAt": at})
	s.wakeScheduler(at)
	rel, err = s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	return data(w, rel)
}

func (s *Server) deleteRelease(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	id, err := releaseID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	if _, err := s.loadRelease(ctx, id); err != nil {
		return err
	}
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where release_id = %s`, s.q(actionsTable), s.ph(1)), id); err != nil {
		return err
	}
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where id = %s`, s.q(releasesTable), s.ph(1)), id); err != nil {
		return err
	}
	s.audit(r, u, "release.delete", map[string]any{"id": id})
	return data(w, map[string]any{"id": id})
}

func (s *Server) addReleaseAction(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	id, err := releaseID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	rel, err := s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	if rel.ReleasedAt != nil {
		return errReleased
	}
	var b struct {
		Type       string `json:"type"`
		UID        string `json:"uid"`
		DocumentID string `json:"documentId"`
		Locale     string `json:"locale"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	if b.Type != "publish" && b.Type != "unpublish" {
		return validation([]FieldError{{Path: []string{"type"}, Message: "type must be publish or unpublish"}})
	}
	ct, ok := s.set.ContentType(schema.UID(b.UID))
	if !ok || !ct.DraftAndPublish() || !contains(s.ContentManagerTypes(), b.UID) {
		return validation([]FieldError{{Path: []string{"uid"}, Message: "only the content types with draft and publish go in a release"}})
	}
	var locale any
	if ct.Localized() {
		if b.Locale == "" {
			b.Locale, _ = s.docs.DefaultLocale(ctx).(string)
		}
		locale = b.Locale
	}
	if !canPublish(a, b.UID, locale) {
		return errForbidden
	}
	row, err := s.docs.FindOne(ctx, b.UID, b.DocumentID, document.Params{"locale": locale})
	if err != nil {
		return asValidation(err)
	}
	if row == nil {
		return validation([]FieldError{{Path: []string{"documentId"}, Message: "the document does not exist"}})
	}
	for _, x := range rel.Actions {
		if x.UID == b.UID && x.DocumentID == b.DocumentID && fmt.Sprint(deref(x.Locale)) == fmt.Sprint(b.Locale) {
			return validation([]FieldError{{Path: []string{"documentId"}, Message: "the document is already in this release"}})
		}
	}
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`insert into %s (release_id, type, uid, document_id, locale, created_at) values (%s, %s, %s, %s, %s, %s)`,
		s.q(actionsTable), s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6)), id, b.Type, b.UID, b.DocumentID, locale, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	s.audit(r, u, "release-action.add", map[string]any{"release": id, "type": b.Type, "uid": b.UID, "documentId": b.DocumentID, "locale": locale})
	rel, err = s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	s.assess(ctx, rel)
	s.saveStatus(ctx, rel)
	writeJSON(w, http.StatusCreated, map[string]any{"data": rel})
	return nil
}

func (s *Server) removeReleaseAction(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	id, err := releaseID(r, "id")
	if err != nil {
		return err
	}
	actionID, err := releaseID(r, "actionId")
	if err != nil {
		return err
	}
	ctx := r.Context()
	rel, err := s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	if rel.ReleasedAt != nil {
		return errReleased
	}
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where id = %s and release_id = %s`, s.q(actionsTable), s.ph(1), s.ph(2)), actionID, id); err != nil {
		return err
	}
	s.audit(r, u, "release-action.remove", map[string]any{"release": id, "action": actionID})
	rel, err = s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	s.assess(ctx, rel)
	s.saveStatus(ctx, rel)
	return data(w, rel)
}

// documentReleases lists the pending releases holding a document.
func (s *Server) documentReleases(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	q := fmt.Sprintf(`select r.id, r.name, a.type, a.locale from %s a join %s r on r.id = a.release_id where a.uid = %s and a.document_id = %s and r.released_at is null order by r.id`,
		s.q(actionsTable), s.q(releasesTable), s.ph(1), s.ph(2))
	rows, err := s.e.DB.Query(r.Context(), q, r.PathValue("uid"), r.PathValue("documentId"))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, typ string
		var locale *string
		if err := rows.Scan(&id, &name, &typ, &locale); err != nil {
			return err
		}
		out = append(out, map[string]any{"id": id, "name": name, "type": typ, "locale": locale})
	}
	return data(w, out)
}

// publishRelease applies every action in one transaction, all or nothing.
// It returns the actions that block it.
func (s *Server) publishRelease(ctx context.Context, rel *Release) ([]ReleaseAction, error) {
	s.assess(ctx, rel)
	var blocking []ReleaseAction
	for _, a := range rel.Actions {
		if a.State == "not-ready" {
			blocking = append(blocking, a)
		}
	}
	if len(blocking) > 0 {
		return blocking, nil
	}
	err := s.e.DB.Tx(ctx, func(ctx context.Context) error {
		for _, a := range rel.Actions {
			if strings.HasPrefix(a.State, "already-") {
				continue
			}
			ct, _ := s.set.ContentType(schema.UID(a.UID))
			p := actionParams(ct, a)
			var err error
			if a.Type == "publish" {
				_, err = s.docs.Publish(ctx, a.UID, a.DocumentID, p)
			} else {
				_, err = s.docs.Unpublish(ctx, a.UID, a.DocumentID, p)
			}
			if err != nil {
				return fmt.Errorf("%s %s: %w", a.UID, a.DocumentID, err)
			}
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		res, err := s.e.DB.Exec(ctx, fmt.Sprintf(`update %s set status = 'done', released_at = %s, error = null, updated_at = %s where id = %s and released_at is null`,
			s.q(releasesTable), s.ph(1), s.ph(2), s.ph(3)), now, now, rel.ID)
		if err != nil {
			return err
		}
		// Another instance published it first.
		if n, _ := res.RowsAffected(); n == 0 {
			return errReleased
		}
		return nil
	})
	return nil, err
}

// releaseAudit lists what a publication changed, locale included.
func releaseAudit(rel *Release) []map[string]any {
	out := []map[string]any{}
	for _, a := range rel.Actions {
		out = append(out, map[string]any{"type": a.Type, "uid": a.UID, "documentId": a.DocumentID, "locale": a.Locale, "state": a.State})
	}
	return out
}

func (s *Server) emitRelease(ctx context.Context, id int64, published bool) {
	if s.docs.Events == nil {
		return
	}
	rel, err := s.loadRelease(ctx, id)
	if err != nil {
		return
	}
	s.docs.Events.Emit("releases.publish", map[string]any{"isPublished": published, "release": map[string]any{
		"id": rel.ID, "name": rel.Name, "releasedAt": rel.ReleasedAt, "scheduledAt": rel.ScheduledAt, "timezone": rel.Timezone,
		"createdAt": rel.CreatedAt, "actions": map[string]any{"count": rel.Count},
	}})
}

func (s *Server) publishReleaseHandler(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	id, err := releaseID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	rel, err := s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	if rel.ReleasedAt != nil {
		return errReleased
	}
	for _, x := range rel.Actions {
		if !canPublish(a, x.UID, deref(x.Locale)) {
			return errForbidden
		}
	}
	blocking, err := s.publishRelease(ctx, rel)
	if err != nil {
		return asValidation(err)
	}
	if len(blocking) > 0 {
		s.saveStatus(ctx, rel)
		details := []FieldError{}
		for _, b := range blocking {
			details = append(details, FieldError{Path: []string{b.UID, b.DocumentID}, Message: strings.Join(b.Errors, ", ")})
		}
		return validation(details)
	}
	s.audit(r, u, "release.publish", map[string]any{"id": id, "actions": releaseAudit(rel)})
	s.emitRelease(ctx, id, true)
	rel, err = s.loadRelease(ctx, id)
	if err != nil {
		return err
	}
	return data(w, rel)
}

// runScheduled publishes the releases whose time has come; a blocked one
// fails, with nothing published.
func (s *Server) runScheduled(ctx context.Context) {
	now := s.now().UTC().Format(time.RFC3339)
	rows, err := s.e.DB.Query(ctx, fmt.Sprintf(`select id from %s where released_at is null and scheduled_at is not null and scheduled_at <= %s and status <> 'failed'`, s.q(releasesTable), s.ph(1)), now)
	if err != nil {
		s.log.Error("releases scheduler", "error", err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()
	for _, id := range ids {
		rel, err := s.loadRelease(ctx, id)
		if err != nil {
			continue
		}
		blocking, err := s.publishRelease(ctx, rel)
		if err == nil && len(blocking) == 0 {
			s.writeAudit(ctx, nil, "scheduler", "release.publish", map[string]any{"id": id, "actions": releaseAudit(rel)})
			s.emitRelease(ctx, id, true)
			continue
		}
		msg := ""
		if err != nil {
			msg = err.Error()
		} else {
			parts := []string{}
			for _, b := range blocking {
				parts = append(parts, b.UID+" "+b.DocumentID+": "+strings.Join(b.Errors, ", "))
			}
			msg = strings.Join(parts, "; ")
		}
		_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`update %s set status = 'failed', error = %s where id = %s and released_at is null`, s.q(releasesTable), s.ph(1), s.ph(2)), msg, id)
		s.writeAudit(ctx, nil, "scheduler", "release.fail", map[string]any{"id": id, "error": msg})
		s.emitRelease(ctx, id, false)
	}
}

// wakeScheduler runs a release scheduled in the past without waiting for
// the next tick.
func (s *Server) wakeScheduler(at any) {
	if at == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// StartScheduler checks the scheduled releases every interval until ctx
// ends; a time passed while the server was down runs at once.
func (s *Server) StartScheduler(ctx context.Context, interval time.Duration) {
	go func() {
		s.runScheduled(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.runScheduled(ctx)
			case <-s.wake:
				s.runScheduled(ctx)
			}
		}
	}()
}
