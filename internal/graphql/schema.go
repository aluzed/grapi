// Package graphql is the GraphQL plugin (@strapi/plugin-graphql 5.59): the
// schema Strapi's shadow CRUD derives from the content types, and an executor
// on top of gqlparser. The resolvers live in httpapi, next to the REST ones.
package graphql

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/aluzed/grapi/schema"
)

// DefKind is the kind of a schema definition.
type DefKind int

// Definition kinds.
const (
	DefScalar DefKind = iota
	DefObject
	DefInput
	DefEnum
	DefUnion
)

// FieldKind tells the resolvers what a field is.
type FieldKind int

// Field kinds.
const (
	// FieldValue reads the parent's value under the field name.
	FieldValue FieldKind = iota
	FieldFindOne
	FieldFind
	FieldFindConnection
	FieldSingleFind
	FieldCreate
	FieldUpdate
	FieldDelete
	FieldSingleUpdate
	FieldSingleDelete
	FieldRelation
	FieldRelationList
	FieldRelationConnection
	FieldMorph
	FieldComponent
	FieldDynamicZone
	FieldNodes
	FieldPageInfo
	FieldEntityData
	FieldCustom
)

// Arg is a field argument.
type Arg struct{ Name, Type, Default, Description string }

// Field is an object or input field.
type Field struct {
	Name, Type, Description string
	Args                    []Arg
	Kind                    FieldKind
	// UID is the model owning Attr, or the content type a root field works on.
	UID    schema.UID
	Attr   string
	Target schema.UID
	// Scope is the permission the field needs. Root fields without a scope
	// still need an authenticated request unless NoAuth.
	Scope  string
	NoAuth bool
	// Default is an input field's default value.
	Default string
}

// EnumValue maps a GraphQL enum value to the stored one.
type EnumValue struct{ Name, Value string }

// Def is one type of the schema.
type Def struct {
	Kind              DefKind
	Name, Description string
	Fields            []*Field
	Values            []EnumValue
	Members           []string
	UID               schema.UID
	fields            map[string]*Field
}

// Field returns the field name of d.
func (d *Def) Field(name string) *Field { return d.fields[name] }

func (d *Def) add(f *Field) *Field {
	if d.fields == nil {
		d.fields = map[string]*Field{}
	}
	if _, ok := d.fields[f.Name]; ok {
		return d.fields[f.Name]
	}
	d.Fields = append(d.Fields, f)
	d.fields[f.Name] = f
	return f
}

// Schema is the generated schema.
type Schema struct {
	Defs []*Def
	AST  *ast.Schema
	SDL  string
	defs map[string]*Def
	// typeOf is the object type name of each model.
	typeOf map[schema.UID]string
	// componentOf is the component of each object type name.
	componentOf map[string]schema.UID
	set         *schema.Set
}

// Def returns the definition named name.
func (s *Schema) Def(name string) *Def { return s.defs[name] }

// TypeName is the object type name of a model.
func (s *Schema) TypeName(uid schema.UID) string { return s.typeOf[uid] }

// ComponentOf is the component whose object type is name.
func (s *Schema) ComponentOf(name string) (schema.UID, bool) {
	uid, ok := s.componentOf[name]
	return uid, ok
}

// PublicationFilters maps the PublicationFilter values to the document
// service's publicationFilter.
var PublicationFilters = []EnumValue{
	{"NEVER_PUBLISHED", "never-published"},
	{"HAS_PUBLISHED_VERSION", "has-published-version"},
	{"MODIFIED", "modified"},
	{"UNMODIFIED", "unmodified"},
	{"NEVER_PUBLISHED_DOCUMENT", "never-published-document"},
	{"HAS_PUBLISHED_VERSION_DOCUMENT", "has-published-version-document"},
	{"PUBLISHED_WITHOUT_DRAFT", "published-without-draft"},
	{"PUBLISHED_WITH_DRAFT", "published-with-draft"},
}

var scalarOf = map[string]string{
	schema.TypeUID: "String", schema.TypeEmail: "String", schema.TypePassword: "String", schema.TypeText: "String",
	schema.TypeBoolean: "Boolean", schema.TypeInteger: "Int", schema.TypeString: "String", schema.TypeEnumeration: "String",
	schema.TypeRichText: "String", schema.TypeBlocks: "JSON", schema.TypeBigInteger: "Long", schema.TypeFloat: "Float",
	schema.TypeDecimal: "Float", schema.TypeJSON: "JSON", schema.TypeDate: "Date", schema.TypeTime: "Time",
	schema.TypeDateTime: "DateTime", schema.TypeTimestamp: "DateTime",
}

var filterOperators = []string{
	"and", "or", "not", "eq", "eqi", "ne", "nei", "startsWith", "endsWith", "contains", "notContains",
	"containsi", "notContainsi", "gt", "gte", "lt", "lte", "null", "notNull", "in", "notIn", "between",
}

// FilterOperators maps the GraphQL filter operators to Strapi's.
var FilterOperators = func() map[string]string {
	m := map[string]string{}
	for _, op := range filterOperators {
		m[op] = "$" + op
	}
	return m
}()

const (
	fileUID       = schema.UID("plugin::upload.file")
	folderUID     = schema.UID("plugin::upload.folder")
	localeUID     = schema.UID("plugin::i18n.locale")
	userUID       = schema.UID("plugin::users-permissions.user")
	roleUID       = schema.UID("plugin::users-permissions.role")
	permissionUID = schema.UID("plugin::users-permissions.permission")
)

type model struct {
	uid        schema.UID
	ct         *schema.ContentType
	attrs      *schema.OrderedMap[*schema.Attribute]
	component  bool
	localized  bool
	singleType bool
}

type builder struct {
	set      *schema.Set
	out      *Schema
	query    *Def
	mutation *Def
	i18n     bool
	models   []model
}

// Build derives the schema of set.
func Build(set *schema.Set) (*Schema, error) {
	b := &builder{set: set, out: &Schema{defs: map[string]*Def{}, typeOf: map[schema.UID]string{}, componentOf: map[string]schema.UID{}, set: set}}
	_, b.i18n = set.ContentType(localeUID)
	b.scalars()
	b.internals()
	b.query = &Def{Kind: DefObject, Name: "Query"}
	b.mutation = &Def{Kind: DefObject, Name: "Mutation"}
	for uid, c := range set.Components.All() {
		b.models = append(b.models, model{uid: schema.UID(uid), attrs: &c.Attributes, component: true})
		b.out.typeOf[schema.UID(uid)] = c.GlobalID
		b.out.componentOf[c.GlobalID] = schema.UID(uid)
	}
	for uid, ct := range set.ContentTypes.All() {
		u := schema.UID(uid)
		if b.disabled(u) {
			continue
		}
		b.models = append(b.models, model{uid: u, ct: ct, attrs: &ct.Attributes, localized: ct.Localized(), singleType: ct.IsSingleType()})
		b.out.typeOf[u] = typeName(ct, false)
	}
	var members []string
	for _, m := range b.models {
		b.register(m)
		members = append(members, b.out.typeOf[m.uid])
	}
	b.def(&Def{Kind: DefUnion, Name: "GenericMorph", Members: members})
	b.extensions()
	b.def(&Def{Kind: DefInput, Name: "PaginationArg", Fields: []*Field{
		{Name: "page", Type: "Int"}, {Name: "pageSize", Type: "Int"}, {Name: "start", Type: "Int"}, {Name: "limit", Type: "Int"},
	}})
	b.def(b.query)
	b.def(b.mutation)
	b.prune()
	b.out.SDL = b.out.print()
	s, err := gqlparser.LoadSchema(&ast.Source{Name: "grapi", Input: b.out.SDL})
	if err != nil {
		return nil, fmt.Errorf("graphql schema: %w", err)
	}
	b.out.AST = s
	return b.out, nil
}

func (b *builder) def(d *Def) *Def {
	if old, ok := b.out.defs[d.Name]; ok {
		return old
	}
	if d.fields == nil {
		fields := d.Fields
		d.Fields = nil
		for _, f := range fields {
			d.add(f)
		}
	}
	b.out.defs[d.Name] = d
	b.out.Defs = append(b.out.Defs, d)
	return d
}

// typeName is getTypeName: the plugin then the singular (or plural) name.
func typeName(ct *schema.ContentType, plural bool) string {
	name := ct.Info.SingularName
	if plural {
		name = ct.Info.PluralName
	}
	prefix := ""
	if ct.UID.Namespace() == schema.NamespacePlugin {
		prefix = schema.PascalCase(ct.UID.Origin())
	}
	return prefix + schema.PascalCase(name)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// disabled is the shadow CRUD of the content types nothing exposes.
func (b *builder) disabled(uid schema.UID) bool {
	return uid.Namespace() == schema.NamespaceAdmin || uid == folderUID
}

func (b *builder) queriesEnabled(uid schema.UID) bool { return uid != permissionUID }

func (b *builder) mutationsEnabled(uid schema.UID) bool {
	switch uid {
	case permissionUID, fileUID, localeUID, userUID, roleUID:
		return false
	}
	return true
}

func (b *builder) inputEnabled(m model, name string) bool {
	return !m.localized || (name != schema.AttrLocale && name != schema.AttrLocalizations)
}

func (b *builder) enabled(uid schema.UID) bool {
	if uid.IsComponent() {
		_, ok := b.set.Component(uid)
		return ok
	}
	_, ok := b.set.ContentType(uid)
	return ok && !b.disabled(uid)
}

func (b *builder) scalars() {
	for _, s := range []struct{ name, desc string }{
		{"JSON", "The `JSON` scalar type represents JSON values as specified by [ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf)."},
		{"DateTime", "A date-time string at UTC, such as 2007-12-03T10:15:30Z, compliant with the `date-time` format outlined in section 5.6 of the RFC 3339 profile of the ISO 8601 standard for representation of dates and times using the Gregorian calendar."},
		{"Time", "A time string with format HH:mm:ss.SSS"},
		{"Date", "A date string, such as 2007-12-03, compliant with the `full-date` format outlined in section 5.6 of the RFC 3339 profile of the ISO 8601 standard for representation of dates and times using the Gregorian calendar."},
		{"Long", "The `BigInt` scalar type represents non-fractional signed whole numeric values."},
	} {
		b.def(&Def{Kind: DefScalar, Name: s.name, Description: s.desc})
	}
}

func (b *builder) internals() {
	b.def(&Def{Kind: DefObject, Name: "Error", Fields: []*Field{{Name: "code", Type: "String!"}, {Name: "message", Type: "String"}}})
	b.def(&Def{Kind: DefObject, Name: "Pagination", Fields: []*Field{
		{Name: "total", Type: "Int!"}, {Name: "page", Type: "Int!"}, {Name: "pageSize", Type: "Int!"}, {Name: "pageCount", Type: "Int!"},
	}})
	b.def(&Def{Kind: DefObject, Name: "DeleteMutationResponse", Fields: []*Field{{Name: "documentId", Type: "ID!"}}})
	b.def(&Def{Kind: DefEnum, Name: "PublicationStatus", Values: []EnumValue{{"DRAFT", "draft"}, {"PUBLISHED", "published"}}})
	b.def(&Def{Kind: DefEnum, Name: "PublicationFilter", Values: PublicationFilters})
	for _, s := range []string{"ID", "Boolean", "String", "Int", "Long", "Float", "Date", "Time", "DateTime", "JSON"} {
		d := &Def{Kind: DefInput, Name: s + "FilterInput"}
		for _, op := range filterOperators {
			t := s
			switch op {
			case "and", "or", "in", "notIn", "between":
				t = "[" + s + "]"
			case "not":
				t = s + "FilterInput"
			case "null", "notNull":
				t = "Boolean"
			}
			d.Fields = append(d.Fields, &Field{Name: op, Type: t})
		}
		b.def(d)
	}
}

func (b *builder) enumName(m model, attr string, a *schema.Attribute) string {
	if a.EnumName != "" {
		return a.EnumName
	}
	return "ENUM_" + strings.ToUpper(b.out.typeOf[m.uid]) + "_" + strings.ToUpper(attr)
}

func (b *builder) dzName(m model, attr string) string {
	return b.out.typeOf[m.uid] + schema.PascalCase(attr) + "DynamicZone"
}

func (b *builder) filtersName(uid schema.UID) string { return b.out.typeOf[uid] + "FiltersInput" }

func (b *builder) inputName(uid schema.UID) string { return b.out.typeOf[uid] + "Input" }

func isToMany(a *schema.Attribute) bool {
	if a.Type == schema.TypeMedia {
		return a.IsMultiple()
	}
	return strings.HasSuffix(a.Relation, "Many") || a.Relation == schema.ManyWay
}

func isMorph(a *schema.Attribute) bool {
	return a.Type == schema.TypeRelation && strings.Contains(strings.ToLower(a.Relation), "morph")
}

func nonNull(t string, required bool) string {
	if required && !strings.HasSuffix(t, "!") {
		return t + "!"
	}
	return t
}

// enumValue is toRegressedEnumValue: slugified with underscores.
func enumValue(v string) string {
	var b strings.Builder
	sep := false
	for _, r := range v {
		ok := r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !ok {
			sep = b.Len() > 0
			continue
		}
		if sep {
			b.WriteByte('_')
			sep = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func nestedArgs(filters string) []Arg {
	return []Arg{{Name: "filters", Type: filters}, {Name: "pagination", Type: "PaginationArg", Default: "{}"}, {Name: "sort", Type: "[String]", Default: "[]"}}
}

func publicationArgs() []Arg {
	return []Arg{
		{Name: "status", Type: "PublicationStatus", Default: "PUBLISHED"},
		{Name: "hasPublishedVersion", Type: "Boolean"},
		{Name: "publicationFilter", Type: "PublicationFilter"},
	}
}

func (b *builder) localeArg(m model, args []Arg) []Arg {
	if !b.i18n || !m.localized {
		return args
	}
	return append(args, Arg{Name: "locale", Type: "I18NLocaleCode", Description: "The locale to use for the query"})
}

func (b *builder) register(m model) {
	for name, a := range m.attrs.All() {
		switch a.Type {
		case schema.TypeEnumeration:
			d := &Def{Kind: DefEnum, Name: b.enumName(m, name, a), UID: m.uid}
			for _, v := range a.Enum {
				d.Values = append(d.Values, EnumValue{enumValue(v), v})
			}
			b.def(d)
		case schema.TypeDynamicZone:
			n := b.dzName(m, name)
			var members []string
			for _, c := range a.Components {
				members = append(members, b.out.typeOf[schema.UID(c)])
			}
			b.def(&Def{Kind: DefUnion, Name: n, Members: append(members, "Error"), UID: m.uid})
			b.def(&Def{Kind: DefScalar, Name: n + "Input", UID: m.uid})
		}
	}
	b.filters(m)
	b.input(m)
	name := b.out.typeOf[m.uid]
	b.def(b.object(m))
	if m.component {
		return
	}
	b.def(&Def{Kind: DefObject, Name: name + "EntityResponse", UID: m.uid, Fields: []*Field{{Name: "data", Type: name, Kind: FieldEntityData}}})
	b.def(&Def{Kind: DefObject, Name: name + "EntityResponseCollection", UID: m.uid, Fields: []*Field{
		{Name: "nodes", Type: "[" + name + "!]!", Kind: FieldNodes},
		{Name: "pageInfo", Type: "Pagination!", Kind: FieldPageInfo},
	}})
	b.def(&Def{Kind: DefObject, Name: name + "RelationResponseCollection", UID: m.uid, Fields: []*Field{{Name: "nodes", Type: "[" + name + "!]!", Kind: FieldNodes}}})
	b.queries(m)
	b.mutations(m)
}

func (b *builder) filters(m model) {
	self := b.filtersName(m.uid)
	d := &Def{Kind: DefInput, Name: self, UID: m.uid}
	if !m.component && !m.singleType {
		d.Fields = append(d.Fields, &Field{Name: "documentId", Type: "IDFilterInput"})
	}
	for name, a := range m.attrs.All() {
		if a.IsPrivate() {
			continue
		}
		switch {
		case scalarOf[a.Type] != "":
			d.Fields = append(d.Fields, &Field{Name: name, Type: scalarOf[a.Type] + "FilterInput"})
		case a.Type == schema.TypeRelation && !isMorph(a) && a.Target != "" && b.enabled(schema.UID(a.Target)):
			d.Fields = append(d.Fields, &Field{Name: name, Type: b.filtersName(schema.UID(a.Target))})
		case a.Type == schema.TypeComponent:
			d.Fields = append(d.Fields, &Field{Name: name, Type: b.filtersName(schema.UID(a.Component))})
		}
	}
	d.Fields = append(d.Fields, &Field{Name: "and", Type: "[" + self + "]"}, &Field{Name: "or", Type: "[" + self + "]"}, &Field{Name: "not", Type: self})
	b.def(d)
}

func writable(name string, a *schema.Attribute) bool {
	switch name {
	case "id", schema.AttrDocumentID, schema.AttrCreatedAt, schema.AttrUpdatedAt:
		return false
	}
	return a.IsWritable()
}

func (b *builder) input(m model) {
	d := &Def{Kind: DefInput, Name: b.inputName(m.uid), UID: m.uid}
	if m.component {
		d.Fields = append(d.Fields, &Field{Name: "id", Type: "ID"})
	}
	_, fileOK := b.set.ContentType(fileUID)
	for name, a := range m.attrs.All() {
		if !writable(name, a) || !b.inputEnabled(m, name) {
			continue
		}
		f := &Field{Name: name, UID: m.uid, Attr: name}
		switch {
		case a.Type == schema.TypeEnumeration:
			f.Type = b.enumName(m, name, a)
		case scalarOf[a.Type] != "":
			f.Type = scalarOf[a.Type]
		case a.Type == schema.TypeMedia:
			if !fileOK {
				continue
			}
			f.Type = "ID"
			if a.IsMultiple() {
				f.Type = "[ID]"
			}
		case a.Type == schema.TypeRelation && !isMorph(a):
			if !b.enabled(schema.UID(a.Target)) {
				continue
			}
			f.Type = "ID"
			if isToMany(a) {
				f.Type = "[ID]"
			}
		case a.Type == schema.TypeComponent:
			f.Type = b.inputName(schema.UID(a.Component))
			if a.IsRepeatable() {
				f.Type = "[" + f.Type + "]"
			}
		case a.Type == schema.TypeDynamicZone:
			f.Type = "[" + b.dzName(m, name) + "Input!]"
		default:
			continue
		}
		d.Fields = append(d.Fields, f)
	}
	b.def(d)
}

func (b *builder) object(m model) *Def {
	name := b.out.typeOf[m.uid]
	d := &Def{Kind: DefObject, Name: name, UID: m.uid}
	if m.component {
		d.Fields = append(d.Fields, &Field{Name: "id", Type: "ID!"})
	} else {
		d.Fields = append(d.Fields, &Field{Name: "documentId", Type: "ID!"})
	}
	_, fileOK := b.set.ContentType(fileUID)
	for attr, a := range m.attrs.All() {
		if a.IsPrivate() {
			continue
		}
		req := a.IsRequired()
		f := &Field{Name: attr, UID: m.uid, Attr: attr}
		switch {
		case a.Type == schema.TypeEnumeration:
			f.Type = nonNull(b.enumName(m, attr, a), req)
		case scalarOf[a.Type] != "":
			f.Type = nonNull(scalarOf[a.Type], req)
		case a.Type == schema.TypeComponent:
			f.Kind = FieldComponent
			f.Target = schema.UID(a.Component)
			t := b.out.typeOf[f.Target]
			if a.IsRepeatable() {
				t = "[" + t + "]"
				f.Args = nestedArgs(b.filtersName(f.Target))
			}
			f.Type = nonNull(t, req)
		case a.Type == schema.TypeDynamicZone:
			f.Kind = FieldDynamicZone
			f.Type = nonNull("["+b.dzName(m, attr)+"]", req)
		case a.Type == schema.TypeMedia || a.Type == schema.TypeRelation && !isMorph(a):
			target := schema.UID(a.Target)
			if a.Type == schema.TypeMedia {
				if !fileOK {
					continue
				}
				target = fileUID
			}
			if !b.enabled(target) {
				continue
			}
			f.Target = target
			f.Scope = string(target) + ".find"
			if a.Type == schema.TypeMedia {
				f.Scope = ""
			}
			t := b.out.typeOf[target]
			if !isToMany(a) {
				f.Kind = FieldRelation
				f.Type = nonNull(t, req)
				break
			}
			var args []Arg
			if !target.IsComponent() {
				if ct, ok := b.set.ContentType(target); ok && !ct.IsSingleType() {
					args = nestedArgs(b.filtersName(target))
				}
			}
			d.Fields = append(d.Fields, &Field{
				Name: attr + "_connection", Type: nonNull(t+"RelationResponseCollection", req), Args: args,
				Kind: FieldRelationConnection, UID: m.uid, Attr: attr, Target: target, Scope: f.Scope,
			})
			f.Kind = FieldRelationList
			f.Type = "[" + t + "]!"
			f.Args = args
		case isMorph(a) && a.Target == "":
			f.Kind = FieldMorph
			f.Type = "GenericMorph"
			if isToMany(a) {
				f.Type = "[GenericMorph]"
			}
		default:
			continue
		}
		d.Fields = append(d.Fields, f)
	}
	return d
}

var queryScopes = map[string]string{
	"uploadFiles": "plugin::upload.content-api.find", "uploadFiles_connection": "plugin::upload.content-api.find",
	"uploadFile": "plugin::upload.content-api.findOne",
	"i18NLocale": "plugin::i18n.locales.listLocales", "i18NLocales": "plugin::i18n.locales.listLocales",
}

func (b *builder) rootField(f *Field) {
	if s, ok := queryScopes[f.Name]; ok {
		f.Scope = s
	}
	if f.Kind >= FieldCreate && f.Kind <= FieldSingleDelete {
		b.mutation.add(f)
		return
	}
	b.query.add(f)
}

func (b *builder) queries(m model) {
	if !b.queriesEnabled(m.uid) {
		return
	}
	name := b.out.typeOf[m.uid]
	one := lowerFirst(name)
	uid := string(m.uid)
	if m.singleType {
		b.rootField(&Field{Name: one, Type: name, Args: b.localeArg(m, publicationArgs()), Kind: FieldSingleFind, UID: m.uid, Scope: uid + ".find"})
		return
	}
	many := lowerFirst(typeName(m.ct, true))
	b.rootField(&Field{
		Name: one, Type: name, Args: b.localeArg(m, append([]Arg{{Name: "documentId", Type: "ID!"}}, publicationArgs()...)),
		Kind: FieldFindOne, UID: m.uid, Scope: uid + ".findOne",
	})
	listArgs := b.localeArg(m, append(nestedArgs(b.filtersName(m.uid)), publicationArgs()...))
	b.rootField(&Field{Name: many + "_connection", Type: name + "EntityResponseCollection", Args: listArgs, Kind: FieldFindConnection, UID: m.uid, Scope: uid + ".find"})
	b.rootField(&Field{Name: many, Type: "[" + name + "]!", Args: listArgs, Kind: FieldFind, UID: m.uid, Scope: uid + ".find"})
}

func (b *builder) mutations(m model) {
	if !b.mutationsEnabled(m.uid) {
		return
	}
	name := b.out.typeOf[m.uid]
	uid := string(m.uid)
	status := Arg{Name: "status", Type: "PublicationStatus", Default: "PUBLISHED"}
	data := Arg{Name: "data", Type: name + "Input!"}
	docID := Arg{Name: "documentId", Type: "ID!"}
	if m.singleType {
		b.rootField(&Field{Name: "update" + name, Type: name, Args: b.localeArg(m, []Arg{status, data}), Kind: FieldSingleUpdate, UID: m.uid, Scope: uid + ".update"})
		b.rootField(&Field{Name: "delete" + name, Type: "DeleteMutationResponse", Args: b.localeArg(m, nil), Kind: FieldSingleDelete, UID: m.uid, Scope: uid + ".delete"})
		return
	}
	b.rootField(&Field{Name: "create" + name, Type: name, Args: b.localeArg(m, []Arg{status, data}), Kind: FieldCreate, UID: m.uid, Scope: uid + ".create"})
	b.rootField(&Field{Name: "update" + name, Type: name, Args: b.localeArg(m, []Arg{docID, status, data}), Kind: FieldUpdate, UID: m.uid, Scope: uid + ".update"})
	b.rootField(&Field{Name: "delete" + name, Type: "DeleteMutationResponse", Args: b.localeArg(m, []Arg{docID}), Kind: FieldDelete, UID: m.uid, Scope: uid + ".delete"})
}

// extensions are the types the upload, users-permissions and i18n plugins add.
func (b *builder) extensions() {
	if _, ok := b.set.ContentType(fileUID); ok {
		b.def(&Def{Kind: DefInput, Name: "FileInfoInput", Fields: []*Field{{Name: "name", Type: "String"}, {Name: "alternativeText", Type: "String"}, {Name: "caption", Type: "String"}}})
		b.mutation.add(&Field{Name: "updateUploadFile", Type: "UploadFile!", Args: []Arg{{Name: "id", Type: "ID!"}, {Name: "info", Type: "FileInfoInput"}}, Kind: FieldCustom, Scope: "plugin::upload.content-api.upload"})
		b.mutation.add(&Field{Name: "deleteUploadFile", Type: "UploadFile", Args: []Arg{{Name: "id", Type: "ID!"}}, Kind: FieldCustom, Scope: "plugin::upload.content-api.destroy"})
	}
	if _, ok := b.set.ContentType(userUID); ok {
		b.def(&Def{Kind: DefObject, Name: "UsersPermissionsMe", Fields: []*Field{
			{Name: "id", Type: "ID!"},
			{Name: "documentId", Type: "ID!"},
			{Name: "username", Type: "String!"},
			{Name: "email", Type: "String"},
			{Name: "confirmed", Type: "Boolean"},
			{Name: "blocked", Type: "Boolean"},
			{Name: "role", Type: "UsersPermissionsMeRole"},
		}})
		b.def(&Def{Kind: DefObject, Name: "UsersPermissionsMeRole", Fields: []*Field{
			{Name: "id", Type: "ID!"}, {Name: "name", Type: "String!"}, {Name: "description", Type: "String"}, {Name: "type", Type: "String"},
		}})
		b.def(&Def{Kind: DefInput, Name: "UsersPermissionsRegisterInput", Fields: []*Field{{Name: "username", Type: "String!"}, {Name: "email", Type: "String!"}, {Name: "password", Type: "String!"}}})
		b.def(&Def{Kind: DefInput, Name: "UsersPermissionsLoginInput", Fields: []*Field{{Name: "identifier", Type: "String!"}, {Name: "password", Type: "String!"}, {Name: "provider", Type: "String!", Default: `"local"`}}})
		for _, n := range []string{"UsersPermissionsPasswordPayload"} {
			b.def(&Def{Kind: DefObject, Name: n, Fields: []*Field{{Name: "ok", Type: "Boolean!"}}})
		}
		b.def(&Def{Kind: DefObject, Name: "UsersPermissionsLoginPayload", Fields: []*Field{{Name: "jwt", Type: "String"}, {Name: "user", Type: "UsersPermissionsMe!"}}})
		for _, n := range []string{"UsersPermissionsCreateRolePayload", "UsersPermissionsUpdateRolePayload", "UsersPermissionsDeleteRolePayload"} {
			b.def(&Def{Kind: DefObject, Name: n, Fields: []*Field{{Name: "ok", Type: "Boolean!"}}})
		}
		b.query.add(&Field{Name: "me", Type: "UsersPermissionsMe", Kind: FieldCustom})
		up := "plugin::users-permissions."
		for _, f := range []*Field{
			{Name: "createUsersPermissionsRole", Description: "Create a new role", Type: "UsersPermissionsCreateRolePayload", Args: []Arg{{Name: "data", Type: "UsersPermissionsRoleInput!"}}, Scope: up + "role.createRole"},
			{Name: "updateUsersPermissionsRole", Description: "Update an existing role", Type: "UsersPermissionsUpdateRolePayload", Args: []Arg{{Name: "id", Type: "ID!"}, {Name: "data", Type: "UsersPermissionsRoleInput!"}}, Scope: up + "role.updateRole"},
			{Name: "deleteUsersPermissionsRole", Description: "Delete an existing role", Type: "UsersPermissionsDeleteRolePayload", Args: []Arg{{Name: "id", Type: "ID!"}}, Scope: up + "role.deleteRole"},
			{Name: "createUsersPermissionsUser", Description: "Create a new user", Type: "UsersPermissionsUserEntityResponse!", Args: []Arg{{Name: "data", Type: "UsersPermissionsUserInput!"}}, Scope: up + "user.create"},
			{Name: "updateUsersPermissionsUser", Description: "Update an existing user", Type: "UsersPermissionsUserEntityResponse!", Args: []Arg{{Name: "id", Type: "ID!"}, {Name: "data", Type: "UsersPermissionsUserInput!"}}, Scope: up + "user.update"},
			{Name: "deleteUsersPermissionsUser", Description: "Delete an existing user", Type: "UsersPermissionsUserEntityResponse!", Args: []Arg{{Name: "id", Type: "ID!"}}, Scope: up + "user.destroy"},
			{Name: "login", Type: "UsersPermissionsLoginPayload!", Args: []Arg{{Name: "input", Type: "UsersPermissionsLoginInput!"}}, NoAuth: true},
			{Name: "register", Description: "Register a user", Type: "UsersPermissionsLoginPayload!", Args: []Arg{{Name: "input", Type: "UsersPermissionsRegisterInput!"}}, NoAuth: true},
			{Name: "forgotPassword", Description: "Request a reset password token", Type: "UsersPermissionsPasswordPayload", Args: []Arg{{Name: "email", Type: "String!"}}, NoAuth: true},
			{Name: "resetPassword", Description: "Reset user password. Confirm with a code (resetToken from forgotPassword)", Type: "UsersPermissionsLoginPayload", Args: []Arg{{Name: "password", Type: "String!"}, {Name: "passwordConfirmation", Type: "String!"}, {Name: "code", Type: "String!"}}, NoAuth: true},
			{Name: "changePassword", Description: "Change user password. Confirm with the current password.", Type: "UsersPermissionsLoginPayload", Args: []Arg{{Name: "currentPassword", Type: "String!"}, {Name: "password", Type: "String!"}, {Name: "passwordConfirmation", Type: "String!"}}, Scope: up + "auth.changePassword"},
			{Name: "emailConfirmation", Description: "Confirm an email users email address", Type: "UsersPermissionsLoginPayload", Args: []Arg{{Name: "confirmation", Type: "String!"}}, NoAuth: true},
		} {
			f.Kind = FieldCustom
			b.mutation.add(f)
		}
	}
	if b.i18n {
		b.def(&Def{Kind: DefScalar, Name: "I18NLocaleCode", Description: "A string used to identify an i18n locale"})
	}
}

// baseType is the named type of a type reference.
func baseType(t string) string { return strings.Trim(t, "[]!") }

// prune drops the types Query and Mutation cannot reach, like
// graphql-tools' pruneSchema.
func (b *builder) prune() {
	seen := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		d := b.out.defs[name]
		if d == nil {
			return
		}
		for _, f := range d.Fields {
			visit(baseType(f.Type))
			for _, a := range f.Args {
				visit(baseType(a.Type))
			}
		}
		for _, m := range d.Members {
			visit(m)
		}
	}
	visit("Query")
	visit("Mutation")
	var kept []*Def
	for _, d := range b.out.Defs {
		if seen[d.Name] {
			kept = append(kept, d)
		} else {
			delete(b.out.defs, d.Name)
		}
	}
	b.out.Defs = kept
	if gm := b.out.defs["GenericMorph"]; gm != nil {
		var members []string
		for _, m := range gm.Members {
			if b.out.defs[m] != nil {
				members = append(members, m)
			}
		}
		gm.Members = members
	}
}

func quoteDesc(s string) string { return `"""` + strings.ReplaceAll(s, `"""`, `\"""`) + `"""` }

func (s *Schema) print() string {
	var sb strings.Builder
	for _, d := range s.Defs {
		if d.Description != "" {
			sb.WriteString(quoteDesc(d.Description) + "\n")
		}
		switch d.Kind {
		case DefScalar:
			fmt.Fprintf(&sb, "scalar %s\n\n", d.Name)
			continue
		case DefUnion:
			fmt.Fprintf(&sb, "union %s = %s\n\n", d.Name, strings.Join(d.Members, " | "))
			continue
		case DefEnum:
			fmt.Fprintf(&sb, "enum %s {\n", d.Name)
			for _, v := range d.Values {
				sb.WriteString("  " + v.Name + "\n")
			}
			sb.WriteString("}\n\n")
			continue
		case DefInput:
			fmt.Fprintf(&sb, "input %s {\n", d.Name)
		default:
			fmt.Fprintf(&sb, "type %s {\n", d.Name)
		}
		for _, f := range d.Fields {
			if f.Description != "" && d.Kind == DefObject {
				sb.WriteString("  " + quoteDesc(f.Description) + "\n")
			}
			sb.WriteString("  " + f.Name)
			if len(f.Args) > 0 {
				parts := make([]string, len(f.Args))
				for i, a := range f.Args {
					parts[i] = a.Name + ": " + a.Type
					if a.Default != "" {
						parts[i] += " = " + a.Default
					}
				}
				sb.WriteString("(" + strings.Join(parts, ", ") + ")")
			}
			sb.WriteString(": " + f.Type)
			if f.Default != "" {
				sb.WriteString(" = " + f.Default)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("}\n\n")
	}
	return sb.String()
}
