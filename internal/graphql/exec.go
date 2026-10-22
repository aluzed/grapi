package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"

	// The standard validation rules register themselves.
	_ "github.com/vektah/gqlparser/v2/validator/rules"
)

// Object is a response object, its keys in selection order.
type Object struct {
	keys   []string
	values map[string]any
}

func (o *Object) set(k string, v any) {
	if o.values == nil {
		o.values = map[string]any{}
	}
	if _, ok := o.values[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.values[k] = v
}

// Get returns the value of k.
func (o *Object) Get(k string) any { return o.values[k] }

// MarshalJSON writes the keys in order.
func (o *Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		vb, err := marshal(o.values[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// message is an error whose text is graphql-js' message, kept as is.
type message string

func (m message) Error() string { return string(m) }

func errorf(format string, args ...any) error { return message(fmt.Sprintf(format, args...)) }

// Error is a GraphQL error. Err is the resolver's error, formatted by the caller.
type Error struct {
	Message   string
	Locations []gqlerror.Location
	Path      []any
	Err       error
}

func (e *Error) Error() string { return e.Message }

// FieldContext is what a resolver gets.
type FieldContext struct {
	Parent any
	Args   map[string]any
	// Type is the parent type, Field its definition (nil on introspection types).
	Type  *Def
	Field *Field
	// Selection is the field in the query.
	Selection *ast.Field
	Path      []any
	// Root is the root field of the branch and its arguments.
	Root     *Field
	RootArgs map[string]any
}

// Resolver resolves the fields that are not plain values of their parent,
// and the concrete type of union values.
type Resolver interface {
	Resolve(ctx context.Context, fc *FieldContext) (any, error)
	TypeOf(union *Def, value any) string
}

// Request is one GraphQL request.
type Request struct {
	Query         string
	OperationName string
	Variables     map[string]any
	// Introspection allows __schema and __type.
	Introspection bool
}

// Result of Execute. Status is 400 when the request was rejected before
// execution, Code is then the Apollo error code.
type Result struct {
	Data   any
	Errors []*Error
	Status int
	Code   string
	// Extensions of a rejected request's errors.
	Extensions map[string]any
}

func locations(e *gqlerror.Error) []gqlerror.Location { return e.Locations }

func rejected(code string, errs gqlerror.List) *Result {
	r := &Result{Status: 400, Code: code}
	for _, e := range errs {
		r.Errors = append(r.Errors, &Error{Message: e.Message, Locations: locations(e)})
	}
	return r
}

// Execute runs req on s.
func (s *Schema) Execute(ctx context.Context, req Request, res Resolver) *Result {
	doc, perr := parser.ParseQuery(&ast.Source{Input: req.Query})
	if perr != nil {
		var ge *gqlerror.Error
		if errors.As(perr, &ge) {
			ge.Message = "Syntax Error: " + ge.Message
			if !strings.HasSuffix(ge.Message, ".") {
				ge.Message += "."
			}
			return rejected("GRAPHQL_PARSE_FAILED", gqlerror.List{ge})
		}
		return rejected("GRAPHQL_PARSE_FAILED", gqlerror.List{gqlerror.Errorf("Syntax Error: %s", perr)})
	}
	if errs := validator.ValidateWithRules(s.AST, doc, nil); len(errs) > 0 {
		fixStringLocations(doc, errs)
		return rejected("GRAPHQL_VALIDATION_FAILED", errs)
	}
	if !req.Introspection {
		if f := introspectionField(doc); f != nil {
			r := rejected("GRAPHQL_VALIDATION_FAILED", gqlerror.List{{
				Message:   "GraphQL introspection is not allowed by Apollo Server, but the query contained __schema or __type. To enable introspection, pass introspection: true to ApolloServer in production",
				Locations: []gqlerror.Location{{Line: f.Position.Line, Column: f.Position.Column}},
			}})
			r.Extensions = map[string]any{"validationErrorCode": "INTROSPECTION_DISABLED"}
			return r
		}
	}
	op, err := operation(doc, req.OperationName)
	if err != nil {
		return &Result{Status: 400, Code: "OPERATION_RESOLUTION_FAILURE", Errors: []*Error{{Message: err.Error()}}}
	}
	if r := missingVariables(op, req.Variables); r != nil {
		return r
	}
	vars, verr := validator.VariableValues(s.AST, op, req.Variables)
	if verr != nil {
		r := &Result{Status: 400, Code: "BAD_USER_INPUT"}
		var ge *gqlerror.Error
		if errors.As(verr, &ge) {
			r.Errors = []*Error{{Message: ge.Message, Locations: locations(ge)}}
		} else {
			r.Errors = []*Error{{Message: verr.Error()}}
		}
		return r
	}
	e := &executor{s: s, res: res, vars: vars, doc: doc}
	root := s.Def("Query")
	if op.Operation == ast.Mutation {
		root = s.Def("Mutation")
	}
	data, _ := e.selectionSet(ctx, root, nil, op.SelectionSet, nil, nil, nil)
	return &Result{Data: data, Errors: e.errors, Status: 200}
}

// fixStringLocations moves the errors located on a string value to its
// opening quote, where graphql-js puts them (gqlparser points past it).
func fixStringLocations(doc *ast.QueryDocument, errs gqlerror.List) {
	shift := map[[2]int]int{}
	var walkValue func(v *ast.Value)
	walkValue = func(v *ast.Value) {
		if v == nil || v.Position == nil {
			return
		}
		switch v.Kind {
		case ast.StringValue:
			shift[[2]int{v.Position.Line, v.Position.Column}] = 1
		case ast.BlockValue:
			shift[[2]int{v.Position.Line, v.Position.Column}] = 3
		}
		for _, c := range v.Children {
			walkValue(c.Value)
		}
	}
	var walk func(set ast.SelectionSet)
	walk = func(set ast.SelectionSet) {
		for _, sel := range set {
			switch x := sel.(type) {
			case *ast.Field:
				for _, a := range x.Arguments {
					walkValue(a.Value)
				}
				walk(x.SelectionSet)
			case *ast.InlineFragment:
				walk(x.SelectionSet)
			}
		}
	}
	for _, op := range doc.Operations {
		walk(op.SelectionSet)
	}
	for _, f := range doc.Fragments {
		walk(f.SelectionSet)
	}
	for _, e := range errs {
		for i, l := range e.Locations {
			if n, ok := shift[[2]int{l.Line, l.Column}]; ok {
				e.Locations[i].Column -= n
			}
		}
	}
}

// missingVariables gives graphql-js' messages for absent or null non-null
// variables.
func missingVariables(op *ast.OperationDefinition, vars map[string]any) *Result {
	for _, v := range op.VariableDefinitions {
		if !v.Type.NonNull || v.DefaultValue != nil {
			continue
		}
		val, ok := vars[v.Variable]
		msg := ""
		switch {
		case !ok:
			msg = fmt.Sprintf("Variable \"$%s\" of required type \"%s\" was not provided.", v.Variable, v.Type.String())
		case val == nil:
			msg = fmt.Sprintf("Variable \"$%s\" of non-null type \"%s\" must not be null.", v.Variable, v.Type.String())
		default:
			continue
		}
		return &Result{Status: 400, Code: "BAD_USER_INPUT", Errors: []*Error{{
			Message: msg, Locations: []gqlerror.Location{{Line: v.Position.Line, Column: v.Position.Column}},
		}}}
	}
	return nil
}

func introspectionField(doc *ast.QueryDocument) *ast.Field {
	var found *ast.Field
	var walk func(ast.SelectionSet)
	walk = func(set ast.SelectionSet) {
		for _, sel := range set {
			if found != nil {
				return
			}
			switch x := sel.(type) {
			case *ast.Field:
				if x.Name == "__schema" || x.Name == "__type" {
					found = x
					return
				}
				walk(x.SelectionSet)
			case *ast.InlineFragment:
				walk(x.SelectionSet)
			case *ast.FragmentSpread:
				if x.Definition != nil {
					walk(x.Definition.SelectionSet)
				}
			}
		}
	}
	for _, op := range doc.Operations {
		walk(op.SelectionSet)
	}
	return found
}

func operation(doc *ast.QueryDocument, name string) (*ast.OperationDefinition, error) {
	if name == "" {
		if len(doc.Operations) != 1 {
			return nil, message("Must provide operation name if query contains multiple operations.")
		}
		return doc.Operations[0], nil
	}
	op := doc.Operations.ForName(name)
	if op == nil {
		return nil, errorf("Unknown operation named %q.", name)
	}
	return op, nil
}

type executor struct {
	s      *Schema
	res    Resolver
	vars   map[string]any
	doc    *ast.QueryDocument
	errors []*Error
}

// errNull marks a value that became null because of an error on a
// non-null position: the parent takes the null.
var errNull = errors.New("null")

func (e *executor) fail(path []any, sel *ast.Field, err error) {
	ge := &Error{Message: err.Error(), Path: append([]any(nil), path...), Err: err}
	if sel != nil {
		ge.Locations = []gqlerror.Location{{Line: sel.Position.Line, Column: sel.Position.Column}}
	}
	e.errors = append(e.errors, ge)
}

func skipped(dirs ast.DirectiveList, vars map[string]any) bool {
	for _, d := range dirs {
		if d.Name != "skip" && d.Name != "include" {
			continue
		}
		v, _ := d.ArgumentMap(vars)["if"].(bool)
		if d.Name == "skip" && v || d.Name == "include" && !v {
			return true
		}
	}
	return false
}

// collect is CollectFields: the fields of set that apply to type typ.
func (e *executor) collect(typ string, set ast.SelectionSet, out []*ast.Field, seen map[string]bool) []*ast.Field {
	for _, sel := range set {
		switch x := sel.(type) {
		case *ast.Field:
			if !skipped(x.Directives, e.vars) {
				out = append(out, x)
			}
		case *ast.InlineFragment:
			if skipped(x.Directives, e.vars) || x.TypeCondition != "" && !e.applies(x.TypeCondition, typ) {
				continue
			}
			out = e.collect(typ, x.SelectionSet, out, seen)
		case *ast.FragmentSpread:
			if skipped(x.Directives, e.vars) || seen[x.Name] || x.Definition == nil {
				continue
			}
			seen[x.Name] = true
			if !e.applies(x.Definition.TypeCondition, typ) {
				continue
			}
			out = e.collect(typ, x.Definition.SelectionSet, out, seen)
		}
	}
	return out
}

func (e *executor) applies(cond, typ string) bool {
	if cond == typ {
		return true
	}
	d := e.s.AST.Types[cond]
	if d == nil || d.Kind != ast.Union && d.Kind != ast.Interface {
		return false
	}
	for _, t := range e.s.AST.GetPossibleTypes(d) {
		if t.Name == typ {
			return true
		}
	}
	return false
}

func (e *executor) selectionSet(ctx context.Context, typ *Def, parent any, set ast.SelectionSet, path []any, root *Field, rootArgs map[string]any) (*Object, error) {
	out := &Object{}
	fields := e.collect(typ.Name, set, nil, map[string]bool{})
	// Fields of the same response key are merged, as validated.
	byKey := map[string][]*ast.Field{}
	var order []string
	for _, f := range fields {
		k := f.Alias
		if k == "" {
			k = f.Name
		}
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], f)
	}
	for _, key := range order {
		sels := byKey[key]
		sel := sels[0]
		fpath := append(append([]any(nil), path...), key)
		if sel.Name == "__typename" {
			out.set(key, typ.Name)
			continue
		}
		var merged ast.SelectionSet
		for _, s := range sels {
			merged = append(merged, s.SelectionSet...)
		}
		v, err := e.field(ctx, typ, parent, sel, merged, fpath, root, rootArgs)
		if err != nil {
			return nil, errNull
		}
		out.set(key, v)
	}
	return out, nil
}

func (e *executor) coerceArgs(sel *ast.Field) map[string]any {
	if sel.Definition == nil {
		return map[string]any{}
	}
	args := map[string]any{}
	for _, def := range sel.Definition.Arguments {
		var val any
		has := false
		if a := sel.Arguments.ForName(def.Name); a != nil {
			if a.Value.Kind == ast.Variable {
				val, has = e.vars[a.Value.Raw]
				if !has && a.Value.VariableDefinition != nil && a.Value.VariableDefinition.DefaultValue != nil {
					val, _ = a.Value.VariableDefinition.DefaultValue.Value(e.vars)
					has = true
				}
			} else {
				val = e.literal(a.Value)
				has = true
			}
		}
		if !has && def.DefaultValue != nil {
			val = e.literal(def.DefaultValue)
			has = true
		}
		if has {
			args[def.Name] = e.input(val, def.Type)
		}
	}
	return args
}

// literal is the value of an AST value, variables included, keeping the
// difference between an absent object field and a null one.
func (e *executor) literal(v *ast.Value) any {
	switch v.Kind {
	case ast.Variable:
		return e.vars[v.Raw]
	case ast.IntValue:
		n, err := strconv.ParseInt(v.Raw, 10, 64)
		if err != nil {
			f, _ := strconv.ParseFloat(v.Raw, 64)
			return f
		}
		return n
	case ast.FloatValue:
		f, _ := strconv.ParseFloat(v.Raw, 64)
		return f
	case ast.BooleanValue:
		return v.Raw == "true"
	case ast.NullValue:
		return nil
	case ast.ListValue:
		out := []any{}
		for _, c := range v.Children {
			out = append(out, e.literal(c.Value))
		}
		return out
	case ast.ObjectValue:
		out := map[string]any{}
		for _, c := range v.Children {
			if c.Value.Kind == ast.Variable {
				if _, ok := e.vars[c.Value.Raw]; !ok {
					continue
				}
			}
			out[c.Name] = e.literal(c.Value)
		}
		return out
	}
	return v.Raw
}

// input converts enum names to their values, following the input types.
func (e *executor) input(v any, t *ast.Type) any {
	if v == nil {
		return nil
	}
	if t.Elem != nil {
		list, ok := v.([]any)
		if !ok {
			return []any{e.input(v, t.Elem)}
		}
		out := make([]any, len(list))
		for i, x := range list {
			out[i] = e.input(x, t.Elem)
		}
		return out
	}
	d := e.s.defs[t.NamedType]
	if d == nil {
		if t.NamedType == "Int" {
			if f, ok := v.(float64); ok {
				return int64(f)
			}
			if n, ok := v.(json.Number); ok {
				i, _ := n.Int64()
				return i
			}
		}
		return v
	}
	switch d.Kind {
	case DefEnum:
		s, _ := v.(string)
		for _, ev := range d.Values {
			if ev.Name == s {
				return ev.Value
			}
		}
		return v
	case DefInput:
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		def := e.s.AST.Types[d.Name]
		out := map[string]any{}
		for k, x := range m {
			if fd := def.Fields.ForName(k); fd != nil {
				out[k] = e.input(x, fd.Type)
			} else {
				out[k] = x
			}
		}
		for _, fd := range def.Fields {
			if _, ok := out[fd.Name]; !ok && fd.DefaultValue != nil {
				out[fd.Name] = e.input(e.literal(fd.DefaultValue), fd.Type)
			}
		}
		return out
	}
	return v
}

// valueOf is the default resolver: the parent's property.
func valueOf(parent any, name string) any {
	switch p := parent.(type) {
	case map[string]any:
		return p[name]
	case interface{ Get(string) (any, bool) }:
		v, _ := p.Get(name)
		return v
	}
	return nil
}

func (e *executor) field(ctx context.Context, typ *Def, parent any, sel *ast.Field, set ast.SelectionSet, path []any, root *Field, rootArgs map[string]any) (any, error) {
	if sel.Name == "__schema" || sel.Name == "__type" {
		v := e.introspect(sel)
		return e.complete(ctx, sel.Definition.Type, v, sel, set, path, root, rootArgs)
	}
	meta := typ.Field(sel.Name)
	args := e.coerceArgs(sel)
	if root == nil {
		root, rootArgs = meta, args
	}
	var v any
	if meta == nil || meta.Kind == FieldValue {
		v = valueOf(parent, sel.Name)
	} else {
		var err error
		v, err = e.res.Resolve(ctx, &FieldContext{Parent: parent, Args: args, Type: typ, Field: meta, Selection: sel, Path: path, Root: root, RootArgs: rootArgs})
		if err != nil {
			e.fail(path, sel, err)
			if sel.Definition.Type.NonNull {
				return nil, errNull
			}
			return nil, nil
		}
	}
	out, err := e.complete(ctx, sel.Definition.Type, v, sel, set, path, root, rootArgs)
	if err != nil {
		if sel.Definition.Type.NonNull {
			return nil, errNull
		}
		return nil, nil
	}
	return out, nil
}

func (e *executor) complete(ctx context.Context, t *ast.Type, v any, sel *ast.Field, set ast.SelectionSet, path []any, root *Field, rootArgs map[string]any) (any, error) {
	if t.NonNull {
		inner := *t
		inner.NonNull = false
		out, err := e.complete(ctx, &inner, v, sel, set, path, root, rootArgs)
		if err != nil {
			return nil, err
		}
		if out == nil {
			e.fail(path, sel, errorf("Cannot return null for non-nullable field %s.%s.", sel.ObjectDefinition.Name, sel.Name))
			return nil, errNull
		}
		return out, nil
	}
	if isNil(v) {
		return nil, nil
	}
	if t.Elem != nil {
		list := toList(v)
		out := make([]any, len(list))
		for i, x := range list {
			c, err := e.complete(ctx, t.Elem, x, sel, set, append(append([]any(nil), path...), i), root, rootArgs)
			if err != nil {
				if t.Elem.NonNull {
					return nil, errNull
				}
				c = nil
			}
			out[i] = c
		}
		return out, nil
	}
	d := e.s.AST.Types[t.NamedType]
	switch d.Kind {
	case ast.Scalar:
		out, err := serialize(t.NamedType, v)
		if err != nil {
			e.fail(path, sel, err)
			return nil, errNull
		}
		return out, nil
	case ast.Enum:
		def := e.s.defs[t.NamedType]
		s := fmt.Sprint(v)
		if def != nil {
			for _, ev := range def.Values {
				if ev.Value == s {
					return ev.Name, nil
				}
			}
		} else if d.EnumValues.ForName(s) != nil {
			return s, nil
		}
		e.fail(path, sel, errorf("Enum %q cannot represent value: %s", t.NamedType, jsonString(v)))
		return nil, errNull
	case ast.Union, ast.Interface:
		name := e.res.TypeOf(e.s.defs[t.NamedType], v)
		od := e.s.defs[name]
		if od == nil {
			e.fail(path, sel, errorf("Abstract type %q must resolve to an Object type at runtime for field %q.%q. Either the %q type should provide a \"resolveType\" function or each possible type should provide an \"isTypeOf\" function.", t.NamedType, sel.ObjectDefinition.Name, sel.Name, t.NamedType))
			return nil, errNull
		}
		return e.selectionSet(ctx, od, v, set, path, root, rootArgs)
	}
	od := e.s.defs[t.NamedType]
	if od == nil {
		od = introDef(t.NamedType)
	}
	return e.selectionSet(ctx, od, v, set, path, root, rootArgs)
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case map[string]any:
		return x == nil
	case []any:
		return x == nil
	}
	return false
}

func toList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, len(x))
		for i, m := range x {
			out[i] = m
		}
		return out
	}
	return []any{v}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func serialize(scalar string, v any) (any, error) {
	switch scalar {
	case "Int":
		f, ok := toFloat(v)
		if !ok {
			if b, isBool := v.(bool); isBool {
				if b {
					return 1, nil
				}
				return 0, nil
			}
			if s, isStr := v.(string); isStr {
				if n, err := strconv.ParseFloat(s, 64); err == nil {
					f, ok = n, true
				}
			}
		}
		if !ok || f != math.Trunc(f) {
			return nil, errorf("Int cannot represent non-integer value: %s", jsonString(v))
		}
		if f > math.MaxInt32 || f < math.MinInt32 {
			return nil, errorf("Int cannot represent non 32-bit signed integer value: %s", jsonString(v))
		}
		return int64(f), nil
	case "Float":
		f, ok := toFloat(v)
		if !ok {
			if s, isStr := v.(string); isStr {
				if n, err := strconv.ParseFloat(s, 64); err == nil {
					return n, nil
				}
			}
			return nil, errorf("Float cannot represent non numeric value: %s", jsonString(v))
		}
		return f, nil
	case "String", "ID":
		switch x := v.(type) {
		case string:
			return x, nil
		case bool:
			return strconv.FormatBool(x), nil
		}
		if f, ok := toFloat(v); ok {
			return strconv.FormatFloat(f, 'f', -1, 64), nil
		}
		return nil, errorf("%s cannot represent value: %s", scalar, jsonString(v))
	case "Boolean":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if f, ok := toFloat(v); ok {
			return f != 0, nil
		}
		return nil, errorf("Boolean cannot represent a non boolean value: %s", jsonString(v))
	case "Long":
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n <= 1<<53 && n >= -(1<<53) {
				return n, nil
			}
		}
		return v, nil
	}
	return v, nil
}
