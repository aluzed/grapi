package adminapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

// The content history (a grapi design, Strapi's is Enterprise): one row
// per save or publication from the admin, in a table of grapi's own, out of
// the schema registry so that Strapi never sees it.
const historyTable = "grapi_history_versions"

// storedTime writes the dates of grapi's own tables with a fixed width, so
// that they sort as strings (RFC 3339 nano drops trailing zeros).
const storedTime = "2006-01-02T15:04:05.000000000Z07:00"

func (s *Server) ensureHistoryTable(ctx context.Context) error {
	id := "id integer primary key autoincrement"
	if s.e.DB.Dialect.Name() == "postgres" {
		id = "id serial primary key"
	}
	q := s.e.DB.Dialect.Quote
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`create table if not exists %s (%s, uid varchar(255) not null, document_id varchar(255) not null,
		locale varchar(255), status varchar(32) not null, data text not null, schema_fields text not null, created_at varchar(64) not null, created_by_id integer)`,
		q(historyTable), id)); err != nil {
		return err
	}
	_, err := s.e.DB.Exec(ctx, fmt.Sprintf(`create index if not exists %s on %s (uid, document_id, created_at)`, q("grapi_history_versions_document_idx"), q(historyTable)))
	return err
}

func (s *Server) historyRoutes() {
	s.authed("GET /content-manager/history/{uid}/{documentId}", s.listVersions)
	s.authed("GET /content-manager/history/versions/{id}", s.getVersion)
	s.authed("GET /content-manager/history/versions/{id}/diff", s.diffVersion)
	s.authed("POST /content-manager/history/versions/{id}/restore", s.restoreVersion)
}

func (s *Server) ph(n int) string { return s.e.DB.Dialect.Placeholder(n) }

func schemaFields(ct *schema.ContentType) map[string]string {
	out := map[string]string{}
	for name, a := range ct.Attributes.All() {
		out[name] = a.Type
	}
	return out
}

// recordVersion snapshots a document after an admin write. A failure is
// logged: the write itself succeeded.
// snapshot reads a document populated deep, as a version stores it.
func (s *Server) snapshot(ctx context.Context, ct *schema.ContentType, documentID string, locale any, published bool) (map[string]any, error) {
	p := document.Params{"populate": s.deepPopulate(&ct.Attributes, 0)}
	if ct.Localized() && locale != nil && locale != "" {
		p["locale"] = locale
	}
	if published {
		p["status"] = "published"
	}
	row, err := s.docs.FindOne(ctx, string(ct.UID), documentID, p)
	if err != nil || row == nil {
		return nil, err
	}
	return toMap(row), nil
}

func (s *Server) recordVersion(ctx context.Context, ct *schema.ContentType, documentID string, locale any, status string, userID any) {
	if !s.cfg.HistoryEnabled {
		return
	}
	uid := string(ct.UID)
	m, err := s.snapshot(ctx, ct, documentID, locale, status == "published")
	if err != nil || m == nil {
		if err != nil {
			s.log.Error("history snapshot", "uid", uid, "error", err)
		}
		return
	}
	if status != "published" && ct.DraftAndPublish() {
		res := []map[string]any{{schema.AttrDocumentID: documentID, "updatedAt": m["updatedAt"]}}
		if err := s.withStatus(ctx, uid, m["locale"], "draft", res, []any{documentID}); err == nil {
			status, _ = res[0]["status"].(string)
		}
	}
	if status == "" {
		status = "draft"
	}
	for _, k := range []string{"id", "createdAt", "updatedAt", "publishedAt", schema.AttrCreatedBy, schema.AttrUpdatedBy, "localizations"} {
		delete(m, k)
	}
	dataJSON, _ := json.Marshal(m)
	fieldsJSON, _ := json.Marshal(schemaFields(ct))
	var loc any
	if l, ok := m["locale"].(string); ok && l != "" {
		loc = l
	}
	q := fmt.Sprintf(`insert into %s (uid, document_id, locale, status, data, schema_fields, created_at, created_by_id) values (%s, %s, %s, %s, %s, %s, %s, %s)`,
		s.e.DB.Dialect.Quote(historyTable), s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7), s.ph(8))
	now := s.now().UTC()
	if _, err := s.e.DB.Exec(ctx, q, uid, documentID, loc, status, string(dataJSON), string(fieldsJSON), now.Format(storedTime), userID); err != nil {
		s.log.Error("history insert", "uid", uid, "error", err)
		return
	}
	if n := s.cfg.HistoryMaxVersions; n > 0 {
		s.trimVersions(ctx, uid, documentID, loc, n)
	}
}

// recordFromService records the versions of writes from the other origins
// listed in admin.history.origins; the admin panel records its own.
func (s *Server) recordFromService(c *document.Context, next document.Next) (any, error) {
	res, err := next()
	origin := document.OriginOf(c.Ctx)
	if err != nil || origin == "admin-panel" || !contains(s.cfg.HistoryOrigins, origin) || c.ContentType == nil {
		return res, err
	}
	status := ""
	switch c.Action {
	case document.Create, document.Update:
	case document.Publish:
		status = "published"
	default:
		return res, err
	}
	ct := c.ContentType
	if !contains(s.ContentManagerTypes(), string(ct.UID)) {
		return res, err
	}
	row := toMap(res)
	id, _ := c.Params["documentId"].(string)
	if id == "" {
		id, _ = row[schema.AttrDocumentID].(string)
	}
	locale := c.Params["locale"]
	if locale == nil {
		locale = row["locale"]
	}
	if id != "" {
		db.OnCommit(c.Ctx, func() { s.recordVersion(context.Background(), ct, id, locale, status, nil) })
	}
	return res, err
}

// trimVersions keeps the n latest versions of a document in a locale.
func (s *Server) trimVersions(ctx context.Context, uid, documentID string, locale any, n int) {
	q := fmt.Sprintf(`select id from %s where uid = %s and document_id = %s`, s.e.DB.Dialect.Quote(historyTable), s.ph(1), s.ph(2))
	args := []any{uid, documentID}
	if locale != nil {
		q += " and locale = " + s.ph(3)
		args = append(args, locale)
	} else {
		q += " and locale is null"
	}
	rows, err := s.e.DB.Query(ctx, q+" order by created_at desc, id desc", args...)
	if err != nil {
		return
	}
	var old []any
	for i := 0; rows.Next(); i++ {
		var id int64
		if rows.Scan(&id) == nil && i >= n {
			old = append(old, id)
		}
	}
	_ = rows.Close()
	for _, id := range old {
		_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where id = %s`, s.e.DB.Dialect.Quote(historyTable), s.ph(1)), id)
	}
}

// purgeHistory drops the versions older than the retention, at most once
// an hour.
func (s *Server) purgeHistory(ctx context.Context) {
	days := s.cfg.HistoryRetentionDays
	if days <= 0 {
		return
	}
	now := s.now().UTC()
	if now.Sub(s.historyPurgedAt) < time.Hour {
		return
	}
	s.historyPurgedAt = now
	cut := now.Add(-time.Duration(days) * 24 * time.Hour).Format(storedTime)
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where created_at < %s`, s.e.DB.Dialect.Quote(historyTable), s.ph(1)), cut); err != nil {
		s.log.Error("history purge", "error", err)
	}
}

// forgetContentType drops the versions and pending release entries of a
// deleted content type.
func (s *Server) forgetContentType(ctx context.Context, uid string) {
	_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where uid = %s`, s.e.DB.Dialect.Quote(historyTable), s.ph(1)), uid)
	_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where uid = %s and release_id in (select id from %s where released_at is null)`,
		s.q(actionsTable), s.ph(1), s.q(releasesTable)), uid)
}

// forgetDocument drops the versions of a deleted document (in a locale,
// or all of them) and takes it out of the pending releases.
func (s *Server) forgetDocument(ctx context.Context, uid, documentID string, locale any) {
	where := fmt.Sprintf(`uid = %s and document_id = %s`, s.ph(1), s.ph(2))
	args := []any{uid, documentID}
	if l, ok := locale.(string); ok && l != "" {
		where += " and locale = " + s.ph(3)
		args = append(args, l)
	}
	_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where %s`, s.e.DB.Dialect.Quote(historyTable), where), args...)
	_, _ = s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where %s and release_id in (select id from %s where released_at is null)`,
		s.q(actionsTable), where, s.q(releasesTable)), args...)
}

// HistoryVersion is one entry of a document's history.
type HistoryVersion struct {
	ID         int64          `json:"id"`
	UID        string         `json:"uid"`
	DocumentID string         `json:"documentId"`
	Locale     *string        `json:"locale"`
	Status     string         `json:"status"`
	CreatedAt  string         `json:"createdAt"`
	CreatedBy  map[string]any `json:"createdBy"`
	Data       map[string]any `json:"data,omitempty"`
	// Unknown are the fields of the version the schema no longer has,
	// Missing those added since.
	Unknown map[string]any `json:"unknownFields,omitempty"`
	Missing []string       `json:"missingFields,omitempty"`
	// Dangling are the relations and media the version holds that no
	// longer exist: a restore leaves them out.
	Dangling []Reference `json:"danglingReferences,omitempty"`
}

func (s *Server) versionAccess(r *http.Request, u *User, uid, action string) (*schema.ContentType, error) {
	ct, ok := s.set.ContentType(schema.UID(uid))
	if !ok || !contains(s.ContentManagerTypes(), uid) {
		return nil, errNotFound
	}
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return nil, err
	}
	if _, allowed := a.Filter(action, uid); !allowed {
		return nil, errForbidden
	}
	return ct, nil
}

// versionAllowed applies the same row and locale restrictions as the Content
// Manager to the live document. A historical snapshot cannot grant access to
// a document the user can no longer read or update.
func (s *Server) versionAllowed(ctx context.Context, ct *schema.ContentType, v *HistoryVersion, a *permissions.Ability, action string) (bool, error) {
	filter, allowed := a.Filter(action, v.UID)
	if !allowed {
		return false, nil
	}
	var locale any
	if v.Locale != nil {
		locale = *v.Locale
	}
	return s.allows(ctx, ct, v.DocumentID, locale, filter)
}

func (s *Server) authorName(ctx context.Context, id *int64, cache map[int64]map[string]any) map[string]any {
	if id == nil {
		return nil
	}
	if m, ok := cache[*id]; ok {
		return m
	}
	u, err := s.findUser(ctx, map[string]any{"id": *id})
	var m map[string]any
	if err == nil && u != nil {
		m = map[string]any{"id": u.ID, "firstname": u.Firstname, "lastname": u.Lastname, "email": u.Email}
	}
	cache[*id] = m
	return m
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request, u *User) error {
	uid, documentID := r.PathValue("uid"), r.PathValue("documentId")
	ct, err := s.versionAccess(r, u, uid, actionRead)
	if err != nil {
		return err
	}
	ctx := r.Context()
	a, err := s.Ability(ctx, u)
	if err != nil {
		return err
	}
	q := fmt.Sprintf(`select id, locale, status, created_at, created_by_id from %s where uid = %s and document_id = %s`, s.e.DB.Dialect.Quote(historyTable), s.ph(1), s.ph(2))
	args := []any{uid, documentID}
	if l := r.URL.Query().Get("locale"); l != "" && ct.Localized() {
		q += " and locale = " + s.ph(3)
		args = append(args, l)
	}
	q += " order by created_at desc, id desc limit 200"
	rows, err := s.e.DB.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	out := []HistoryVersion{}
	cache := map[int64]map[string]any{}
	access := map[string]bool{}
	for rows.Next() {
		v := HistoryVersion{UID: uid, DocumentID: documentID}
		var by *int64
		if err := rows.Scan(&v.ID, &v.Locale, &v.Status, &v.CreatedAt, &by); err != nil {
			return err
		}
		locale := ""
		if v.Locale != nil {
			locale = *v.Locale
		}
		allowed, checked := access[locale]
		if !checked {
			allowed, err = s.versionAllowed(ctx, ct, &v, a, actionRead)
			if err != nil {
				return err
			}
			access[locale] = allowed
		}
		if !allowed {
			continue
		}
		v.CreatedBy = s.authorName(ctx, by, cache)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return data(w, out)
}

func (s *Server) loadVersion(ctx context.Context, id int64) (*HistoryVersion, map[string]string, error) {
	q := fmt.Sprintf(`select id, uid, document_id, locale, status, data, schema_fields, created_at, created_by_id from %s where id = %s`, s.e.DB.Dialect.Quote(historyTable), s.ph(1))
	v := &HistoryVersion{}
	var raw, fields string
	var by *int64
	if err := s.e.DB.QueryRow(ctx, q, id).Scan(&v.ID, &v.UID, &v.DocumentID, &v.Locale, &v.Status, &raw, &fields, &v.CreatedAt, &by); err != nil {
		return nil, nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "HistoryVersion not found"}
	}
	_ = json.Unmarshal([]byte(raw), &v.Data)
	var schemaThen map[string]string
	_ = json.Unmarshal([]byte(fields), &schemaThen)
	v.CreatedBy = s.authorName(ctx, by, map[int64]map[string]any{})
	return v, schemaThen, nil
}

func versionID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errNotFound
	}
	return id, nil
}

// splitFields sorts a version's data against the current schema.
func splitFields(v *HistoryVersion, ct *schema.ContentType) {
	v.Unknown = map[string]any{}
	for k, val := range v.Data {
		if k == schema.AttrDocumentID || k == "locale" {
			continue
		}
		if _, ok := ct.Attributes.Get(k); !ok {
			v.Unknown[k] = val
			delete(v.Data, k)
		}
	}
	for name, a := range ct.Attributes.All() {
		if _, ok := v.Data[name]; !ok && editable(name, a) && a.Type != schema.TypePassword {
			v.Missing = append(v.Missing, name)
		}
	}
	sort.Strings(v.Missing)
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := versionID(r)
	if err != nil {
		return err
	}
	v, _, err := s.loadVersion(r.Context(), id)
	if err != nil {
		return err
	}
	ct, err := s.versionAccess(r, u, v.UID, actionRead)
	if err != nil {
		return err
	}
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return err
	}
	if ok, err := s.versionAllowed(r.Context(), ct, v, a, actionRead); err != nil {
		return err
	} else if !ok {
		return errNotFound
	}
	splitFields(v, ct)
	// Removed attributes have no schema with which to sanitize their values.
	// Keep them available only to super admins; field permissions must still
	// apply to the names of attributes added since the snapshot.
	if !a.IsSuperAdmin() {
		v.Unknown = nil
		v.Missing = slices.DeleteFunc(v.Missing, func(name string) bool { return !a.Can(actionRead, v.UID, name) })
	}
	v.Data = s.visible(v.Data, ct, a)
	v.Dangling = s.prune(r.Context(), v.Data, &ct.Attributes, "")
	return data(w, v)
}

// restorable turns a version's data into Document Service input: relations
// by documentId, media by id, components without their ids (new rows).
func restorable(v any, a *schema.Attribute, set *schema.Set) any {
	switch a.Type {
	case schema.TypeRelation:
		var ids []any
		for _, x := range listOf(v) {
			if m, ok := x.(map[string]any); ok && m[schema.AttrDocumentID] != nil {
				ids = append(ids, map[string]any{schema.AttrDocumentID: m[schema.AttrDocumentID]})
			}
		}
		if ids == nil {
			ids = []any{}
		}
		return map[string]any{"set": ids}
	case schema.TypeMedia:
		var ids []any
		for _, x := range listOf(v) {
			if m, ok := x.(map[string]any); ok && m["id"] != nil {
				ids = append(ids, m["id"])
			}
		}
		if a.IsMultiple() {
			if ids == nil {
				ids = []any{}
			}
			return ids
		}
		if len(ids) == 0 {
			return nil
		}
		return ids[0]
	case schema.TypeComponent:
		c, ok := set.Component(schema.UID(a.Component))
		if !ok || v == nil {
			return v
		}
		if a.IsRepeatable() {
			out := []any{}
			for _, x := range listOf(v) {
				out = append(out, restoreComponent(x, &c.Attributes, set))
			}
			return out
		}
		return restoreComponent(v, &c.Attributes, set)
	case schema.TypeDynamicZone:
		out := []any{}
		for _, x := range listOf(v) {
			m, _ := x.(map[string]any)
			cu, _ := m["__component"].(string)
			c, ok := set.Component(schema.UID(cu))
			if !ok {
				continue
			}
			item := restoreComponent(m, &c.Attributes, set)
			if im, ok := item.(map[string]any); ok {
				im["__component"] = cu
			}
			out = append(out, item)
		}
		return out
	}
	return v
}

func listOf(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case nil:
		return nil
	}
	return []any{v}
}

func restoreComponent(v any, attrs *schema.OrderedMap[*schema.Attribute], set *schema.Set) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := map[string]any{}
	for name, a := range attrs.All() {
		if val, ok := m[name]; ok {
			out[name] = restorable(val, a, set)
		}
	}
	return out
}

// restoreVersion writes a version back as the draft: unknown fields are
// ignored, fields added since take their default.
func (s *Server) restoreVersion(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := versionID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	v, _, err := s.loadVersion(ctx, id)
	if err != nil {
		return err
	}
	ct, err := s.versionAccess(r, u, v.UID, actionUpdate)
	if err != nil {
		return err
	}
	a, err := s.Ability(ctx, u)
	if err != nil {
		return err
	}
	if ok, err := s.versionAllowed(ctx, ct, v, a, actionUpdate); err != nil {
		return err
	} else if !ok {
		return errNotFound
	}
	splitFields(v, ct)
	s.prune(ctx, v.Data, &ct.Attributes, "")
	d := map[string]any{}
	for name, attr := range ct.Attributes.All() {
		if !editable(name, attr) || attr.Type == schema.TypePassword || !a.Can(actionUpdate, v.UID, name) {
			continue
		}
		val, ok := v.Data[name]
		if !ok {
			val = nil
			if len(attr.Default) > 0 {
				_ = json.Unmarshal(attr.Default, &val)
			}
		}
		d[name] = restorable(val, attr, s.set)
	}
	d[schema.AttrUpdatedBy] = u.ID
	p := document.Params{"data": d}
	if v.Locale != nil && ct.Localized() {
		p["locale"] = *v.Locale
	}
	if _, err := s.docs.Update(ctx, v.UID, v.DocumentID, p); err != nil {
		return asValidation(err)
	}
	s.recordVersion(ctx, ct, v.DocumentID, p["locale"], "", u.ID)
	s.audit(r, u, "entry.update", map[string]any{"uid": v.UID, "documentId": v.DocumentID, "locale": p["locale"], "restoredVersion": v.ID})
	return data(w, map[string]any{"documentId": v.DocumentID})
}
