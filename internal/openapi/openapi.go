// Package openapi generates the content API's OpenAPI 3.1 document like
// Strapi 5.59's @strapi/openapi (server.openapi): the route schemas of the
// core API, as zod's toJSONSchema writes them, plus the plugins' routes.
package openapi

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/aluzed/grapi/schema"
)

// The plugins' routes do not depend on the project: they are Strapi's.
//
//go:embed plugins.json
var pluginsJSON []byte

// Info is the document's info block, from the project's package.json.
type Info struct{ Name, Version string }

// Options of Generate.
type Options struct {
	Info Info
	// Served tells whether the server answers method and path (OpenAPI
	// form, "/users/{id}"); the plugin routes it does not serve are left out.
	// Nil keeps them all.
	Served func(method, path string) bool
	// Now is the time datetime defaults are evaluated at.
	Now time.Time
	// Custom are the operations the project's routes declare.
	Custom []Operation
}

// Operation is an OpenAPI operation of a custom route; Path is in OpenAPI
// form, relative to the API prefix ("/accounts/{id}").
type Operation struct {
	Method, Path string
	Spec         map[string]any
}

type obj = map[string]any

const (
	jsonSchema  = "https://json-schema.org/draft/2020-12/schema"
	uuidPattern = "^([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}|00000000-0000-0000-0000-000000000000|ffffffff-ffff-ffff-ffff-ffffffffffff)$"
	emailPat    = "^(?!\\.)(?!.*\\.\\.)([A-Za-z0-9_'+\\-\\.]*)[A-Za-z0-9_+-]@([A-Za-z0-9][A-Za-z0-9\\-]*\\.)+[A-Za-z]{2,}$"
	maxSafe     = 9007199254740991
)

type generator struct {
	set        *schema.Set
	now        string
	components obj
	building   map[string]bool
}

// Generate builds the document of set.
func Generate(set *schema.Set, opts Options) map[string]any {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	g := &generator{set: set, now: opts.Now.UTC().Format("2006-01-02T15:04:05.000Z"), components: obj{}, building: map[string]bool{}}
	paths := obj{}
	for uid, ct := range set.ContentTypes.All() {
		if schema.UID(uid).Namespace() != schema.NamespaceAPI {
			continue
		}
		g.contentTypePaths(paths, ct)
	}
	var plugins map[string]map[string]any
	_ = json.Unmarshal(pluginsJSON, &plugins)
	for p, ops := range plugins {
		for m, op := range ops {
			if opts.Served != nil && !opts.Served(strings.ToUpper(m), p) {
				continue
			}
			if paths[p] == nil {
				paths[p] = obj{}
			}
			paths[p].(obj)[m] = op
		}
	}
	for _, op := range opts.Custom {
		if paths[op.Path] == nil {
			paths[op.Path] = obj{}
		}
		paths[op.Path].(obj)[strings.ToLower(op.Method)] = op.Spec
	}
	name, version := opts.Info.Name, opts.Info.Version
	return obj{
		"openapi": "3.1.0",
		"info":    obj{"title": name, "description": "API documentation for " + name + " v" + version, "version": version},
		"servers": []any{obj{"url": "", "description": "Default server"}},
		"components": obj{
			"securitySchemes": obj{"bearerAuth": obj{"type": "http", "scheme": "bearer", "bearerFormat": "JWT", "description": "JWT Bearer token for authentication"}},
			"schemas":         g.components,
		},
		"security": []any{obj{"bearerAuth": []any{}}},
		"paths":    paths,
	}
}

// refName is transformUidToValidOpenApiName.
func refName(uid string) string {
	pascal := func(s string) string {
		var b strings.Builder
		for _, p := range regexp.MustCompile(`[-_]`).Split(s, -1) {
			if p != "" {
				b.WriteString(strings.ToUpper(p[:1]) + p[1:])
			}
		}
		return b.String()
	}
	if ns, rest, ok := strings.Cut(uid, "::"); ok {
		var b strings.Builder
		b.WriteString(pascal(ns))
		for _, p := range strings.Split(rest, ".") {
			b.WriteString(pascal(p))
		}
		return b.String() + "Document"
	}
	var b strings.Builder
	for _, p := range strings.Split(uid, ".") {
		b.WriteString(pascal(p))
	}
	return b.String() + "Entry"
}

// ref registers the schema of a model in the components and points to it.
func (g *generator) ref(uid string) obj {
	name := refName(uid)
	if _, ok := g.components[name]; !ok && !g.building[name] {
		g.building[name] = true
		var s obj
		if schema.UID(uid).IsComponent() {
			c, _ := g.set.Component(schema.UID(uid))
			s = g.entry(&c.Attributes, nil)
		} else {
			ct, _ := g.set.ContentType(schema.UID(uid))
			s = g.document(ct)
		}
		s["$schema"] = jsonSchema
		g.components[name] = s
		delete(g.building, name)
	}
	return obj{"$ref": "#/components/schemas/" + name}
}

func isScalar(a *schema.Attribute) bool {
	switch a.Type {
	case schema.TypeRelation, schema.TypeComponent, schema.TypeDynamicZone, schema.TypeMedia:
		return false
	}
	return true
}

type field struct {
	name string
	a    *schema.Attribute
}

// fields are the public attributes, scalars first (the validators'
// {...scalarFields, ...populatableFields}).
func fields(attrs *schema.OrderedMap[*schema.Attribute]) (scalars, populatable []field) {
	for name, a := range attrs.All() {
		if a.IsPrivate() {
			continue
		}
		if isScalar(a) {
			scalars = append(scalars, field{name, a})
		} else {
			populatable = append(populatable, field{name, a})
		}
	}
	return scalars, populatable
}

func names(fs []field) []any {
	out := make([]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.name)
	}
	return out
}

// object is z.object(shape).toJSONSchema(): the required keys are the ones
// neither optional nor without default.
func object(props []string, schemas map[string]obj, required map[string]bool) obj {
	p := obj{}
	var req []any
	for _, name := range props {
		p[name] = schemas[name]
		if required[name] {
			req = append(req, name)
		}
	}
	out := obj{"type": "object", "properties": p, "additionalProperties": false}
	if len(req) > 0 {
		out["required"] = req
	}
	return out
}

func (g *generator) entry(attrs *schema.OrderedMap[*schema.Attribute], head []field) obj {
	scalars, populatable := fields(attrs)
	all := append(append(append([]field(nil), head...), scalars...), populatable...)
	var props []string
	schemas := map[string]obj{}
	required := map[string]bool{}
	for _, f := range all {
		s, req := g.attribute(f.name, f.a)
		props = append(props, f.name)
		schemas[f.name] = s
		required[f.name] = req
	}
	return object(props, schemas, required)
}

var (
	documentIDAttr = &schema.Attribute{Type: "documentId"}
	idAttr         = &schema.Attribute{Type: "id"}
)

func (g *generator) document(ct *schema.ContentType) obj {
	head := []field{{"documentId", documentIDAttr}, {"id", idAttr}}
	attrs := schema.OrderedMap[*schema.Attribute]{}
	for name, a := range ct.Attributes.All() {
		if a.Type != schema.TypePassword {
			attrs.Set(name, a)
		}
	}
	return g.entry(&attrs, head)
}

func documentIDSchema() obj {
	return obj{"type": "string", "format": "uuid", "pattern": uuidPattern, "description": "The document ID, represented by a UUID"}
}

func numberOf(raw json.RawMessage) (float64, bool) {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	return f, true
}

func defaultOf(name string, a *schema.Attribute, now string) (any, bool) {
	if name == schema.AttrPublishedAt {
		return now, true
	}
	if len(a.Default) == 0 {
		return nil, false
	}
	var v any
	if json.Unmarshal(a.Default, &v) != nil {
		return nil, false
	}
	return v, true
}

// toMany is isAnyToMany: morph relations are not.
func toMany(a *schema.Attribute) bool {
	if a.Type == schema.TypeMedia {
		return a.IsMultiple()
	}
	return a.Relation == schema.OneToMany || a.Relation == schema.ManyToMany
}

// attribute is mapAttributeToSchema; req tells whether the key is required.
func (g *generator) attribute(name string, a *schema.Attribute) (obj, bool) {
	required := a.IsRequired()
	var s obj
	desc := ""
	minMax := func(lo, hi json.RawMessage, kmin, kmax string) {
		l, okl := numberOf(lo)
		h, okh := numberOf(hi)
		if okl && okh {
			s[kmin], s[kmax] = l, h
		}
	}
	lengths := func() {
		if a.MinLength != nil && a.MaxLength != nil {
			s["minLength"], s["maxLength"] = *a.MinLength, *a.MaxLength
		}
	}
	switch a.Type {
	case "documentId":
		return documentIDSchema(), true
	case "id":
		return obj{"anyOf": []any{obj{"type": "string"}, obj{"type": "number"}}}, true
	case schema.TypeBigInteger:
		s, desc = obj{"type": "string"}, "A biginteger field"
		if v, ok := numberOf(a.Min); ok {
			s["minLength"] = v
		}
		if v, ok := numberOf(a.Max); ok {
			s["maxLength"] = v
		}
	case schema.TypeBlocks:
		return obj{"type": "array", "items": obj{}, "description": "A blocks field"}, true
	case schema.TypeBoolean:
		s, desc = obj{"anyOf": []any{obj{"type": "boolean"}, obj{"type": "null"}}}, "A boolean field"
	case schema.TypeComponent:
		desc = "A component field"
		r := g.ref(a.Component)
		if a.IsRepeatable() {
			s = obj{"type": "array", "items": r}
			if v, ok := numberOf(a.Min); ok {
				s["minItems"] = v
			}
			if v, ok := numberOf(a.Max); ok {
				s["maxItems"] = v
			}
		} else {
			s = r
		}
	case schema.TypeDate:
		s, desc = obj{"type": "string"}, "A date field"
	case schema.TypeDateTime:
		s, desc = obj{"type": "string"}, "A datetime field"
	case schema.TypeDecimal, schema.TypeFloat:
		s, desc = obj{"type": "number"}, "A "+a.Type+" field"
		minMax(a.Min, a.Max, "minimum", "maximum")
	case schema.TypeDynamicZone:
		s, desc = obj{"type": "array", "items": obj{}}, "A dynamic zone field"
		minMax(a.Min, a.Max, "minItems", "maxItems")
	case schema.TypeEmail:
		s, desc = obj{"type": "string", "format": "email", "pattern": emailPat}, "An email field"
		lengths()
	case schema.TypeEnumeration:
		enum := make([]any, len(a.Enum))
		for i, e := range a.Enum {
			enum[i] = e
		}
		s, desc = obj{"type": "string", "enum": enum}, "An enum field"
	case schema.TypeInteger:
		s, desc = obj{"type": "integer", "minimum": float64(-maxSafe), "maximum": float64(maxSafe)}, "An integer field"
		minMax(a.Min, a.Max, "minimum", "maximum")
	case schema.TypeJSON:
		s, desc = obj{}, "A JSON field"
	case schema.TypeMedia:
		desc = "A media field"
		r := g.ref("plugin::upload.file")
		s = r
		if a.IsMultiple() {
			s = obj{"type": "array", "items": r}
		}
	case schema.TypeRelation:
		if a.Target == "" {
			return obj{}, true
		}
		desc = "A relational field"
		r := g.ref(a.Target)
		s = r
		if toMany(a) {
			s = obj{"type": "array", "items": r}
		}
	case schema.TypeText, schema.TypeRichText, schema.TypeString, schema.TypePassword:
		s, desc = obj{"type": "string"}, "A "+a.Type+" field"
		lengths()
	case schema.TypeTime:
		s, desc = obj{"type": "string"}, "A time field"
	case schema.TypeTimestamp:
		s, desc = obj{"anyOf": []any{obj{"type": "string"}, obj{"type": "number"}}}, "A timestamp field"
	case schema.TypeUID:
		s, desc = obj{"type": "string"}, "A UID field"
		lengths()
	default:
		return obj{}, false
	}
	hasDefault := false
	switch a.Type {
	case schema.TypeComponent, schema.TypeDynamicZone, schema.TypeMedia, schema.TypeRelation:
	default:
		if v, ok := defaultOf(name, a, g.now); ok {
			hasDefault = true
			s = withKey(s, "default", v)
		}
	}
	if !a.IsWritable() {
		s = withKey(s, "readOnly", true)
	}
	s = withKey(s, "description", desc)
	return s, required || hasDefault
}

// withKey adds k to s; a $ref gets its siblings next to it, as zod does.
func withKey(s obj, k string, v any) obj {
	out := obj{}
	for kk, vv := range s {
		out[kk] = vv
	}
	out[k] = v
	return out
}
