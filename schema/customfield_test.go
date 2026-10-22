package schema

import (
	"encoding/json"
	"testing"
)

func TestRegisterCustomField(t *testing.T) {
	var r CustomFields
	installed := func(p string) bool { return p == "color-picker" }
	uid, err := r.Register(CustomField{Name: "color", Plugin: "color-picker", Type: TypeString}, installed)
	if err != nil || uid != "plugin::color-picker.color" {
		t.Fatalf("uid = %q, %v", uid, err)
	}
	if uid, _ := r.Register(CustomField{Name: "rating", Plugin: "missing", Type: TypeInteger}, installed); uid != "global::rating" {
		t.Errorf("unknown plugin must fall back to global, got %q", uid)
	}
	bad := []CustomField{
		{Name: "x", Type: TypeMedia},
		{Name: "x", Type: TypeRelation},
		{Name: "1x", Type: TypeString},
		{Name: "", Type: TypeString},
		{Name: "color", Plugin: "color-picker", Type: TypeString},
		{Name: "y", Type: TypeString, InputSize: &InputSize{Default: 5}},
	}
	for _, cf := range bad {
		if _, err := r.Register(cf, installed); err == nil {
			t.Errorf("Register(%+v) should fail", cf)
		}
	}
}

func TestResolveCustomFields(t *testing.T) {
	var r CustomFields
	_, _ = r.Register(CustomField{Name: "color", Plugin: "color-picker", Type: TypeString}, func(string) bool { return true })
	src := `{"collectionName":"as","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"c":{"type":"customField","customField":"plugin::color-picker.color","options":{"format":"hex"}},"u":{"type":"customField","customField":"plugin::nope.nope"}}}`
	c, _ := ParseContentType([]byte(src))
	_ = c.Prepare(APIUID("a", "a"), "")
	s := &Set{}
	s.AddContentType(c)
	s.ResolveCustomFields(&r)
	attr, _ := c.Attributes.Get("c")
	if attr.Type != TypeString || !attr.IsCustomField() {
		t.Errorf("resolved attribute = %+v", attr)
	}
	out, _ := json.Marshal(attr)
	var back map[string]any
	_ = json.Unmarshal(out, &back)
	if back["type"] != "customField" || back["customField"] != "plugin::color-picker.color" {
		t.Errorf("custom field must serialize as declared: %s", out)
	}
	if !hasIssue(Validate(s), CodeUnknownType, SeverityError) {
		t.Error("unregistered custom field must be an error")
	}
}
