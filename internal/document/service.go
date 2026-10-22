package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// Service is the documents API of every content type.
type Service struct {
	E    *query.Engine
	Set  *schema.Set
	comp *Components
}

// New returns the document service of an application.
func New(e *query.Engine, set *schema.Set) *Service {
	return &Service{E: e, Set: set, comp: &Components{E: e, Set: set}}
}

func (s *Service) contentType(uid string) (*schema.ContentType, error) {
	ct, ok := s.Set.ContentType(schema.UID(uid))
	if !ok {
		return nil, fmt.Errorf("content type %s not found", uid)
	}
	return ct, nil
}

func clone(p Params) Params {
	out := Params{}
	for k, v := range p {
		out[k] = v
	}
	return out
}

func isEmptyParam(v any) bool { return v == nil || v == "" }

func lookupOf(p Params) map[string]any {
	l, _ := p["lookup"].(map[string]any)
	out := map[string]any{}
	for k, v := range l {
		out[k] = v
	}
	return out
}

func validateParams(p Params) error {
	if _, ok := p["lookup"]; ok {
		return validation("Invalid params: 'lookup'")
	}
	return nil
}

// defaultToDraft: any status but "published" is "draft".
func defaultToDraft(p Params) Params {
	out := clone(p)
	if st, _ := p["status"].(string); st != "published" {
		out["status"] = "draft"
	}
	return out
}

func (s *Service) statusToLookup(ct *schema.ContentType, p Params) Params {
	if !ct.DraftAndPublish() {
		return p
	}
	out := clone(p)
	l := lookupOf(p)
	switch p["status"] {
	case "published":
		l[schema.AttrPublishedAt] = map[string]any{"$notNull": true}
	case "draft":
		l[schema.AttrPublishedAt] = map[string]any{"$null": true}
	}
	out["lookup"] = l
	return out
}

func (s *Service) withDefaultLocale(ctx context.Context, ct *schema.ContentType, p Params) Params {
	if !ct.Localized() || truthy(p["locale"]) {
		return p
	}
	out := clone(p)
	out["locale"] = s.defaultLocale(ctx)
	return out
}

func (s *Service) localeToLookup(ct *schema.ContentType, p Params) (Params, error) {
	if !truthy(p["locale"]) || !ct.Localized() {
		return p, nil
	}
	loc, ok := p["locale"].(string)
	if !ok {
		return nil, validation("Invalid locale param %v provided. Document locales must be strings.", p["locale"])
	}
	out := clone(p)
	l := lookupOf(p)
	l[schema.AttrLocale] = loc
	out["lookup"] = l
	return out, nil
}

func (s *Service) multiLocaleToLookup(ct *schema.ContentType, p Params) Params {
	if !ct.Localized() || !truthy(p["locale"]) || p["locale"] == "*" {
		return p
	}
	out := clone(p)
	l := lookupOf(p)
	l[schema.AttrLocale] = p["locale"]
	out["lookup"] = l
	return out
}

func (s *Service) localeToData(ct *schema.ContentType, p Params) (Params, error) {
	if !ct.Localized() || !truthy(p["locale"]) {
		return p, nil
	}
	loc, ok := p["locale"].(string)
	if !ok || loc == "*" {
		return nil, validation("Invalid locale param %v provided. Document locales must be strings.", p["locale"])
	}
	out := clone(p)
	out["data"] = merge(dataOf(p), map[string]any{schema.AttrLocale: loc})
	return out, nil
}

func dataOf(p Params) map[string]any {
	d, _ := p["data"].(map[string]any)
	if d == nil {
		return map[string]any{}
	}
	return d
}

func filterDataPublishedAt(p Params) Params {
	d := dataOf(p)
	if truthy(d[schema.AttrPublishedAt]) {
		out := clone(p)
		out["data"] = merge(d, map[string]any{schema.AttrPublishedAt: nil})
		return out
	}
	return p
}

func setStatusToDraft(ct *schema.ContentType, p Params) Params {
	if !ct.DraftAndPublish() {
		return p
	}
	out := clone(p)
	out["status"] = "draft"
	return out
}

func statusToData(ct *schema.ContentType, p Params) Params {
	out := clone(p)
	if !ct.DraftAndPublish() {
		out["data"] = merge(dataOf(p), map[string]any{schema.AttrPublishedAt: time.Now()})
		return out
	}
	switch p["status"] {
	case "published":
		out["data"] = merge(dataOf(p), map[string]any{schema.AttrPublishedAt: time.Now()})
	case "draft":
		out["data"] = merge(dataOf(p), map[string]any{schema.AttrPublishedAt: nil})
	}
	return out
}

// addDocumentIDToFields is Strapi's transformFields: documentId is always read.
func addDocumentIDToFields(fields any) any {
	switch x := fields.(type) {
	case string:
		if x == "*" {
			return x
		}
		if x == "" {
			return schema.AttrDocumentID
		}
		for _, f := range strings.Split(x, ",") {
			if f == schema.AttrDocumentID {
				return x
			}
		}
		return x + "," + schema.AttrDocumentID
	case []any:
		for _, f := range x {
			if f == schema.AttrDocumentID {
				return x
			}
		}
		return append(append([]any{}, x...), schema.AttrDocumentID)
	}
	return fields
}

// addDocumentIDToPopulate applies addDocumentIDToFields to the populated
// relations that pick their fields.
func (s *Service) addDocumentIDToPopulate(uid string, populate any) any {
	pm, ok := populate.(map[string]any)
	m := s.model(uid)
	if !ok || m == nil {
		return populate
	}
	out := map[string]any{}
	for k, v := range pm {
		out[k] = v
		a, ok := m.attrs.Get(k)
		vm, isMap := v.(map[string]any)
		if !ok || !isMap {
			continue
		}
		nv := merge(vm, nil)
		if a.Type == schema.TypeRelation {
			if f, ok := vm["fields"].([]any); ok {
				nv["fields"] = addDocumentIDToFields(f)
			}
		}
		if target := s.targetOf(a); target != "" && vm["populate"] != nil {
			nv["populate"] = s.addDocumentIDToPopulate(target, vm["populate"])
		}
		out[k] = nv
	}
	return out
}

// toQuery is transformParamsDocumentId (fields and populate) then
// transformParamsToQuery: the lookup goes into the where.
func (s *Service) toQuery(uid string, p Params) (query.Params, error) {
	allowed := Params{}
	for _, k := range []string{"_q", "sort", "filters", "fields", "populate", "page", "pageSize", "start", "limit", "status", "locale"} {
		if v, ok := p[k]; ok {
			allowed[k] = v
		}
	}
	if f, ok := allowed["fields"]; ok && f != nil {
		allowed["fields"] = addDocumentIDToFields(f)
	}
	if pop, ok := allowed["populate"]; ok && pop != nil {
		allowed["populate"] = s.addDocumentIDToPopulate(uid, pop)
	}
	q, err := s.TransformQuery(uid, allowed)
	if err != nil {
		return q, err
	}
	lookup := lookupOf(p)
	switch w := q.Where.(type) {
	case nil:
		if len(lookup) > 0 {
			q.Where = lookup
		}
	case map[string]any:
		q.Where = merge(lookup, w)
	case []any:
		if len(lookup) > 0 {
			q.Where = map[string]any{"$and": append([]any{lookup}, w...)}
		} else {
			q.Where = map[string]any{"$and": w}
		}
	}
	return q, nil
}

func selection(p Params) Params {
	out := Params{}
	for _, k := range []string{"fields", "populate", "status"} {
		if v, ok := p[k]; ok {
			out[k] = v
		}
	}
	return out
}

func (s *Service) readParams(ctx context.Context, uid string, p Params, single bool) (query.Params, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return query.Params{}, err
	}
	if err := validateParams(p); err != nil {
		return query.Params{}, err
	}
	p = defaultToDraft(p)
	p = s.statusToLookup(ct, p)
	p = s.withDefaultLocale(ctx, ct, p)
	if single {
		if p, err = s.localeToLookup(ct, p); err != nil {
			return query.Params{}, err
		}
	} else {
		p = s.multiLocaleToLookup(ct, p)
	}
	return s.toQuery(uid, p)
}

// FindMany returns the entries matching p, drafts unless status is
// "published", in the default locale unless locale is given ("*": all).
func (s *Service) FindMany(ctx context.Context, uid string, p Params) ([]query.Row, error) {
	q, err := s.readParams(ctx, uid, p, false)
	if err != nil {
		return nil, err
	}
	return s.E.FindMany(ctx, uid, q)
}

// FindFirst returns the first entry matching p.
func (s *Service) FindFirst(ctx context.Context, uid string, p Params) (query.Row, error) {
	q, err := s.readParams(ctx, uid, p, true)
	if err != nil {
		return nil, err
	}
	return s.E.FindOne(ctx, uid, q)
}

// FindOne returns the entry of documentID for the status and locale of p.
func (s *Service) FindOne(ctx context.Context, uid, documentID string, p Params) (query.Row, error) {
	q, err := s.readParams(ctx, uid, p, true)
	if err != nil {
		return nil, err
	}
	q.Where = mergeWhere(q.Where, map[string]any{schema.AttrDocumentID: documentID})
	return s.E.FindOne(ctx, uid, q)
}

func mergeWhere(where any, extra map[string]any) any {
	switch w := where.(type) {
	case nil:
		return extra
	case map[string]any:
		return merge(w, extra)
	}
	return map[string]any{"$and": []any{where, extra}}
}

// Count counts the entries matching p.
func (s *Service) Count(ctx context.Context, uid string, p Params) (int64, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return 0, err
	}
	if err := validateParams(p); err != nil {
		return 0, err
	}
	if ct.DraftAndPublish() {
		p = defaultToDraft(p)
	}
	p = s.statusToLookup(ct, p)
	p = s.withDefaultLocale(ctx, ct, p)
	p = s.multiLocaleToLookup(ct, p)
	q, err := s.toQuery(uid, p)
	if err != nil {
		return 0, err
	}
	return s.E.Count(ctx, uid, query.Params{Where: q.Where, Filters: q.Filters, Search: q.Search})
}

// Create creates a document, published right away when status is
// "published" on a draft and publish type.
func (s *Service) Create(ctx context.Context, uid string, p Params) (query.Row, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return nil, err
	}
	var doc query.Row
	err = s.E.DB.Tx(ctx, func(ctx context.Context) error {
		if err := validateParams(p); err != nil {
			return err
		}
		qp := clone(p)
		delete(qp, "documentId")
		qp = filterDataPublishedAt(qp)
		qp = setStatusToDraft(ct, qp)
		qp = statusToData(ct, qp)
		qp = s.withDefaultLocale(ctx, ct, qp)
		if qp, err = s.localeToData(ct, qp); err != nil {
			return err
		}
		if doc, err = s.createEntry(ctx, ct, qp); err != nil {
			return err
		}
		if ct.DraftAndPublish() && p["status"] == "published" {
			pp := clone(p)
			id, _ := doc.Get(schema.AttrDocumentID)
			res, err := s.publish(ctx, ct, fmt.Sprint(id), pp)
			if err != nil {
				return err
			}
			doc = nil
			if len(res) > 0 {
				doc = res[0]
			}
		}
		return nil
	})
	return doc, err
}

func (s *Service) createEntry(ctx context.Context, ct *schema.ContentType, p Params) (query.Row, error) {
	uid := string(ct.UID)
	input, ok := p["data"].(map[string]any)
	if !ok {
		return nil, errors.New("Create requires data attribute") //nolint:revive,staticcheck // Strapi's message
	}
	status, _ := p["status"].(string)
	data, err := s.transformData(ctx, input, relSource{uid: uid, locale: p["locale"], status: status, allowMissing: truthy(p["allowMissingId"])})
	if err != nil {
		return nil, err
	}
	q, err := s.toQuery(uid, selection(p))
	if err != nil {
		return nil, err
	}
	if isEmptyParam(data[schema.AttrDocumentID]) {
		delete(data, schema.AttrDocumentID)
	}
	if doc, ok := data[schema.AttrDocumentID]; ok {
		where := map[string]any{schema.AttrDocumentID: doc}
		state := ""
		if ct.Localized() {
			where[schema.AttrLocale] = data[schema.AttrLocale]
		}
		if ct.DraftAndPublish() {
			if truthy(data[schema.AttrPublishedAt]) {
				where[schema.AttrPublishedAt] = map[string]any{"$notNull": true}
				state = "published"
			} else {
				where[schema.AttrPublishedAt] = map[string]any{"$null": true}
				state = "draft"
			}
		}
		existing, err := s.E.FindOne(ctx, uid, query.Params{Select: []string{"id"}, Where: where})
		if err != nil {
			return nil, err
		}
		if existing != nil {
			msg := fmt.Sprintf("A %s entry with documentId \"%v\"", state, doc)
			if ct.Localized() && truthy(data[schema.AttrLocale]) {
				msg += fmt.Sprintf(" and locale \"%v\"", data[schema.AttrLocale])
			}
			return nil, &appError{msg + fmt.Sprintf(" already exists for UID \"%s\". This combination must be unique.", uid)}
		}
	}
	componentData, err := s.comp.Create(ctx, uid, data)
	if err != nil {
		return nil, err
	}
	entry, err := s.comp.assign(uid, componentData, data)
	if err != nil {
		return nil, err
	}
	if err := s.applyTransforms(uid, entry); err != nil {
		return nil, err
	}
	return s.E.Create(ctx, uid, entry, q)
}

// applyTransforms hashes passwords before they are stored.
func (s *Service) applyTransforms(uid string, data query.Data) error {
	m := s.model(uid)
	if m == nil {
		return nil
	}
	for k, v := range data {
		a, ok := m.attrs.Get(k)
		if !ok || a.Type != schema.TypePassword {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(str), 10)
		if err != nil {
			return err
		}
		data[k] = string(hash)
	}
	return nil
}

// deleteEntry deletes a row then its components, loaded before the row goes.
func (s *Service) deleteEntry(ctx context.Context, uid string, id any, q query.Params) (query.Row, error) {
	loaded, err := s.comp.load(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	row, err := s.E.Delete(ctx, uid, map[string]any{"id": id}, q)
	if err != nil {
		return nil, err
	}
	if err := s.comp.deleteLoaded(ctx, uid, loaded); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Service) updateEntry(ctx context.Context, ct *schema.ContentType, entryID any, p Params) (query.Row, error) {
	uid := string(ct.UID)
	status, _ := p["status"].(string)
	data, err := s.transformData(ctx, dataOf(p), relSource{uid: uid, locale: p["locale"], status: status})
	if err != nil {
		return nil, err
	}
	delete(data, schema.AttrDocumentID)
	q, err := s.toQuery(uid, selection(p))
	if err != nil {
		return nil, err
	}
	componentData, err := s.comp.Update(ctx, uid, entryID, data)
	if err != nil {
		return nil, err
	}
	entry, err := s.comp.assign(uid, componentData, data)
	if err != nil {
		return nil, err
	}
	if err := s.applyTransforms(uid, entry); err != nil {
		return nil, err
	}
	return s.E.Update(ctx, uid, map[string]any{"id": entryID}, entry, q)
}

// Update updates the draft of documentID (or creates the requested locale of
// an existing document), then publishes when status is "published".
func (s *Service) Update(ctx context.Context, uid, documentID string, p Params) (query.Row, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return nil, err
	}
	var result query.Row
	err = s.E.DB.Tx(ctx, func(ctx context.Context) error {
		if err := validateParams(p); err != nil {
			return err
		}
		qp := filterDataPublishedAt(p)
		qp = setStatusToDraft(ct, qp)
		qp = s.statusToLookup(ct, qp)
		qp = statusToData(ct, qp)
		qp = s.withDefaultLocale(ctx, ct, qp)
		if qp, err = s.localeToLookup(ct, qp); err != nil {
			return err
		}
		if qp, err = s.localeToData(ct, qp); err != nil {
			return err
		}
		q, err := s.toQuery(uid, merge(selection(qp), map[string]any{"lookup": qp["lookup"]}))
		if err != nil {
			return err
		}
		q.Where = mergeWhere(q.Where, map[string]any{schema.AttrDocumentID: documentID})
		entry, err := s.E.FindOne(ctx, uid, q)
		if err != nil {
			return err
		}
		if entry != nil {
			id, _ := entry.Get("id")
			if result, err = s.updateEntry(ctx, ct, id, qp); err != nil {
				return err
			}
		} else {
			exists, err := s.E.FindOne(ctx, uid, query.Params{Where: map[string]any{schema.AttrDocumentID: documentID}})
			if err != nil {
				return err
			}
			if exists != nil {
				cp := clone(qp)
				cp["data"] = merge(dataOf(qp), map[string]any{schema.AttrDocumentID: documentID})
				if result, err = s.createEntry(ctx, ct, cp); err != nil {
					return err
				}
			}
		}
		if ct.DraftAndPublish() && result != nil && p["status"] == "published" {
			res, err := s.publish(ctx, ct, documentID, clone(p))
			if err != nil {
				return err
			}
			result = nil
			if len(res) > 0 {
				result = res[0]
			}
		}
		return nil
	})
	return result, err
}

// DeleteResult is what deleting a document returns.
type DeleteResult struct {
	DocumentID string
	Entries    []query.Row
}

// Delete deletes every version of documentID (in the locale of p).
func (s *Service) Delete(ctx context.Context, uid, documentID string, p Params) (*DeleteResult, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return nil, err
	}
	res := &DeleteResult{DocumentID: documentID}
	err = s.E.DB.Tx(ctx, func(ctx context.Context) error {
		if err := validateParams(p); err != nil {
			return err
		}
		lp := clone(p)
		delete(lp, "status")
		lp = s.withDefaultLocale(ctx, ct, lp)
		lp = s.multiLocaleToLookup(ct, lp)
		lookup, err := s.toQuery(uid, lp)
		if err != nil {
			return err
		}
		lookup.Where = mergeWhere(lookup.Where, map[string]any{schema.AttrDocumentID: documentID})
		sel, err := s.toQuery(uid, selection(lp))
		if err != nil {
			return err
		}
		if ct.DraftAndPublish() && p["status"] == "draft" {
			return errors.New("Cannot delete a draft document") //nolint:revive,staticcheck // Strapi's message
		}
		rows, err := s.E.FindMany(ctx, uid, lookup)
		if err != nil {
			return err
		}
		for _, r := range rows {
			id, _ := r.Get("id")
			deleted, err := s.deleteEntry(ctx, uid, id, sel)
			if err != nil {
				return err
			}
			res.Entries = append(res.Entries, deleted)
		}
		return nil
	})
	return res, err
}

// deepPopulate is Strapi's getDeepPopulate: every relation (some fields),
// media, and components and dynamic zones all the way down.
func (s *Service) deepPopulate(uid string, relationalFields []any) map[string]any {
	m := s.model(uid)
	out := map[string]any{}
	if m == nil {
		return out
	}
	for name, a := range m.attrs.All() {
		switch a.Type {
		case schema.TypeRelation:
			if a.Virtual {
				continue
			}
			out[name] = map[string]any{"select": relationalFields}
		case schema.TypeMedia:
			out[name] = map[string]any{"select": []any{"*"}}
		case schema.TypeComponent:
			out[name] = map[string]any{"populate": s.deepPopulate(a.Component, relationalFields)}
		case schema.TypeDynamicZone:
			on := map[string]any{}
			for _, c := range a.Components {
				on[c] = map[string]any{"populate": s.deepPopulate(c, relationalFields)}
			}
			out[name] = map[string]any{"on": on}
		}
	}
	return out
}

func (s *Service) versionLookup(ctx context.Context, ct *schema.ContentType, p Params) (map[string]any, Params, error) {
	if err := validateParams(p); err != nil {
		return nil, nil, err
	}
	qp := s.withDefaultLocale(ctx, ct, p)
	qp = s.multiLocaleToLookup(ct, qp)
	return lookupOf(qp), qp, nil
}

// Publish publishes the drafts of documentID: their published versions are
// replaced by copies of the drafts.
func (s *Service) Publish(ctx context.Context, uid, documentID string, p Params) ([]query.Row, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return nil, err
	}
	var out []query.Row
	err = s.E.DB.Tx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.publish(ctx, ct, documentID, p)
		return err
	})
	return out, err
}

func (s *Service) publish(ctx context.Context, ct *schema.ContentType, documentID string, p Params) ([]query.Row, error) {
	if isEmptyParam(documentID) {
		return nil, validation("Cannot publish a document without a documentId")
	}
	uid := string(ct.UID)
	lookup, qp, err := s.versionLookup(ctx, ct, p)
	if err != nil {
		return nil, err
	}
	drafts, err := s.E.FindMany(ctx, uid, query.Params{
		Where:    merge(lookup, map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: nil}),
		Populate: s.deepPopulate(uid, []any{schema.AttrDocumentID, schema.AttrLocale}),
	})
	if err != nil {
		return nil, err
	}
	old, err := s.E.FindMany(ctx, uid, query.Params{
		Where:  merge(lookup, map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: map[string]any{"$ne": nil}}),
		Select: []string{"id", schema.AttrLocale},
	})
	if err != nil {
		return nil, err
	}
	for _, o := range old {
		id, _ := o.Get("id")
		if _, err := s.deleteEntry(ctx, uid, id, query.Params{}); err != nil {
			return nil, err
		}
	}
	var published []query.Row
	for _, d := range drafts {
		data := rowToData(d)
		delete(data, "id")
		data[schema.AttrPublishedAt] = time.Now()
		cp := clone(qp)
		delete(cp, "lookup")
		cp["data"] = data
		cp["locale"] = data[schema.AttrLocale]
		cp["status"] = "published"
		cp["allowMissingId"] = true
		row, err := s.createEntry(ctx, ct, cp)
		if err != nil {
			return nil, err
		}
		published = append(published, row)
	}
	return published, nil
}

// rowToData turns a populated row back into input data.
func rowToData(r query.Row) map[string]any {
	out := map[string]any{}
	for k, v := range r.All() {
		out[k] = rowValue(v)
	}
	return out
}

func rowValue(v any) any {
	switch x := v.(type) {
	case query.Row:
		return rowToData(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = rowValue(e)
		}
		return out
	case int64:
		return float64(x)
	}
	return v
}

// Unpublish deletes the published versions of documentID.
func (s *Service) Unpublish(ctx context.Context, uid, documentID string, p Params) ([]query.Row, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return nil, err
	}
	if isEmptyParam(documentID) {
		return nil, validation("Cannot unpublish a document without a documentId")
	}
	var out []query.Row
	err = s.E.DB.Tx(ctx, func(ctx context.Context) error {
		if err := validateParams(p); err != nil {
			return err
		}
		qp := s.withDefaultLocale(ctx, ct, p)
		qp = s.multiLocaleToLookup(ct, qp)
		q, err := s.toQuery(uid, qp)
		if err != nil {
			return err
		}
		q.Where = mergeWhere(q.Where, map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: map[string]any{"$ne": nil}})
		rows, err := s.E.FindMany(ctx, uid, q)
		if err != nil {
			return err
		}
		for _, r := range rows {
			id, _ := r.Get("id")
			if _, err := s.deleteEntry(ctx, uid, id, query.Params{}); err != nil {
				return err
			}
		}
		out = rows
		return nil
	})
	return out, err
}

// DiscardDraft replaces the drafts of documentID with copies of its
// published versions.
func (s *Service) DiscardDraft(ctx context.Context, uid, documentID string, p Params) ([]query.Row, error) {
	ct, err := s.contentType(uid)
	if err != nil {
		return nil, err
	}
	if isEmptyParam(documentID) {
		return nil, validation("Cannot discard the draft of a document without a documentId")
	}
	var out []query.Row
	err = s.E.DB.Tx(ctx, func(ctx context.Context) error {
		lookup, qp, err := s.versionLookup(ctx, ct, p)
		if err != nil {
			return err
		}
		versions, err := s.E.FindMany(ctx, uid, query.Params{
			Where:    merge(lookup, map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: map[string]any{"$ne": nil}}),
			Populate: s.deepPopulate(uid, []any{schema.AttrDocumentID, schema.AttrLocale}),
		})
		if err != nil {
			return err
		}
		drafts, err := s.E.FindMany(ctx, uid, query.Params{
			Where:  merge(lookup, map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: nil}),
			Select: []string{"id", schema.AttrLocale},
		})
		if err != nil {
			return err
		}
		for _, d := range drafts {
			id, _ := d.Get("id")
			if _, err := s.deleteEntry(ctx, uid, id, query.Params{}); err != nil {
				return err
			}
		}
		for _, v := range versions {
			data := rowToData(v)
			delete(data, "id")
			data[schema.AttrPublishedAt] = nil
			cp := clone(qp)
			delete(cp, "lookup")
			cp["data"] = data
			cp["locale"] = data[schema.AttrLocale]
			cp["status"] = "draft"
			cp["allowMissingId"] = true
			row, err := s.createEntry(ctx, ct, cp)
			if err != nil {
				return err
			}
			out = append(out, row)
		}
		return nil
	})
	return out, err
}

func jsonString(v any) (string, error) {
	b, err := marshalJSON(v)
	return string(b), err
}
