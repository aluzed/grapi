package document

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// Strapi's entity validator: a yup 0.32 object schema built from the
// content type (strict false, abortEarly false). Values are cast the way
// yup casts them and the cast data is what gets saved; the messages are
// yup's, so the 400 responses are the same.

// fieldError is one yup error.
type fieldError struct {
	Path    []string `json:"path"`
	Message string   `json:"message"`
	Name    string   `json:"name"`
	Value   any      `json:"value"`
}

type validator struct {
	s        *Service
	ctx      context.Context
	creation bool
	isDraft  bool
	locale   any
	root     *schema.ContentType
	entityID any
	errs     []fieldError
}

func pathString(path []string) string {
	var b strings.Builder
	for i, p := range path {
		if _, err := strconv.Atoi(p); err == nil && i > 0 {
			b.WriteString("[" + p + "]")
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(p)
	}
	return b.String()
}

func (v *validator) fail(path []string, value any, format string, args ...any) {
	msg := strings.ReplaceAll(fmt.Sprintf(format, args...), "${path}", pathString(path))
	v.errs = append(v.errs, fieldError{Path: append([]string(nil), path...), Message: msg, Name: "ValidationError", Value: value})
}

func (v *validator) failMsg(path []string, value any, msg string) {
	v.errs = append(v.errs, fieldError{Path: append([]string(nil), path...), Message: msg, Name: "ValidationError", Value: value})
}

// printValue is yup's printValue with quoted strings.
func printValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if math.IsNaN(x) {
			return "NaN"
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return `"` + x + `"`
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func notType(path []string, typ string, value, original any) string {
	cast := original != nil && !sameValue(original, value)
	msg := fmt.Sprintf("%s must be a `%s` type, but the final value was: `%s`", pathString(path), typ, printValue(value))
	if cast {
		return msg + fmt.Sprintf(" (cast from the value `%s`).", printValue(original))
	}
	return msg + "."
}

func sameValue(a, b any) bool {
	fa, aok := a.(float64)
	fb, bok := b.(float64)
	if aok && bok {
		return fa == fb || (math.IsNaN(fa) && math.IsNaN(fb))
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func writableAttrs(attrs *schema.OrderedMap[*schema.Attribute]) []string {
	var out []string
	for name, a := range attrs.All() {
		switch name {
		case "id", schema.AttrDocumentID, schema.AttrCreatedAt, schema.AttrUpdatedAt:
			continue
		}
		if a.Writable != nil && !*a.Writable {
			continue
		}
		out = append(out, name)
	}
	return out
}

// validateEntity validates and casts data for a creation or an update.
func (s *Service) validateEntity(ctx context.Context, ct *schema.ContentType, data map[string]any, creation, isDraft bool, locale any, entityID any) (map[string]any, error) {
	v := &validator{s: s, ctx: ctx, creation: creation, isDraft: isDraft, locale: locale, root: ct, entityID: entityID}
	out := v.object(&ct.Attributes, data, nil, nil)
	if len(v.errs) == 0 {
		if err := s.checkRelationsExist(ctx, string(ct.UID), out); err != nil {
			v.errs = append(v.errs, fieldError{Path: []string{}, Message: err.Error(), Name: "ValidationError"})
		}
	}
	if len(v.errs) == 0 {
		return out, nil
	}
	msg := v.errs[0].Message
	if len(v.errs) > 1 {
		msg = fmt.Sprintf("%d errors occurred", len(v.errs))
	}
	return nil, &ValidationError{Msg: msg, Details: map[string]any{"errors": v.errs}}
}

type componentScope struct {
	path []string
}

func (v *validator) object(attrs *schema.OrderedMap[*schema.Attribute], data map[string]any, path []string, comp *componentScope) map[string]any {
	out := map[string]any{}
	for k, val := range data {
		out[k] = val
	}
	for _, name := range writableAttrs(attrs) {
		a, _ := attrs.Get(name)
		value, present := data[name]
		p := append(append([]string(nil), path...), name)
		cast, keep := v.attribute(a, name, value, present, p, comp)
		if keep {
			out[name] = cast
		} else if !present {
			delete(out, name)
		}
	}
	return out
}

func boolOf(b *bool) bool { return b != nil && *b }

// attribute validates one value and returns its cast form; keep is false
// when the key stays absent.
func (v *validator) attribute(a *schema.Attribute, name string, value any, present bool, path []string, comp *componentScope) (any, bool) {
	if !present {
		value = v.defaultOf(a)
		present = value != nil || (v.creation && hasDefault(a))
	}
	switch a.Type {
	case schema.TypeMedia, schema.TypeRelation:
		return value, present
	case schema.TypeComponent:
		return v.component(a, value, present, path)
	case schema.TypeDynamicZone:
		return v.dynamicZone(a, value, present, path)
	}
	required := !v.isDraft && boolOf(a.Required)
	if !present {
		if required && v.creation {
			v.fail(path, nil, "${path} must be defined.")
		}
		return nil, false
	}
	cast, ok := v.scalar(a, name, value, path, comp)
	if !ok {
		return cast, true
	}
	if required && !v.creation && cast == nil {
		v.fail(path, cast, "${path} cannot be null.")
	}
	return cast, true
}

func hasDefault(a *schema.Attribute) bool {
	if len(a.Default) > 0 {
		return true
	}
	return (a.Type == schema.TypeComponent && boolOf(a.Repeatable) || a.Type == schema.TypeDynamicZone) && !boolOf(a.Required)
}

func (v *validator) defaultOf(a *schema.Attribute) any {
	if !v.creation {
		return nil
	}
	if (a.Type == schema.TypeComponent && boolOf(a.Repeatable) || a.Type == schema.TypeDynamicZone) && !boolOf(a.Required) {
		return []any{}
	}
	if len(a.Default) > 0 {
		var d any
		if json.Unmarshal(a.Default, &d) == nil {
			return d
		}
	}
	return nil
}

var (
	emailPattern = regexp.MustCompile(`^((([a-z]|\d|[!#\$%&'\*\+\-\/=\?\^_` + "`" + `{\|}~]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])+(\.([a-z]|\d|[!#\$%&'\*\+\-\/=\?\^_` + "`" + `{\|}~]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])+)*)|((\x22)((((\x20|\x09)*(\x0d\x0a))?(\x20|\x09)+)?(([\x01-\x08\x0b\x0c\x0e-\x1f\x7f]|\x21|[\x23-\x5b]|[\x5d-\x7e]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])|(\\([\x01-\x09\x0b\x0c\x0d-\x7f]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}]))))*(((\x20|\x09)*(\x0d\x0a))?(\x20|\x09)+)?(\x22)))@((([a-z]|\d|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])|(([a-z]|\d|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])([a-z]|\d|-|\.|_|~|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])*([a-z]|\d|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])))\.)+(([a-z]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])|(([a-z]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])([a-z]|\d|-|\.|_|~|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])*([a-z]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])))$`)
	uidPattern   = regexp.MustCompile(`^[A-Za-z0-9-_.~]*$`)
	bigIntRegex  = regexp.MustCompile(`^[+-]?\d+$`)
)

func isStringType(t string) bool {
	switch t {
	case schema.TypeString, schema.TypeText, schema.TypeRichText, schema.TypePassword, schema.TypeEmail, schema.TypeUID:
		return true
	}
	return false
}

// yupNumberCast is yup's number transform: strings are parsed with
// parseFloat after removing spaces, anything else goes through Number.
func yupNumberCast(v any) any {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		t := strings.ReplaceAll(x, " ", "")
		if t == "" {
			return math.NaN()
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
		// parseFloat keeps a numeric prefix; Number() of the rest is NaN.
		return math.NaN()
	case bool:
		if x {
			return float64(1)
		}
		return float64(0)
	}
	return math.NaN()
}

func (v *validator) scalar(a *schema.Attribute, name string, value any, path []string, comp *componentScope) (any, bool) {
	switch {
	case isStringType(a.Type):
		if value == nil {
			if boolOf(a.Required) && !v.isDraft {
				v.failMsg(path, nil, notType(path, "string", nil, nil))
				return nil, false
			}
			return nil, true
		}
		str, ok := value.(string)
		if !ok {
			v.failMsg(path, value, notType(path, "string", value, nil))
			return value, false
		}
		n := len([]rune(str))
		if a.MinLength != nil && !v.isDraft && n < *a.MinLength {
			v.fail(path, str, "${path} must be at least %d characters", *a.MinLength)
		}
		maxLen := 0
		if a.MaxLength != nil {
			maxLen = *a.MaxLength
		} else if a.Type == schema.TypeString {
			maxLen = 255
		}
		if maxLen > 0 && n > maxLen {
			v.fail(path, str, "${path} must be at most %d characters", maxLen)
		}
		if a.Regex != "" && !v.isDraft && (str != "" || boolOf(a.Required)) {
			if re, err := regexp.Compile(a.Regex); err == nil && !re.MatchString(str) {
				v.fail(path, str, `${path} must match the following: "/%s/"`, a.Regex)
			}
		}
		if a.Type == schema.TypeEmail && !v.isDraft {
			if str != "" && !emailPattern.MatchString(str) {
				v.fail(path, str, "${path} must be a valid email")
			}
			if n < 1 {
				v.fail(path, str, "${path} cannot be empty")
			}
		}
		if a.Type == schema.TypeUID && !v.isDraft {
			re := uidPattern
			display := uidPattern.String()
			if a.Regex != "" {
				if r, err := regexp.Compile(a.Regex); err == nil {
					re, display = r, a.Regex
				}
			}
			if !re.MatchString(str) {
				v.fail(path, str, `${path} must match the following: "/%s/"`, display)
			}
		}
		v.unique(a, name, str, path, comp)
		return str, true
	case a.Type == schema.TypeEnumeration:
		str := value
		switch x := value.(type) {
		case nil:
		case string:
		case float64:
			str = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			str = strconv.FormatBool(x)
		}
		if s, ok := str.(string); ok || str == nil {
			for _, e := range a.Enum {
				if s == e && str != nil {
					return str, true
				}
			}
			if str == nil {
				return nil, true
			}
		}
		values := strings.Join(append(append([]string{}, a.Enum...), ""), ", ")
		v.fail(path, str, "${path} must be one of the following values: %s", values)
		return str, false
	case a.Type == schema.TypeBoolean:
		switch x := value.(type) {
		case nil, bool:
			return x, true
		case string:
			switch strings.ToLower(x) {
			case "true", "1":
				return true, true
			case "false", "0":
				return false, true
			}
		case float64:
			if x == 1 {
				return true, true
			}
			if x == 0 {
				return false, true
			}
		}
		v.failMsg(path, value, notType(path, "boolean", value, nil))
		return value, false
	case a.Type == schema.TypeInteger, a.Type == schema.TypeFloat, a.Type == schema.TypeDecimal:
		if value == nil {
			return nil, true
		}
		f := yupNumberCast(value).(float64)
		if math.IsNaN(f) {
			v.failMsg(path, f, notType(path, "number", f, value))
			return value, false
		}
		if a.Type == schema.TypeInteger && f != math.Trunc(f) {
			v.fail(path, f, "${path} must be an integer")
		}
		if a.Type != schema.TypeInteger && math.IsInf(f, 0) {
			v.fail(path, f, "${path} must be a finite number")
		}
		if lo, ok := numberOption(a.Min); ok && !v.isDraft {
			if a.Type == schema.TypeInteger {
				lo = math.Trunc(lo)
			}
			if f < lo {
				v.fail(path, f, "${path} must be greater than or equal to %s", jsNumber(lo))
			}
		}
		if hi, ok := numberOption(a.Max); ok {
			if a.Type == schema.TypeInteger {
				hi = math.Trunc(hi)
			}
			if f > hi {
				v.fail(path, f, "${path} must be less than or equal to %s", jsNumber(hi))
			}
		}
		v.unique(a, name, f, path, comp)
		return f, true
	case a.Type == schema.TypeBigInteger:
		if value == nil {
			return nil, true
		}
		str, ok := bigIntString(value)
		if !ok {
			v.fail(path, value, "${path} must be a valid integer")
			return value, false
		}
		v.unique(a, name, str, path, comp)
		return str, true
	case a.Type == schema.TypeDate, a.Type == schema.TypeTime, a.Type == schema.TypeDateTime, a.Type == schema.TypeTimestamp:
		v.unique(a, name, value, path, comp)
	}
	return value, true
}

func numberOption(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return 0, false
	}
	f := toNumber(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func bigIntString(v any) (string, bool) {
	switch x := v.(type) {
	case float64:
		if x != math.Trunc(x) || math.IsInf(x, 0) {
			return "", false
		}
		return new(big.Float).SetFloat64(x).Text('f', 0), true
	case string:
		t := strings.TrimSpace(x)
		if !bigIntRegex.MatchString(t) {
			return "", false
		}
		n, _ := new(big.Int).SetString(strings.TrimPrefix(t, "+"), 10)
		return n.String(), true
	}
	return "", false
}

// unique checks a unique (or uid) value against the published entries of
// the same locale, Strapi's addUniqueValidator for content type fields.
func (v *validator) unique(a *schema.Attribute, name string, value any, path []string, comp *componentScope) {
	if a.Type != schema.TypeUID && !boolOf(a.Unique) {
		return
	}
	if value == nil || value == "" || v.isDraft || comp != nil {
		return
	}
	if f, ok := value.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
		return
	}
	where := map[string]any{name: value, schema.AttrPublishedAt: map[string]any{"$notNull": true}}
	if v.locale != nil && v.locale != "" {
		where[schema.AttrLocale] = v.locale
	}
	if v.entityID != nil {
		where["id"] = map[string]any{"$ne": v.entityID}
	}
	row, err := v.s.E.FindOne(v.ctx, string(v.root.UID), query.Params{Where: where, Select: []string{"id"}})
	if err == nil && row != nil {
		v.fail(path, value, "This attribute must be unique")
	}
}

func (v *validator) component(a *schema.Attribute, value any, present bool, path []string) (any, bool) {
	c, ok := v.s.Set.Component(schema.UID(a.Component))
	if !ok {
		return value, present
	}
	scope := &componentScope{path: path}
	if boolOf(a.Repeatable) {
		if !present {
			if v.creation {
				v.fail(path, nil, "${path} must be defined.")
			}
			return nil, false
		}
		if value == nil {
			v.fail(path, nil, "${path} cannot be null.")
			return nil, true
		}
		list, ok := value.([]any)
		if !ok {
			v.failMsg(path, value, notType(path, "array", value, nil))
			return value, true
		}
		out := make([]any, len(list))
		for i, item := range list {
			p := append(append([]string(nil), path...), strconv.Itoa(i))
			m, ok := item.(map[string]any)
			if !ok {
				if item == nil {
					v.fail(p, nil, "${path} cannot be null.")
				} else {
					v.failMsg(p, item, notType(p, "object", item, nil))
				}
				out[i] = item
				continue
			}
			out[i] = v.object(&c.Attributes, m, p, scope)
		}
		if !v.isDraft {
			if lo, ok := numberOption(a.Min); ok && lo == math.Trunc(lo) && (boolOf(a.Required) || len(list) > 0) && float64(len(list)) < lo {
				v.fail(path, out, "${path} field must have at least %s items", jsNumber(lo))
			}
			if hi, ok := numberOption(a.Max); ok && hi == math.Trunc(hi) && float64(len(list)) > hi {
				v.fail(path, out, "${path} field must have less than or equal to %s items", jsNumber(hi))
			}
		}
		return out, true
	}
	required := !v.isDraft && boolOf(a.Required)
	if !present {
		if required && v.creation {
			v.fail(path, nil, "${path} must be defined.")
		}
		return nil, false
	}
	if value == nil {
		if required {
			v.fail(path, nil, "${path} cannot be null.")
		}
		return nil, true
	}
	m, ok := value.(map[string]any)
	if !ok {
		v.failMsg(path, value, notType(path, "object", value, nil))
		return value, true
	}
	return v.object(&c.Attributes, m, path, scope), true
}

func (v *validator) dynamicZone(a *schema.Attribute, value any, present bool, path []string) (any, bool) {
	if !present {
		if v.creation {
			v.fail(path, nil, "${path} must be defined.")
		}
		return nil, false
	}
	if value == nil {
		v.fail(path, nil, "${path} cannot be null.")
		return nil, true
	}
	list, ok := value.([]any)
	if !ok {
		v.failMsg(path, value, notType(path, "array", value, nil))
		return value, true
	}
	var all []string
	for uid := range v.s.Set.Components.All() {
		all = append(all, uid)
	}
	sort.Strings(all)
	out := make([]any, len(list))
	for i, item := range list {
		p := append(append([]string(nil), path...), strconv.Itoa(i))
		m, ok := item.(map[string]any)
		if !ok {
			v.fail(p, item, "${path} cannot be null.")
			out[i] = item
			continue
		}
		comp, _ := m["__component"].(string)
		cp := append(append([]string(nil), p...), "__component")
		c, known := v.s.Set.Component(schema.UID(comp))
		switch {
		case m["__component"] == nil:
			v.fail(cp, nil, "${path} is a required field")
		case !known:
			v.fail(cp, comp, "${path} must be one of the following values: %s", strings.Join(all, ", "))
		}
		if known {
			out[i] = v.object(&c.Attributes, m, p, &componentScope{path: p})
		} else {
			out[i] = m
		}
	}
	if !v.isDraft {
		if lo, ok := numberOption(a.Min); ok && lo == math.Trunc(lo) && (boolOf(a.Required) || len(list) > 0) && float64(len(list)) < lo {
			v.fail(path, out, "${path} field must have at least %s items", jsNumber(lo))
		}
		if hi, ok := numberOption(a.Max); ok && hi == math.Trunc(hi) && float64(len(list)) > hi {
			v.fail(path, out, "${path} field must have less than or equal to %s items", jsNumber(hi))
		}
	}
	return out, true
}

// checkRelationsExist is Strapi's relations test: every related id given
// must exist.
func (s *Service) checkRelationsExist(ctx context.Context, uid string, data map[string]any) error {
	store := map[string][]any{}
	var order []string
	s.relationsStore(uid, data, store, &order)
	for _, target := range order {
		seen := map[string]bool{}
		var ids []any
		for _, id := range store[target] {
			k := fmt.Sprint(id)
			if !seen[k] {
				seen[k] = true
				ids = append(ids, id)
			}
		}
		n, err := s.E.Count(ctx, target, query.Params{Where: map[string]any{"id": map[string]any{"$in": ids}}})
		if err != nil {
			return err
		}
		if int(n) != len(ids) {
			return validation("%d relation(s) of type %s associated with this entity do not exist", len(ids)-int(n), target)
		}
	}
	return nil
}

func (s *Service) relationsStore(uid string, data map[string]any, store map[string][]any, order *[]string) {
	m := s.model(uid)
	if m == nil || len(data) == 0 {
		return
	}
	for name, a := range m.attrs.All() {
		value := data[name]
		if value == nil {
			continue
		}
		switch a.Type {
		case schema.TypeRelation, schema.TypeMedia:
			if a.Type == schema.TypeRelation && (a.Relation == schema.MorphToMany || a.Relation == schema.MorphToOne) {
				continue
			}
			target := a.Target
			if a.Type == schema.TypeMedia {
				target = "plugin::upload.file"
			}
			var source []any
			switch x := value.(type) {
			case []any:
				source = x
			case map[string]any:
				if c, ok := x["connect"]; ok && c != nil {
					source = toArray(c)
				} else if st, ok := x["set"]; ok && st != nil {
					source = toArray(st)
				}
			default:
				source = []any{value}
			}
			if _, ok := store[target]; !ok {
				*order = append(*order, target)
			}
			for _, e := range source {
				if em, ok := e.(map[string]any); ok {
					store[target] = append(store[target], em["id"])
				} else {
					store[target] = append(store[target], e)
				}
			}
		case schema.TypeComponent:
			for _, c := range toArray(value) {
				if cm, ok := c.(map[string]any); ok {
					s.relationsStore(a.Component, cm, store, order)
				}
			}
		case schema.TypeDynamicZone:
			for _, c := range toArray(value) {
				if cm, ok := c.(map[string]any); ok {
					comp, _ := cm["__component"].(string)
					s.relationsStore(comp, cm, store, order)
				}
			}
		}
	}
}
