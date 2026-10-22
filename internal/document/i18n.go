package document

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// isLocalizedAttribute is i18n's: localized option, any relation, or a uid.
func isLocalizedAttribute(a *schema.Attribute) bool {
	if a.Type == schema.TypeRelation || a.Type == schema.TypeUID {
		return true
	}
	var opts struct {
		I18n struct {
			Localized bool `json:"localized"`
		} `json:"i18n"`
	}
	return len(a.PluginOptions) > 0 && json.Unmarshal(a.PluginOptions, &opts) == nil && opts.I18n.Localized
}

func visibleAttributes(m *model) []string {
	var out []string
	for name, a := range m.attrs.All() {
		switch name {
		case "id", schema.AttrDocumentID, schema.AttrPublishedAt, schema.AttrCreatedAt, schema.AttrUpdatedAt:
			continue
		}
		if a.Visible != nil && !*a.Visible {
			continue
		}
		out = append(out, name)
	}
	return out
}

func nonLocalizedAttributes(m *model) []string {
	var out []string
	for _, name := range visibleAttributes(m) {
		if a, _ := m.attrs.Get(name); !isLocalizedAttribute(a) {
			out = append(out, name)
		}
	}
	return out
}

func isScalarAttr(a *schema.Attribute) bool {
	switch a.Type {
	case schema.TypeRelation, schema.TypeComponent, schema.TypeDynamicZone, schema.TypeMedia:
		return false
	}
	return true
}

// nestedPopulateOfNonLocalized is getNestedPopulateOfNonLocalizedAttributes.
func (s *Service) nestedPopulateOfNonLocalized(uid string) []string {
	m := s.model(uid)
	if m == nil {
		return nil
	}
	var all []string
	for name, a := range m.attrs.All() {
		if isScalarAttr(a) {
			all = append(all, name)
		}
	}
	all = append(all, nonLocalizedAttributes(m)...)
	if !m.isContentType {
		for name, a := range m.attrs.All() {
			if a.Type == schema.TypeRelation || a.Type == schema.TypeMedia {
				all = append(all, name)
			}
		}
	}
	// Strapi keeps the names that appear exactly once.
	count := map[string]int{}
	for _, n := range all {
		count[n]++
	}
	var current []string
	for _, n := range all {
		if count[n] == 1 {
			current = append(current, n)
		}
	}
	out := append([]string(nil), current...)
	for _, name := range current {
		a, _ := m.attrs.Get(name)
		switch a.Type {
		case schema.TypeComponent:
			for _, nested := range s.nestedPopulateOfNonLocalized(a.Component) {
				out = append(out, name+"."+nested)
			}
		case schema.TypeDynamicZone:
			for _, c := range a.Components {
				for _, nested := range s.nestedPopulateOfNonLocalized(c) {
					out = append(out, name+"."+nested)
				}
			}
		}
	}
	return out
}

func plainValue(v any) any {
	switch x := v.(type) {
	case query.Row:
		if x == nil {
			return nil
		}
		out := map[string]any{}
		for k, e := range x.All() {
			out[k] = plainValue(e)
		}
		return out
	case []query.Row:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plainValue(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plainValue(e)
		}
		return out
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return v
}

func (s *Service) removeIDs(uid string, entry any) {
	e, ok := entry.(map[string]any)
	if !ok {
		return
	}
	delete(e, "id")
	m := s.model(uid)
	if m == nil {
		return
	}
	for name, a := range m.attrs.All() {
		switch a.Type {
		case schema.TypeDynamicZone:
			list, _ := e[name].([]any)
			for _, c := range list {
				cm, _ := c.(map[string]any)
				if comp, ok := cm["__component"].(string); ok {
					s.removeIDs(comp, cm)
				}
			}
		case schema.TypeComponent:
			if list, ok := e[name].([]any); ok {
				for _, c := range list {
					s.removeIDs(a.Component, c)
				}
			} else {
				s.removeIDs(a.Component, e[name])
			}
		}
	}
}

func (s *Service) normalizeMediaIDs(uid string, data map[string]any) {
	m := s.model(uid)
	if m == nil || data == nil {
		return
	}
	idOf := func(v any) any {
		if o, ok := v.(map[string]any); ok {
			if id, has := o["id"]; has {
				return id
			}
		}
		return v
	}
	for name, a := range m.attrs.All() {
		value := data[name]
		if value == nil {
			continue
		}
		switch a.Type {
		case schema.TypeMedia:
			if list, ok := value.([]any); ok && a.IsMultiple() {
				for i, f := range list {
					list[i] = idOf(f)
				}
			} else if !a.IsMultiple() {
				data[name] = idOf(value)
			}
		case schema.TypeComponent:
			if list, ok := value.([]any); ok && a.IsRepeatable() {
				for _, c := range list {
					cm, _ := c.(map[string]any)
					s.normalizeMediaIDs(a.Component, cm)
				}
			} else if cm, ok := value.(map[string]any); ok {
				s.normalizeMediaIDs(a.Component, cm)
			}
		case schema.TypeDynamicZone:
			list, _ := value.([]any)
			for _, c := range list {
				cm, _ := c.(map[string]any)
				if comp, ok := cm["__component"].(string); ok {
					s.normalizeMediaIDs(comp, cm)
				}
			}
		}
	}
}

// copyNonLocalizedFields fills the nil non localized fields of a new locale
// from another entry of the same document, the published one first.
func (s *Service) copyNonLocalizedFields(ctx context.Context, ct *schema.ContentType, documentID string, data map[string]any) (map[string]any, error) {
	if !ct.Localized() {
		return data, nil
	}
	uid := string(ct.UID)
	paths := s.nestedPopulateOfNonLocalized(uid)
	populate := make([]any, len(paths))
	for i, p := range paths {
		populate[i] = p
	}
	existing, err := s.E.FindOne(ctx, uid, query.Params{
		Where:    map[string]any{schema.AttrDocumentID: documentID},
		OrderBy:  map[string]any{schema.AttrPublishedAt: "desc"},
		Populate: populate,
	})
	if err != nil || existing == nil {
		return data, err
	}
	row, _ := plainValue(existing).(map[string]any)
	m := s.model(uid)
	picked := map[string]any{}
	for _, name := range nonLocalizedAttributes(m) {
		if v, ok := row[name]; ok {
			picked[name] = deepCopy(v)
		}
	}
	s.removeIDs(uid, picked)
	merged := map[string]any{}
	for k, v := range data {
		merged[k] = v
	}
	for field, v := range picked {
		if merged[field] == nil {
			merged[field] = v
		}
	}
	s.normalizeMediaIDs(uid, merged)
	return merged, nil
}

func (s *Service) copyNonLocalizedAttributes(uid string, entry any) map[string]any {
	row, _ := plainValue(entry).(map[string]any)
	picked := map[string]any{}
	if row == nil {
		return picked
	}
	for _, name := range nonLocalizedAttributes(s.model(uid)) {
		if v, ok := row[name]; ok {
			picked[name] = deepCopy(v)
		}
	}
	s.removeIDs(uid, picked)
	return picked
}

func (s *Service) nonLocalizedPopulate(uid string) []any {
	paths := s.nestedPopulateOfNonLocalized(uid)
	out := make([]any, len(paths))
	for i, p := range paths {
		out[i] = p
	}
	return out
}

// syncLocales is the i18n document middleware on create, update,
// discardDraft and publish: when the write changed a non localized field,
// the other locales of the document in the same status get it too.
func (s *Service) syncLocales(ctx context.Context, ct *schema.ContentType, documentID string, write func() (any, error)) error {
	if !ct.Localized() {
		_, err := write()
		return err
	}
	uid := string(ct.UID)
	populate := s.nonLocalizedPopulate(uid)
	var original query.Row
	if documentID != "" {
		var err error
		original, err = s.E.FindOne(ctx, uid, query.Params{Where: map[string]any{schema.AttrDocumentID: documentID}, Populate: populate})
		if err != nil {
			return err
		}
	}
	id, err := write()
	if err != nil || id == nil {
		return err
	}
	current, err := s.E.FindOne(ctx, uid, query.Params{Where: map[string]any{"id": id}, Populate: populate})
	if err != nil || current == nil {
		return err
	}
	if original != nil && reflect.DeepEqual(s.copyNonLocalizedAttributes(uid, original), s.copyNonLocalizedAttributes(uid, current)) {
		return nil
	}
	return s.syncNonLocalizedAttributes(ctx, ct, current)
}

func (s *Service) syncNonLocalizedAttributes(ctx context.Context, ct *schema.ContentType, source query.Row) error {
	uid := string(ct.UID)
	fields := s.copyNonLocalizedAttributes(uid, source)
	if len(fields) == 0 {
		return nil
	}
	s.normalizeMediaIDs(uid, fields)
	documentID, _ := source.Get(schema.AttrDocumentID)
	locale, _ := source.Get(schema.AttrLocale)
	published, _ := source.Get(schema.AttrPublishedAt)
	status := "draft"
	var publishedWhere any = map[string]any{"$null": true}
	if published != nil {
		status = "published"
		publishedWhere = map[string]any{"$ne": nil}
	}
	entries, err := s.E.FindMany(ctx, uid, query.Params{
		Where:  map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: publishedWhere, schema.AttrLocale: map[string]any{"$ne": locale}},
		Select: []string{schema.AttrLocale, "id"},
	})
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryLocale, _ := entry.Get(schema.AttrLocale)
		entryID, _ := entry.Get("id")
		transformed, err := s.transformData(ctx, deepCopy(fields).(map[string]any), relSource{uid: uid, status: status, locale: entryLocale, allowMissing: true})
		if err != nil {
			return err
		}
		componentData, err := s.comp.Update(ctx, uid, entryID, transformed)
		if err != nil {
			return err
		}
		data, err := s.comp.assign(uid, componentData, deepCopy(fields).(map[string]any))
		if err != nil {
			return err
		}
		where := map[string]any{schema.AttrDocumentID: documentID, schema.AttrPublishedAt: publishedWhere, schema.AttrLocale: map[string]any{"$eq": entryLocale}}
		if _, err := s.E.Update(ctx, uid, where, data, query.Params{}); err != nil {
			return err
		}
	}
	return nil
}
