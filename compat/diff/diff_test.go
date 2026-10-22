package diff

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestValues(t *testing.T) {
	var a, b any
	_ = json.Unmarshal([]byte(`{"status":200,"body":{"data":[{"title":"x","n":1}],"meta":{}}}`), &a)
	_ = json.Unmarshal([]byte(`{"body":{"meta":{},"data":[{"name":"x","n":1}]},"status":200}`), &b)
	got := Values("", a, b)
	want := []string{
		`body.data[0].name: only in B = "x"`,
		`body.data[0].title: only in A = "x"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values = %q, want %q", got, want)
	}
}

func TestValuesKeepsArrayOrder(t *testing.T) {
	got := Values("data", []any{"a", "b"}, []any{"b", "a"})
	if len(got) != 2 {
		t.Errorf("reordered array must differ, got %q", got)
	}
}
