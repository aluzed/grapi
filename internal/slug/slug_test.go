package slug

import (
	"encoding/json"
	"os"
	"testing"
)

type goldenCase struct {
	Input   string `json:"input"`
	Output  string `json:"output"`
	Options struct {
		Separator                 *string     `json:"separator"`
		Lowercase                 *bool       `json:"lowercase"`
		Decamelize                *bool       `json:"decamelize"`
		CustomReplacements        [][2]string `json:"customReplacements"`
		PreserveLeadingUnderscore bool        `json:"preserveLeadingUnderscore"`
	} `json:"options"`
}

// testdata/golden.json comes from the real slugify (compat/tools/gen-slug-golden.js).
func TestMakeMatchesSlugify(t *testing.T) {
	raw, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		o := Defaults()
		if c.Options.Separator != nil {
			o.Separator = *c.Options.Separator
		}
		if c.Options.Lowercase != nil {
			o.Lowercase = *c.Options.Lowercase
		}
		if c.Options.Decamelize != nil {
			o.Decamelize = *c.Options.Decamelize
		}
		o.CustomReplacements = c.Options.CustomReplacements
		o.PreserveLeadingUnderscore = c.Options.PreserveLeadingUnderscore
		if got := Make(c.Input, o); got != c.Output {
			t.Errorf("Make(%q, %+v) = %q, want %q", c.Input, c.Options, got, c.Output)
		}
	}
}
