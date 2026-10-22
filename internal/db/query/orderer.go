package query

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrInvalidRelation is Strapi's InvalidRelationError (a 400 at the API).
var ErrInvalidRelation = errors.New("invalid relation")

// RelationError carries Strapi's message for an invalid connect.
type RelationError struct{ Msg string }

func (e *RelationError) Error() string { return e.Msg }

// Unwrap lets errors.Is match ErrInvalidRelation.
func (e *RelationError) Unwrap() error { return ErrInvalidRelation }

// Position places a connected relation: before or after another one, at the
// start or at the end.
type Position struct {
	Before, After any
	Start, End    bool
	// keys keeps the order of the position's keys for error messages.
	keys []string
}

func (p *Position) before() string { return idKey(p.Before) }
func (p *Position) after() string  { return idKey(p.After) }

// adjacent is "before || after" in JavaScript.
func (p *Position) adjacent() string {
	if p == nil {
		return ""
	}
	if b := p.before(); b != "" {
		return b
	}
	return p.after()
}

func (p *Position) json() string {
	if p == nil {
		return "undefined"
	}
	keys := p.keys
	if len(keys) == 0 {
		for _, k := range []string{"before", "after", "start", "end"} {
			if p.has(k) {
				keys = append(keys, k)
			}
		}
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		var v string
		switch k {
		case "before":
			v = jsonScalar(p.Before)
		case "after":
			v = jsonScalar(p.After)
		case "start":
			v = strconv.FormatBool(p.Start)
		case "end":
			v = strconv.FormatBool(p.End)
		default:
			continue
		}
		parts = append(parts, strconv.Quote(k)+":"+v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func (p *Position) has(k string) bool {
	switch k {
	case "before":
		return p.Before != nil
	case "after":
		return p.After != nil
	case "start":
		return p.Start
	case "end":
		return p.End
	}
	return false
}

func jsonScalar(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case nil:
		return "null"
	}
	return jsString(v)
}

// Link is a relation being connected, or one already stored.
type Link struct {
	ID        any
	Position  *Position
	Order     float64
	Component string
	init      bool
}

// idKey is the string form of an id, the key JavaScript objects use.
func idKey(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == 0 {
			return ""
		}
		return formatJSNumber(x)
	case int64:
		if x == 0 {
			return ""
		}
		return strconv.FormatInt(x, 10)
	case int:
		if x == 0 {
			return ""
		}
		return strconv.Itoa(x)
	}
	return fmt.Sprint(v)
}

// sortConnectArray orders a connect payload so that every relation comes
// after the one it is positioned against, like Strapi's sortConnectArray.
func sortConnectArray(connect []*Link, initial []*Link, strict bool) ([]*Link, error) {
	inInitial := map[string]bool{}
	for _, r := range initial {
		inInitial[idKey(r.ID)] = true
	}
	type mapped struct {
		link     *Link
		computed bool
	}
	byID := map[string]*mapped{}
	needsSorting := false
	for _, r := range connect {
		adj := r.Position.adjacent()
		if adj == "" || (!inInitial[adj] && byID[adj] == nil) {
			needsSorting = true
		}
		if existing := byID[idKey(r.ID)]; existing != nil {
			if existing.link.Component == "" || existing.link.Component == r.Component {
				return nil, &RelationError{fmt.Sprintf("The relation with id %s is already connected. You cannot connect the same relation twice.", jsString(r.ID))}
			}
			continue
		}
		byID[idKey(r.ID)] = &mapped{link: r}
	}
	if !needsSorting {
		return connect, nil
	}
	var sorted []*Link
	var compute func(r *Link, seen map[string]bool) error
	compute = func(r *Link, seen map[string]bool) error {
		adj := r.Position.adjacent()
		if adj != "" && seen[adj] {
			return &RelationError{"A circular reference was found in the connect array. One relation is trying to connect before/after another one that is trying to connect before/after it"}
		}
		m := byID[idKey(r.ID)]
		if m != nil && m.computed {
			return nil
		}
		if m != nil {
			m.computed = true
		}
		if adj == "" || inInitial[adj] {
			sorted = append(sorted, r)
			return nil
		}
		if next := byID[adj]; next != nil {
			branch := map[string]bool{idKey(r.ID): true}
			for k, v := range seen {
				branch[k] = v
			}
			if err := compute(next.link, branch); err != nil {
				return err
			}
			sorted = append(sorted, r)
			return nil
		}
		if strict {
			return &RelationError{fmt.Sprintf("There was a problem connecting relation with id %s at position %s. The relation with id %s needs to be connected first.", jsString(r.ID), r.Position.json(), adj)}
		}
		sorted = append(sorted, &Link{ID: r.ID, Position: &Position{End: true}, Component: r.Component})
		return nil
	}
	for _, r := range connect {
		if err := compute(r, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	return sorted, nil
}

// Orderer computes the order values of connected relations, Strapi's
// relationsOrderer: fractional orders between stored neighbours.
type Orderer struct {
	rels     []*Link
	maxOrder float64
	strict   bool
}

// NewOrderer starts from the stored relations (a null order counts as 1).
func NewOrderer(initial []*Link, strict bool) *Orderer {
	rels := make([]*Link, len(initial))
	for i, r := range initial {
		c := *r
		c.init = true
		rels[i] = &c
	}
	sort.SliceStable(rels, func(i, j int) bool { return rels[i].Order < rels[j].Order })
	o := &Orderer{rels: rels, strict: strict}
	for _, r := range rels {
		if r.Order > o.maxOrder {
			o.maxOrder = r.Order
		}
	}
	return o
}

func (o *Orderer) find(id any) int {
	k := idKey(id)
	for i, r := range o.rels {
		if idKey(r.ID) == k {
			return i
		}
	}
	return -1
}

// Disconnect removes relations.
func (o *Orderer) Disconnect(links ...*Link) {
	for _, l := range links {
		if i := o.find(l.ID); i >= 0 {
			o.rels = append(o.rels[:i], o.rels[i+1:]...)
		}
	}
}

// Connect inserts relations at their positions.
func (o *Orderer) Connect(links []*Link) error {
	sorted, err := sortConnectArray(links, o.rels, o.strict)
	if err != nil {
		return err
	}
	for _, l := range sorted {
		o.Disconnect(l)
		if err := o.insert(l); err != nil {
			return fmt.Errorf("There was a problem connecting relation with id %s at position %s. The list of connect relations is not valid", jsString(l.ID), l.Position.json()) //nolint:revive,staticcheck // Strapi's message
		}
	}
	return nil
}

func (o *Orderer) insert(src *Link) error {
	r := &Link{ID: src.ID, Position: src.Position, Component: src.Component}
	var idx int
	p := r.Position
	switch {
	case p != nil && p.before() != "":
		i := o.find(p.Before)
		if i < 0 {
			return errors.New("missing")
		}
		r.Order = o.rels[i].Order
		if o.rels[i].init {
			r.Order -= 0.5
		}
		idx = i
	case p != nil && p.after() != "":
		i := o.find(p.After)
		if i < 0 {
			return errors.New("missing")
		}
		r.Order = o.rels[i].Order
		if o.rels[i].init {
			r.Order += 0.5
		}
		idx = i + 1
	case p != nil && p.Start:
		if len(o.rels) > 0 {
			first := o.rels[0]
			r.Order = first.Order
			if first.init {
				r.Order -= 0.5
			}
		} else {
			r.Order = 0.5
		}
		idx = 0
	default:
		r.Order = o.maxOrder + 0.5
		idx = len(o.rels)
	}
	o.rels = append(o.rels, nil)
	copy(o.rels[idx+1:], o.rels[idx:])
	o.rels[idx] = r
	return nil
}

// Relations returns the relations in their computed order.
func (o *Orderer) Relations() []*Link { return o.rels }

// OrderMap spreads the new relations sharing an order value between that
// value and the next half unit, by id.
func (o *Orderer) OrderMap() map[string]float64 {
	groups := map[float64][]*Link{}
	var orders []float64
	for _, r := range o.rels {
		if _, ok := groups[r.Order]; !ok {
			orders = append(orders, r.Order)
		}
		groups[r.Order] = append(groups[r.Order], r)
	}
	sort.Float64s(orders)
	out := map[string]float64{}
	for _, order := range orders {
		rels := groups[order]
		offset := 1.0
		for _, r := range rels {
			if !r.init {
				out[idKey(r.ID)] = order + (offset/float64(len(rels)+1))*0.49
				offset++
			}
		}
	}
	return out
}
