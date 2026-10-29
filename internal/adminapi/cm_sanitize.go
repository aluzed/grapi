package adminapi

import (
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/schema"
)

// adminUserFields are the admin::user fields a relation exposes, as
// Strapi's ADMIN_USER_ALLOWED_FIELDS.
var adminUserFields = map[string]bool{"id": true, schema.AttrDocumentID: true, "firstname": true, "lastname": true, "username": true, "email": true, "isActive": true}

// queryStatic are queryable whatever the field permissions, as in Strapi's
// getQueryFields.
var queryStatic = map[string]bool{
	schema.AttrCreatedAt: true, schema.AttrUpdatedAt: true, schema.AttrPublishedAt: true,
	schema.AttrCreatedBy: true, schema.AttrUpdatedBy: true, schema.AttrLocale: true,
}

// fieldScope is where a key is read: a content type, or one of its
// components (prefix is then the path of the component from the type).
type fieldScope struct {
	uid    string
	attrs  *schema.OrderedMap[*schema.Attribute]
	prefix string
	// rbac: the fields follow the user's Content Manager read permissions.
	rbac bool
}

func (s *Server) scopeOf(uid string) (fieldScope, bool) {
	ct, ok := s.set.ContentType(schema.UID(uid))
	if !ok {
		return fieldScope{}, false
	}
	rbac := uid != fileUID && contains(s.ContentManagerTypes(), uid)
	return fieldScope{uid: uid, attrs: &ct.Attributes, rbac: rbac}, true
}

func (s *Server) cmScope(ct *schema.ContentType) fieldScope {
	return fieldScope{uid: string(ct.UID), attrs: &ct.Attributes, rbac: true}
}

// field reports whether the user may read key in sc (query: in a filter or
// a sort), with its attribute, nil for id and documentId.
func (s *Server) field(sc fieldScope, key string, a *permissions.Ability, query bool) (*schema.Attribute, bool) {
	if key == "id" || key == schema.AttrDocumentID {
		return nil, true
	}
	attr, ok := sc.attrs.Get(key)
	if !ok || attr.Type == schema.TypePassword {
		return nil, false
	}
	if sc.uid == userUID {
		return attr, adminUserFields[key]
	}
	if !sc.rbac {
		return attr, !attr.IsPrivate()
	}
	if (query && queryStatic[key]) || (!query && alwaysReadable[key]) {
		return attr, true
	}
	return attr, a.Can(actionRead, sc.uid, sc.prefix+key)
}

// inner is the scope the value of attr is read in, for a relation, a media
// or a component.
func (s *Server) inner(sc fieldScope, key string, attr *schema.Attribute) (fieldScope, bool) {
	switch attr.Type {
	case schema.TypeRelation:
		if attr.IsMorph() || attr.Target == "" {
			return fieldScope{}, false
		}
		return s.scopeOf(attr.Target)
	case schema.TypeMedia:
		return s.scopeOf(fileUID)
	case schema.TypeComponent:
		c, ok := s.set.Component(schema.UID(attr.Component))
		if !ok {
			return fieldScope{}, false
		}
		return fieldScope{uid: sc.uid, attrs: &c.Attributes, prefix: sc.prefix + key + ".", rbac: sc.rbac}, true
	}
	return fieldScope{}, false
}

// visible keeps the fields of row the user may read, down through its
// relations, media and components.
func (s *Server) visible(row map[string]any, ct *schema.ContentType, a *permissions.Ability) map[string]any {
	return s.sanitizeRow(row, s.cmScope(ct), a)
}

func (s *Server) sanitizeRow(row map[string]any, sc fieldScope, a *permissions.Ability) map[string]any {
	for k, v := range row {
		if k == schema.AttrComponentKey {
			continue
		}
		attr, ok := s.field(sc, k, a, false)
		if !ok {
			delete(row, k)
			continue
		}
		if attr != nil && v != nil {
			row[k] = s.sanitizeValue(v, sc, k, attr, a)
		}
	}
	return row
}

func (s *Server) sanitizeValue(v any, sc fieldScope, key string, attr *schema.Attribute, a *permissions.Ability) any {
	each := func(f func(m map[string]any) map[string]any) any {
		switch x := v.(type) {
		case map[string]any:
			return f(x)
		case []any:
			for i, e := range x {
				if m, ok := e.(map[string]any); ok {
					x[i] = f(m)
				}
			}
			return x
		}
		return v
	}
	switch attr.Type {
	case schema.TypeDynamicZone:
		return each(func(m map[string]any) map[string]any {
			uid, _ := m[schema.AttrComponentKey].(string)
			c, ok := s.set.Component(schema.UID(uid))
			if !ok {
				return map[string]any{schema.AttrComponentKey: uid}
			}
			return s.sanitizeRow(m, fieldScope{uid: sc.uid, attrs: &c.Attributes, prefix: sc.prefix + key + ".", rbac: sc.rbac}, a)
		})
	case schema.TypeRelation:
		if attr.IsMorph() {
			return each(func(m map[string]any) map[string]any {
				uid, _ := m["__type"].(string)
				if in, ok := s.scopeOf(uid); ok {
					return s.sanitizeRow(m, in, a)
				}
				return idsOnly(m)
			})
		}
	}
	in, ok := s.inner(sc, key, attr)
	if !ok {
		if attr.Type == schema.TypeRelation {
			return each(idsOnly)
		}
		return v
	}
	return each(func(m map[string]any) map[string]any { return s.sanitizeRow(m, in, a) })
}

func idsOnly(m map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"id", schema.AttrDocumentID, "__type"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// sanitizeFilters drops what the user may not query, as Strapi's
// sanitizedQuery.read: unreadable and password fields, at every depth.
func (s *Server) sanitizeFilters(v any, sc fieldScope, a *permissions.Ability) any {
	switch x := v.(type) {
	case []any:
		var kept []any
		for _, e := range x {
			if f := s.sanitizeFilters(e, sc, a); f != nil {
				kept = append(kept, f)
			}
		}
		if len(kept) == 0 {
			return nil
		}
		return kept
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			switch {
			case k == "$and" || k == "$or" || k == "$not":
				if f := s.sanitizeFilters(val, sc, a); f != nil {
					out[k] = f
				}
			case strings.HasPrefix(k, "$"):
				out[k] = val
			default:
				attr, ok := s.field(sc, k, a, true)
				if !ok {
					continue
				}
				if attr == nil {
					out[k] = val
					continue
				}
				if attr.Type == schema.TypeDynamicZone || attr.IsMorph() {
					continue
				}
				in, ok := s.inner(sc, k, attr)
				if !ok {
					out[k] = val
					continue
				}
				if f := s.sanitizeFilters(val, in, a); f != nil {
					out[k] = f
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return v
}

// sanitizeSort keeps the parts of a "path:order,..." sort the user may
// query.
func (s *Server) sanitizeSort(sort string, sc fieldScope, a *permissions.Ability) string {
	var kept []string
	for _, part := range strings.Split(sort, ",") {
		path, _, _ := strings.Cut(strings.TrimSpace(part), ":")
		if path != "" && s.sortable(strings.Split(path, "."), sc, a) {
			kept = append(kept, strings.TrimSpace(part))
		}
	}
	return strings.Join(kept, ",")
}

func (s *Server) sortable(path []string, sc fieldScope, a *permissions.Ability) bool {
	attr, ok := s.field(sc, path[0], a, true)
	if !ok {
		return false
	}
	if len(path) == 1 {
		return attr == nil || attr.IsScalar()
	}
	if attr == nil || attr.Type == schema.TypeDynamicZone || attr.IsMorph() {
		return false
	}
	in, ok := s.inner(sc, path[0], attr)
	return ok && s.sortable(path[1:], in, a)
}
