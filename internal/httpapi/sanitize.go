package httpapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

// Content API validation (throws) and sanitization (removes) of queries,
// inputs and outputs, by schema and permissions: Strapi's contentAPI
// validate and sanitize.

type schemaModel struct {
	uid           string
	attrs         *schema.OrderedMap[*schema.Attribute]
	isContentType bool
	ct            *schema.ContentType
}

func (s *Server) schemaOf(uid string) *schemaModel {
	if ct, ok := s.set.ContentType(schema.UID(uid)); ok {
		return &schemaModel{uid: uid, attrs: &ct.Attributes, isContentType: true, ct: ct}
	}
	if c, ok := s.set.Component(schema.UID(uid)); ok {
		return &schemaModel{uid: uid, attrs: &c.Attributes}
	}
	return nil
}

func (m *schemaModel) attr(key string) *schema.Attribute {
	if m == nil {
		return nil
	}
	a, _ := m.attrs.Get(key)
	return a
}

func isPrivate(a *schema.Attribute) bool { return a != nil && a.Private != nil && *a.Private }

func isScalar(a *schema.Attribute) bool {
	if a == nil {
		return false
	}
	switch a.Type {
	case schema.TypeMedia, schema.TypeComponent, schema.TypeRelation, schema.TypeDynamicZone:
		return false
	}
	return true
}

func isMorphTo(a *schema.Attribute) bool {
	return a != nil && a.Type == schema.TypeRelation && (a.Relation == schema.MorphToOne || a.Relation == schema.MorphToMany)
}

func (s *Server) target(a *schema.Attribute) *schemaModel {
	switch a.Type {
	case schema.TypeRelation:
		return s.schemaOf(a.Target)
	case schema.TypeComponent:
		return s.schemaOf(a.Component)
	case schema.TypeMedia:
		return s.schemaOf("plugin::upload.file")
	}
	return nil
}

func invalidKey(key, path, param string) *APIError {
	msg := "Invalid key " + key
	if path != "" && path != key {
		msg += " at " + path
	}
	details := map[string]any{"key": key, "path": nil, "source": "query", "param": param}
	if path != "" {
		details["path"] = path
	}
	return validationError(msg, details)
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// restricted reports whether a relation's target cannot be read with auth.
func (s *Server) restricted(m *schemaModel, key string, a *schema.Attribute, auth *Auth) bool {
	if auth == nil || a == nil || a.Type != schema.TypeRelation || isMorphTo(a) {
		return false
	}
	if (key == schema.AttrCreatedBy || key == schema.AttrUpdatedBy) && m.ct != nil && m.ct.PopulateCreatorFields() {
		return false
	}
	return !auth.Allowed(a.Target + ".find")
}

// validateFilters throws on keys that are not attributes or operators,
// dynamic zones, morphTo relations, passwords, private fields and
// relations auth cannot read.
func (s *Server) validateFilters(f any, m *schemaModel, path string, auth *Auth) error {
	switch x := f.(type) {
	case []any:
		for _, e := range x {
			if err := s.validateFilters(e, m, path, auth); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, key := range sortedKeys(x) {
			value := x[key]
			if value == nil || key == "id" || key == schema.AttrDocumentID {
				continue
			}
			a := m.attr(key)
			if a == nil {
				if !query.IsOperator(key) {
					return invalidKey(key, path, "filters")
				}
				if vm, ok := value.(map[string]any); ok && key != "$not" && !isFilterLike(vm, m) {
					continue
				}
				if err := s.validateFilters(value, m, path, auth); err != nil {
					return err
				}
				continue
			}
			p := joinPath(path, key)
			if a.Type == schema.TypeDynamicZone || isMorphTo(a) || a.Type == schema.TypePassword || isPrivate(a) {
				return invalidKey(key, p, "filters")
			}
			if auth != nil && s.restricted(m, key, a, auth) {
				return invalidKey(key, p, "filters")
			}
			if t := s.target(a); t != nil {
				if err := s.validateFilters(value, t, p, auth); err != nil {
					return err
				}
			} else if _, ok := value.(map[string]any); ok && isScalar(a) {
				if err := s.validateFilters(value, m, p, auth); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// isFilterLike tells a nested filter from an opaque operand ($null: {...}).
func isFilterLike(v map[string]any, m *schemaModel) bool {
	for k := range v {
		if query.IsOperator(k) || m.attr(k) != nil {
			return true
		}
	}
	return false
}

// sortEntries flattens Strapi's sort forms into attribute paths.
func (s *Server) validateSort(sort any, m *schemaModel, path string, auth *Auth) error {
	switch x := sort.(type) {
	case string:
		for _, seg := range strings.Split(x, ",") {
			field, _, _ := strings.Cut(strings.TrimSpace(seg), ":")
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			parts := strings.Split(field, ".")
			cur := m
			p := path
			for i, part := range parts {
				var nested any = "asc"
				if i < len(parts)-1 {
					nested = map[string]any{parts[i+1]: "asc"}
				}
				if err := s.validateSortKey(part, nested, cur, &p, auth); err != nil {
					return err
				}
				if a := cur.attr(part); a != nil {
					cur = s.target(a)
				}
				if cur == nil {
					break
				}
			}
		}
	case []any:
		for _, e := range x {
			if err := s.validateSort(e, m, path, auth); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, key := range sortedKeys(x) {
			p := path
			if err := s.validateSortKey(key, x[key], m, &p, auth); err != nil {
				return err
			}
			if a := m.attr(key); a != nil {
				if t := s.target(a); t != nil {
					if err := s.validateSort(x[key], t, p, auth); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (s *Server) validateSortKey(key string, value any, m *schemaModel, path *string, auth *Auth) error {
	if key == "id" || key == schema.AttrDocumentID {
		*path = joinPath(*path, key)
		return nil
	}
	if key == "status" && m.ct != nil && m.ct.DraftAndPublish() {
		return nil
	}
	a := m.attr(key)
	if a == nil {
		return invalidKey(key, *path, "sort")
	}
	*path = joinPath(*path, key)
	if a.Type == schema.TypeDynamicZone || isMorphTo(a) || a.Type == schema.TypePassword || isPrivate(a) {
		return invalidKey(key, *path, "sort")
	}
	if auth != nil && s.restricted(m, key, a, auth) {
		return invalidKey(key, *path, "sort")
	}
	if !isScalar(a) {
		if vm, ok := value.(map[string]any); ok && len(vm) == 0 || value == "" || value == nil {
			return invalidKey(key, *path, "sort")
		}
	}
	return nil
}

func (s *Server) validateFields(fields any, m *schemaModel, path string) error {
	var list []string
	switch x := fields.(type) {
	case string:
		for _, f := range strings.Split(x, ",") {
			list = append(list, strings.TrimSpace(f))
		}
	case []any:
		for _, e := range x {
			if str, ok := e.(string); ok {
				for _, f := range strings.Split(str, ",") {
					list = append(list, strings.TrimSpace(f))
				}
			}
		}
	}
	for _, f := range list {
		if f == "id" || f == schema.AttrDocumentID || f == "*" {
			continue
		}
		a := m.attr(f)
		if a == nil {
			return invalidKey(f, path, "fields")
		}
		if !isScalar(a) || isPrivate(a) || a.Type == schema.TypePassword {
			return invalidKey(f, joinPath(path, f), "fields")
		}
	}
	return nil
}

func isPopulatable(a *schema.Attribute) bool {
	if a == nil {
		return false
	}
	switch a.Type {
	case schema.TypeRelation, schema.TypeDynamicZone, schema.TypeComponent, schema.TypeMedia:
		return true
	}
	return false
}

func isBooleanLike(v any) bool {
	switch x := v.(type) {
	case bool:
		return true
	case string:
		switch x {
		case "true", "t", "1", "false", "f", "0":
			return true
		}
	case float64:
		return x == 0 || x == 1
	}
	return false
}

func (s *Server) validatePopulate(populate any, m *schemaModel, path string) error {
	switch x := populate.(type) {
	case string:
		if x == "*" {
			return nil
		}
		for _, p := range strings.Split(x, ",") {
			if err := s.validatePopulatePath(strings.Split(strings.TrimSpace(p), "."), m, path); err != nil {
				return err
			}
		}
		for _, p := range strings.Split(x, ",") {
			if err := s.privateInPopulatePath(strings.Split(strings.TrimSpace(p), "."), m, path); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range x {
			str, ok := e.(string)
			if !ok {
				return invalidKey("populate", path, "populate")
			}
			if err := s.validatePopulate(str, m, path); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, key := range sortedKeys(x) {
			value := x[key]
			a := m.attr(key)
			p := joinPath(path, key)
			if a == nil {
				if isBooleanLike(key) {
					continue
				}
				return invalidKey(key, path, "populate")
			}
			if !isPopulatable(a) {
				return invalidKey(key, p, "populate")
			}
			if isPrivate(a) {
				return invalidKey(key, p, "populate")
			}
			if a.Type == schema.TypeDynamicZone || isMorphTo(a) {
				if vm, ok := value.(map[string]any); ok {
					if pop, has := vm["populate"]; has && pop != nil && pop != "*" {
						return invalidKey("populate", p, "populate")
					}
					if on, ok := vm["on"]; ok {
						onMap, isMap := on.(map[string]any)
						if !isMap {
							return invalidKey("on", p+".on", "populate")
						}
						for _, t := range sortedKeys(onMap) {
							tm := s.schemaOf(t)
							if tm == nil {
								return invalidKey(t, p+".on."+t, "populate")
							}
							if err := s.validatePopulateOptions(onMap[t], tm, p); err != nil {
								return err
							}
						}
					}
				}
				continue
			}
			if err := s.validatePopulateOptions(value, s.target(a), p); err != nil {
				return err
			}
		}
	}
	return nil
}

var rootOnly = map[string]bool{"status": true, "publicationFilter": true, "hasPublishedVersion": true}

func (s *Server) validatePopulateOptions(value any, m *schemaModel, path string) error {
	vm, ok := value.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	for _, k := range sortedKeys(vm) {
		v := vm[k]
		var nested error
		switch k {
		case "sort":
			nested = s.validateSort(v, m, "", nil)
		case "filters":
			nested = s.validateFilters(v, m, "", nil)
		case "fields":
			nested = s.validateFields(v, m, "")
		}
		if api, ok := nested.(*APIError); ok && api != nil {
			if d, ok := api.Details.(map[string]any); ok {
				d["param"] = "populate"
			}
			return api
		}
		switch k {
		case "sort", "filters", "fields":
		case "populate":
			if err := s.validatePopulate(v, m, path); err != nil {
				return err
			}
		case "count":
			if !isBooleanLike(v) {
				return invalidKey(k, path, "populate")
			}
		case "on", "limit", "start", "page", "pageSize", "ordering", "_q", "locale":
		default:
			if isBooleanLike(k) {
				continue
			}
			api := invalidKey(k, path, "populate")
			if rootOnly[k] {
				api.Message += ": " + k + " is only accepted at the root of the query, and it also applies to populated relations"
			}
			return api
		}
	}
	return nil
}

// privateInPopulatePath is the throwPrivate pass over a dot notation path.
func (s *Server) privateInPopulatePath(segments []string, m *schemaModel, prefix string) error {
	for _, seg := range segments {
		if m == nil || seg == "" {
			return nil
		}
		a := m.attr(seg)
		if a == nil {
			return nil
		}
		prefix = joinPath(prefix, seg)
		if isPrivate(a) {
			return invalidKey(seg, prefix, "populate")
		}
		m = s.target(a)
	}
	return nil
}

func (s *Server) validatePopulatePath(segments []string, m *schemaModel, prefix string) error {
	if len(segments) == 0 || segments[0] == "" {
		return nil
	}
	head, tail := segments[0], segments[1:]
	full := joinPath(prefix, head)
	a := m.attr(head)
	if !isPopulatable(a) {
		return invalidKey(head, full, "populate")
	}
	if len(tail) == 0 {
		return nil
	}
	switch a.Type {
	case schema.TypeComponent:
		return s.validatePopulatePath(tail, s.schemaOf(a.Component), full)
	case schema.TypeRelation:
		if isMorphTo(a) {
			return invalidKey(head, full, "populate")
		}
		return s.validatePopulatePath(tail, s.schemaOf(a.Target), full)
	case schema.TypeMedia:
		return invalidKey(head, full, "populate")
	case schema.TypeDynamicZone:
		next := tail[0]
		for _, c := range a.Components {
			cm := s.schemaOf(c)
			if cm == nil || cm.attr(next) == nil {
				continue
			}
			if len(tail) == 1 {
				return nil
			}
			if s.validatePopulatePath(tail, cm, full) == nil {
				return nil
			}
		}
		return invalidKey(next, full+"."+next, "populate")
	}
	return nil
}

// validateQuery is contentAPI.validate.query.
func (s *Server) validateQuery(q map[string]any, m *schemaModel, auth *Auth) error {
	if v, ok := q["publicationFilter"]; ok {
		if _, err := document.ParsePublicationFilter(v); err != nil {
			return validationError(err.Error(), map[string]any{"source": "query", "param": "publicationFilter"})
		}
	}
	if f, ok := q["filters"]; ok && truthy(f) {
		if err := s.validateFilters(f, m, "", auth); err != nil {
			return err
		}
	}
	if sort, ok := q["sort"]; ok && truthy(sort) {
		if err := s.validateSort(sort, m, "", auth); err != nil {
			return err
		}
	}
	if f, ok := q["fields"]; ok && truthy(f) {
		if err := s.validateFields(f, m, ""); err != nil {
			return err
		}
	}
	if p, ok := q["populate"]; ok && truthy(p) && p != "*" {
		if err := s.validatePopulate(p, m, ""); err != nil {
			return err
		}
	}
	return nil
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case float64:
		return x != 0
	}
	return true
}

// sanitizePopulate expands "*" and removes relations auth cannot read.
func (s *Server) sanitizePopulate(populate any, m *schemaModel, auth *Auth) any {
	if m == nil {
		return populate
	}
	switch x := populate.(type) {
	case string:
		if x == "*" {
			out := map[string]any{}
			for name, a := range m.attrs.All() {
				if isPopulatable(a) {
					out[name] = true
				}
			}
			return s.sanitizePopulate(out, m, auth)
		}
		var kept []any
		for _, p := range strings.Split(x, ",") {
			p = strings.TrimSpace(p)
			if s.populatePathAllowed(strings.Split(p, "."), m, auth) {
				kept = append(kept, p)
			}
		}
		return kept
	case []any:
		var kept []any
		for _, e := range x {
			if str, ok := e.(string); ok && s.populatePathAllowed(strings.Split(str, "."), m, auth) {
				kept = append(kept, e)
			}
		}
		return kept
	case map[string]any:
		out := map[string]any{}
		for _, key := range sortedKeys(x) {
			value := x[key]
			a := m.attr(key)
			if a == nil || isPrivate(a) || s.restricted(m, key, a, auth) {
				continue
			}
			if isMorphTo(a) {
				if v, keep := s.sanitizeMorphPopulate(value, auth); keep {
					out[key] = v
				}
				continue
			}
			if a.Type == schema.TypeDynamicZone {
				if vm, ok := value.(map[string]any); ok {
					if on, ok := vm["on"].(map[string]any); ok {
						newOn := map[string]any{}
						for t, tp := range on {
							if isMorphTo(a) && !auth.Allowed(t+".find") {
								continue
							}
							newOn[t] = s.sanitizePopulateOptions(tp, s.schemaOf(t), auth)
						}
						if len(newOn) == 0 && isMorphTo(a) {
							continue
						}
						nv := map[string]any{}
						for k, v := range vm {
							nv[k] = v
						}
						nv["on"] = newOn
						out[key] = nv
						continue
					}
				}
				out[key] = value
				continue
			}
			out[key] = s.sanitizePopulateOptions(value, s.target(a), auth)
		}
		return out
	}
	return populate
}

func isMorphCount(v any) bool { return v == true || v == "true" }

// sanitizeMorphPopulate is removeRestrictedRelations on a morphTo populate:
// "true" and {count} become a fragment over every readable content type.
func (s *Server) sanitizeMorphPopulate(value any, auth *Auth) (any, bool) {
	vm, isObj := value.(map[string]any)
	on, hasOn := vm["on"].(map[string]any)
	if isObj && vm["__type"] == nil && hasOn {
		newOn := map[string]any{}
		for t, tp := range on {
			if auth.Allowed(t + ".find") {
				newOn[t] = s.sanitizePopulateOptions(tp, s.schemaOf(t), auth)
			}
		}
		if len(newOn) == 0 {
			return nil, false
		}
		nv := map[string]any{}
		for k, v := range vm {
			nv[k] = v
		}
		if c, ok := vm["count"]; ok && isMorphCount(c) {
			nv["count"] = true
		}
		nv["on"] = newOn
		return nv, true
	}
	_, onKey := vm["on"]
	if isMorphCount(value) || isObj && isMorphCount(vm["count"]) && !onKey {
		newOn := map[string]any{}
		for uid := range s.set.ContentTypes.All() {
			if auth.Allowed(uid + ".find") {
				newOn[uid] = true
			}
		}
		if len(newOn) == 0 {
			return nil, false
		}
		if !isObj {
			return map[string]any{"on": newOn}, true
		}
		nv := map[string]any{}
		for k, v := range vm {
			nv[k] = v
		}
		nv["count"] = true
		nv["on"] = newOn
		return nv, true
	}
	if c, ok := vm["count"].(float64); ok && len(vm) == 1 {
		return map[string]any{"count": c}, true
	}
	if list, ok := value.([]any); ok && len(list) == 0 {
		return value, true
	}
	return nil, false
}

func (s *Server) sanitizePopulateOptions(value any, m *schemaModel, auth *Auth) any {
	vm, ok := value.(map[string]any)
	if !ok || m == nil {
		return value
	}
	out := map[string]any{}
	for k, v := range vm {
		if k == "populate" {
			out[k] = s.sanitizePopulate(v, m, auth)
			continue
		}
		out[k] = v
	}
	return out
}

func (s *Server) populatePathAllowed(segments []string, m *schemaModel, auth *Auth) bool {
	if len(segments) == 0 || m == nil {
		return true
	}
	a := m.attr(segments[0])
	if a == nil {
		return true
	}
	if isPrivate(a) || s.restricted(m, segments[0], a, auth) {
		return false
	}
	return s.populatePathAllowed(segments[1:], s.target(a), auth)
}

// sanitizeOutput removes passwords, private fields and relations auth
// cannot read, in the entry and everything populated.
func (s *Server) sanitizeOutputDeep(v any, m *schemaModel, auth *Auth) any {
	if m == nil || v == nil {
		return v
	}
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = s.sanitizeOutputDeep(e, m, auth)
		}
		return out
	case []query.Row:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = s.sanitizeOutputDeep(e, m, auth)
		}
		return out
	case query.Row:
		if x == nil {
			return nil
		}
		out := &schema.OrderedMap[any]{}
		for k, val := range x.All() {
			a := m.attr(k)
			if a != nil && (a.Type == schema.TypePassword || isPrivate(a)) {
				continue
			}
			if a != nil && s.restricted(m, k, a, auth) {
				continue
			}
			if a != nil {
				switch {
				case a.Type == schema.TypeDynamicZone || isMorphTo(a):
					val = s.sanitizePolymorphic(val, auth, a.Type == schema.TypeDynamicZone)
				default:
					val = s.sanitizeOutputDeep(val, s.target(a), auth)
				}
			}
			out.Set(k, val)
		}
		return out
	}
	return v
}

func (s *Server) sanitizePolymorphic(v any, auth *Auth, dz bool) any {
	field := "__type"
	if dz {
		field = "__component"
	}
	one := func(e any) (any, bool) {
		row, ok := e.(query.Row)
		if !ok || row == nil {
			return e, true
		}
		t, _ := row.Get(field)
		ts := fmt.Sprint(t)
		if !dz && !auth.Allowed(ts+".find") {
			return nil, false
		}
		return s.sanitizeOutputDeep(row, s.schemaOf(ts), auth), true
	}
	if list, ok := v.([]any); ok {
		out := []any{}
		for _, e := range list {
			if r, keep := one(e); keep {
				out = append(out, r)
			}
		}
		return out
	}
	r, _ := one(v)
	return r
}

// sanitizeInput removes id, documentId, non writable attributes and
// relations auth cannot read.
func (s *Server) sanitizeInputBuiltin(data map[string]any, m *schemaModel, auth *Auth) map[string]any {
	out := map[string]any{}
	for k, v := range data {
		if k == "id" || k == schema.AttrDocumentID {
			continue
		}
		a := m.attr(k)
		if a != nil && !writable(k, a) {
			continue
		}
		if a != nil && s.restricted(m, k, a, auth) {
			continue
		}
		out[k] = v
	}
	return out
}

// writable is Strapi's isWritableAttribute.
func writable(name string, a *schema.Attribute) bool {
	switch name {
	case "id", schema.AttrDocumentID, schema.AttrCreatedAt, schema.AttrUpdatedAt:
		return false
	}
	if a.Writable != nil && !*a.Writable {
		return false
	}
	return true
}

func bodyInvalidKey(key string, path any) *APIError {
	msg := "Invalid key " + key
	if p, ok := path.(string); ok && p != "" && p != key {
		msg += " at " + p
	}
	details := map[string]any{"key": key, "source": "body"}
	if path != false {
		details["path"] = path
	}
	return validationError(msg, details)
}

// validateInput is Strapi's validate.input: id and documentId first, then
// the non writable root attributes, then the unrecognized keys, each pass
// in body order.
func (s *Server) validateInput(data map[string]any, keys []string, m *schemaModel, auth *Auth) error {
	for _, k := range []string{"id", schema.AttrDocumentID} {
		if _, ok := data[k]; ok {
			return bodyInvalidKey(k, false)
		}
	}
	for _, k := range keys {
		if a := m.attr(k); a != nil && !writable(k, a) {
			return bodyInvalidKey(k, k)
		}
	}
	for _, k := range keys {
		a := m.attr(k)
		if a == nil {
			return bodyInvalidKey(k, nil)
		}
		if err := s.unrecognizedIn(data[k], a, k); err != nil {
			return err
		}
	}
	if auth == nil {
		return nil
	}
	for _, k := range keys {
		if err := s.restrictedRelation(data, k, m.attr(k), m, k, auth); err != nil {
			return err
		}
	}
	return nil
}

// restrictedRelation is Strapi's throwRestrictedRelations for one key.
func (s *Server) restrictedRelation(data map[string]any, key string, a *schema.Attribute, m *schemaModel, path string, auth *Auth) error {
	if a == nil {
		return nil
	}
	switch a.Type {
	case schema.TypeComponent:
		c := s.schemaOf(a.Component)
		for _, item := range toList(data[key]) {
			if err := s.restrictedIn(item, c, path, auth); err != nil {
				return err
			}
		}
		return nil
	case schema.TypeDynamicZone:
		for _, item := range toList(data[key]) {
			im, _ := item.(map[string]any)
			uid, _ := im["__component"].(string)
			if err := s.restrictedIn(item, s.schemaOf(uid), path, auth); err != nil {
				return err
			}
		}
		return nil
	case schema.TypeRelation:
	default:
		return nil
	}
	invalid := bodyInvalidKey(key, path)
	if !isMorphTo(a) {
		if (key == schema.AttrCreatedBy || key == schema.AttrUpdatedBy) && m.ct != nil && m.ct.PopulateCreatorFields() {
			return nil
		}
		if !auth.Allowed(a.Target + ".find") {
			return invalid
		}
		return nil
	}
	value := data[key]
	if value == true {
		return nil
	}
	elements := func(v any) error {
		list, ok := v.([]any)
		if !ok {
			return invalid
		}
		for _, e := range list {
			em, ok := e.(map[string]any)
			t, isStr := em["__type"].(string)
			if !ok || !isStr || !auth.Allowed(t+".find") {
				return invalid
			}
		}
		return nil
	}
	vm, isObj := value.(map[string]any)
	if isObj && vm["count"] == true {
		return nil
	}
	_, hasConnect := vm["connect"]
	_, hasSet := vm["set"]
	_, hasDisconnect := vm["disconnect"]
	opts, hasOptions := vm["options"]
	switch {
	case isObj && (hasConnect || hasSet || hasDisconnect || hasOptions):
		for _, op := range []string{"connect", "set", "disconnect"} {
			v := vm[op]
			if v == nil {
				v = []any{}
			}
			if err := elements(v); err != nil {
				return err
			}
		}
		if opts == nil {
			return nil
		}
		om, ok := opts.(map[string]any)
		if !ok {
			return invalid
		}
		for _, k := range sortedKeys(om) {
			if _, isBool := om[k].(bool); k != "strict" || !isBool {
				return bodyInvalidKey(k, path)
			}
		}
		return nil
	case isObj && vm["__type"] == nil && isMapValue(vm["on"]):
		for _, uid := range sortedKeys(vm["on"].(map[string]any)) {
			if !auth.Allowed(uid + ".find") {
				return invalid
			}
		}
		return nil
	}
	return elements(value)
}

func isMapValue(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

func (s *Server) restrictedIn(item any, m *schemaModel, path string, auth *Auth) error {
	obj, ok := item.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	for _, k := range sortedKeys(obj) {
		a := m.attr(k)
		if a == nil {
			continue
		}
		if err := s.restrictedRelation(obj, k, a, m, path+"."+k, auth); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) unrecognizedIn(value any, a *schema.Attribute, path string) error {
	switch a.Type {
	case schema.TypeComponent:
		c := s.schemaOf(a.Component)
		for _, item := range toList(value) {
			if err := s.unrecognizedObject(item, c, path, false); err != nil {
				return err
			}
		}
	case schema.TypeDynamicZone:
		for _, item := range toList(value) {
			im, _ := item.(map[string]any)
			uid, _ := im["__component"].(string)
			if c := s.schemaOf(uid); c != nil {
				if err := s.unrecognizedObject(item, c, path, true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func toList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	if v == nil {
		return nil
	}
	return []any{v}
}

func (s *Server) unrecognizedObject(item any, m *schemaModel, path string, dz bool) error {
	obj, ok := item.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	for _, k := range sortedKeys(obj) {
		a := m.attr(k)
		if a == nil {
			if k == "id" || k == schema.AttrDocumentID || dz && k == "__component" {
				continue
			}
			return bodyInvalidKey(k, path)
		}
		if err := s.unrecognizedIn(obj[k], a, path+"."+k); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// Sanitizer is a function of strapi.sanitizers.add('content-api.input' or
// 'content-api.output'): it gets the model and one entry after the
// built-in rules, and returns the entry to keep.
type Sanitizer func(uid string, entry map[string]any) map[string]any

// Sanitizers are the registered content API sanitizers.
type Sanitizers struct{ Input, Output []Sanitizer }

// sanitizeOutput is Strapi's contentAPI.sanitize.output: the built-in rules
// at every level, then the registered sanitizers on each top-level entry.
func (s *Server) sanitizeOutput(v any, m *schemaModel, auth *Auth) any {
	out := s.sanitizeOutputDeep(v, m, auth)
	if m == nil || len(s.cfg.Sanitizers.Output) == 0 {
		return out
	}
	if list, ok := out.([]any); ok {
		for i, e := range list {
			list[i] = applySanitizers(e, m.uid, s.cfg.Sanitizers.Output)
		}
		return list
	}
	return applySanitizers(out, m.uid, s.cfg.Sanitizers.Output)
}

func (s *Server) sanitizeInput(data map[string]any, m *schemaModel, auth *Auth) map[string]any {
	out := s.sanitizeInputBuiltin(data, m, auth)
	if m == nil || out == nil {
		return out
	}
	for _, fn := range s.cfg.Sanitizers.Input {
		out = fn(m.uid, out)
	}
	return out
}

// applySanitizers runs fns on an entry, keeping the key order: the kept
// keys stay in place, the added ones follow in name order.
func applySanitizers(v any, uid string, fns []Sanitizer) any {
	om, ok := v.(*schema.OrderedMap[any])
	if !ok || om == nil {
		return v
	}
	entry := map[string]any{}
	var order []string
	for k, val := range om.All() {
		entry[k] = val
		order = append(order, k)
	}
	for _, fn := range fns {
		entry = fn(uid, entry)
	}
	out := &schema.OrderedMap[any]{}
	for _, k := range order {
		if val, ok := entry[k]; ok {
			out.Set(k, val)
			delete(entry, k)
		}
	}
	rest := make([]string, 0, len(entry))
	for k := range entry {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		out.Set(k, entry[k])
	}
	return out
}
