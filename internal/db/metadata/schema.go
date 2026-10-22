package metadata

import (
	"bytes"
	"encoding/json"
	"sort"
)

// Schema is the physical schema Strapi derives from its models and stores in
// strapi_database_schema. Its JSON encoding matches Strapi's key for key, so
// that both compute the same hash for the same database.
type Schema struct {
	Tables []*Table `json:"tables"`
}

// Table of the physical schema.
type Table struct {
	Name        string       `json:"name"`
	Indexes     []Index      `json:"indexes"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
	Columns     []Column     `json:"columns"`
}

// Column of a table. Extra holds the keys Strapi adds beyond the six it
// always writes (unique, primary...), in order.
type Column struct {
	Name        string
	Type        string
	Args        json.RawMessage
	DefaultTo   json.RawMessage
	NotNullable bool
	Unsigned    bool
	Extra       []KV
}

// KV is one key of an ordered JSON object.
type KV struct {
	Key   string
	Value json.RawMessage
}

// Has reports whether the extra key is set to true (unique, primary).
func (c Column) Has(key string) bool {
	for _, kv := range c.Extra {
		if kv.Key == key {
			return string(kv.Value) == "true"
		}
	}
	return false
}

// MarshalJSON writes name, type, args, defaultTo, notNullable, unsigned, then Extra.
func (c Column) MarshalJSON() ([]byte, error) {
	o := object{}
	o.add("name", c.Name)
	o.add("type", c.Type)
	o.raw("args", orDefault(c.Args, "[]"))
	o.raw("defaultTo", orDefault(c.DefaultTo, "null"))
	o.add("notNullable", c.NotNullable)
	o.add("unsigned", c.Unsigned)
	for _, kv := range c.Extra {
		o.raw(kv.Key, kv.Value)
	}
	return o.bytes()
}

// Index of a table. Strapi writes the keys in an order that depends on where
// the index comes from; Layout keeps it.
type Index struct {
	Name    string
	Columns []string
	Type    string
	Layout  IndexLayout
	raw     json.RawMessage
}

// IndexLayout is the key order of an index object.
type IndexLayout int

// Index layouts.
const (
	// {name, columns} or {name, columns, type}.
	NameFirst IndexLayout = iota
	// {type, name, columns}: unique and primary indexes of a column.
	TypeFirst
	// Written as declared in the schema's "indexes".
	Declared
)

// MarshalJSON writes the index in its layout.
func (i Index) MarshalJSON() ([]byte, error) {
	if i.Layout == Declared {
		return i.raw, nil
	}
	o := object{}
	if i.Layout == TypeFirst {
		o.add("type", i.Type)
	}
	o.add("name", i.Name)
	o.add("columns", i.Columns)
	if i.Layout == NameFirst && i.Type != "" {
		o.add("type", i.Type)
	}
	return o.bytes()
}

// ForeignKey of a table.
type ForeignKey struct {
	Name              string
	Columns           []string
	ReferencedTable   string
	ReferencedColumns []string
	OnDelete          string
	Layout            FKLayout
	raw               json.RawMessage
}

// FKLayout is the key order of a foreign key object.
type FKLayout int

// Foreign key layouts.
const (
	// {name, columns, referencedTable, referencedColumns, onDelete}: join columns.
	TableFirst FKLayout = iota
	// {name, columns, referencedColumns, referencedTable, onDelete}: link tables.
	ColumnsFirst
	// Written as declared in the schema's "foreignKeys".
	DeclaredFK
)

// MarshalJSON writes the foreign key in its layout.
func (f ForeignKey) MarshalJSON() ([]byte, error) {
	if f.Layout == DeclaredFK {
		return f.raw, nil
	}
	o := object{}
	o.add("name", f.Name)
	o.add("columns", f.Columns)
	if f.Layout == TableFirst {
		o.add("referencedTable", f.ReferencedTable)
		o.add("referencedColumns", f.ReferencedColumns)
	} else {
		o.add("referencedColumns", f.ReferencedColumns)
		o.add("referencedTable", f.ReferencedTable)
	}
	o.add("onDelete", f.OnDelete)
	return o.bytes()
}

// Sorted returns the tables sorted by name, the order Strapi hashes them in.
func (s *Schema) Sorted() []*Table {
	out := append([]*Table(nil), s.Tables...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Table returns the table named name.
func (s *Schema) Table(name string) (*Table, bool) {
	for _, t := range s.Tables {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// Marshal encodes v like JSON.stringify: no HTML escaping, no indentation.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

type object struct {
	buf bytes.Buffer
	err error
}

func (o *object) add(key string, v any) {
	b, err := Marshal(v)
	if err != nil {
		o.err = err
		return
	}
	o.raw(key, b)
}

func (o *object) raw(key string, v json.RawMessage) {
	if o.buf.Len() == 0 {
		o.buf.WriteByte('{')
	} else {
		o.buf.WriteByte(',')
	}
	k, _ := Marshal(key)
	o.buf.Write(k)
	o.buf.WriteByte(':')
	o.buf.Write(v)
}

func (o *object) bytes() ([]byte, error) {
	if o.err != nil {
		return nil, o.err
	}
	if o.buf.Len() == 0 {
		return []byte("{}"), nil
	}
	o.buf.WriteByte('}')
	return o.buf.Bytes(), nil
}

func orDefault(v json.RawMessage, def string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(def)
	}
	return v
}
