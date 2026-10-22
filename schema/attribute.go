package schema

import (
	"encoding/json"
	"fmt"
)

// Attribute types of Strapi 5.
const (
	TypeString      = "string"
	TypeText        = "text"
	TypeRichText    = "richtext"
	TypeBlocks      = "blocks"
	TypeEmail       = "email"
	TypePassword    = "password"
	TypeUID         = "uid"
	TypeEnumeration = "enumeration"
	TypeInteger     = "integer"
	TypeBigInteger  = "biginteger"
	TypeFloat       = "float"
	TypeDecimal     = "decimal"
	TypeDate        = "date"
	TypeTime        = "time"
	TypeDateTime    = "datetime"
	TypeTimestamp   = "timestamp"
	TypeBoolean     = "boolean"
	TypeJSON        = "json"
	TypeMedia       = "media"
	TypeRelation    = "relation"
	TypeComponent   = "component"
	TypeDynamicZone = "dynamiczone"
	TypeCustomField = "customField"
)

var scalarTypes = map[string]bool{
	TypeString: true, TypeText: true, TypeRichText: true, TypeBlocks: true, TypeEmail: true,
	TypePassword: true, TypeUID: true, TypeEnumeration: true, TypeInteger: true,
	TypeBigInteger: true, TypeFloat: true, TypeDecimal: true, TypeDate: true, TypeTime: true,
	TypeDateTime: true, TypeTimestamp: true, TypeBoolean: true, TypeJSON: true,
}

// Relation kinds.
const (
	OneToOne    = "oneToOne"
	OneToMany   = "oneToMany"
	ManyToOne   = "manyToOne"
	ManyToMany  = "manyToMany"
	OneWay      = "oneWay"
	ManyWay     = "manyWay"
	MorphOne    = "morphOne"
	MorphMany   = "morphMany"
	MorphToOne  = "morphToOne"
	MorphToMany = "morphToMany"
)

var relationKinds = map[string]bool{
	OneToOne: true, OneToMany: true, ManyToOne: true, ManyToMany: true, OneWay: true,
	ManyWay: true, MorphOne: true, MorphMany: true, MorphToOne: true, MorphToMany: true,
}

// Attribute is one entry of a schema's "attributes". A single struct covers
// every type: fields that do not apply to a type stay nil. Keys grapi does not
// know are kept in Extra so that a schema survives a load and save unchanged.
type Attribute struct {
	Type string `json:"type"`

	Required     *bool `json:"required,omitempty"`
	Unique       *bool `json:"unique,omitempty"`
	Private      *bool `json:"private,omitempty"`
	Configurable *bool `json:"configurable,omitempty"`
	Writable     *bool `json:"writable,omitempty"`
	Visible      *bool `json:"visible,omitempty"`
	Searchable   *bool `json:"searchable,omitempty"`

	Default   json.RawMessage `json:"default,omitempty"`
	Min       json.RawMessage `json:"min,omitempty"`
	Max       json.RawMessage `json:"max,omitempty"`
	MinLength *int            `json:"minLength,omitempty"`
	MaxLength *int            `json:"maxLength,omitempty"`
	Regex     string          `json:"regex,omitempty"`

	Enum     []string `json:"enum,omitempty"`
	EnumName string   `json:"enumName,omitempty"`

	TargetField string          `json:"targetField,omitempty"`
	Options     json.RawMessage `json:"options,omitempty"`

	Multiple     *bool    `json:"multiple,omitempty"`
	AllowedTypes []string `json:"allowedTypes,omitempty"`

	Relation     string `json:"relation,omitempty"`
	Target       string `json:"target,omitempty"`
	InversedBy   string `json:"inversedBy,omitempty"`
	MappedBy     string `json:"mappedBy,omitempty"`
	MorphBy      string `json:"morphBy,omitempty"`
	UseJoinTable *bool  `json:"useJoinTable,omitempty"`

	Component  string   `json:"component,omitempty"`
	Repeatable *bool    `json:"repeatable,omitempty"`
	Components []string `json:"components,omitempty"`

	CustomField string `json:"customField,omitempty"`

	PluginOptions json.RawMessage `json:"pluginOptions,omitempty"`
	Conditions    json.RawMessage `json:"conditions,omitempty"`
	Column        json.RawMessage `json:"column,omitempty"`

	Extra map[string]json.RawMessage `json:"-"`

	// Auto marks the attributes grapi adds itself (timestamps, publishedAt,
	// creator fields): they are not part of the user's schema.json.
	Auto bool `json:"-"`

	resolvedCustomField bool
	empty               emptyKeys
}

type attributeFields Attribute

var knownAttributeKeys = func() map[string]bool {
	keys := map[string]bool{}
	b, _ := json.Marshal(attributeFields{
		Type: "x", Required: new(bool), Unique: new(bool), Private: new(bool),
		Configurable: new(bool), Writable: new(bool), Visible: new(bool), Searchable: new(bool),
		Default: json.RawMessage("0"), Min: json.RawMessage("0"), Max: json.RawMessage("0"),
		MinLength: new(int), MaxLength: new(int), Regex: "x", Enum: []string{"x"}, EnumName: "x",
		TargetField: "x", Options: json.RawMessage("0"), Multiple: new(bool),
		AllowedTypes: []string{"x"}, Relation: "x", Target: "x", InversedBy: "x", MappedBy: "x",
		MorphBy: "x", UseJoinTable: new(bool), Component: "x", Repeatable: new(bool),
		Components: []string{"x"}, CustomField: "x", PluginOptions: json.RawMessage("0"),
		Conditions: json.RawMessage("0"), Column: json.RawMessage("0"),
	})
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	for k := range m {
		keys[k] = true
	}
	return keys
}()

// UnmarshalJSON decodes the known keys and keeps the others in Extra.
func (a *Attribute) UnmarshalJSON(data []byte) error {
	var f attributeFields
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	*a = Attribute(f)
	for k, v := range all {
		if !knownAttributeKeys[k] {
			if a.Extra == nil {
				a.Extra = map[string]json.RawMessage{}
			}
			a.Extra[k] = v
		}
	}
	if a.Type == "" {
		return fmt.Errorf("missing attribute type")
	}
	a.empty = collectEmpty(data)
	return nil
}

// MarshalJSON writes the known keys then Extra.
func (a Attribute) MarshalJSON() ([]byte, error) {
	if a.resolvedCustomField {
		a.Type = TypeCustomField
	}
	b, err := withExtra(attributeFields(a), a.Extra)
	if err != nil {
		return nil, err
	}
	return a.empty.restore(b)
}

// IsScalar reports whether the attribute is stored in a column of its own table.
func (a *Attribute) IsScalar() bool { return scalarTypes[a.Type] }

// IsRelation reports whether the attribute is a relation (media excluded).
func (a *Attribute) IsRelation() bool { return a.Type == TypeRelation }

// IsMorph reports whether the attribute is a polymorphic relation.
func (a *Attribute) IsMorph() bool {
	switch a.Relation {
	case MorphOne, MorphMany, MorphToOne, MorphToMany:
		return true
	}
	return false
}

// IsPrivate reports whether the attribute is hidden from API responses.
func (a *Attribute) IsPrivate() bool { return is(a.Private, false) }

// IsRequired reports whether a value is mandatory (enforced at publish for drafts).
func (a *Attribute) IsRequired() bool { return is(a.Required, false) }

// IsUnique reports whether values must be unique among published entries.
func (a *Attribute) IsUnique() bool { return is(a.Unique, false) }

// IsWritable reports whether API input may set the attribute.
func (a *Attribute) IsWritable() bool { return is(a.Writable, true) }

// IsConfigurable reports whether the content-type builder may edit the attribute.
func (a *Attribute) IsConfigurable() bool { return is(a.Configurable, true) }

// IsVisible reports whether the admin shows the attribute.
func (a *Attribute) IsVisible() bool { return is(a.Visible, true) }

// IsRepeatable reports whether a component attribute holds a list.
func (a *Attribute) IsRepeatable() bool { return is(a.Repeatable, false) }

// IsMultiple reports whether a media attribute holds several files.
func (a *Attribute) IsMultiple() bool { return is(a.Multiple, false) }

func is(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

func boolPtr(b bool) *bool { return &b }

// IsRelationKind reports whether kind is one of the ten relation kinds of Strapi 5.
func IsRelationKind(kind string) bool { return relationKinds[kind] }

// IsScalarType reports whether t is a scalar attribute type.
func IsScalarType(t string) bool { return scalarTypes[t] }

// IsCustomField reports whether the attribute was declared as a custom field.
func (a *Attribute) IsCustomField() bool { return a.resolvedCustomField || a.Type == TypeCustomField }
