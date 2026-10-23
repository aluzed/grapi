package document

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/document"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// ErrValidation is Strapi's ValidationError (a 400 at the API).
var ErrValidation = errors.New("validation error")

// ValidationError carries Strapi's message.
type ValidationError struct {
	Msg     string
	Details any
}

func (e *ValidationError) Error() string { return e.Msg }

// Unwrap lets errors.Is match ErrValidation.
func (e *ValidationError) Unwrap() error { return ErrValidation }

func validation(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// Params are the public document.Params.
type Params = document.Params

// model is a content type or a component, as the param conversion sees it.
type model struct {
	uid           string
	isContentType bool
	attrs         *schema.OrderedMap[*schema.Attribute]
	info          schema.Info
}

func (s *Service) model(uid string) *model {
	if ct, ok := s.Set.ContentType(schema.UID(uid)); ok {
		return &model{uid: uid, isContentType: true, attrs: &ct.Attributes, info: ct.Info}
	}
	if c, ok := s.Set.Component(schema.UID(uid)); ok {
		return &model{uid: uid, attrs: &c.Attributes, info: c.Info}
	}
	return nil
}

// TransformQuery is Strapi's transformQueryParams: REST params to a query.
func (s *Service) TransformQuery(uid string, p Params) (query.Params, error) {
	m := s.model(uid)
	var q query.Params
	if status, ok := p["status"].(string); ok && status != "" {
		q.Filters = s.statusFilter(status)
	}
	if v, ok := p["_q"]; ok && v != nil {
		q.Search = fmt.Sprint(v)
	}
	if err := applySort(&q, p["sort"]); err != nil {
		return q, err
	}
	if f, ok := p["filters"]; ok && f != nil {
		w, err := s.convertFilters(f, m)
		if err != nil {
			return q, err
		}
		q.Where = w
	}
	if f, ok := p["fields"]; ok && f != nil {
		sel, err := convertFields(f, m, 0)
		if err != nil {
			return q, err
		}
		q.Select = sel
	}
	if pop, ok := p["populate"]; ok && pop != nil {
		v, err := s.convertPopulate(pop, m, 0)
		if err != nil {
			return q, err
		}
		q.Populate = v
	}
	if err := applyPagination(&q, p); err != nil {
		return q, err
	}
	return q, nil
}

// statusFilter keeps the draft or the published rows of every model that
// has draft and publish, the populated ones included.
func (s *Service) statusFilter(status string) query.FilterFunc {
	return func(m *metadata.Model) any {
		ct, ok := s.Set.ContentType(schema.UID(m.UID))
		if !ok || !ct.DraftAndPublish() {
			return map[string]any{}
		}
		return map[string]any{schema.AttrPublishedAt: map[string]any{"$null": status == "draft"}}
	}
}

// lodashToNumber is lodash's toNumber for pagination values.
func toNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		t := strings.TrimSpace(x)
		if t == "" {
			return 0
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	case nil:
		return 0
	}
	return math.NaN()
}

func isInteger(f float64) bool { return f == math.Trunc(f) && !math.IsInf(f, 0) && !math.IsNaN(f) }

func applyPagination(q *query.Params, p Params) error {
	page, pageSize, start, limit := p["page"], p["pageSize"], p["start"], p["limit"]
	if (page != nil || pageSize != nil) && (start != nil || limit != nil) {
		return &PaginationError{"Invalid pagination attributes. The page parameters are incorrect and must be in the pagination object"}
	}
	if start != nil {
		n := toNumber(start)
		if !isInteger(n) || n < 0 {
			return validation("convertStartQueryParams expected a positive integer got %s", jsNumber(n))
		}
		i := int(n)
		q.Offset = &i
	}
	if limit != nil {
		n := toNumber(limit)
		if !isInteger(n) || (n != -1 && n < 0) {
			return validation("convertLimitQueryParams expected a positive integer got %s", jsNumber(n))
		}
		if n != -1 {
			i := int(n)
			q.Limit = &i
		}
	}
	return nil
}

func jsNumber(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// PaginationError is Strapi's PaginationError (a 400).
type PaginationError struct{ Msg string }

func (e *PaginationError) Error() string { return e.Msg }

// Unwrap lets errors.Is match ErrValidation.
func (e *PaginationError) Unwrap() error { return ErrValidation }

func validateOrder(o string) error {
	l := strings.ToLower(o)
	if l != "asc" && l != "desc" {
		return validation("Invalid order. order can only be one of asc|desc|ASC|DESC")
	}
	return nil
}

func applySort(q *query.Params, sort any) error {
	if !hasSortValue(sort) {
		return nil
	}
	ob, err := convertSort(sort)
	if err != nil {
		return err
	}
	ob = normalizeOrderBy(ob)
	if ob != nil {
		q.OrderBy = ob
	}
	return nil
}

// hasSortValue is @strapi/utils' hasSort.
func hasSortValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(strings.ReplaceAll(x, ",", "")) != ""
	case []any:
		for _, e := range x {
			if hasSortValue(e) {
				return true
			}
		}
		return false
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func convertSort(sort any) (any, error) {
	switch x := sort.(type) {
	case string:
		return convertStringSort(x)
	case []any:
		allStrings := true
		for _, e := range x {
			if _, ok := e.(string); !ok {
				allStrings = false
			}
		}
		out := []any{}
		for _, e := range x {
			if allStrings {
				list, err := convertStringSort(e.(string))
				if err != nil {
					return nil, err
				}
				out = append(out, list...)
				continue
			}
			obj, ok := e.(map[string]any)
			if !ok {
				return nil, validation("Invalid sort parameter. Expected a string, an array of strings, a sort object or an array of sort objects")
			}
			m, err := convertNestedSort(obj)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, nil
	case map[string]any:
		return convertNestedSort(x)
	}
	return nil, validation("Invalid sort parameter. Expected a string, an array of strings, a sort object or an array of sort objects")
}

func convertStringSort(s string) ([]any, error) {
	var out []any
	for _, seg := range strings.Split(s, ",") {
		t := strings.TrimSpace(seg)
		if t == "" {
			continue
		}
		field, order, _ := strings.Cut(t, ":")
		field = strings.TrimSpace(field)
		order = strings.TrimSpace(order)
		if order == "" {
			order = "asc"
		}
		if field == "" {
			return nil, validation("Field cannot be empty")
		}
		if err := validateOrder(order); err != nil {
			return nil, err
		}
		out = append(out, sortPath(strings.Split(field, "."), order))
	}
	return out, nil
}

func sortPath(path []string, order string) map[string]any {
	if len(path) == 1 {
		return map[string]any{path[0]: order}
	}
	return map[string]any{path[0]: sortPath(path[1:], order)}
}

func convertNestedSort(m map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			nested, err := convertNestedSort(x)
			if err != nil {
				return nil, err
			}
			if !emptySortMap(nested) {
				out[k] = nested
			}
		case string:
			t := strings.TrimSpace(x)
			if t != "" {
				if err := validateOrder(t); err != nil {
					return nil, err
				}
				out[k] = t
			}
		default:
			return nil, validation("Invalid sort type expected object or string got %s", jsType(v))
		}
	}
	return out, nil
}

func jsType(v any) string {
	switch v.(type) {
	case float64, int, int64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "object"
	case []any, map[string]any:
		return "object"
	}
	return "string"
}

func emptySortMap(m map[string]any) bool {
	for _, v := range m {
		switch x := v.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				return false
			}
		case map[string]any:
			if !emptySortMap(x) {
				return false
			}
		}
	}
	return true
}

func normalizeOrderBy(ob any) any {
	switch x := ob.(type) {
	case []any:
		var out []any
		for _, e := range x {
			if m, ok := e.(map[string]any); ok && emptySortMap(m) {
				continue
			}
			out = append(out, e)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]any:
		if emptySortMap(x) {
			return nil
		}
	}
	return ob
}

func convertFields(fields any, m *model, depth int) ([]string, error) {
	withIDs := func(values []string) []string {
		base := []string{"id"}
		if m != nil && m.isContentType {
			base = append(base, schema.AttrDocumentID)
		}
		return uniq(append(base, values...))
	}
	switch x := fields.(type) {
	case string:
		if depth == 0 && x == "*" {
			return nil, nil
		}
		var values []string
		for _, f := range strings.Split(x, ",") {
			values = append(values, strings.TrimSpace(f))
		}
		return withIDs(values), nil
	case []any:
		var values []string
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, validation("Invalid fields parameter. Expected a string or an array of strings")
			}
			sub, err := convertFields(s, m, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, sub...)
		}
		return withIDs(values), nil
	}
	return nil, validation("Invalid fields parameter. Expected a string or an array of strings")
}

func uniq(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// parseBool is @strapi/utils' parseType for booleans.
func parseBool(v any, force bool) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch x {
		case "true", "t", "1":
			return true, nil
		case "false", "f", "0":
			return false, nil
		}
	case float64:
		switch x {
		case 1:
			return true, nil
		case 0:
			return false, nil
		}
	}
	if force {
		return truthy(v), nil
	}
	return false, validation("Invalid boolean input. Expected \"t\",\"1\",\"true\",\"false\",\"0\",\"f\"")
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && !math.IsNaN(x)
	}
	return true
}

func (s *Service) targetOf(a *schema.Attribute) string {
	switch a.Type {
	case schema.TypeRelation:
		return a.Target
	case schema.TypeComponent:
		return a.Component
	case schema.TypeMedia:
		return "plugin::upload.file"
	}
	return ""
}

func (s *Service) convertFilters(filters any, m *model) (any, error) {
	switch filters.(type) {
	case map[string]any, []any:
	default:
		return nil, validation("The filters parameter must be an object or an array")
	}
	return s.sanitizeFilters(deepCopy(filters), m), nil
}

func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = deepCopy(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = deepCopy(val)
		}
		return out
	}
	return v
}

func (s *Service) sanitizeFilters(filters any, m *model) any {
	switch x := filters.(type) {
	case []any:
		out := []any{}
		for _, f := range x {
			c := s.sanitizeFilters(f, m)
			if cm, ok := c.(map[string]any); ok && len(cm) == 0 {
				continue
			}
			out = append(out, c)
		}
		return out
	case map[string]any:
		for key, value := range x {
			var attr *schema.Attribute
			if m != nil {
				attr, _ = m.attrs.Get(key)
			}
			valid := query.IsOperator(key) || key == "id" || key == schema.AttrDocumentID || attr != nil
			switch {
			case !valid:
				delete(x, key)
				continue
			case attr != nil:
				switch attr.Type {
				case schema.TypeRelation, schema.TypeComponent, schema.TypeMedia:
					x[key] = s.sanitizeFilters(value, s.model(s.targetOf(attr)))
				case schema.TypeDynamicZone, schema.TypePassword:
					delete(x, key)
					continue
				default:
					x[key] = s.sanitizeFilters(value, m)
				}
			case key == "$null" || key == "$notNull":
				b, _ := parseBool(value, true)
				x[key] = b
			default:
				switch value.(type) {
				case map[string]any, []any:
					x[key] = s.sanitizeFilters(value, m)
				}
			}
			if cm, ok := x[key].(map[string]any); ok && len(cm) == 0 {
				delete(x, key)
			}
		}
		return x
	}
	return filters
}

func (s *Service) convertPopulate(populate any, m *model, depth int) (any, error) {
	if depth == 0 {
		if str, ok := populate.(string); ok && str == "*" {
			return true, nil
		}
	}
	switch x := populate.(type) {
	case string:
		var out []any
		for _, v := range strings.Split(x, ",") {
			out = append(out, strings.TrimSpace(v))
		}
		return out, nil
	case []any:
		var out []any
		seen := map[string]bool{}
		for _, v := range x {
			str, ok := v.(string)
			if !ok {
				return nil, validation("Invalid populate parameter. Expected a string, an array of strings, a populate object")
			}
			for _, part := range strings.Split(str, ",") {
				t := strings.TrimSpace(part)
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		}
		return out, nil
	case map[string]any:
		return s.convertPopulateObject(x, m)
	}
	return nil, validation("Invalid populate parameter. Expected a string, an array of strings, a populate object")
}

func isMorphLike(a *schema.Attribute) bool {
	return a.Type == schema.TypeDynamicZone || (a.Type == schema.TypeRelation && (a.Relation == schema.MorphToOne || a.Relation == schema.MorphToMany))
}

func (s *Service) convertPopulateObject(populate map[string]any, m *model) (any, error) {
	out := map[string]any{}
	if m == nil {
		return out, nil
	}
	for key, sub := range populate {
		if str, ok := sub.(string); ok {
			if b, err := parseBool(str, false); err == nil {
				if b {
					out[key] = true
				}
				continue
			}
		}
		if b, ok := sub.(bool); ok {
			if b {
				out[key] = true
			}
			continue
		}
		attr, ok := m.attrs.Get(key)
		if !ok {
			continue
		}
		subMap, _ := sub.(map[string]any)
		if isMorphLike(attr) {
			keys := make([]string, 0, len(subMap))
			for k := range subMap {
				keys = append(keys, k)
			}
			switch x := sub.(type) {
			case string:
				if x != "" {
					keys = append(keys, "0")
				}
			case []any:
				if len(x) > 0 {
					keys = append(keys, "0")
				}
			}
			for _, k := range keys {
				if k != "populate" && k != "on" && k != "count" {
					raw, _ := jsonString(sub)
					return nil, validation("Invalid nested populate for %s.%s (%s). Expected a fragment (\"on\") or \"count\" but found %s", m.info.SingularName, key, m.uid, raw)
				}
			}
			if p, ok := subMap["populate"]; ok && p != nil && p != "*" {
				return nil, validation("Invalid nested population query detected. When using 'populate' within polymorphic structures, its value must be '*' to indicate all second level links. Specific field targeting is not supported here. Consider using the fragment API for more granular population control.")
			}
			np := map[string]any{}
			if subMap["populate"] == "*" {
				np["populate"] = true
			}
			if on, ok := subMap["on"].(map[string]any); ok {
				onOut := map[string]any{}
				for t, tp := range on {
					v, err := s.convertNestedPopulate(tp, s.model(t))
					if err != nil {
						return nil, err
					}
					onOut[t] = v
				}
				np["on"] = onOut
			}
			if c, ok := subMap["count"].(bool); ok {
				np["count"] = c
			}
			out[key] = np
			continue
		}
		if _, hasOn := subMap["on"]; hasOn && subMap["on"] != nil {
			return nil, validation("Using fragments is not permitted to populate \"%s\" in \"%s\"", key, m.uid)
		}
		target := s.targetOf(attr)
		if target == "" {
			continue
		}
		tm := s.model(target)
		if tm == nil {
			continue
		}
		v, err := s.convertNestedPopulate(sub, tm)
		if err != nil {
			return nil, err
		}
		if v == nil || v == false {
			continue
		}
		out[key] = v
	}
	return out, nil
}

func (s *Service) convertNestedPopulate(sub any, m *model) (any, error) {
	if str, ok := sub.(string); ok {
		b, _ := parseBool(str, true)
		return b, nil
	}
	if b, ok := sub.(bool); ok {
		return b, nil
	}
	p, ok := sub.(map[string]any)
	if !ok {
		return nil, validation("Invalid nested populate. Expected '*' or an object")
	}
	q := map[string]any{}
	var qp query.Params
	if err := applySort(&qp, p["sort"]); err != nil {
		return nil, err
	}
	if qp.OrderBy != nil {
		q["orderBy"] = qp.OrderBy
	}
	if f, ok := p["filters"]; ok && f != nil && truthy(f) {
		w, err := s.convertFilters(f, m)
		if err != nil {
			return nil, err
		}
		q["where"] = w
	}
	if f, ok := p["fields"]; ok && f != nil && truthy(f) {
		sel, err := convertFields(f, m, 0)
		if err != nil {
			return nil, err
		}
		if sel != nil {
			list := make([]any, len(sel))
			for i, s := range sel {
				list[i] = s
			}
			q["select"] = list
		}
	}
	if pop, ok := p["populate"]; ok && pop != nil && truthy(pop) {
		v, err := s.convertPopulate(pop, m, 1)
		if err != nil {
			return nil, err
		}
		q["populate"] = v
	}
	if c, ok := p["count"]; ok && truthy(c) {
		b, err := parseBool(c, false)
		if err != nil {
			return nil, err
		}
		q["count"] = b
	}
	if o, ok := p["ordering"]; ok && truthy(o) {
		q["ordering"] = o
	}
	var pg query.Params
	if err := applyPagination(&pg, Params(p)); err != nil {
		return nil, err
	}
	if pg.Offset != nil {
		q["offset"] = float64(*pg.Offset)
	}
	if pg.Limit != nil {
		q["limit"] = float64(*pg.Limit)
	}
	return q, nil
}
