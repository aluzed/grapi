package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Kinds of content types.
const (
	CollectionType = "collectionType"
	SingleType     = "singleType"
)

// Names of the attributes grapi manages itself.
const (
	AttrID                = "id"
	AttrDocumentID        = "documentId"
	AttrCreatedAt         = "createdAt"
	AttrUpdatedAt         = "updatedAt"
	AttrPublishedAt       = "publishedAt"
	AttrCreatedBy         = "createdBy"
	AttrUpdatedBy         = "updatedBy"
	AttrLocale            = "locale"
	AttrLocalizations     = "localizations"
	AttrComponentKey      = "__component"
	adminUserUID      UID = "admin::user"
)

// Info is the "info" block of a schema.
type Info struct {
	SingularName string                     `json:"singularName,omitempty"`
	PluralName   string                     `json:"pluralName,omitempty"`
	DisplayName  string                     `json:"displayName,omitempty"`
	Description  string                     `json:"description,omitempty"`
	Icon         string                     `json:"icon,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
}

type infoFields Info

// UnmarshalJSON keeps unknown keys in Extra.
func (i *Info) UnmarshalJSON(data []byte) error {
	var f infoFields
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*i = Info(f)
	extra, err := unknownKeys(data, "singularName", "pluralName", "displayName", "description", "icon")
	i.Extra = extra
	return err
}

// MarshalJSON writes the known keys then Extra.
func (i Info) MarshalJSON() ([]byte, error) { return withExtra(infoFields(i), i.Extra) }

// ContentType is a schema.json of src/api or of a plugin, plus what grapi derives from it.
type ContentType struct {
	Kind           string                      `json:"kind,omitempty"`
	CollectionName string                      `json:"collectionName,omitempty"`
	Info           Info                        `json:"info"`
	Options        OrderedMap[json.RawMessage] `json:"options,omitzero"`
	PluginOptions  json.RawMessage             `json:"pluginOptions,omitempty"`
	Attributes     OrderedMap[*Attribute]      `json:"attributes"`
	Indexes        json.RawMessage             `json:"indexes,omitempty"`
	ForeignKeys    json.RawMessage             `json:"foreignKeys,omitempty"`
	Extra          map[string]json.RawMessage  `json:"-"`

	UID       UID      `json:"-"`
	ModelName string   `json:"-"`
	GlobalID  string   `json:"-"`
	Warnings  []string `json:"-"`
}

type contentTypeFields ContentType

var contentTypeKeys = []string{
	"kind", "collectionName", "info", "options", "pluginOptions", "attributes", "indexes", "foreignKeys",
}

// ParseContentType decodes a schema.json without deriving anything: see Prepare.
func ParseContentType(data []byte) (*ContentType, error) {
	var ct ContentType
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, err
	}
	return &ct, nil
}

// UnmarshalJSON keeps unknown top-level keys in Extra.
func (ct *ContentType) UnmarshalJSON(data []byte) error {
	var f contentTypeFields
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*ct = ContentType(f)
	extra, err := unknownKeys(data, contentTypeKeys...)
	ct.Extra = extra
	return err
}

// MarshalJSON writes the schema as the user wrote it: attributes added by
// Prepare are left out.
func (ct ContentType) MarshalJSON() ([]byte, error) {
	out := contentTypeFields(ct)
	out.Attributes = OrderedMap[*Attribute]{}
	for name, a := range ct.Attributes.All() {
		if !a.Auto {
			out.Attributes.Set(name, a)
		}
	}
	return withExtra(out, ct.Extra)
}

// DraftAndPublish reports whether options.draftAndPublish is true (false by default).
func (ct *ContentType) DraftAndPublish() bool { return ct.boolOption("draftAndPublish") }

// PopulateCreatorFields reports whether createdBy and updatedBy are public.
func (ct *ContentType) PopulateCreatorFields() bool { return ct.boolOption("populateCreatorFields") }

// IsSingleType reports whether the content type holds a single document per locale.
func (ct *ContentType) IsSingleType() bool { return ct.Kind == SingleType }

func (ct *ContentType) boolOption(name string) bool {
	raw, ok := ct.Options.Get(name)
	if !ok {
		return false
	}
	var b bool
	return json.Unmarshal(raw, &b) == nil && b
}

// Prepare derives uid, model name and global id, applies the defaults and adds
// the attributes every content type has. globalPrefix is the plugin name for
// plugin types, "admin" for admin types, and empty for api types.
func (ct *ContentType) Prepare(uid UID, globalPrefix string) error {
	if ct.Info.SingularName == "" || ct.Info.PluralName == "" || ct.Info.DisplayName == "" {
		return fmt.Errorf("%s: info.singularName, info.pluralName and info.displayName are required", uid)
	}
	if !isKebabCase(ct.Info.SingularName) || !isKebabCase(ct.Info.PluralName) {
		return fmt.Errorf("%s: info.singularName and info.pluralName must be in kebab case", uid)
	}
	if ct.CollectionName == "" {
		return fmt.Errorf("%s: collectionName is required", uid)
	}
	for name := range ct.Attributes.All() {
		if isLoadReserved(name) {
			return fmt.Errorf("the attribute %q is reserved and cannot be used in a model. Please rename %q attribute %q to something else", name, ct.Info.SingularName, name)
		}
	}
	if ct.DraftAndPublish() {
		for name := range ct.Attributes.All() {
			if snakeCase(name) == "status" {
				ct.Warnings = append(ct.Warnings, fmt.Sprintf("attribute %q of %s conflicts with the status query parameter when draftAndPublish is enabled", name, uid))
			}
		}
	}

	ct.UID = uid
	if ct.Kind == "" {
		ct.Kind = CollectionType
	}
	ct.ModelName = ct.Info.SingularName
	if raw, ok := ct.Extra["globalId"]; ok {
		_ = json.Unmarshal(raw, &ct.GlobalID)
	}
	if ct.GlobalID == "" {
		name := ct.ModelName
		if globalPrefix != "" {
			name = globalPrefix + "-" + name
		}
		ct.GlobalID = pascalCase(name)
	}

	private := !ct.PopulateCreatorFields()
	ct.addAuto(AttrCreatedAt, &Attribute{Type: TypeDateTime})
	ct.addAuto(AttrUpdatedAt, &Attribute{Type: TypeDateTime})
	// Present even without draft and publish: the value is then always set.
	ct.addAuto(AttrPublishedAt, &Attribute{
		Type: TypeDateTime, Configurable: boolPtr(false), Writable: boolPtr(true), Visible: boolPtr(true),
	})
	for _, name := range []string{AttrCreatedBy, AttrUpdatedBy} {
		ct.addAuto(name, &Attribute{
			Type: TypeRelation, Relation: OneToOne, Target: string(adminUserUID),
			Configurable: boolPtr(false), Writable: boolPtr(false), Visible: boolPtr(false),
			UseJoinTable: boolPtr(false), Private: boolPtr(private),
		})
	}
	return nil
}

func (ct *ContentType) addAuto(name string, a *Attribute) {
	a.Auto = true
	ct.Attributes.Set(name, a)
}

func unknownKeys(data []byte, known ...string) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	for _, k := range known {
		delete(all, k)
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

func withExtra(v any, extra map[string]json.RawMessage) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil || len(extra) == 0 {
		return b, err
	}
	e, err := json.Marshal(extra)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(b, []byte("{}")) {
		return e, nil
	}
	b = bytes.TrimSuffix(b, []byte("}"))
	return append(append(b, ','), e[1:]...), nil
}
