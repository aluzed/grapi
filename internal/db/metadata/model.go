// Package metadata turns content types and components into database models
// (tables, columns, link tables, indexes, foreign keys) with Strapi 5's exact
// names, and derives the physical schema Strapi stores in
// strapi_database_schema.
package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/aluzed/grapi/schema"
)

// Model is a table and what its attributes map to.
type Model struct {
	UID          string
	SingularName string
	TableName    string
	Attributes   schema.OrderedMap[*Attribute]
	Indexes      []Index
	ForeignKeys  []ForeignKey
	// ColumnToAttribute maps a column to the attribute it stores.
	ColumnToAttribute map[string]string
}

// Attribute is a model attribute as the database sees it: a scalar with its
// column, or a relation with how it is stored.
type Attribute struct {
	Type       string
	ColumnName string
	// Column overrides the column definition (unsigned, defaultTo...).
	Column []KV

	Relation   string
	Target     string
	InversedBy string
	MappedBy   string
	MorphBy    string
	// UseJoinTable is false when the relation is a column of the owner's table.
	UseJoinTable *bool
	Virtual      bool
	// Searchable false excludes a column from the _q search.
	Searchable *bool
	// Default is the schema's default value; DefaultFunc names a computed
	// one ("documentId": a new document id, "now": the current time).
	Default     json.RawMessage
	DefaultFunc string

	Owner       bool
	JoinColumn  *JoinColumn
	JoinTable   *JoinTable
	MorphColumn *MorphColumn
}

// JoinColumn is a column referencing another table.
type JoinColumn struct {
	Name             string
	ReferencedColumn string
	ReferencedTable  string
}

// JoinTable is a link table between two models.
type JoinTable struct {
	Name                   string
	JoinColumn             JoinColumn
	InverseJoinColumn      JoinColumn
	MorphColumn            *MorphColumn
	OrderColumnName        string
	InverseOrderColumnName string
	// OrderBy is the column the links are read in (ascending), when any.
	OrderBy string
	// On restricts the rows to one attribute (component link tables).
	On           map[string]string
	PivotColumns []string
	// Generated is false for the link tables given by the content type
	// transformation (components, dynamic zones): they are not created again.
	Generated bool
}

// MorphColumn is the (id, type) pair of a polymorphic relation.
type MorphColumn struct {
	IDColumn   JoinColumn
	TypeColumn string
	TypeField  string
}

func (a *Attribute) isRelation() bool { return a.Type == schema.TypeRelation }

func (a *Attribute) bidirectional() bool { return a.InversedBy != "" || a.MappedBy != "" }

func (a *Attribute) isOwner() bool { return !a.bidirectional() || a.InversedBy != "" }

func (a *Attribute) useJoinTable() bool { return a.UseJoinTable == nil || *a.UseJoinTable }

func (a *Attribute) anyToMany() bool {
	return a.Relation == schema.OneToMany || a.Relation == schema.ManyToMany
}

func (a *Attribute) manyToAny() bool {
	return a.Relation == schema.ManyToMany || a.Relation == schema.ManyToOne
}

var scalarTypes = map[string]bool{
	"increments": true, schema.TypePassword: true, schema.TypeEmail: true, schema.TypeString: true,
	schema.TypeUID: true, schema.TypeRichText: true, schema.TypeText: true, schema.TypeJSON: true,
	schema.TypeEnumeration: true, schema.TypeInteger: true, schema.TypeBigInteger: true,
	schema.TypeFloat: true, schema.TypeDecimal: true, schema.TypeDate: true, schema.TypeTime: true,
	schema.TypeDateTime: true, schema.TypeTimestamp: true, schema.TypeBoolean: true,
	schema.TypeBlocks: true,
}

// rawModel is the JSON format of the internal models (core store, webhooks).
type rawModel struct {
	UID          string                                `json:"uid"`
	SingularName string                                `json:"singularName"`
	TableName    string                                `json:"tableName"`
	Attributes   schema.OrderedMap[*rawModelAttribute] `json:"attributes"`
	Indexes      []json.RawMessage                     `json:"indexes"`
	ForeignKeys  []json.RawMessage                     `json:"foreignKeys"`
}

type rawModelAttribute struct {
	Type   string          `json:"type"`
	Column json.RawMessage `json:"column"`
}

// ParseModel decodes an internal model.
func ParseModel(data []byte) (*Model, error) {
	var r rawModel
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	m := &Model{UID: r.UID, SingularName: r.SingularName, TableName: r.TableName}
	for name, a := range r.Attributes.All() {
		col, err := orderedKVs(a.Column)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: column: %w", r.UID, name, err)
		}
		m.Attributes.Set(name, &Attribute{Type: a.Type, Column: col})
	}
	var err error
	if m.Indexes, err = declaredIndexes(r.Indexes); err != nil {
		return nil, fmt.Errorf("%s: indexes: %w", r.UID, err)
	}
	if m.ForeignKeys, err = declaredForeignKeys(r.ForeignKeys); err != nil {
		return nil, fmt.Errorf("%s: foreignKeys: %w", r.UID, err)
	}
	return m, nil
}

func orderedKVs(raw json.RawMessage) ([]KV, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var m schema.OrderedMap[json.RawMessage]
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make([]KV, 0, m.Len())
	for k, v := range m.All() {
		out = append(out, KV{Key: k, Value: v})
	}
	return out, nil
}

func declaredIndexes(raws []json.RawMessage) ([]Index, error) {
	out := make([]Index, 0, len(raws))
	for _, raw := range raws {
		var i struct {
			Name    string   `json:"name"`
			Columns []string `json:"columns"`
			Type    *string  `json:"type"`
		}
		if err := json.Unmarshal(raw, &i); err != nil {
			return nil, err
		}
		idx := Index{Name: i.Name, Columns: i.Columns, Layout: Declared, raw: compact(raw)}
		if i.Type != nil {
			idx.Type = *i.Type
		}
		out = append(out, idx)
	}
	return out, nil
}

func declaredForeignKeys(raws []json.RawMessage) ([]ForeignKey, error) {
	out := make([]ForeignKey, 0, len(raws))
	for _, raw := range raws {
		var f struct {
			Name              string   `json:"name"`
			Columns           []string `json:"columns"`
			ReferencedTable   string   `json:"referencedTable"`
			ReferencedColumns []string `json:"referencedColumns"`
			OnDelete          string   `json:"onDelete"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		out = append(out, ForeignKey{
			Name: f.Name, Columns: f.Columns, ReferencedTable: f.ReferencedTable,
			ReferencedColumns: f.ReferencedColumns, OnDelete: f.OnDelete, Layout: DeclaredFK, raw: compact(raw),
		})
	}
	return out, nil
}

func compact(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

// must panics on a naming error: the inputs come from validated schemas
// and Strapi would fail the same way at startup.
func must(name string, err error) string {
	if err != nil {
		panic(namingError{err})
	}
	return name
}

type namingError struct{ error }
