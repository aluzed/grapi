package document

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// Relation values of a write name their targets by documentId; the
// database needs entry ids, one per locale and status. This is Strapi's
// transformData: extract the documentIds, load them in batches, rewrite.

type relSource struct {
	uid          string
	locale       any
	status       string
	allowMissing bool
}

type idKey struct {
	uid, documentID, locale, status string
}

type idMap struct {
	s      *Service
	loaded map[idKey]any
	toLoad map[idKey]bool
}

func (s *Service) newIDMap() *idMap {
	return &idMap{s: s, loaded: map[idKey]any{}, toLoad: map[idKey]bool{}}
}

func (s *Service) hasDP(uid string) bool {
	ct, ok := s.Set.ContentType(schema.UID(uid))
	return ok && ct.DraftAndPublish()
}

func (s *Service) isLocalized(uid string) bool {
	ct, ok := s.Set.ContentType(schema.UID(uid))
	return ok && ct.Localized()
}

func localeString(v any) string {
	if v == nil {
		return "null"
	}
	return fmt.Sprint(v)
}

func (m *idMap) key(uid, documentID string, locale any, status string) idKey {
	if !m.s.hasDP(uid) {
		status = ""
	}
	return idKey{uid, documentID, localeString(locale), status}
}

func (m *idMap) add(uid, documentID string, locale any, status string) {
	k := m.key(uid, documentID, locale, status)
	if _, ok := m.loaded[k]; ok {
		return
	}
	m.toLoad[k] = true
}

func (m *idMap) load(ctx context.Context) error {
	type group struct {
		uid, locale, status string
		docs                []any
	}
	groups := map[string]*group{}
	var order []string
	for k := range m.toLoad {
		gk := k.uid + "\x00" + k.locale + "\x00" + k.status
		g, ok := groups[gk]
		if !ok {
			g = &group{uid: k.uid, locale: k.locale, status: k.status}
			groups[gk] = g
			order = append(order, gk)
		}
		g.docs = append(g.docs, k.documentID)
	}
	sort.Strings(order)
	for _, gk := range order {
		g := groups[gk]
		where := map[string]any{schema.AttrDocumentID: map[string]any{"$in": g.docs}}
		sel := []string{"id", schema.AttrDocumentID, schema.AttrPublishedAt}
		if m.s.isLocalized(g.uid) {
			sel = append(sel, schema.AttrLocale)
			if g.locale == "null" {
				where[schema.AttrLocale] = nil
			} else {
				where[schema.AttrLocale] = g.locale
			}
		}
		if m.s.hasDP(g.uid) {
			if g.status == "draft" {
				where[schema.AttrPublishedAt] = nil
			} else {
				where[schema.AttrPublishedAt] = map[string]any{"$ne": nil}
			}
		}
		rows, err := m.s.E.FindMany(ctx, g.uid, query.Params{Select: sel, Where: where})
		if err != nil {
			return err
		}
		for _, r := range rows {
			doc, _ := r.Get(schema.AttrDocumentID)
			id, _ := r.Get("id")
			pub, _ := r.Get(schema.AttrPublishedAt)
			var loc any
			if m.s.isLocalized(g.uid) {
				loc, _ = r.Get(schema.AttrLocale)
			}
			status := "draft"
			if pub != nil {
				status = "published"
			}
			m.loaded[m.key(g.uid, fmt.Sprint(doc), loc, status)] = id
		}
	}
	m.toLoad = map[idKey]bool{}
	return nil
}

func (m *idMap) get(uid, documentID string, locale any, status string) (any, bool) {
	v, ok := m.loaded[m.key(uid, documentID, locale, status)]
	return v, ok
}

// targetLocale is Strapi's getRelationTargetLocale.
func (s *Service) targetLocale(rel map[string]any, targetUID string, src relSource) any {
	target := rel["locale"]
	if target == nil || target == "" {
		target = src.locale
	}
	if s.isLocalized(src.uid) && s.isLocalized(targetUID) {
		return src.locale
	}
	if s.isLocalized(targetUID) {
		return target
	}
	return nil
}

// targetStatuses is Strapi's getRelationTargetStatus.
func (s *Service) targetStatuses(rel map[string]any, targetUID string, src relSource) []string {
	if !s.hasDP(targetUID) {
		return []string{"published"}
	}
	if s.hasDP(src.uid) && src.status != "" {
		return []string{src.status}
	}
	if st, ok := rel["status"].(string); ok && st != "" {
		if st == "published" {
			return []string{"published"}
		}
		return []string{"draft"}
	}
	if !s.hasDP(src.uid) {
		return []string{"draft", "published"}
	}
	return []string{"draft"}
}

var numericID = regexp.MustCompile(`^\d+$`)

// mapRelation is Strapi's mapRelation: every way of writing a relation is
// brought to {set} or {connect, disconnect}, each entry passed to fn.
func mapRelation(value any, fn func(rel map[string]any) ([]any, error), recursive bool) (any, error) {
	wrap := func(v []any) any {
		if recursive {
			return v
		}
		return map[string]any{"set": v}
	}
	switch x := value.(type) {
	case nil:
		return nil, nil
	case []any:
		var out []any
		for _, r := range x {
			sub, err := mapRelation(r, fn, true)
			if err != nil {
				return nil, err
			}
			if list, ok := sub.([]any); ok {
				for _, e := range list {
					if e != nil {
						out = append(out, e)
					}
				}
			} else if sub != nil {
				out = append(out, sub)
			}
		}
		if out == nil {
			out = []any{}
		}
		return wrap(out), nil
	case map[string]any:
		_, hasID := x["id"]
		_, hasDoc := x["documentId"]
		if hasID || hasDoc {
			res, err := fn(x)
			if err != nil {
				return nil, err
			}
			return wrap(res), nil
		}
		set, hasSet := x["set"]
		disc, hasDisc := x["disconnect"]
		conn, hasConn := x["connect"]
		if !truthy(set) && !truthy(disc) && !truthy(conn) {
			return x, nil
		}
		out := map[string]any{}
		for k, v := range x {
			out[k] = v
		}
		for _, part := range []struct {
			name string
			v    any
			has  bool
		}{{"set", set, hasSet}, {"disconnect", disc, hasDisc}, {"connect", conn, hasConn}} {
			if !part.has || !truthy(part.v) {
				continue
			}
			sub, err := mapRelation(part.v, fn, true)
			if err != nil {
				return nil, err
			}
			out[part.name] = toArray(sub)
		}
		return out, nil
	case float64:
		res, err := fn(map[string]any{"id": x})
		if err != nil {
			return nil, err
		}
		return wrap(res), nil
	case string:
		if numericID.MatchString(x) {
			res, err := fn(map[string]any{"id": x})
			if err != nil {
				return nil, err
			}
			return wrap(res), nil
		}
		res, err := fn(map[string]any{"documentId": x})
		if err != nil {
			return nil, err
		}
		return wrap(res), nil
	}
	return value, nil
}

func toArray(v any) []any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	}
	return []any{v}
}

// normalizeXToOne keeps the last entry of a "relates to one" payload.
func normalizeXToOne(a *schema.Attribute, v any) any {
	if a.Relation != schema.OneToOne && a.Relation != schema.ManyToOne {
		return v
	}
	switch x := v.(type) {
	case []any:
		if len(x) > 1 {
			return []any{x[len(x)-1]}
		}
	case map[string]any:
		if set, ok := x["set"].([]any); ok && len(set) > 1 {
			out := map[string]any{}
			for k, val := range x {
				out[k] = val
			}
			out["set"] = []any{set[len(set)-1]}
			return out
		}
	}
	return v
}

func isPolymorphic(a *schema.Attribute) bool {
	switch a.Relation {
	case schema.MorphOne, schema.MorphMany, schema.MorphToOne, schema.MorphToMany:
		return true
	}
	return false
}

// walkRelations visits the relation and media values of data, components
// and dynamic zones included, and replaces them with fn's result.
func (s *Service) walkRelations(uid string, data map[string]any, fn func(a *schema.Attribute, v any) (any, error)) error {
	m := s.model(uid)
	if m == nil || data == nil {
		return nil
	}
	for name, a := range m.attrs.All() {
		v, ok := data[name]
		if !ok {
			continue
		}
		switch a.Type {
		case schema.TypeRelation:
			if a.UseJoinTable != nil && !*a.UseJoinTable || a.Relation == schema.MorphToOne {
				continue
			}
			nv, err := fn(a, v)
			if err != nil {
				return err
			}
			data[name] = nv
		case schema.TypeMedia:
			nv, err := fn(a, v)
			if err != nil {
				return err
			}
			data[name] = nv
		case schema.TypeComponent:
			for _, item := range toArray(v) {
				if im, ok := item.(map[string]any); ok {
					if err := s.walkRelations(a.Component, im, fn); err != nil {
						return err
					}
				}
			}
		case schema.TypeDynamicZone:
			for _, item := range toArray(v) {
				if im, ok := item.(map[string]any); ok {
					comp, _ := im["__component"].(string)
					if err := s.walkRelations(comp, im, fn); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func targetUID(a *schema.Attribute, rel map[string]any) string {
	switch {
	case a.Type == schema.TypeMedia:
		return "plugin::upload.file"
	case isPolymorphic(a):
		s, _ := rel["__type"].(string)
		return s
	}
	return a.Target
}

// transformData rewrites the documentIds of data's relations to entry ids.
func (s *Service) transformData(ctx context.Context, data map[string]any, src relSource) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}
	data = deepCopy(data).(map[string]any)
	if !s.isLocalized(src.uid) {
		var def any
		var loaded bool
		err := s.walkRelations(src.uid, data, func(_ *schema.Attribute, v any) (any, error) {
			return mapRelation(v, func(rel map[string]any) ([]any, error) {
				if _, ok := rel["documentId"]; !ok || rel["locale"] != nil {
					return []any{rel}, nil
				}
				if !loaded {
					def = s.defaultLocale(ctx)
					loaded = true
				}
				out := map[string]any{}
				for k, val := range rel {
					out[k] = val
				}
				out["locale"] = def
				if p, ok := out["position"].(map[string]any); ok && p["locale"] == nil {
					p["locale"] = def
				}
				return []any{out}, nil
			}, false)
		})
		if err != nil {
			return nil, err
		}
	}
	ids := s.newIDMap()
	addDoc := func(target string, rel map[string]any) {
		loc := s.targetLocale(rel, target, src)
		for _, st := range s.targetStatuses(rel, target, src) {
			ids.add(target, fmt.Sprint(rel["documentId"]), loc, st)
		}
	}
	err := s.walkRelations(src.uid, data, func(a *schema.Attribute, v any) (any, error) {
		_, err := mapRelation(normalizeXToOne(a, v), func(rel map[string]any) ([]any, error) {
			if rel["documentId"] == nil {
				return []any{rel}, nil
			}
			target := targetUID(a, rel)
			addDoc(target, rel)
			if p, ok := rel["position"].(map[string]any); ok {
				pt := target
				if t, ok := p["__type"].(string); ok && isPolymorphic(a) {
					pt = t
				}
				for _, k := range []string{"before", "after"} {
					if p[k] != nil {
						r := merge(rel, p)
						r["documentId"] = p[k]
						addDoc(pt, r)
					}
				}
			}
			return []any{rel}, nil
		}, false)
		return v, err
	})
	if err != nil {
		return nil, err
	}
	if err := ids.load(ctx); err != nil {
		return nil, err
	}
	getIDs := func(target string, rel map[string]any) ([]any, error) {
		loc := s.targetLocale(rel, target, src)
		var out []any
		for _, st := range s.targetStatuses(rel, target, src) {
			if id, ok := ids.get(target, fmt.Sprint(rel["documentId"]), loc, st); ok {
				out = append(out, id)
			}
		}
		if len(out) == 0 && !src.allowMissing {
			return nil, validation("Document with id \"%s\", locale \"%s\" not found", fmt.Sprint(rel["documentId"]), localeString(loc))
		}
		return out, nil
	}
	err = s.walkRelations(src.uid, data, func(a *schema.Attribute, v any) (any, error) {
		return mapRelation(normalizeXToOne(a, v), func(rel map[string]any) ([]any, error) {
			if rel["documentId"] == nil {
				return []any{rel}, nil
			}
			target := targetUID(a, rel)
			found, err := getIDs(target, rel)
			if err != nil {
				return nil, err
			}
			var position map[string]any
			if p, ok := rel["position"].(map[string]any); ok {
				position = merge(p, nil)
				pt := target
				if t, ok := p["__type"].(string); ok && isPolymorphic(a) {
					pt = t
				}
				for _, k := range []string{"before", "after"} {
					if p[k] == nil {
						continue
					}
					r := merge(rel, p)
					r["documentId"] = p[k]
					pos, err := getIDs(pt, r)
					if err != nil {
						return nil, err
					}
					if len(pos) > 0 {
						position[k] = pos[0]
					} else {
						delete(position, k)
					}
				}
			}
			out := make([]any, 0, len(found))
			for _, id := range found {
				nr := map[string]any{"id": id}
				if _, ok := rel["position"]; ok {
					nr["position"] = position
				}
				if isPolymorphic(a) {
					nr["__type"] = target
				}
				out = append(out, nr)
			}
			return out, nil
		}, false)
	})
	return data, err
}

func merge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// DefaultLocale is the i18n default locale code from the core store.
func (s *Service) DefaultLocale(ctx context.Context) any { return s.defaultLocale(ctx) }

// defaultLocale reads i18n's default locale from the core store.
func (s *Service) defaultLocale(ctx context.Context) any {
	var raw string
	err := s.E.DB.QueryRow(ctx, `select "value" from "strapi_core_store_settings" where "key" = 'plugin_i18n_default_locale'`).Scan(&raw)
	if err != nil {
		return "en"
	}
	return strings.Trim(raw, `"`)
}
