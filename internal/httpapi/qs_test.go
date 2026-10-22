package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Results of the qs package itself, with Strapi's settings.
func TestParseQueryMatchesQs(t *testing.T) {
	b, err := os.ReadFile("testdata/qs-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input  string `json:"input"`
		Output any    `json:"output"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got, _ := json.Marshal(ParseQuery(c.Input))
		var g any
		_ = json.Unmarshal(got, &g)
		if !reflect.DeepEqual(g, c.Output) {
			want, _ := json.Marshal(c.Output)
			t.Errorf("%q:\n got %s\nwant %s", c.Input, got, want)
		}
	}
}
