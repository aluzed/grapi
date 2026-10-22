package schema

import (
	"encoding/json"
	"fmt"
)

// Component is a src/components/<category>/<name>.json file. Components have an
// id but no documentId, timestamps or publication state.
type Component struct {
	CollectionName string                      `json:"collectionName,omitempty"`
	Info           Info                        `json:"info"`
	Options        OrderedMap[json.RawMessage] `json:"options,omitzero"`
	Attributes     OrderedMap[*Attribute]      `json:"attributes"`
	Extra          map[string]json.RawMessage  `json:"-"`

	UID       UID    `json:"-"`
	Category  string `json:"-"`
	ModelName string `json:"-"`
	GlobalID  string `json:"-"`
}

type componentFields Component

// ParseComponent decodes a component file without deriving anything: see Prepare.
func ParseComponent(data []byte) (*Component, error) {
	var c Component
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// UnmarshalJSON keeps unknown top-level keys in Extra.
func (c *Component) UnmarshalJSON(data []byte) error {
	var f componentFields
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*c = Component(f)
	extra, err := unknownKeys(data, "collectionName", "info", "options", "attributes")
	c.Extra = extra
	return err
}

// MarshalJSON writes the known keys then Extra.
func (c Component) MarshalJSON() ([]byte, error) { return withExtra(componentFields(c), c.Extra) }

// Prepare derives uid, model name and global id from the file location.
func (c *Component) Prepare(category, name string) error {
	c.UID = ComponentUID(category, name)
	if c.CollectionName == "" {
		return fmt.Errorf("component %s is missing a \"collectionName\" property", c.UID)
	}
	c.Category = category
	c.ModelName = name
	if raw, ok := c.Extra["globalId"]; ok {
		_ = json.Unmarshal(raw, &c.GlobalID)
	}
	if c.GlobalID == "" {
		c.GlobalID = pascalCase("component_" + string(c.UID))
	}
	for attrName, a := range c.Attributes.All() {
		if a.Type == TypeDynamicZone {
			return fmt.Errorf("component %s: attribute %q: dynamic zones are not allowed in components", c.UID, attrName)
		}
	}
	return nil
}
