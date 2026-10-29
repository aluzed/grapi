package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// Reference is a relation or a medium a version points to.
type Reference struct {
	Path       string `json:"path"`
	Target     string `json:"target"`
	DocumentID string `json:"documentId,omitempty"`
	FileID     any    `json:"fileId,omitempty"`
}

func (s *Server) exists(ctx context.Context, uid string, where map[string]any) bool {
	row, err := s.e.FindOne(ctx, uid, query.Params{Where: where, Select: []string{"id"}})
	return err == nil && row != nil
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "." + name
}

// prune removes from data, in place and down the components, the relations
// and media that no longer exist, and returns them.
func (s *Server) prune(ctx context.Context, data map[string]any, attrs *schema.OrderedMap[*schema.Attribute], path string) []Reference {
	var out []Reference
	for name, a := range attrs.All() {
		v, ok := data[name]
		if !ok || v == nil {
			continue
		}
		p := joinPath(path, name)
		switch a.Type {
		case schema.TypeRelation:
			if a.Target == "" {
				continue
			}
			kept := []any{}
			for _, x := range listOf(v) {
				m, _ := x.(map[string]any)
				id, _ := m[schema.AttrDocumentID].(string)
				if id != "" && !s.exists(ctx, a.Target, map[string]any{schema.AttrDocumentID: id}) {
					out = append(out, Reference{Path: p, Target: a.Target, DocumentID: id})
					continue
				}
				kept = append(kept, x)
			}
			data[name] = keepShape(v, kept)
		case schema.TypeMedia:
			kept := []any{}
			for _, x := range listOf(v) {
				m, _ := x.(map[string]any)
				if id := m["id"]; id != nil && !s.exists(ctx, fileUID, map[string]any{"id": id}) {
					out = append(out, Reference{Path: p, Target: fileUID, FileID: id})
					continue
				}
				kept = append(kept, x)
			}
			data[name] = keepShape(v, kept)
		case schema.TypeComponent:
			c, ok := s.set.Component(schema.UID(a.Component))
			if !ok {
				continue
			}
			for i, x := range listOf(v) {
				if m, ok := x.(map[string]any); ok {
					cp := p
					if a.IsRepeatable() {
						cp += "." + strconv.Itoa(i)
					}
					out = append(out, s.prune(ctx, m, &c.Attributes, cp)...)
				}
			}
		case schema.TypeDynamicZone:
			for i, x := range listOf(v) {
				m, _ := x.(map[string]any)
				cu, _ := m["__component"].(string)
				if c, ok := s.set.Component(schema.UID(cu)); ok {
					out = append(out, s.prune(ctx, m, &c.Attributes, p+"."+strconv.Itoa(i))...)
				}
			}
		}
	}
	return out
}

// keepShape gives back a single relation or medium as a value, a list as a
// list.
func keepShape(orig any, kept []any) any {
	if _, ok := orig.([]any); ok {
		return kept
	}
	if len(kept) == 0 {
		return nil
	}
	return kept[0]
}

// FieldChange is one field that differs between two versions.
type FieldChange struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// comparedValue reduces a value to what the comparison looks at: relations
// to the sorted set of their documentIds, media to their ids.
func comparedValue(v any, a *schema.Attribute) any {
	switch a.Type {
	case schema.TypeRelation:
		ids := []string{}
		for _, x := range listOf(v) {
			if m, ok := x.(map[string]any); ok {
				if id, ok := m[schema.AttrDocumentID].(string); ok {
					ids = append(ids, id)
				}
			}
		}
		sort.Strings(ids)
		return ids
	case schema.TypeMedia:
		ids := []any{}
		for _, x := range listOf(v) {
			if m, ok := x.(map[string]any); ok {
				ids = append(ids, m["id"])
			}
		}
		return ids
	}
	return v
}

func normalize(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// diff compares two data maps field by field: components and dynamic zone
// items by position, the rest as values.
func (s *Server) diff(before, after map[string]any, attrs *schema.OrderedMap[*schema.Attribute], path string) []FieldChange {
	var out []FieldChange
	for name, a := range attrs.All() {
		if !editable(name, a) || a.Type == schema.TypePassword {
			continue
		}
		p := joinPath(path, name)
		b, aft := before[name], after[name]
		switch a.Type {
		case schema.TypeComponent, schema.TypeDynamicZone:
			bl, al := listOf(b), listOf(aft)
			if a.Type == schema.TypeComponent && !a.IsRepeatable() {
				out = append(out, s.diffItem(b, aft, a, "", p)...)
				continue
			}
			for i := 0; i < len(bl) || i < len(al); i++ {
				var bi, ai any
				if i < len(bl) {
					bi = bl[i]
				}
				if i < len(al) {
					ai = al[i]
				}
				out = append(out, s.diffItem(bi, ai, a, strconv.Itoa(i), p)...)
			}
		default:
			cb, ca := normalize(comparedValue(b, a)), normalize(comparedValue(aft, a))
			if !reflect.DeepEqual(cb, ca) {
				out = append(out, FieldChange{Path: p, Before: b, After: aft})
			}
		}
	}
	return out
}

func (s *Server) diffItem(b, a any, attr *schema.Attribute, index, path string) []FieldChange {
	p := path
	if index != "" {
		p += "." + index
	}
	bm, _ := b.(map[string]any)
	am, _ := a.(map[string]any)
	if bm == nil || am == nil {
		if bm == nil && am == nil {
			return nil
		}
		return []FieldChange{{Path: p, Before: b, After: a}}
	}
	cu := attr.Component
	if attr.Type == schema.TypeDynamicZone {
		bc, _ := bm["__component"].(string)
		ac, _ := am["__component"].(string)
		if bc != ac {
			return []FieldChange{{Path: p, Before: b, After: a}}
		}
		cu = bc
	}
	c, ok := s.set.Component(schema.UID(cu))
	if !ok {
		return nil
	}
	return s.diff(bm, am, &c.Attributes, p)
}

// diffVersion compares a version with the current draft (against=current,
// the default) or with another version of the same document.
func (s *Server) diffVersion(w http.ResponseWriter, r *http.Request, u *User) error {
	id, err := versionID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	v, _, err := s.loadVersion(ctx, id)
	if err != nil {
		return err
	}
	ct, err := s.versionAccess(r, u, v.UID, actionRead)
	if err != nil {
		return err
	}
	a, err := s.Ability(ctx, u)
	if err != nil {
		return err
	}
	var other map[string]any
	against := r.URL.Query().Get("against")
	if against == "" || against == "current" {
		var locale any
		if v.Locale != nil {
			locale = *v.Locale
		}
		if other, err = s.snapshot(ctx, ct, v.DocumentID, locale, false); err != nil {
			return err
		}
		if other == nil {
			return errNotFound
		}
	} else {
		oid, err := strconv.ParseInt(against, 10, 64)
		if err != nil {
			return errNotFound
		}
		o, _, err := s.loadVersion(ctx, oid)
		if err != nil {
			return err
		}
		if o.UID != v.UID || o.DocumentID != v.DocumentID {
			return errNotFound
		}
		other = o.Data
	}
	changes := s.diff(s.visible(v.Data, ct, a), s.visible(other, ct, a), &ct.Attributes, "")
	if changes == nil {
		changes = []FieldChange{}
	}
	return data(w, map[string]any{"against": against, "changes": changes})
}
