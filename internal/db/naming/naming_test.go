package naming

import (
	"encoding/json"
	"os"
	"testing"
)

type goldenCase struct {
	MaxLength int      `json:"maxLength"`
	Method    string   `json:"method"`
	Args      []string `json:"args"`
	Prefix    string   `json:"prefix"`
	Suffix    string   `json:"suffix"`
	Result    string   `json:"result"`
	Error     bool     `json:"error"`
}

func call(n Namer, c goldenCase) (string, error) {
	a := c.Args
	switch c.Method {
	case "getTableName":
		return n.Table(a[0])
	case "getColumnName":
		return n.Column(a[0])
	case "getJoinTableName":
		return n.JoinTable(a[0], a[1])
	case "getMorphTableName":
		return n.MorphTable(a[0], a[1])
	case "getJoinColumnAttributeIdName":
		return n.JoinColumnID(a[0])
	case "getInverseJoinColumnAttributeIdName":
		return n.InverseJoinColumnID(a[0])
	case "getOrderColumnName":
		return n.OrderColumnName(a[0])
	case "getInverseOrderColumnName":
		return n.InverseOrderColumnName(a[0])
	case "getMorphColumnJoinTableIdName":
		return n.MorphJoinTableID(a[0])
	case "getMorphColumnAttributeIdName":
		return n.MorphAttributeID(a[0])
	case "getMorphColumnTypeName":
		return n.MorphType(a[0])
	case "getIndexName":
		return n.Index(a...)
	case "getFkIndexName":
		return n.FkIndex(a...)
	case "getUniqueIndexName":
		return n.UniqueIndex(a...)
	case "getPrimaryIndexName":
		return n.PrimaryIndex(a...)
	case "getInverseFkIndexName":
		return n.InverseFkIndex(a...)
	case "getOrderFkIndexName":
		return n.OrderFkIndex(a...)
	case "getOrderInverseFkIndexName":
		return n.OrderInverseFkIndex(a...)
	case "getIdColumnIndexName":
		return n.IDColumnIndex(a...)
	case "getOrderIndexName":
		return n.OrderIndex(a...)
	case "getName":
		return n.Name(a, Options{Prefix: c.Prefix, Suffix: c.Suffix})
	}
	panic("unknown method " + c.Method)
}

// testdata/golden.json comes from Strapi's Identifiers (compat/tools/gen-naming-golden.js).
func TestNamesMatchStrapi(t *testing.T) {
	raw, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got, err := call(New(c.MaxLength), c)
		switch {
		case c.Error && err == nil:
			t.Errorf("%s(%q) max=%d: want an error, got %q", c.Method, c.Args, c.MaxLength, got)
		case !c.Error && err != nil:
			t.Errorf("%s(%q) max=%d: unexpected error %v", c.Method, c.Args, c.MaxLength, err)
		case !c.Error && got != c.Result:
			t.Errorf("%s(%q) max=%d = %q, want %q", c.Method, c.Args, c.MaxLength, got, c.Result)
		}
	}
}

func TestHash(t *testing.T) {
	// Values from Strapi's own identifiers tests.
	if got := Hash("123456789", 2); got != "24" {
		t.Errorf("Hash 2 = %q", got)
	}
	if got := Hash("123456789", 50); got != "24347b9c4b6da2fc9cde08c87f33edd2e603c8dcd6840e6b39" {
		t.Errorf("Hash 50 = %q", got)
	}
}
