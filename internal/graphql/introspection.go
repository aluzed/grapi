package graphql

import (
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// introDef is a definition without metadata: every field is the property
// of its parent.
func introDef(name string) *Def { return &Def{Kind: DefObject, Name: name} }

func (e *executor) introspect(sel *ast.Field) any {
	in := &introspector{s: e.s.AST, types: map[string]map[string]any{}}
	if sel.Name == "__type" {
		name, _ := sel.ArgumentMap(e.vars)["name"].(string)
		if e.s.AST.Types[name] == nil {
			return nil
		}
		return in.named(name)
	}
	var types []any
	for _, name := range in.order() {
		types = append(types, in.named(name))
	}
	var dirs []any
	for _, d := range e.s.AST.Directives {
		dirs = append(dirs, in.directive(d))
	}
	out := map[string]any{"description": nil, "types": types, "directives": dirs, "subscriptionType": nil}
	out["queryType"] = in.named(e.s.AST.Query.Name)
	if e.s.AST.Mutation != nil {
		out["mutationType"] = in.named(e.s.AST.Mutation.Name)
	} else {
		out["mutationType"] = nil
	}
	return out
}

type introspector struct {
	s     *ast.Schema
	types map[string]map[string]any
}

// order is graphql-js' type map order: the schema types first, then the
// introspection ones.
func (in *introspector) order() []string {
	var user, builtin []string
	seen := map[string]bool{}
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if in.s.Types[name].BuiltIn {
			builtin = append(builtin, name)
		} else {
			user = append(user, name)
		}
	}
	for name := range in.s.Types {
		add(name)
	}
	sort.Strings(user)
	sort.Strings(builtin)
	return append(user, builtin...)
}

func desc(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (in *introspector) ref(t *ast.Type) map[string]any {
	if t.NonNull {
		inner := *t
		inner.NonNull = false
		return map[string]any{"kind": "NON_NULL", "name": nil, "ofType": in.ref(&inner)}
	}
	if t.Elem != nil {
		return map[string]any{"kind": "LIST", "name": nil, "ofType": in.ref(t.Elem)}
	}
	return in.named(t.NamedType)
}

func (in *introspector) inputValue(name, description string, t *ast.Type, def *ast.Value) map[string]any {
	var dv any
	if def != nil {
		dv = def.String()
	}
	return map[string]any{"name": name, "description": desc(description), "type": in.ref(t), "defaultValue": dv, "isDeprecated": false, "deprecationReason": nil}
}

func (in *introspector) named(name string) map[string]any {
	if t, ok := in.types[name]; ok {
		return t
	}
	d := in.s.Types[name]
	t := map[string]any{
		"kind": string(d.Kind), "name": d.Name, "description": desc(d.Description), "specifiedByURL": nil, "isOneOf": false,
		"fields": nil, "inputFields": nil, "interfaces": nil, "enumValues": nil, "possibleTypes": nil, "ofType": nil,
	}
	in.types[name] = t
	switch d.Kind {
	case ast.Object, ast.Interface:
		fields := []any{}
		for _, f := range d.Fields {
			if strings.HasPrefix(f.Name, "__") {
				continue
			}
			args := []any{}
			for _, a := range f.Arguments {
				args = append(args, in.inputValue(a.Name, a.Description, a.Type, a.DefaultValue))
			}
			fields = append(fields, map[string]any{"name": f.Name, "description": desc(f.Description), "args": args, "type": in.ref(f.Type), "isDeprecated": false, "deprecationReason": nil})
		}
		t["fields"] = fields
		t["interfaces"] = []any{}
	case ast.InputObject:
		fields := []any{}
		for _, f := range d.Fields {
			fields = append(fields, in.inputValue(f.Name, f.Description, f.Type, f.DefaultValue))
		}
		t["inputFields"] = fields
	case ast.Enum:
		values := []any{}
		for _, v := range d.EnumValues {
			values = append(values, map[string]any{"name": v.Name, "description": desc(v.Description), "isDeprecated": false, "deprecationReason": nil})
		}
		t["enumValues"] = values
	case ast.Union:
		types := []any{}
		for _, m := range d.Types {
			types = append(types, in.named(m))
		}
		t["possibleTypes"] = types
	}
	return t
}

func (in *introspector) directive(d *ast.DirectiveDefinition) map[string]any {
	args := []any{}
	for _, a := range d.Arguments {
		args = append(args, in.inputValue(a.Name, a.Description, a.Type, a.DefaultValue))
	}
	locs := []any{}
	for _, l := range d.Locations {
		locs = append(locs, string(l))
	}
	return map[string]any{"name": d.Name, "description": desc(d.Description), "locations": locs, "args": args, "isRepeatable": d.IsRepeatable}
}
