package query

import (
	"strings"
	"testing"
)

func rel(id any, order float64) *Link { return &Link{ID: id, Order: order} }

func conn(id any, p *Position) *Link { return &Link{ID: id, Position: p} }

func ids(links []*Link) string {
	var out []string
	for _, l := range links {
		out = append(out, jsString(l.ID)+"@"+formatJSNumber(l.Order))
	}
	return strings.Join(out, " ")
}

// Cases of Strapi's relations-orderer.test.ts.
func TestOrderer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial []*Link
		strict  bool
		connect []*Link
		want    string
	}{
		{"end", []*Link{rel(2, 4), rel(3, 10)}, true, []*Link{conn(4, &Position{End: true}), conn(5, nil)}, "2@4 3@10 4@10.5 5@10.5"},
		{"start", []*Link{rel(2, 4), rel(3, 10)}, true, []*Link{conn(4, &Position{Start: true})}, "4@3.5 2@4 3@10"},
		{"before chain", []*Link{rel(2, 4), rel(3, 10)}, true, []*Link{conn(4, &Position{Before: 3}), conn(5, &Position{Before: 4})}, "2@4 5@9.5 4@9.5 3@10"},
		{"disordered", []*Link{rel(1, 1), rel(2, 2), rel(3, 3)}, true, []*Link{conn(5, &Position{Before: 1}), conn(1, &Position{Before: 2}), conn(2, &Position{End: true})}, "5@0.5 1@1.5 3@3 2@3.5"},
		{"non strict missing", []*Link{rel(1, 1), rel(2, 2), rel(3, 3)}, false, []*Link{conn(4, &Position{Before: 5})}, "1@1 2@2 3@3 4@3.5"},
		{"null orders", []*Link{rel(2, 1), rel(3, 1)}, true, []*Link{conn(4, &Position{Before: 3}), conn(5, nil)}, "2@1 4@0.5 3@1 5@1.5"},
		{"order zero", []*Link{rel(1, 0), rel(2, 5)}, true, []*Link{conn(3, &Position{Start: true})}, "3@-0.5 1@0 2@5"},
		{"after shared order", []*Link{rel(2, 1), rel(3, 1)}, true, []*Link{conn(4, &Position{After: 2})}, "2@1 4@1.5 3@1"},
		{"empty starts", nil, true, []*Link{conn(1, &Position{Start: true}), conn(2, &Position{Start: true}), conn(3, &Position{After: 1})}, "2@0.5 1@0.5 3@0.5"},
		{"empty disordered", nil, true, []*Link{conn(5, &Position{Before: 1}), conn(1, &Position{Before: 2}), conn(2, &Position{End: true}), conn(3, &Position{After: 1})}, "5@0.5 1@0.5 3@0.5 2@0.5"},
	} {
		o := NewOrderer(tc.initial, tc.strict)
		if err := o.Connect(tc.connect); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := ids(o.Relations()); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, got, tc.want)
		}
	}
	o := NewOrderer([]*Link{rel(1, 0.5), rel(2, 2)}, true)
	_ = o.Connect([]*Link{conn(3, &Position{Start: true})})
	if m := o.OrderMap(); m["3"] >= 0.5 {
		t.Errorf("start below a fractional minimum: %v", m)
	}
	o = NewOrderer([]*Link{rel(1, 1), rel(5, 5), rel(10, 10)}, true)
	_ = o.Connect([]*Link{conn(6, &Position{Before: 5}), conn(7, &Position{Before: 5})})
	if m := o.OrderMap(); !(m["6"] > 4 && m["7"] < 5 && m["6"] < m["7"]) {
		t.Errorf("several before the same anchor: %v", m)
	}
}

func TestSortConnectArray(t *testing.T) {
	links := []*Link{conn(5, &Position{Before: 1}), conn(1, &Position{Before: 2}), conn(2, &Position{End: true}), conn(3, &Position{After: 1})}
	got, err := sortConnectArray(links, nil, true)
	if err != nil || ids(got) != "2@0 1@0 5@0 3@0" {
		t.Errorf("got %s, %v", ids(got), err)
	}
	got, _ = sortConnectArray(links, []*Link{{ID: 1}}, true)
	if ids(got) != "5@0 2@0 1@0 3@0" {
		t.Errorf("with initial: %s", ids(got))
	}
	for _, tc := range []struct {
		links []*Link
		want  string
	}{
		{[]*Link{conn(1, &Position{After: 2, keys: []string{"after"}})}, `There was a problem connecting relation with id 1 at position {"after":2}. The relation with id 2 needs to be connected first.`},
		{[]*Link{conn(2, &Position{After: 1}), conn(3, &Position{After: 1}), conn(1, &Position{After: 3})}, "A circular reference was found in the connect array. One relation is trying to connect before/after another one that is trying to connect before/after it"},
		{[]*Link{conn(1, &Position{After: 2}), conn(1, &Position{After: 3})}, "The relation with id 1 is already connected. You cannot connect the same relation twice."},
	} {
		if _, err := sortConnectArray(tc.links, nil, true); err == nil || err.Error() != tc.want {
			t.Errorf("got %v\nwant %s", err, tc.want)
		}
	}
}
