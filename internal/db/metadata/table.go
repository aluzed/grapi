package metadata

import (
	"encoding/json"
	"fmt"

	"github.com/aluzed/grapi/schema"
)

// Schema derives the physical schema, one table per model in order.
func (md *Metadata) Schema() (s *Schema, err error) {
	defer func() {
		if r := recover(); r != nil {
			ne, ok := r.(namingError)
			if !ok {
				panic(r)
			}
			err = ne.error
		}
	}()
	s = &Schema{Tables: []*Table{}}
	for _, m := range md.models.All() {
		t, err := md.table(m)
		if err != nil {
			return nil, err
		}
		s.Tables = append(s.Tables, t)
	}
	return s, nil
}

func (md *Metadata) name(n string) string { return must(md.Namer.Table(n)) }

func (md *Metadata) table(m *Model) (*Table, error) {
	t := &Table{
		Name:        m.TableName,
		Indexes:     append([]Index{}, m.Indexes...),
		ForeignKeys: append([]ForeignKey{}, m.ForeignKeys...),
		Columns:     []Column{},
	}
	for key, a := range m.Attributes.All() {
		switch {
		case a.isRelation():
			switch {
			case a.MorphColumn != nil && a.Owner:
				t.Columns = append(t.Columns,
					newColumn(md.name(a.MorphColumn.IDColumn.Name), knexType{typ: "integer"}, unsigned()),
					newColumn(md.name(a.MorphColumn.TypeColumn), knexType{typ: "string"}, nil))
			case a.JoinColumn != nil && a.Owner && a.JoinColumn.ReferencedTable != "":
				col := newColumn(md.name(a.JoinColumn.Name), knexType{typ: "integer"}, unsigned())
				t.Columns = append(t.Columns, col)
				fk := must(md.Namer.FkIndex(t.Name, col.Name))
				t.ForeignKeys = append(t.ForeignKeys, ForeignKey{
					Name: fk, Columns: []string{col.Name}, ReferencedTable: a.JoinColumn.ReferencedTable,
					ReferencedColumns: []string{a.JoinColumn.ReferencedColumn}, OnDelete: "SET NULL",
				})
				t.Indexes = append(t.Indexes, Index{Name: fk, Columns: []string{col.Name}})
			}
		case scalarTypes[a.Type]:
			name := key
			if a.ColumnName != "" {
				name = a.ColumnName
			}
			name = md.name(name)
			ct, err := columnType(a.Type)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", m.UID, key, err)
			}
			col := newColumn(name, ct, a.Column)
			if col.Has("unique") {
				t.Indexes = append(t.Indexes, Index{
					Type: "unique", Name: must(md.Namer.UniqueIndex(t.Name, col.Name)), Columns: []string{name}, Layout: TypeFirst,
				})
			}
			if col.Has("primary") {
				t.Indexes = append(t.Indexes, Index{
					Type: "primary", Name: must(md.Namer.PrimaryIndex(t.Name, col.Name)), Columns: []string{name}, Layout: TypeFirst,
				})
			}
			t.Columns = append(t.Columns, col)
		}
	}
	return t, nil
}

// newColumn applies Strapi's defaults, then the attribute's column overrides
// in order: known keys keep their position, new ones go to Extra.
func newColumn(name string, kt knexType, overrides []KV) Column {
	c := Column{Name: name, Type: kt.typ, Args: kt.args, NotNullable: kt.notNullable}
	for _, kv := range overrides {
		switch kv.Key {
		case "name":
			_ = json.Unmarshal(kv.Value, &c.Name)
		case "type":
			_ = json.Unmarshal(kv.Value, &c.Type)
		case "args":
			c.Args = kv.Value
		case "defaultTo":
			c.DefaultTo = kv.Value
		case "notNullable":
			_ = json.Unmarshal(kv.Value, &c.NotNullable)
		case "unsigned":
			_ = json.Unmarshal(kv.Value, &c.Unsigned)
		default:
			c.Extra = setKV(c.Extra, kv)
		}
	}
	return c
}

func setKV(kvs []KV, kv KV) []KV {
	for i := range kvs {
		if kvs[i].Key == kv.Key {
			kvs[i].Value = kv.Value
			return kvs
		}
	}
	return append(kvs, kv)
}

type knexType struct {
	typ         string
	args        json.RawMessage
	notNullable bool
}

var (
	argsLongText = json.RawMessage(`["longtext"]`)
	argsDateTime = json.RawMessage(`[{"useTz":false,"precision":6}]`)
)

func columnType(attrType string) (knexType, error) {
	switch attrType {
	case "increments":
		return knexType{typ: "increments", args: json.RawMessage(`[{"primary":true,"primaryKey":true}]`), notNullable: true}, nil
	case schema.TypePassword, schema.TypeEmail, schema.TypeString, schema.TypeEnumeration, schema.TypeUID:
		return knexType{typ: "string"}, nil
	case schema.TypeRichText, schema.TypeText:
		return knexType{typ: "text", args: argsLongText}, nil
	case schema.TypeBlocks, schema.TypeJSON:
		return knexType{typ: "jsonb"}, nil
	case schema.TypeInteger:
		return knexType{typ: "integer"}, nil
	case schema.TypeBigInteger:
		return knexType{typ: "bigInteger"}, nil
	case schema.TypeFloat:
		return knexType{typ: "double"}, nil
	case schema.TypeDecimal:
		return knexType{typ: "decimal", args: json.RawMessage(`[10,2]`)}, nil
	case schema.TypeDate:
		return knexType{typ: "date"}, nil
	case schema.TypeTime:
		return knexType{typ: "time", args: json.RawMessage(`[{"precision":3}]`)}, nil
	case schema.TypeDateTime:
		return knexType{typ: "datetime", args: argsDateTime}, nil
	case schema.TypeTimestamp:
		return knexType{typ: "timestamp", args: argsDateTime}, nil
	case schema.TypeBoolean:
		return knexType{typ: "boolean"}, nil
	}
	return knexType{}, fmt.Errorf("unknown type %s", attrType)
}
