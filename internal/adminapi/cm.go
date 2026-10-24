package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

const (
	actionRead    = explorer + "read"
	actionCreate  = explorer + "create"
	actionUpdate  = explorer + "update"
	actionDelete  = explorer + "delete"
	actionPublish = explorer + "publish"
)

func (s *Server) contentManagerRoutes() {
	s.authed("GET /content-manager/models", s.cmModels)
	s.authed("GET /content-manager/locales", s.cmLocales)
	s.authed("GET /content-manager/collection-types/{uid}", s.cmList)
	s.authed("DELETE /content-manager/collection-types/{uid}/{documentId}", s.cmDelete)
	s.authed("POST /content-manager/collection-types/{uid}/actions/bulk-delete", s.cmBulk("delete"))
	s.authed("POST /content-manager/collection-types/{uid}/actions/bulk-publish", s.cmBulk("publish"))
	s.authed("POST /content-manager/collection-types/{uid}/actions/bulk-unpublish", s.cmBulk("unpublish"))
}

// ModelSettings are the list settings of a content type.
type ModelSettings struct {
	PageSize         int    `json:"pageSize"`
	DefaultSortBy    string `json:"defaultSortBy"`
	DefaultSortOrder string `json:"defaultSortOrder"`
	Searchable       bool   `json:"searchable"`
	Filterable       bool   `json:"filterable"`
	Bulkable         bool   `json:"bulkable"`
	// RelationOpenMode is how the edit view opens a related document:
	// modal (the default), newTab or page.
	RelationOpenMode string `json:"relationOpenMode"`
}

// ModelPermissions are the Content Manager actions the user has on a type.
type ModelPermissions struct {
	Create  bool `json:"create"`
	Read    bool `json:"read"`
	Update  bool `json:"update"`
	Delete  bool `json:"delete"`
	Publish bool `json:"publish"`
	// CreateFields and UpdateFields are the fields the user may write,
	// null for all of them: the edit view shows the others read only.
	CreateFields []string `json:"createFields"`
	UpdateFields []string `json:"updateFields"`
}

// Model is a content type as the Content Manager shows it.
type Model struct {
	UID             string                       `json:"uid"`
	Kind            string                       `json:"kind"`
	Info            schema.Info                  `json:"info"`
	DraftAndPublish bool                         `json:"draftAndPublish"`
	Localized       bool                         `json:"localized"`
	Attributes      map[string]*schema.Attribute `json:"attributes"`
	// Fields are the attribute names in schema order.
	Fields      []string          `json:"fields"`
	MainField   string            `json:"mainField"`
	ListColumns []string          `json:"listColumns"`
	Labels      map[string]string `json:"labels"`
	Settings    ModelSettings     `json:"settings"`
	Permissions ModelPermissions  `json:"permissions"`
	// EditLayout are the rows of the edit form, EditMeta its labels.
	EditLayout   [][]LayoutField     `json:"editLayout"`
	EditMeta     map[string]EditMeta `json:"editMeta"`
	CanConfigure bool                `json:"canConfigure"`
}

// EditMeta is the edit view configuration of a field.
type EditMeta struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Placeholder string `json:"placeholder"`
	Visible     bool   `json:"visible"`
	Editable    bool   `json:"editable"`
}

func (s *Server) model(ctx context.Context, uid string, a *permissions.Ability) (*Model, error) {
	ct, ok := s.set.ContentType(schema.UID(uid))
	if !ok {
		return nil, nil
	}
	perms := ModelPermissions{
		Read: a.Can(actionRead, uid, ""), Create: a.Can(actionCreate, uid, ""), Update: a.Can(actionUpdate, uid, ""),
		Delete: a.Can(actionDelete, uid, ""), Publish: a.Can(actionPublish, uid, ""),
		CreateFields: a.Fields(actionCreate, uid), UpdateFields: a.Fields(actionUpdate, uid),
	}
	if !perms.Read {
		return nil, nil
	}
	m := &Model{
		UID: uid, Kind: string(ct.Kind), Info: ct.Info, DraftAndPublish: ct.DraftAndPublish(), Localized: ct.Localized(),
		Attributes: map[string]*schema.Attribute{}, Labels: map[string]string{}, Permissions: perms,
		Settings: ModelSettings{PageSize: 10, DefaultSortOrder: "ASC", Searchable: true, Filterable: true, Bulkable: true},
	}
	for name, attr := range ct.Attributes.All() {
		if attr.Type == schema.TypePassword || !attr.IsVisible() || !a.Can(actionRead, uid, name) {
			continue
		}
		switch name {
		case schema.AttrCreatedBy, schema.AttrUpdatedBy, "localizations", "locale", "publishedAt":
			continue
		}
		m.Attributes[name] = attr
		m.Fields = append(m.Fields, name)
	}
	cfg, err := s.effectiveConfig(ctx, ct)
	if err != nil {
		return nil, err
	}
	set := cfg.Settings
	m.MainField = set.MainField
	m.Settings = ModelSettings{
		PageSize: set.PageSize, DefaultSortBy: set.DefaultSortBy, DefaultSortOrder: strings.ToUpper(set.DefaultSortOrder),
		Searchable: set.Searchable, Filterable: set.Filterable, Bulkable: set.Bulkable, RelationOpenMode: set.RelationOpenMode,
	}
	if m.Settings.RelationOpenMode == "" {
		m.Settings.RelationOpenMode = "modal"
	}
	if m.Settings.DefaultSortBy == "" {
		m.Settings.DefaultSortBy = m.MainField
	}
	if m.Settings.DefaultSortOrder != "DESC" {
		m.Settings.DefaultSortOrder = "ASC"
	}
	m.EditMeta = map[string]EditMeta{}
	for name, md := range cfg.Metadatas {
		if md == nil {
			continue
		}
		if md.List.Label != "" && md.List.Label != name {
			m.Labels[name] = md.List.Label
		}
		m.EditMeta[name] = EditMeta{Label: md.Edit.Label, Description: md.Edit.Description, Placeholder: md.Edit.Placeholder, Visible: md.Edit.Visible, Editable: md.Edit.Editable}
	}
	m.ListColumns = []string{}
	for _, col := range cfg.Layouts.List {
		if _, ok := m.Attributes[col]; ok || col == "id" {
			m.ListColumns = append(m.ListColumns, col)
		}
	}
	for _, row := range cfg.Layouts.Edit {
		var kept []LayoutField
		for _, f := range row {
			if _, ok := m.Attributes[f.Name]; ok {
				kept = append(kept, f)
			}
		}
		if len(kept) > 0 {
			m.EditLayout = append(m.EditLayout, kept)
		}
	}
	m.CanConfigure = a.Can(configureAction(ct), "", "")
	return m, nil
}

func (s *Server) cmModels(w http.ResponseWriter, r *http.Request, u *User) error {
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return err
	}
	out := []*Model{}
	for _, uid := range s.ContentManagerTypes() {
		m, err := s.model(r.Context(), uid, a)
		if err != nil {
			return err
		}
		if m != nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Info.DisplayName) < strings.ToLower(out[j].Info.DisplayName)
	})
	return data(w, out)
}

func (s *Server) cmLocales(w http.ResponseWriter, r *http.Request, _ *User) error {
	out := []map[string]any{}
	if _, ok := s.set.ContentType("plugin::i18n.locale"); !ok {
		return data(w, out)
	}
	rows, err := s.e.FindMany(r.Context(), "plugin::i18n.locale", query.Params{OrderBy: []any{map[string]any{"id": "asc"}}})
	if err != nil {
		return err
	}
	def := s.docs.DefaultLocale(r.Context())
	for _, row := range rows {
		code := getString(row, "code")
		out = append(out, map[string]any{"id": get(row, "id"), "code": code, "name": getString(row, "name"), "isDefault": code == def})
	}
	return data(w, out)
}

// cmAccess checks action on the collection type of the path and returns its
// model, the user's ability and the row filter of action.
func (s *Server) cmAccess(r *http.Request, u *User, action string) (*schema.ContentType, *permissions.Ability, any, error) {
	uid := r.PathValue("uid")
	ct, ok := s.set.ContentType(schema.UID(uid))
	if !ok || !contains(s.ContentManagerTypes(), uid) {
		return nil, nil, nil, errNotFound
	}
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return nil, nil, nil, err
	}
	filter, allowed := a.Filter(action, uid)
	if !allowed {
		return nil, nil, nil, errForbidden
	}
	return ct, a, filter, nil
}

// scoped wraps permission filters in $and: as top-level keys they would
// replace the Document Service's own locale or status lookup.
func scoped(f any) any {
	if f == nil {
		return nil
	}
	return map[string]any{"$and": []any{f}}
}

func and(filters ...any) any {
	var parts []any
	for _, f := range filters {
		if f == nil {
			continue
		}
		if m, ok := f.(map[string]any); ok && len(m) == 0 {
			continue
		}
		parts = append(parts, f)
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	}
	return map[string]any{"$and": parts}
}

func toMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

var alwaysReadable = map[string]bool{"id": true, schema.AttrDocumentID: true, "createdAt": true, "updatedAt": true, "publishedAt": true, "locale": true}

// visible keeps the fields of row the user may read.
func visible(row map[string]any, ct *schema.ContentType, uid string, a *permissions.Ability) map[string]any {
	for k := range row {
		if alwaysReadable[k] {
			continue
		}
		attr, ok := ct.Attributes.Get(k)
		if !ok || attr.Type == schema.TypePassword || !a.Can(actionRead, uid, k) {
			delete(row, k)
		}
	}
	return row
}

func (s *Server) cmList(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, a, rbac, err := s.cmAccess(r, u, actionRead)
	if err != nil {
		return err
	}
	if ct.IsSingleType() {
		return errNotFound
	}
	uid := string(ct.UID)
	ctx := r.Context()
	m, err := s.model(ctx, uid, a)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	page, size := pageParams(r)
	p := document.Params{}
	var userFilters any
	if raw := q.Get("filters"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &userFilters); err != nil {
			return validation([]FieldError{{Path: []string{"filters"}, Message: "filters must be a JSON object"}})
		}
	}
	if f := and(userFilters, rbac); f != nil {
		p["filters"] = scoped(f)
	}
	if v := strings.TrimSpace(q.Get("_q")); v != "" {
		p["_q"] = v
	}
	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = m.Settings.DefaultSortBy + ":" + strings.ToLower(m.Settings.DefaultSortOrder)
	}
	p["sort"] = sortBy
	if ct.Localized() {
		if l := q.Get("locale"); l != "" {
			p["locale"] = l
		}
	}
	status := q.Get("status")
	if status != "published" {
		status = "draft"
	}
	if ct.DraftAndPublish() {
		p["status"] = status
	}
	populate := map[string]any{}
	for _, col := range m.ListColumns {
		if attr, ok := m.Attributes[col]; ok && (attr.Type == schema.TypeRelation || attr.Type == schema.TypeMedia) {
			populate[col] = true
		}
	}
	if len(populate) > 0 {
		p["populate"] = populate
	}
	total, err := s.docs.Count(ctx, uid, p)
	if err != nil {
		return asValidation(err)
	}
	window(p, page, size)
	rows, err := s.docs.FindMany(ctx, uid, p)
	if err != nil {
		return asValidation(err)
	}
	results := make([]map[string]any, 0, len(rows))
	var ids []any
	for _, row := range rows {
		item := visible(toMap(row), ct, uid, a)
		results = append(results, item)
		ids = append(ids, item[schema.AttrDocumentID])
	}
	if ct.DraftAndPublish() && len(ids) > 0 {
		if err := s.withStatus(ctx, uid, p["locale"], status, results, ids); err != nil {
			return err
		}
	}
	pages := int((total + int64(size) - 1) / int64(size))
	writeJSON(w, http.StatusOK, map[string]any{"data": results, "meta": map[string]any{"pagination": Pagination{Page: page, PageSize: size, PageCount: pages, Total: total}}})
	return nil
}

// withStatus sets each result's status: draft (never published), published,
// or modified (the draft changed since the publication).
func (s *Server) withStatus(ctx context.Context, uid string, locale any, listed string, results []map[string]any, ids []any) error {
	other := "published"
	if listed == "published" {
		other = "draft"
	}
	p := document.Params{"status": other, "filters": map[string]any{schema.AttrDocumentID: map[string]any{"$in": ids}}, "fields": []any{"updatedAt"}}
	if locale != nil {
		p["locale"] = locale
	}
	rows, err := s.docs.FindMany(ctx, uid, p)
	if err != nil {
		return err
	}
	counterpart := map[string]string{}
	for _, row := range rows {
		m := toMap(row)
		id, _ := m[schema.AttrDocumentID].(string)
		at, _ := m["updatedAt"].(string)
		counterpart[id] = at
	}
	for _, res := range results {
		id, _ := res[schema.AttrDocumentID].(string)
		at, _ := res["updatedAt"].(string)
		other, has := counterpart[id]
		switch {
		case listed == "published":
			res["status"] = "published"
			if has && other > at {
				res["status"] = "modified"
			}
		case !has:
			res["status"] = "draft"
		case at > other:
			res["status"] = "modified"
		default:
			res["status"] = "published"
		}
	}
	return nil
}

func asValidation(err error) error {
	var ve *document.ValidationError
	if errors.As(err, &ve) {
		return &Error{Status: http.StatusBadRequest, Name: "ValidationError", Message: ve.Error(), Details: ve.Details}
	}
	return err
}

// allows reports whether documentID passes the row filter of an action.
func (s *Server) allows(ctx context.Context, ct *schema.ContentType, documentID string, locale any, filter any) (bool, error) {
	p := document.Params{"filters": and(map[string]any{schema.AttrDocumentID: map[string]any{"$eq": documentID}}, filter)}
	if ct.Localized() && locale != nil {
		p["locale"] = locale
	}
	n, err := s.docs.Count(ctx, string(ct.UID), p)
	return n > 0, err
}

func localeParam(ct *schema.ContentType, l string) document.Params {
	if ct.Localized() && l != "" {
		return document.Params{"locale": l}
	}
	return document.Params{}
}

func (s *Server) cmDelete(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, _, filter, err := s.cmAccess(r, u, actionDelete)
	if err != nil {
		return err
	}
	id := r.PathValue("documentId")
	locale := r.URL.Query().Get("locale")
	p := localeParam(ct, locale)
	ok, err := s.allows(r.Context(), ct, id, p["locale"], filter)
	if err != nil {
		return err
	}
	if !ok {
		return errNotFound
	}
	if _, err := s.docs.Delete(r.Context(), string(ct.UID), id, p); err != nil {
		return asValidation(err)
	}
	s.forgetDocument(r.Context(), string(ct.UID), id, p["locale"])
	s.audit(r, u, "entry.delete", map[string]any{"uid": string(ct.UID), "documentId": id, "locale": p["locale"]})
	return data(w, map[string]any{"documentId": id})
}

// BulkError is a document a bulk action skipped.
type BulkError struct {
	DocumentID string `json:"documentId"`
	Message    string `json:"message"`
	Details    any    `json:"details,omitempty"`
}

// cmBulk runs op on each document in its own call: a failure is reported,
// the others still go through.
func (s *Server) cmBulk(op string) authedHandler {
	action := map[string]string{"delete": actionDelete, "publish": actionPublish, "unpublish": actionPublish}[op]
	return func(w http.ResponseWriter, r *http.Request, u *User) error {
		ct, _, filter, err := s.cmAccess(r, u, action)
		if err != nil {
			return err
		}
		if op != "delete" && !ct.DraftAndPublish() {
			return appError("This content type has no draft and publish")
		}
		var b struct {
			DocumentIDs []string `json:"documentIds"`
			Locale      string   `json:"locale"`
		}
		if err := decode(r, &b); err != nil {
			return err
		}
		if len(b.DocumentIDs) == 0 {
			return validation([]FieldError{{Path: []string{"documentIds"}, Message: "documentIds must have at least 1 items"}})
		}
		ctx := r.Context()
		uid := string(ct.UID)
		p := localeParam(ct, b.Locale)
		done := []string{}
		failed := []BulkError{}
		for _, id := range b.DocumentIDs {
			ok, err := s.allows(ctx, ct, id, p["locale"], filter)
			if err != nil {
				return err
			}
			if !ok {
				failed = append(failed, BulkError{DocumentID: id, Message: "Not found"})
				continue
			}
			switch op {
			case "delete":
				_, err = s.docs.Delete(ctx, uid, id, p)
			case "publish":
				_, err = s.docs.Publish(ctx, uid, id, p)
			case "unpublish":
				_, err = s.docs.Unpublish(ctx, uid, id, p)
			}
			if err != nil {
				var ve *document.ValidationError
				if !errors.As(err, &ve) {
					return err
				}
				failed = append(failed, BulkError{DocumentID: id, Message: ve.Error(), Details: ve.Details})
				continue
			}
			switch op {
			case "delete":
				s.forgetDocument(ctx, uid, id, p["locale"])
			case "publish":
				s.recordVersion(ctx, ct, id, p["locale"], "published", u.ID)
			}
			s.audit(r, u, "entry."+op, map[string]any{"uid": uid, "documentId": id, "locale": p["locale"]})
			done = append(done, id)
		}
		return data(w, map[string]any{"count": len(done), "documentIds": done, "errors": failed})
	}
}

func (s *Server) contentManagerEditRoutes() {
	s.authed("GET /content-manager/collection-types/{uid}/{documentId}", s.cmFindOne)
	s.authed("POST /content-manager/collection-types/{uid}", s.cmCreate)
	s.authed("PUT /content-manager/collection-types/{uid}/{documentId}", s.cmUpdate)
	s.authed("POST /content-manager/collection-types/{uid}/{documentId}/actions/discard", s.cmDiscard)
}

func (s *Server) cmFindOne(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, a, filter, err := s.cmAccess(r, u, actionRead)
	if err != nil {
		return err
	}
	id := r.PathValue("documentId")
	q := r.URL.Query()
	p := localeParam(ct, q.Get("locale"))
	ok, err := s.allows(r.Context(), ct, id, p["locale"], filter)
	if err != nil {
		return err
	}
	if !ok {
		return errNotFound
	}
	if ct.DraftAndPublish() && q.Get("status") == "published" {
		p["status"] = "published"
	}
	pop := s.deepPopulate(&ct.Attributes, 0)
	// The edit view shows who created and last changed the document, by name only.
	_, stamped := ct.Attributes.Get(schema.AttrCreatedBy)
	if stamped {
		author := map[string]any{"fields": []any{"firstname", "lastname", "username"}}
		pop[schema.AttrCreatedBy], pop[schema.AttrUpdatedBy] = author, author
	}
	p["populate"] = pop
	row, err := s.docs.FindOne(r.Context(), string(ct.UID), id, p)
	if err != nil {
		return asValidation(err)
	}
	if row == nil {
		return errNotFound
	}
	m := toMap(row)
	created, updated := m[schema.AttrCreatedBy], m[schema.AttrUpdatedBy]
	out := visible(m, ct, string(ct.UID), a)
	if stamped {
		out[schema.AttrCreatedBy], out[schema.AttrUpdatedBy] = created, updated
	}
	return data(w, out)
}

// requirePublished keeps a discard from deleting a document: the Document
// Service drops the drafts and recreates them from the published versions,
// so without one nothing would be left.
func (s *Server) requirePublished(r *http.Request, ct *schema.ContentType, id string, p document.Params) error {
	pp := document.Params{"status": "published"}
	if l, ok := p["locale"]; ok {
		pp["locale"] = l
	}
	row, err := s.docs.FindOne(r.Context(), string(ct.UID), id, pp)
	if err != nil {
		return asValidation(err)
	}
	if row == nil {
		return appError("This document has never been published: there is no published version to go back to")
	}
	return nil
}

// cmDiscard drops the draft's changes: it becomes the published version again.
func (s *Server) cmDiscard(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, _, filter, err := s.cmAccess(r, u, actionUpdate)
	if err != nil {
		return err
	}
	if !ct.DraftAndPublish() {
		return appError("This content type has no draft and publish")
	}
	var b struct {
		Locale string `json:"locale"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	id := r.PathValue("documentId")
	p := localeParam(ct, b.Locale)
	ok, err := s.allows(r.Context(), ct, id, p["locale"], filter)
	if err != nil {
		return err
	}
	if !ok {
		return errNotFound
	}
	if err := s.requirePublished(r, ct, id, p); err != nil {
		return err
	}
	if _, err := s.docs.DiscardDraft(r.Context(), string(ct.UID), id, p); err != nil {
		return asValidation(err)
	}
	s.audit(r, u, "entry.discard", map[string]any{"uid": string(ct.UID), "documentId": id, "locale": p["locale"]})
	return data(w, map[string]any{"documentId": id})
}

type writeBody struct {
	Data   map[string]any `json:"data"`
	Locale string         `json:"locale"`
}

// writable keeps the fields of data that action allows, and stamps the
// author.
func writable(data map[string]any, ct *schema.ContentType, action string, a *permissions.Ability) map[string]any {
	out := map[string]any{}
	uid := string(ct.UID)
	for k, v := range data {
		attr, ok := ct.Attributes.Get(k)
		if !ok || !attr.IsWritable() || !a.Can(action, uid, k) {
			continue
		}
		switch k {
		case schema.AttrCreatedBy, schema.AttrUpdatedBy, "publishedAt", "locale", "localizations":
			continue
		}
		out[k] = v
	}
	return out
}

func (s *Server) cmCreate(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, a, _, err := s.cmAccess(r, u, actionCreate)
	if err != nil {
		return err
	}
	if ct.IsSingleType() {
		return errNotFound
	}
	var b writeBody
	if err := decode(r, &b); err != nil {
		return err
	}
	d := writable(b.Data, ct, actionCreate, a)
	d[schema.AttrCreatedBy], d[schema.AttrUpdatedBy] = u.ID, u.ID
	p := localeParam(ct, b.Locale)
	p["data"] = d
	row, err := s.docs.Create(r.Context(), string(ct.UID), p)
	if err != nil {
		return asValidation(err)
	}
	out := toMap(row)
	if id, _ := out[schema.AttrDocumentID].(string); id != "" {
		s.recordVersion(r.Context(), ct, id, p["locale"], "", u.ID)
		s.audit(r, u, "entry.create", map[string]any{"uid": string(ct.UID), "documentId": id, "locale": p["locale"]})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": visible(out, ct, string(ct.UID), a)})
	return nil
}

func (s *Server) cmUpdate(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, a, filter, err := s.cmAccess(r, u, actionUpdate)
	if err != nil {
		return err
	}
	var b writeBody
	if err := decode(r, &b); err != nil {
		return err
	}
	id := r.PathValue("documentId")
	if b.Locale == "" {
		b.Locale = r.URL.Query().Get("locale")
	}
	p := localeParam(ct, b.Locale)
	ctx := r.Context()
	ok, err := s.allows(ctx, ct, id, p["locale"], filter)
	if err != nil {
		return err
	}
	action := actionUpdate
	if !ok && p["locale"] != nil {
		// A document of another locale: this writes its first version in
		// the requested one.
		if ok, err = s.allows(ctx, ct, id, "*", filter); err != nil {
			return err
		}
		if ok && !a.Can(actionCreate, string(ct.UID), "") {
			return errForbidden
		}
		action = actionCreate
	}
	if !ok {
		return errNotFound
	}
	d := writable(b.Data, ct, action, a)
	d[schema.AttrUpdatedBy] = u.ID
	if action == actionCreate {
		d[schema.AttrCreatedBy] = u.ID
	}
	p["data"] = d
	var before map[string]any
	if action == actionUpdate && s.cfg.AuditEnabled {
		before, _ = s.snapshot(ctx, ct, id, p["locale"], false)
	}
	row, err := s.docs.Update(ctx, string(ct.UID), id, p)
	if err != nil {
		return asValidation(err)
	}
	s.recordVersion(ctx, ct, id, p["locale"], "", u.ID)
	if action == actionCreate {
		s.audit(r, u, "entry.create", map[string]any{"uid": string(ct.UID), "documentId": id, "locale": p["locale"], "fields": changedFields(b.Data)})
	} else {
		s.auditEntryUpdate(r, u, ct, id, p["locale"], before)
	}
	return data(w, visible(toMap(row), ct, string(ct.UID), a))
}

func (s *Server) relationRoutes() {
	s.authed("GET /content-manager/relations/{uid}/{field}", s.cmRelationTargets)
}

// RelationTarget is a document a relation field can point to.
type RelationTarget struct {
	ID         any    `json:"id"`
	DocumentID string `json:"documentId"`
	Label      string `json:"label"`
	// Status is draft, published or modified, for a target with draft & publish.
	Status string `json:"status,omitempty"`
	Locale string `json:"locale,omitempty"`
}

// window turns a page into the start and limit the Document Service reads:
// like Strapi's, it ignores page and pageSize, which its callers convert.
func window(p document.Params, page, size int) {
	p["start"], p["limit"] = (page-1)*size, size
}

// documentIDsParam reads a list of document ids given as repeated or comma
// separated query values.
func documentIDsParam(r *http.Request, name string) []any {
	var out []any
	for _, v := range r.URL.Query()[name] {
		for _, id := range strings.Split(v, ",") {
			if id = strings.TrimSpace(id); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

// cmRelationTargets lists the documents the relation field of uid can link,
// searched with _q, labelled by the target's main field.
func (s *Server) cmRelationTargets(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, a, _, err := s.cmAccess(r, u, actionRead)
	if err != nil {
		return err
	}
	field := r.PathValue("field")
	attrs, readable := &ct.Attributes, a.Can(actionRead, string(ct.UID), field)
	if cu := r.URL.Query().Get("component"); cu != "" {
		comp, ok := s.set.Component(schema.UID(cu))
		if !ok {
			return errNotFound
		}
		attrs, readable = &comp.Attributes, true
	}
	attr, ok := attrs.Get(field)
	if !ok || attr.Type != schema.TypeRelation || attr.IsMorph() || attr.Target == "" || !readable {
		return errNotFound
	}
	target, ok := s.set.ContentType(schema.UID(attr.Target))
	if !ok {
		return errNotFound
	}
	ctx := r.Context()
	cfg, err := s.effectiveConfig(ctx, target)
	if err != nil {
		return err
	}
	mainField := cfg.Settings.MainField
	page, size := pageParams(r)
	p := document.Params{"sort": mainField + ":asc"}
	fields := []any{schema.AttrUpdatedAt}
	if mainField != "id" {
		fields = append(fields, mainField)
	}
	p["fields"] = fields
	if q := strings.TrimSpace(r.URL.Query().Get("_q")); q != "" {
		p["_q"] = q
	}
	// idsToOmit leaves out the documents the field already holds, so that
	// every page of the picker brings new ones; documentIds reads back the
	// held ones, for their label and status.
	filter := map[string]any{}
	if ids := documentIDsParam(r, "idsToOmit"); len(ids) > 0 {
		filter["$notIn"] = ids
	}
	if ids := documentIDsParam(r, "documentIds"); len(ids) > 0 {
		filter["$in"] = ids
		page, size = 1, len(ids)
		delete(p, "sort")
	}
	if len(filter) > 0 {
		p["filters"] = map[string]any{schema.AttrDocumentID: filter}
	}
	if target.Localized() {
		if l := r.URL.Query().Get("locale"); l != "" {
			p["locale"] = l
		}
	}
	total, err := s.docs.Count(ctx, attr.Target, p)
	if err != nil {
		return asValidation(err)
	}
	window(p, page, size)
	rows, err := s.docs.FindMany(ctx, attr.Target, p)
	if err != nil {
		return asValidation(err)
	}
	maps := make([]map[string]any, 0, len(rows))
	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		m := toMap(row)
		maps = append(maps, m)
		ids = append(ids, m[schema.AttrDocumentID])
	}
	if target.DraftAndPublish() && len(maps) > 0 {
		if err := s.withStatus(ctx, attr.Target, p["locale"], "draft", maps, ids); err != nil {
			return err
		}
	}
	out := make([]RelationTarget, 0, len(maps))
	for _, m := range maps {
		label := ""
		if v := m[mainField]; v != nil {
			label = fmt.Sprint(v)
		}
		id, _ := m[schema.AttrDocumentID].(string)
		st, _ := m["status"].(string)
		loc, _ := m["locale"].(string)
		out = append(out, RelationTarget{ID: m["id"], DocumentID: id, Label: label, Status: st, Locale: loc})
	}
	pages := int((total + int64(size) - 1) / int64(size))
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"pagination": Pagination{Page: page, PageSize: size, PageCount: pages, Total: total}, "mainField": mainField}})
	return nil
}

func (s *Server) singleTypeRoutes() {
	s.authed("GET /content-manager/single-types/{uid}", s.stFind)
	s.authed("PUT /content-manager/single-types/{uid}", s.stSave)
	s.authed("DELETE /content-manager/single-types/{uid}", s.stDelete)
	s.authed("POST /content-manager/single-types/{uid}/actions/publish", s.stAction("publish"))
	s.authed("POST /content-manager/single-types/{uid}/actions/unpublish", s.stAction("unpublish"))
	s.authed("POST /content-manager/single-types/{uid}/actions/discard", s.stAction("discard"))
}

// singleAccess is cmAccess for a single type: its document id, empty when
// it has none yet (in that locale), or when the row filter hides it.
func (s *Server) singleAccess(r *http.Request, u *User, action, locale string) (*schema.ContentType, *permissions.Ability, string, error) {
	uid := r.PathValue("uid")
	ct, ok := s.set.ContentType(schema.UID(uid))
	if !ok || !ct.IsSingleType() || !contains(s.ContentManagerTypes(), uid) {
		return nil, nil, "", errNotFound
	}
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return nil, nil, "", err
	}
	filter, allowed := a.Filter(action, uid)
	if !allowed {
		return nil, nil, "", errForbidden
	}
	p := localeParam(ct, locale)
	if f := and(filter); f != nil {
		p["filters"] = scoped(f)
	}
	p["fields"] = []any{schema.AttrDocumentID}
	row, err := s.docs.FindFirst(r.Context(), uid, p)
	if err != nil {
		return nil, nil, "", asValidation(err)
	}
	id := ""
	if row != nil {
		id, _ = toMap(row)[schema.AttrDocumentID].(string)
	}
	return ct, a, id, nil
}

func (s *Server) stFind(w http.ResponseWriter, r *http.Request, u *User) error {
	q := r.URL.Query()
	ct, a, id, err := s.singleAccess(r, u, actionRead, q.Get("locale"))
	if err != nil {
		return err
	}
	if id == "" {
		return data(w, nil)
	}
	p := localeParam(ct, q.Get("locale"))
	p["populate"] = s.deepPopulate(&ct.Attributes, 0)
	if ct.DraftAndPublish() && q.Get("status") == "published" {
		p["status"] = "published"
	}
	row, err := s.docs.FindOne(r.Context(), string(ct.UID), id, p)
	if err != nil {
		return asValidation(err)
	}
	if row == nil {
		return data(w, nil)
	}
	out := visible(toMap(row), ct, string(ct.UID), a)
	if ct.DraftAndPublish() {
		if err := s.withStatus(r.Context(), string(ct.UID), p["locale"], "draft", []map[string]any{out}, []any{id}); err != nil {
			return err
		}
	}
	return data(w, out)
}

// stSave creates the document of a single type, or updates it.
func (s *Server) stSave(w http.ResponseWriter, r *http.Request, u *User) error {
	var b writeBody
	if err := decode(r, &b); err != nil {
		return err
	}
	ct, a, id, err := s.singleAccess(r, u, actionRead, b.Locale)
	if err != nil {
		return err
	}
	action := actionUpdate
	if id == "" {
		action = actionCreate
	}
	if _, allowed := a.Filter(action, string(ct.UID)); !allowed {
		return errForbidden
	}
	if id != "" {
		if _, _, id, err = s.singleAccess(r, u, actionUpdate, b.Locale); err != nil {
			return err
		}
		if id == "" {
			return errNotFound
		}
	}
	d := writable(b.Data, ct, action, a)
	d[schema.AttrUpdatedBy] = u.ID
	p := localeParam(ct, b.Locale)
	var row any
	var before map[string]any
	if id != "" && s.cfg.AuditEnabled {
		before, _ = s.snapshot(r.Context(), ct, id, p["locale"], false)
	}
	if id == "" {
		d[schema.AttrCreatedBy] = u.ID
		p["data"] = d
		row, err = s.docs.Create(r.Context(), string(ct.UID), p)
	} else {
		p["data"] = d
		row, err = s.docs.Update(r.Context(), string(ct.UID), id, p)
	}
	if err != nil {
		return asValidation(err)
	}
	out := toMap(row)
	if docID, _ := out[schema.AttrDocumentID].(string); docID != "" {
		s.recordVersion(r.Context(), ct, docID, p["locale"], "", u.ID)
		if before == nil {
			s.audit(r, u, "entry.create", map[string]any{"uid": string(ct.UID), "documentId": docID, "locale": p["locale"], "fields": changedFields(b.Data)})
		} else {
			s.auditEntryUpdate(r, u, ct, docID, p["locale"], before)
		}
	}
	return data(w, visible(out, ct, string(ct.UID), a))
}

func (s *Server) stDelete(w http.ResponseWriter, r *http.Request, u *User) error {
	locale := r.URL.Query().Get("locale")
	ct, _, id, err := s.singleAccess(r, u, actionDelete, locale)
	if err != nil {
		return err
	}
	if id == "" {
		return errNotFound
	}
	if _, err := s.docs.Delete(r.Context(), string(ct.UID), id, localeParam(ct, locale)); err != nil {
		return asValidation(err)
	}
	s.forgetDocument(r.Context(), string(ct.UID), id, locale)
	s.audit(r, u, "entry.delete", map[string]any{"uid": string(ct.UID), "documentId": id, "locale": locale})
	return data(w, map[string]any{"documentId": id})
}

func (s *Server) stAction(op string) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, u *User) error {
		var b struct {
			Locale string `json:"locale"`
		}
		if err := decode(r, &b); err != nil {
			return err
		}
		action := actionPublish
		if op == "discard" {
			action = actionUpdate
		}
		ct, _, id, err := s.singleAccess(r, u, action, b.Locale)
		if err != nil {
			return err
		}
		if !ct.DraftAndPublish() {
			return appError("This content type has no draft and publish")
		}
		if id == "" {
			return errNotFound
		}
		p := localeParam(ct, b.Locale)
		switch op {
		case "publish":
			if _, err = s.docs.Publish(r.Context(), string(ct.UID), id, p); err == nil {
				s.recordVersion(r.Context(), ct, id, p["locale"], "published", u.ID)
			}
		case "discard":
			if err = s.requirePublished(r, ct, id, p); err == nil {
				_, err = s.docs.DiscardDraft(r.Context(), string(ct.UID), id, p)
			}
		default:
			_, err = s.docs.Unpublish(r.Context(), string(ct.UID), id, p)
		}
		if err != nil {
			return asValidation(err)
		}
		s.audit(r, u, "entry."+op, map[string]any{"uid": string(ct.UID), "documentId": id, "locale": p["locale"]})
		return data(w, map[string]any{"documentId": id})
	}
}

func (s *Server) componentRoutes() {
	s.authed("GET /content-manager/components", s.cmComponents)
}

// ComponentModel is a component as the edit view needs it.
type ComponentModel struct {
	UID        string                       `json:"uid"`
	Category   string                       `json:"category"`
	Info       schema.Info                  `json:"info"`
	Attributes map[string]*schema.Attribute `json:"attributes"`
	Fields     []string                     `json:"fields"`
}

func (s *Server) cmComponents(w http.ResponseWriter, _ *http.Request, _ *User) error {
	out := []ComponentModel{}
	for uid, c := range s.set.Components.All() {
		cat, _, _ := strings.Cut(uid, ".")
		m := ComponentModel{UID: uid, Category: cat, Info: c.Info, Attributes: map[string]*schema.Attribute{}}
		for name, attr := range c.Attributes.All() {
			if attr.Type == schema.TypePassword || !attr.IsVisible() || name == "id" {
				continue
			}
			m.Attributes[name] = attr
			m.Fields = append(m.Fields, name)
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return data(w, out)
}

// deepPopulate loads every component and dynamic zone level, and the
// relations and media of each, as the edit view writes them back whole.
func (s *Server) deepPopulate(attrs *schema.OrderedMap[*schema.Attribute], depth int) map[string]any {
	out := map[string]any{}
	if depth > 10 {
		return out
	}
	sub := func(uid string) any {
		c, ok := s.set.Component(schema.UID(uid))
		if !ok {
			return true
		}
		if p := s.deepPopulate(&c.Attributes, depth+1); len(p) > 0 {
			return map[string]any{"populate": p}
		}
		return true
	}
	for name, a := range attrs.All() {
		switch a.Type {
		case schema.TypeRelation:
			if a.Virtual || name == schema.AttrCreatedBy || name == schema.AttrUpdatedBy || name == "localizations" {
				continue
			}
			out[name] = true
		case schema.TypeMedia:
			out[name] = true
		case schema.TypeComponent:
			out[name] = sub(a.Component)
		case schema.TypeDynamicZone:
			on := map[string]any{}
			for _, c := range a.Components {
				if p, ok := sub(c).(map[string]any); ok {
					on[c] = p
				} else {
					on[c] = map[string]any{"populate": "*"}
				}
			}
			out[name] = map[string]any{"on": on}
		}
	}
	return out
}
