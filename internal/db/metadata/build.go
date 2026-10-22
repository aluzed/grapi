package metadata

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aluzed/grapi/internal/db/naming"
	"github.com/aluzed/grapi/schema"
)

const (
	idColumn    = naming.IDColumn
	orderColumn = naming.OrderColumn
	fieldColumn = naming.FieldColumn
	uploadFile  = "plugin::upload.file"
)

// Metadata is every model of an application, in Strapi's order: content
// types (each preceded by its component link table), components, internal
// models, then the link tables created for relations.
type Metadata struct {
	Namer  naming.Namer
	models schema.OrderedMap[*Model]
}

// Model returns the model of uid (a content type, component or link table).
func (md *Metadata) Model(uid string) (*Model, bool) { return md.models.Get(uid) }

// Models returns the models in order.
func (md *Metadata) Models() []*Model {
	out := make([]*Model, 0, md.models.Len())
	for _, model := range md.models.All() {
		out = append(out, model)
	}
	return out
}

// Build derives the models of set, followed by the internal models.
func Build(set *schema.Set, internal []*Model, namer naming.Namer) (md *Metadata, err error) {
	defer func() {
		if r := recover(); r != nil {
			ne, ok := r.(namingError)
			if !ok {
				panic(r)
			}
			err = ne.error
		}
	}()
	b := &builder{namer: namer}
	for _, ct := range set.ContentTypes.All() {
		if err := b.contentType(string(ct.UID), ct.ModelName, ct.CollectionName, &ct.Attributes, true, ct.Indexes, ct.ForeignKeys); err != nil {
			return nil, err
		}
	}
	for _, c := range set.Components.All() {
		if err := b.contentType(string(c.UID), c.ModelName, c.CollectionName, &c.Attributes, false, nil, nil); err != nil {
			return nil, err
		}
	}
	b.models = append(b.models, internal...)
	return b.load()
}

type builder struct {
	namer  naming.Namer
	models []*Model
}

func (b *builder) contentType(uid, modelName, collectionName string, attrs *schema.OrderedMap[*schema.Attribute], isContentType bool, indexes, fks json.RawMessage) error {
	if hasComponents(attrs) {
		b.models = append(b.models, b.componentLinkModel(collectionName))
	}
	m := &Model{UID: uid, SingularName: modelName, TableName: collectionName}
	m.Attributes.Set(idColumn, &Attribute{Type: "increments"})
	if isContentType {
		m.Attributes.Set(schema.AttrDocumentID, &Attribute{Type: schema.TypeString, DefaultFunc: "documentId"})
	}
	for name, a := range attrs.All() {
		attr, err := b.transform(name, a, collectionName, isContentType)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", uid, name, err)
		}
		m.Attributes.Set(name, attr)
	}
	var err error
	if m.Indexes, err = declaredIndexes(rawList(indexes)); err != nil {
		return fmt.Errorf("%s: indexes: %w", uid, err)
	}
	if m.ForeignKeys, err = declaredForeignKeys(rawList(fks)); err != nil {
		return fmt.Errorf("%s: foreignKeys: %w", uid, err)
	}
	if isContentType {
		var cols []string
		for _, name := range []string{schema.AttrDocumentID, schema.AttrLocale, schema.AttrPublishedAt} {
			if _, ok := m.Attributes.Get(name); ok {
				cols = append(cols, must(b.namer.Column(naming.SnakeCase(name))))
			}
		}
		m.Indexes = append(m.Indexes, Index{Name: must(b.namer.Index(collectionName, "documents")), Columns: cols})
	}
	b.models = append(b.models, m)
	return nil
}

func rawList(raw json.RawMessage) []json.RawMessage {
	var out []json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func hasComponents(attrs *schema.OrderedMap[*schema.Attribute]) bool {
	for _, a := range attrs.All() {
		if a.Type == schema.TypeComponent || a.Type == schema.TypeDynamicZone {
			return true
		}
	}
	return false
}

// defaultFunc: Strapi gives publishedAt a default of "now" on every content
// type (draft and publish or not).
func defaultFunc(name string, a *schema.Attribute, isContentType bool) string {
	if isContentType && a.Auto && name == schema.AttrPublishedAt {
		return "now"
	}
	return ""
}

func (b *builder) transform(name string, a *schema.Attribute, collectionName string, isContentType bool) (*Attribute, error) {
	switch a.Type {
	case schema.TypeMedia:
		relation := schema.MorphOne
		if a.Multiple != nil && *a.Multiple {
			relation = schema.MorphMany
		}
		return &Attribute{Type: schema.TypeRelation, Relation: relation, Target: uploadFile, MorphBy: "related"}, nil
	case schema.TypeComponent:
		relation := schema.OneToOne
		if a.Repeatable != nil && *a.Repeatable {
			relation = schema.OneToMany
		}
		jt := b.componentJoinTable(name, collectionName)
		jt.OrderColumnName = orderColumn
		jt.OrderBy = orderColumn
		return &Attribute{Type: schema.TypeRelation, Relation: relation, Target: a.Component, JoinTable: jt}, nil
	case schema.TypeDynamicZone:
		jt := b.componentJoinTable(name, collectionName)
		jt.OrderBy = orderColumn
		jt.MorphColumn = &MorphColumn{
			IDColumn:   jt.InverseJoinColumn,
			TypeColumn: b.componentTypeColumn(),
			TypeField:  schema.AttrComponentKey,
		}
		jt.InverseJoinColumn = JoinColumn{}
		return &Attribute{Type: schema.TypeRelation, Relation: schema.MorphToMany, JoinTable: jt}, nil
	}
	col, err := orderedKVs(a.Column)
	if err != nil {
		return nil, fmt.Errorf("column: %w", err)
	}
	return &Attribute{
		Type: a.Type, Column: col, Relation: a.Relation, Target: a.Target, InversedBy: a.InversedBy,
		MappedBy: a.MappedBy, MorphBy: a.MorphBy, UseJoinTable: a.UseJoinTable, Virtual: a.Virtual,
		Searchable: a.Searchable, Default: a.Default, DefaultFunc: defaultFunc(name, a, isContentType),
	}, nil
}

func (b *builder) componentLinkTableName(collectionName string) string {
	return must(b.namer.FromTokens(
		naming.Token{Name: collectionName, Compressible: true},
		naming.Token{Name: "components", ShortName: "cmps"},
	))
}

func (b *builder) entityColumn() string {
	return must(b.namer.FromTokens(naming.Token{Name: "entity"}, naming.Token{Name: "id"}))
}

func (b *builder) componentColumn() string {
	return must(b.namer.FromTokens(naming.Token{Name: "component", ShortName: "cmp"}, naming.Token{Name: "id"}))
}

func (b *builder) componentTypeColumn() string {
	return must(b.namer.FromTokens(naming.Token{Name: "component_type"}))
}

func (b *builder) componentJoinTable(attrName, collectionName string) *JoinTable {
	entity, cmp := b.entityColumn(), b.componentColumn()
	return &JoinTable{
		Name:              b.componentLinkTableName(collectionName),
		JoinColumn:        JoinColumn{Name: entity, ReferencedColumn: idColumn},
		InverseJoinColumn: JoinColumn{Name: cmp, ReferencedColumn: idColumn},
		On:                map[string]string{"field": attrName},
		PivotColumns:      []string{entity, cmp, fieldColumn, b.componentTypeColumn()},
	}
}

func (b *builder) componentLinkModel(collectionName string) *Model {
	name := b.componentLinkTableName(collectionName)
	entity, cmp, compType := b.entityColumn(), b.componentColumn(), b.componentTypeColumn()
	fk := must(b.namer.FromTokens(
		naming.Token{Name: collectionName, Compressible: true},
		naming.Token{Name: "entity"},
		naming.Token{Name: "fk"},
	))
	m := &Model{UID: name, SingularName: name, TableName: name}
	m.Attributes.Set(idColumn, &Attribute{Type: "increments"})
	m.Attributes.Set(entity, &Attribute{Type: schema.TypeInteger, Column: unsigned()})
	m.Attributes.Set(cmp, &Attribute{Type: schema.TypeInteger, Column: unsigned()})
	m.Attributes.Set(compType, &Attribute{Type: schema.TypeString})
	m.Attributes.Set(fieldColumn, &Attribute{Type: schema.TypeString})
	m.Attributes.Set(orderColumn, &Attribute{Type: schema.TypeFloat, Column: unsignedNullDefault()})
	m.Indexes = []Index{
		{Name: must(b.namer.Index(collectionName, fieldColumn)), Columns: []string{fieldColumn}},
		{Name: must(b.namer.Index(collectionName, compType)), Columns: []string{compType}},
		{Name: fk, Columns: []string{entity}},
		{Name: must(b.namer.UniqueIndex(collectionName)), Columns: []string{entity, cmp, fieldColumn, compType}, Type: "unique"},
	}
	m.ForeignKeys = []ForeignKey{{
		Name: fk, Columns: []string{entity}, ReferencedColumns: []string{idColumn},
		ReferencedTable: must(b.namer.Table(collectionName)), OnDelete: "CASCADE", Layout: ColumnsFirst,
	}}
	return m
}

func unsigned() []KV { return []KV{{Key: "unsigned", Value: json.RawMessage("true")}} }

func unsignedNullDefault() []KV {
	return []KV{{Key: "unsigned", Value: json.RawMessage("true")}, {Key: "defaultTo", Value: json.RawMessage("null")}}
}

func (b *builder) load() (*Metadata, error) {
	md := &Metadata{Namer: b.namer}
	for _, m := range b.models {
		m.TableName = must(b.namer.Table(m.TableName))
		md.models.Set(m.UID, m)
	}
	// Link tables are added while iterating and visited too, like a JS Map.
	for i := 0; i < md.models.Len(); i++ {
		meta, _ := md.models.Get(md.models.Keys()[i])
		for name, a := range meta.Attributes.All() {
			if a.Virtual {
				// i18n links the locales of a document through document_id.
				if name == schema.AttrLocalizations {
					a.JoinColumn = &JoinColumn{Name: "document_id", ReferencedColumn: "document_id", ReferencedTable: meta.TableName}
				}
				continue
			}
			if a.isRelation() {
				if err := md.createRelation(name, a, meta); err != nil {
					return nil, fmt.Errorf("error on attribute %s in model %s(%s): %w", name, meta.SingularName, meta.UID, err)
				}
				continue
			}
			if a.ColumnName == "" {
				a.ColumnName = must(b.namer.Column(naming.SnakeCase(name)))
			}
		}
	}
	seen := map[string]bool{}
	for _, m := range md.models.All() {
		m.ColumnToAttribute = map[string]string{}
		for name, a := range m.Attributes.All() {
			col := name
			if a.ColumnName != "" {
				col = a.ColumnName
			}
			m.ColumnToAttribute[col] = name
		}
		if seen[m.TableName] {
			return nil, fmt.Errorf("DB table %q already exists. Change the collectionName of the related content type", m.TableName)
		}
		seen[m.TableName] = true
	}
	return md, nil
}

func (md *Metadata) target(uid string) (*Model, error) {
	t, ok := md.models.Get(uid)
	if !ok {
		return nil, fmt.Errorf("unknown target %s", uid)
	}
	return t, nil
}

func (md *Metadata) createRelation(name string, a *Attribute, meta *Model) error {
	switch a.Relation {
	case schema.OneToOne:
		if !a.isOwner() {
			return nil
		}
		if a.useJoinTable() {
			return md.createJoinTable(name, a, meta)
		}
		return md.createJoinColumn(name, a)
	case schema.OneToMany:
		if a.useJoinTable() && !a.bidirectional() {
			return md.createJoinTable(name, a, meta)
		}
		if a.isOwner() {
			return errors.New("one side of a oneToMany cannot be the owner side in a bidirectional relation")
		}
		return nil
	case schema.ManyToOne:
		if a.bidirectional() && !a.isOwner() {
			return errors.New("the many side of a manyToOne must be the owning side")
		}
		if a.useJoinTable() {
			return md.createJoinTable(name, a, meta)
		}
		return md.createJoinColumn(name, a)
	case schema.ManyToMany:
		if a.useJoinTable() && (!a.bidirectional() || a.isOwner()) {
			return md.createJoinTable(name, a, meta)
		}
		return nil
	case schema.MorphToOne:
		a.Owner = true
		if a.MorphColumn == nil {
			// Strapi names these after "target", not after the attribute.
			a.MorphColumn = &MorphColumn{
				TypeColumn: must(md.Namer.MorphType("target")),
				IDColumn:   JoinColumn{Name: must(md.Namer.JoinColumnID("target")), ReferencedColumn: idColumn},
			}
		}
		return nil
	case schema.MorphToMany:
		return md.createMorphToMany(name, a, meta)
	case schema.MorphOne, schema.MorphMany:
		t, err := md.target(a.Target)
		if err != nil {
			return fmt.Errorf("morph target not found. Looking for %s", a.Target)
		}
		if a.MorphBy != "" {
			if _, ok := t.Attributes.Get(a.MorphBy); !ok {
				return fmt.Errorf("morph target attribute not found. Looking for %s", a.MorphBy)
			}
		}
		return nil
	}
	return errors.New("unknown relation")
}

func (md *Metadata) createJoinColumn(name string, a *Attribute) error {
	t, err := md.target(a.Target)
	if err != nil {
		return err
	}
	col := must(md.Namer.JoinColumnID(naming.SnakeCase(name)))
	jc := JoinColumn{Name: col, ReferencedColumn: idColumn, ReferencedTable: t.TableName}
	if a.JoinColumn != nil {
		if a.JoinColumn.Name != "" {
			jc.Name = a.JoinColumn.Name
		}
		if a.JoinColumn.ReferencedColumn != "" {
			jc.ReferencedColumn = a.JoinColumn.ReferencedColumn
		}
		if a.JoinColumn.ReferencedTable != "" {
			jc.ReferencedTable = a.JoinColumn.ReferencedTable
		}
	}
	a.Owner = true
	a.JoinColumn = &jc
	if a.bidirectional() {
		if inv, ok := t.Attributes.Get(a.InversedBy); ok {
			inv.JoinColumn = &JoinColumn{Name: jc.ReferencedColumn, ReferencedColumn: col}
		}
	}
	return nil
}

func (md *Metadata) createJoinTable(name string, a *Attribute, meta *Model) error {
	t, err := md.target(a.Target)
	if err != nil {
		return err
	}
	if a.JoinTable != nil && !a.JoinTable.Generated {
		return nil
	}
	n := md.Namer
	tableName := must(n.JoinTable(naming.SnakeCase(meta.TableName), naming.SnakeCase(name)))
	joinCol := must(n.JoinColumnID(naming.SnakeCase(meta.SingularName)))
	invJoinCol := must(n.JoinColumnID(naming.SnakeCase(t.SingularName)))
	if joinCol == invJoinCol {
		invJoinCol = must(n.InverseJoinColumnID(naming.SnakeCase(t.SingularName)))
	}
	orderCol := must(n.OrderColumnName(naming.SnakeCase(t.SingularName)))
	invOrderCol := must(n.OrderColumnName(naming.SnakeCase(meta.SingularName)))
	if a.Relation == schema.ManyToMany && orderCol == invOrderCol {
		invOrderCol = must(n.InverseOrderColumnName(naming.SnakeCase(meta.SingularName)))
	}
	fk := must(n.FkIndex(tableName))
	invFk := must(n.InverseFkIndex(tableName))

	link := &Model{UID: tableName, SingularName: tableName, TableName: tableName}
	link.Attributes.Set(idColumn, &Attribute{Type: "increments"})
	link.Attributes.Set(joinCol, &Attribute{Type: schema.TypeInteger, Column: unsigned(), ColumnName: joinCol})
	link.Attributes.Set(invJoinCol, &Attribute{Type: schema.TypeInteger, Column: unsigned(), ColumnName: invJoinCol})
	link.Indexes = []Index{
		{Name: fk, Columns: []string{joinCol}},
		{Name: invFk, Columns: []string{invJoinCol}},
		{Name: must(n.UniqueIndex(tableName)), Columns: []string{joinCol, invJoinCol}, Type: "unique"},
	}
	link.ForeignKeys = []ForeignKey{
		{Name: fk, Columns: []string{joinCol}, ReferencedColumns: []string{idColumn}, ReferencedTable: meta.TableName, OnDelete: "CASCADE", Layout: ColumnsFirst},
		{Name: invFk, Columns: []string{invJoinCol}, ReferencedColumns: []string{idColumn}, ReferencedTable: t.TableName, OnDelete: "CASCADE", Layout: ColumnsFirst},
	}
	jt := &JoinTable{
		Name:              tableName,
		JoinColumn:        JoinColumn{Name: joinCol, ReferencedColumn: idColumn, ReferencedTable: meta.TableName},
		InverseJoinColumn: JoinColumn{Name: invJoinCol, ReferencedColumn: idColumn, ReferencedTable: t.TableName},
		PivotColumns:      []string{joinCol, invJoinCol},
		Generated:         true,
	}
	if a.anyToMany() {
		link.Attributes.Set(orderCol, &Attribute{Type: schema.TypeFloat, Column: unsignedNullDefault(), ColumnName: orderCol})
		link.Indexes = append(link.Indexes, Index{Name: must(n.OrderFkIndex(tableName)), Columns: []string{orderCol}})
		jt.OrderColumnName = orderCol
		jt.OrderBy = orderCol
	}
	if a.bidirectional() && a.manyToAny() {
		link.Attributes.Set(invOrderCol, &Attribute{Type: schema.TypeFloat, Column: unsignedNullDefault(), ColumnName: invOrderCol})
		link.Indexes = append(link.Indexes, Index{Name: must(n.OrderInverseFkIndex(tableName)), Columns: []string{invOrderCol}})
		jt.InverseOrderColumnName = invOrderCol
	}
	md.models.Set(link.UID, link)
	a.JoinTable = jt

	if a.bidirectional() {
		inv, ok := t.Attributes.Get(a.InversedBy)
		if a.InversedBy == "" || !ok {
			return fmt.Errorf("inversedBy attribute %s not found target %s", a.InversedBy, t.UID)
		}
		if !inv.isRelation() {
			return fmt.Errorf("inversedBy attribute %s targets non relational attribute in %s", a.InversedBy, t.UID)
		}
		inv.JoinTable = &JoinTable{
			Name: tableName, JoinColumn: jt.InverseJoinColumn, InverseJoinColumn: jt.JoinColumn,
			PivotColumns: jt.PivotColumns, Generated: true,
		}
		if a.manyToAny() {
			inv.JoinTable.OrderColumnName = invOrderCol
			inv.JoinTable.OrderBy = invOrderCol
		}
		if a.anyToMany() {
			inv.JoinTable.InverseOrderColumnName = orderCol
		}
	}
	return nil
}

func (md *Metadata) createMorphToMany(name string, a *Attribute, meta *Model) error {
	if a.JoinTable != nil && !a.JoinTable.Generated {
		return nil
	}
	n := md.Namer
	tableName := must(n.MorphTable(meta.TableName, name))
	joinCol := must(n.MorphJoinTableID(naming.SnakeCase(meta.SingularName)))
	idCol := must(n.MorphAttributeID(name))
	typeCol := must(n.MorphType(name))
	fk := must(n.FkIndex(tableName))

	link := &Model{UID: tableName, SingularName: tableName, TableName: tableName}
	link.Attributes.Set(idColumn, &Attribute{Type: "increments"})
	link.Attributes.Set(joinCol, &Attribute{Type: schema.TypeInteger, Column: unsigned(), ColumnName: joinCol})
	link.Attributes.Set(idCol, &Attribute{Type: schema.TypeInteger, Column: unsigned()})
	link.Attributes.Set(typeCol, &Attribute{Type: schema.TypeString})
	link.Attributes.Set(fieldColumn, &Attribute{Type: schema.TypeString})
	link.Attributes.Set(orderColumn, &Attribute{Type: schema.TypeFloat, Column: unsigned()})
	link.Indexes = []Index{
		{Name: fk, Columns: []string{joinCol}},
		{Name: must(n.OrderIndex(tableName)), Columns: []string{orderColumn}},
		{Name: must(n.IDColumnIndex(tableName)), Columns: []string{idCol}},
	}
	link.ForeignKeys = []ForeignKey{{
		Name: fk, Columns: []string{joinCol}, ReferencedColumns: []string{idColumn},
		ReferencedTable: meta.TableName, OnDelete: "CASCADE", Layout: ColumnsFirst,
	}}
	md.models.Set(link.UID, link)
	a.JoinTable = &JoinTable{
		Name:         tableName,
		JoinColumn:   JoinColumn{Name: joinCol, ReferencedColumn: idColumn},
		MorphColumn:  &MorphColumn{TypeColumn: typeCol, IDColumn: JoinColumn{Name: idCol, ReferencedColumn: idColumn}},
		PivotColumns: []string{joinCol, typeCol, idCol},
		OrderBy:      orderColumn,
		Generated:    true,
	}
	return nil
}
