package query

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/schema"
)

// Writes of Strapi's entity manager: rows, then their relations. grapi runs
// a write and its relations in one transaction, where Strapi uses two.

// Data is the decoded input of a write, keyed by attribute name.
type Data = map[string]any

func isValidID(v any) bool {
	switch x := v.(type) {
	case string:
		return true
	case float64:
		return x == math.Trunc(x) && !math.IsInf(x, 0)
	case int64, int:
		return true
	}
	return false
}

// normID gives integer ids one Go type.
func normID(v any) any {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	}
	return v
}

func toID(v any) (any, error) {
	if m, ok := v.(map[string]any); ok {
		if id, ok := m["id"]; ok && isValidID(id) {
			return normID(id), nil
		}
	}
	if isValidID(v) {
		return normID(v), nil
	}
	b, _ := json.Marshal(v)
	return nil, fmt.Errorf("Invalid id, expected a string or integer, got %s", b) //nolint:revive,staticcheck // Strapi's message
}

// relItem is one entry of a relation payload.
type relItem struct {
	ID       any
	Position *Position
	Pivot    map[string]any
	Type     string
	raw      any
}

// typeOf reads a morph item's type from its type field (__type, or
// __component for dynamic zones).
func (it relItem) typeOf(field string) string {
	if field == "" || field == "__type" {
		return it.Type
	}
	if m, ok := it.raw.(map[string]any); ok {
		if s, ok := m[field].(string); ok {
			return s
		}
	}
	return ""
}

// withType resolves the type of every item from field.
func withType(items []relItem, field string) []relItem {
	out := make([]relItem, len(items))
	for i, it := range items {
		it.Type = it.typeOf(field)
		out[i] = it
	}
	return out
}

func toIDArray(v any) ([]relItem, error) {
	var list []any
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []any:
		list = x
	default:
		list = []any{x}
	}
	var out []relItem
	seen := map[string]bool{}
	for _, d := range list {
		if d == nil {
			continue
		}
		var it relItem
		if isValidID(d) {
			it = relItem{ID: normID(d), Pivot: map[string]any{}, raw: map[string]any{"id": d, "__pivot": map[string]any{}}}
		} else {
			m, ok := d.(map[string]any)
			if !ok || !isValidID(m["id"]) {
				return nil, fmt.Errorf("Invalid id, expected a string or integer, got %v", jsString(d)) //nolint:revive,staticcheck // Strapi's message
			}
			it = relItem{ID: normID(m["id"]), raw: d}
			if p, ok := m["__pivot"].(map[string]any); ok {
				it.Pivot = p
			}
			if t, ok := m["__type"].(string); ok {
				it.Type = t
			}
			if p, ok := m["position"].(map[string]any); ok {
				it.Position = parsePosition(p)
			}
		}
		key, _ := json.Marshal(it.raw)
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		out = append(out, it)
	}
	return out, nil
}

func parsePosition(m map[string]any) *Position {
	p := &Position{Before: normID(m["before"]), After: normID(m["after"])}
	p.Start, _ = m["start"].(bool)
	p.End, _ = m["end"].(bool)
	for _, k := range []string{"before", "after", "start", "end"} {
		if _, ok := m[k]; ok {
			p.keys = append(p.keys, k)
		}
	}
	return p
}

// assocs is Strapi's toAssocs: a short form is a set, else connect and
// disconnect.
type assocs struct {
	set        []relItem
	hasSet     bool
	setNull    bool
	connect    []relItem
	disconnect []relItem
	strict     *bool
}

func toAssocs(v any) (assocs, error) {
	m, isMap := v.(map[string]any)
	_, hasID := m["id"]
	if !isMap || hasID {
		if v == nil {
			return assocs{hasSet: true, setNull: true}, nil
		}
		set, err := toIDArray(v)
		return assocs{set: set, hasSet: true}, err
	}
	if s, ok := m["set"]; ok && s != nil {
		if list, isList := s.([]any); !isList || len(list) > 0 || true {
			set, err := toIDArray(s)
			return assocs{set: set, hasSet: true}, err
		}
	}
	var a assocs
	if o, ok := m["options"].(map[string]any); ok {
		if st, ok := o["strict"].(bool); ok {
			a.strict = &st
		}
	}
	conn, err := toIDArray(m["connect"])
	if err != nil {
		return a, err
	}
	for i := range conn {
		if conn[i].Position == nil {
			conn[i].Position = &Position{End: true}
		}
		if conn[i].Pivot == nil {
			conn[i].Pivot = map[string]any{}
		}
	}
	a.connect = conn
	a.disconnect, err = toIDArray(m["disconnect"])
	return a, err
}

func itemIDs(items []relItem) []any {
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func uniqByID(items []relItem) []relItem {
	seen := map[string]bool{}
	var out []relItem
	for _, it := range items {
		k := idKey(it.ID)
		if !seen[k] {
			seen[k] = true
			out = append(out, it)
		}
	}
	return out
}

var cuidAlphabet = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

// NewDocumentID returns a 24 character id in the format of cuid2, which
// Strapi uses for documentId: a lowercase letter then base 36 characters.
func NewDocumentID() string {
	out := make([]byte, 24)
	n, _ := rand.Int(rand.Reader, big.NewInt(26))
	out[0] = cuidAlphabet[n.Int64()]
	for i := 1; i < len(out); i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(36))
		out[i] = cuidAlphabet[n.Int64()]
	}
	return string(out)
}

// processData converts the input of a write to column values, Strapi's
// processData. Keys are column names.
func (e *Engine) processData(m *metadata.Model, data Data, withDefaults bool) (*schema.OrderedMap[any], error) {
	sqlite := e.DB.Dialect.Name() == "sqlite"
	out := &schema.OrderedMap[any]{}
	for name, a := range m.Attributes.All() {
		switch {
		case scalarType(a.Type):
			v, present := data[name]
			col := columnName(m, name)
			if !present {
				if !withDefaults {
					continue
				}
				switch {
				case a.DefaultFunc == "documentId":
					out.Set(col, NewDocumentID())
				case a.DefaultFunc == "now":
					conv, _ := toDB(schema.TypeDateTime, time.Now(), sqlite)
					out.Set(col, conv)
				case len(a.Default) > 0:
					var def any
					if err := json.Unmarshal(a.Default, &def); err == nil {
						conv, err := toDB(a.Type, def, sqlite)
						if err != nil {
							return nil, err
						}
						out.Set(col, conv)
					}
				}
				continue
			}
			if v == nil {
				out.Set(col, nil)
				continue
			}
			conv, err := toDB(a.Type, v, sqlite)
			if err != nil {
				return nil, err
			}
			out.Set(col, conv)
		case a.Type == "relation" && a.JoinColumn != nil && a.Owner:
			col := a.JoinColumn.Name
			v, present := data[name]
			if !present {
				v, present = data[col]
			}
			if s, ok := v.(map[string]any); ok {
				if list, ok := s["set"].([]any); ok {
					if len(list) > 0 {
						v = list[len(list)-1]
					} else {
						v = nil
					}
				}
			}
			switch {
			case !present:
			case v == nil:
				out.Set(col, nil)
			default:
				id, err := toID(v)
				if err != nil {
					return nil, err
				}
				out.Set(col, id)
			}
		case a.Type == "relation" && a.MorphColumn != nil && a.Owner:
			v, present := data[name]
			if !present {
				continue
			}
			mc := a.MorphColumn
			typeField := mc.TypeField
			if typeField == "" {
				typeField = "__type"
			}
			if v == nil {
				out.Set(mc.IDColumn.Name, nil)
				out.Set(mc.TypeColumn, nil)
				continue
			}
			obj, _ := v.(map[string]any)
			id, hasID := obj["id"]
			typ, hasType := obj[typeField]
			if !hasID || !hasType {
				return nil, fmt.Errorf("Expects properties %s an id to make a morph association", typeField) //nolint:revive,staticcheck // Strapi's message
			}
			out.Set(mc.IDColumn.Name, normID(id))
			out.Set(mc.TypeColumn, typ)
		}
	}
	return out, nil
}

// sqlExec runs SQL written with "?" placeholders.
func (e *Engine) sqlExec(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := e.DB.Exec(ctx, e.placeholders(query), args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (e *Engine) q(id string) string { return e.DB.Dialect.Quote(id) }

func (e *Engine) insertRow(ctx context.Context, table string, row *schema.OrderedMap[any], returning bool) (any, error) {
	var cols, marks []string
	var args []any
	for k, v := range row.All() {
		cols = append(cols, e.q(k))
		marks = append(marks, "?")
		args = append(args, v)
	}
	stmt := "insert into " + e.q(table)
	if len(cols) == 0 {
		stmt += " default values"
	} else {
		stmt += " (" + strings.Join(cols, ", ") + ") values (" + strings.Join(marks, ", ") + ")"
	}
	if !returning {
		_, err := e.sqlExec(ctx, stmt, args...)
		return nil, err
	}
	var id int64
	err := e.DB.QueryRow(ctx, e.placeholders(stmt+" returning "+e.q("id")), args...).Scan(&id)
	return id, err
}

// insertRows inserts link rows, with knex's onConflict merge or ignore.
func (e *Engine) insertRows(ctx context.Context, table string, rows []*schema.OrderedMap[any], conflict []string, merge []string, ignore bool) error {
	if len(rows) == 0 {
		return nil
	}
	cols := rows[0].Keys()
	batch := 500
	for start := 0; start < len(rows); start += batch {
		end := min(start+batch, len(rows))
		var tuples []string
		var args []any
		for _, r := range rows[start:end] {
			marks := make([]string, len(cols))
			for i, c := range cols {
				marks[i] = "?"
				v, _ := r.Get(c)
				args = append(args, v)
			}
			tuples = append(tuples, "("+strings.Join(marks, ", ")+")")
		}
		qc := make([]string, len(cols))
		for i, c := range cols {
			qc[i] = e.q(c)
		}
		stmt := "insert into " + e.q(table) + " (" + strings.Join(qc, ", ") + ") values " + strings.Join(tuples, ", ")
		if len(conflict) > 0 {
			cq := make([]string, len(conflict))
			for i, c := range conflict {
				cq[i] = e.q(c)
			}
			stmt += " on conflict (" + strings.Join(cq, ", ") + ")"
			switch {
			case len(merge) > 0:
				sets := make([]string, len(merge))
				for i, c := range merge {
					sets[i] = e.q(c) + " = excluded." + e.q(c)
				}
				stmt += " do update set " + strings.Join(sets, ", ")
			case ignore:
				stmt += " do nothing"
			}
		}
		if _, err := e.sqlExec(ctx, stmt, args...); err != nil {
			return err
		}
	}
	return nil
}

// Create inserts an entry and attaches its relations, then reads it back.
func (e *Engine) Create(ctx context.Context, uid string, data Data, p Params) (Row, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	if data == nil {
		return nil, errors.New("Create expects a data object") //nolint:revive,staticcheck // Strapi's message
	}
	data = cloneData(data)
	now := time.Now()
	setDefault(data, schema.AttrCreatedAt, now)
	setDefault(data, schema.AttrUpdatedAt, now)
	row, err := e.processData(m, data, true)
	if err != nil {
		return nil, err
	}
	var id any
	err = e.DB.Tx(ctx, func(ctx context.Context) error {
		var err error
		if id, err = e.insertRow(ctx, m.TableName, row, true); err != nil {
			return err
		}
		return e.attachRelations(ctx, m, id, data)
	})
	if err != nil {
		return nil, err
	}
	return e.FindOne(ctx, uid, Params{Where: map[string]any{"id": id}, Select: p.Select, Populate: p.Populate, Filters: p.Filters})
}

func cloneData(d Data) Data {
	out := make(Data, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}

func setDefault(d Data, k string, v any) {
	if _, ok := d[k]; !ok {
		d[k] = v
	}
}

// CreateMany inserts rows without relations and returns their ids.
func (e *Engine) CreateMany(ctx context.Context, uid string, data []Data) (int, []any, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return 0, nil, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	if len(data) == 0 {
		return 0, nil, errors.New("Nothing to insert") //nolint:revive,staticcheck // Strapi's message
	}
	now := time.Now()
	var ids []any
	err := e.DB.Tx(ctx, func(ctx context.Context) error {
		for _, d := range data {
			d = cloneData(d)
			setDefault(d, schema.AttrCreatedAt, now)
			setDefault(d, schema.AttrUpdatedAt, now)
			row, err := e.processData(m, d, true)
			if err != nil {
				return err
			}
			id, err := e.insertRow(ctx, m.TableName, row, true)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	return len(data), ids, err
}

// Update changes the first entry matching where and its relations.
func (e *Engine) Update(ctx context.Context, uid string, where any, data Data, p Params) (Row, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	if data == nil {
		return nil, errors.New("Update requires a data object") //nolint:revive,staticcheck // Strapi's message
	}
	if w, ok := where.(map[string]any); !ok || len(w) == 0 {
		return nil, errors.New("Update requires a where parameter") //nolint:revive,staticcheck // Strapi's message
	}
	entity, err := e.FindOne(ctx, uid, Params{Where: where, Select: []string{"id"}})
	if err != nil || entity == nil {
		return nil, err
	}
	id, _ := entity.Get("id")
	data = cloneData(data)
	data[schema.AttrUpdatedAt] = time.Now()
	row, err := e.processData(m, data, false)
	if err != nil {
		return nil, err
	}
	err = e.DB.Tx(ctx, func(ctx context.Context) error {
		if row.Len() > 0 {
			if err := e.updateByID(ctx, m.TableName, id, row); err != nil {
				return err
			}
		}
		return e.updateRelations(ctx, m, id, data)
	})
	if err != nil {
		return nil, err
	}
	return e.FindOne(ctx, uid, Params{Where: map[string]any{"id": id}, Select: p.Select, Populate: p.Populate, Filters: p.Filters})
}

func (e *Engine) updateByID(ctx context.Context, table string, id any, row *schema.OrderedMap[any]) error {
	var sets []string
	var args []any
	for k, v := range row.All() {
		sets = append(sets, e.q(k)+" = ?")
		args = append(args, v)
	}
	_, err := e.sqlExec(ctx, "update "+e.q(table)+" set "+strings.Join(sets, ", ")+" where "+e.q("id")+" = ?", append(args, id)...)
	return err
}

// UpdateMany sets data on every entry matching p, without relations nor
// updatedAt (Strapi's timestamps subscriber skips a non array payload).
func (e *Engine) UpdateMany(ctx context.Context, uid string, p Params, data Data) (int64, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	row, err := e.processData(m, data, false)
	if err != nil {
		return 0, err
	}
	if row.Len() == 0 {
		return 0, errors.New("Update requires data") //nolint:revive,staticcheck // Strapi's message
	}
	cond, args, err := e.writeWhere(m, p)
	if err != nil {
		return 0, err
	}
	var sets []string
	var setArgs []any
	for k, v := range row.All() {
		sets = append(sets, e.q(k)+" = ?")
		setArgs = append(setArgs, v)
	}
	return e.sqlExec(ctx, "update "+e.q(m.TableName)+" set "+strings.Join(sets, ", ")+" where "+cond, append(setArgs, args...)...)
}

// writeWhere is the condition of updateMany and deleteMany: the matching
// ids, through a subquery when relations are joined.
func (e *Engine) writeWhere(m *metadata.Model, p Params) (string, []any, error) {
	b, err := e.newBuilder(m.UID, "select")
	if err != nil {
		return "", nil, err
	}
	if err := b.init(Params{Where: p.Where, Filters: p.Filters, Search: p.Search}); err != nil {
		return "", nil, err
	}
	b.selects = []string{"id"}
	b.orderBy = nil
	sub, args, err := b.selectSQL()
	if err != nil {
		return "", nil, err
	}
	return e.q("id") + " in (select " + e.q("id") + " from (" + sub + ") as " + e.q("subQuery") + ")", args, nil
}

// Delete removes the first entry matching where and its relation links.
func (e *Engine) Delete(ctx context.Context, uid string, where any, p Params) (Row, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	if w, ok := where.(map[string]any); !ok || len(w) == 0 {
		return nil, errors.New("Delete requires a where parameter") //nolint:revive,staticcheck // Strapi's message
	}
	sel := p.Select
	if sel != nil {
		sel = append([]string{"id"}, sel...)
	}
	entity, err := e.FindOne(ctx, uid, Params{Where: where, Select: sel, Populate: p.Populate})
	if err != nil || entity == nil {
		return nil, err
	}
	id, _ := entity.Get("id")
	err = e.DB.Tx(ctx, func(ctx context.Context) error {
		if _, err := e.sqlExec(ctx, "delete from "+e.q(m.TableName)+" where "+e.q("id")+" = ?", id); err != nil {
			return err
		}
		return e.deleteRelations(ctx, m, id)
	})
	return entity, err
}

// DeleteMany removes every entry matching p (relation links go through the
// foreign keys' ON DELETE CASCADE).
func (e *Engine) DeleteMany(ctx context.Context, uid string, p Params) (int64, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	cond, args, err := e.writeWhere(m, p)
	if err != nil {
		return 0, err
	}
	return e.sqlExec(ctx, "delete from "+e.q(m.TableName)+" where "+cond, args...)
}

func rowOf(kv ...any) *schema.OrderedMap[any] {
	r := &schema.OrderedMap[any]{}
	for i := 0; i+1 < len(kv); i += 2 {
		r.Set(kv[i].(string), kv[i+1])
	}
	return r
}

func addMap(r *schema.OrderedMap[any], m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r.Set(k, m[k])
	}
}

func addOn(r *schema.OrderedMap[any], on map[string]string) {
	for _, k := range sortedStringKeys(on) {
		r.Set(k, on[k])
	}
}

func onCond(e *Engine, on map[string]string) (string, []any) {
	var parts []string
	var args []any
	for _, k := range sortedStringKeys(on) {
		parts = append(parts, e.q(k)+" = ?")
		args = append(args, on[k])
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " and " + strings.Join(parts, " and "), args
}

func inList(e *Engine, col string, ids []any, not bool) (string, []any) {
	if len(ids) == 0 {
		if not {
			return "1 = 1", nil
		}
		return "1 = 0", nil
	}
	op := " in ("
	if not {
		op = " not in ("
	}
	return e.q(col) + op + strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ") + ")", ids
}

func isBidirectional(a *metadata.Attribute) bool { return a.InversedBy != "" || a.MappedBy != "" }

func isOneToAny(a *metadata.Attribute) bool {
	return a.Relation == schema.OneToOne || a.Relation == schema.OneToMany
}

func isAnyToOne(a *metadata.Attribute) bool {
	return a.Relation == schema.OneToOne || a.Relation == schema.ManyToOne
}

func isManyToAny(a *metadata.Attribute) bool {
	return a.Relation == schema.ManyToMany || a.Relation == schema.ManyToOne
}

func hasOrderColumn(a *metadata.Attribute) bool {
	return a.Relation == schema.OneToMany || a.Relation == schema.ManyToMany
}

func hasInverseOrderColumn(a *metadata.Attribute) bool { return isBidirectional(a) && isManyToAny(a) }

// siblingIDs is the subquery of the ids of every row of the document id
// belongs to, so that its draft and published rows do not steal each
// other's relations; for a component table, just id.
func (e *Engine) siblingIDs(table string, id any) (string, []any) {
	for _, m := range e.MD.Models() {
		if m.TableName != table {
			continue
		}
		if _, ok := m.Attributes.Get(schema.AttrDocumentID); ok {
			return "select " + e.q("id") + " from " + e.q(table) + " where " + e.q("document_id") + " in (select " +
				e.q("document_id") + " from " + e.q(table) + " where " + e.q("id") + " = ?)", []any{id}
		}
	}
	return "?", []any{id}
}

func (e *Engine) attachRelations(ctx context.Context, m *metadata.Model, id any, data Data) error {
	for name, a := range m.Attributes.All() {
		v, present := data[name]
		if a.Type != "relation" || !present || v == nil {
			continue
		}
		rel, err := toAssocs(v)
		if err != nil {
			return err
		}
		switch a.Relation {
		case schema.MorphOne, schema.MorphMany:
			if err := e.attachMorphTarget(ctx, m, id, name, a, rel); err != nil {
				return err
			}
			continue
		case schema.MorphToOne:
			continue
		case schema.MorphToMany:
			if err := e.attachMorphToMany(ctx, m, id, name, a, rel); err != nil {
				return err
			}
			continue
		}
		if a.JoinColumn != nil && a.Owner {
			ids := itemIDs(rel.set)
			if a.Relation == schema.OneToOne && isBidirectional(a) && len(ids) > 0 {
				cond, args := inList(e, a.JoinColumn.Name, ids, false)
				if _, err := e.sqlExec(ctx, "update "+e.q(m.TableName)+" set "+e.q(a.JoinColumn.Name)+" = null where "+cond+" and "+e.q("id")+" <> ?", append(args, id)...); err != nil {
					return err
				}
			}
			continue
		}
		if a.JoinColumn != nil && !a.Owner {
			target, _ := e.MD.Model(a.Target)
			ref := a.JoinColumn.ReferencedColumn
			if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(ref)+" = null where "+e.q(ref)+" = ?", id); err != nil {
				return err
			}
			cond, args := inList(e, "id", itemIDs(rel.set), false)
			if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(ref)+" = ? where "+cond, append([]any{id}, args...)...); err != nil {
				return err
			}
		}
		if a.JoinTable != nil {
			if err := e.attachJoinTable(ctx, id, a, rel); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) attachJoinTable(ctx context.Context, id any, a *metadata.Attribute, rel assocs) error {
	jt := a.JoinTable
	rels := rel.set
	if !rel.hasSet {
		rels = rel.connect
	}
	ids := itemIDs(rels)
	if isBidirectional(a) && isOneToAny(a) {
		if err := e.deletePreviousOneToAny(ctx, id, a, ids); err != nil {
			return err
		}
	}
	var insert []*schema.OrderedMap[any]
	for _, it := range uniqByID(rels) {
		r := rowOf(jt.JoinColumn.Name, id, jt.InverseJoinColumn.Name, it.ID)
		addOn(r, jt.On)
		addMap(r, it.Pivot)
		insert = append(insert, r)
	}
	switch {
	case rel.hasSet && hasOrderColumn(a):
		for i, r := range insert {
			r.Set(jt.OrderColumnName, float64(i+1))
		}
	case !rel.hasSet && hasOrderColumn(a):
		o := NewOrderer(nil, true)
		if err := o.Connect(toLinks(rels)); err != nil {
			return err
		}
		pos := map[string]float64{}
		for i, l := range o.Relations() {
			pos[idKey(l.ID)] = float64(i)
		}
		for _, r := range insert {
			v, _ := r.Get(jt.InverseJoinColumn.Name)
			r.Set(jt.OrderColumnName, pos[idKey(v)])
		}
	}
	if hasInverseOrderColumn(a) {
		maxes, err := e.maxInverseOrder(ctx, a, ids)
		if err != nil {
			return err
		}
		for _, r := range insert {
			v, _ := r.Get(jt.InverseJoinColumn.Name)
			r.Set(jt.InverseOrderColumnName, maxes[idKey(v)]+1)
		}
	}
	return e.insertRows(ctx, jt.Name, insert, nil, nil, false)
}

func toLinks(items []relItem) []*Link {
	out := make([]*Link, len(items))
	for i, it := range items {
		out[i] = &Link{ID: it.ID, Position: it.Position}
	}
	return out
}

func (e *Engine) maxInverseOrder(ctx context.Context, a *metadata.Attribute, ids []any) (map[string]float64, error) {
	jt := a.JoinTable
	out := map[string]float64{}
	if len(ids) == 0 {
		return out, nil
	}
	cond, args := inList(e, jt.InverseJoinColumn.Name, ids, false)
	on, onArgs := onCond(e, jt.On)
	rows, err := e.DB.Query(ctx, e.placeholders("select "+e.q(jt.InverseJoinColumn.Name)+", max("+e.q(jt.InverseOrderColumnName)+") from "+
		e.q(jt.Name)+" where "+cond+on+" group by "+e.q(jt.InverseJoinColumn.Name)), append(args, onArgs...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, maxV any
		if err := rows.Scan(&id, &maxV); err != nil {
			return nil, err
		}
		if f, ok := lodashToNumber(asNumberish(maxV)).(float64); ok {
			out[idKey(normID(id))] = f
		} else if n, ok := lodashToNumber(asNumberish(maxV)).(int64); ok {
			out[idKey(normID(id))] = float64(n)
		}
	}
	return out, rows.Err()
}

func asNumberish(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	if v == nil {
		return int64(0)
	}
	return v
}

func (e *Engine) deletePreviousOneToAny(ctx context.Context, id any, a *metadata.Attribute, ids []any) error {
	jt := a.JoinTable
	sib, sibArgs := e.siblingIDs(jt.JoinColumn.ReferencedTable, id)
	cond, args := inList(e, jt.InverseJoinColumn.Name, ids, false)
	on, onArgs := onCond(e, jt.On)
	q := "delete from " + e.q(jt.Name) + " where " + e.q(jt.JoinColumn.Name) + " not in (" + sib + ") and " + cond + on
	if _, err := e.sqlExec(ctx, q, append(append(sibArgs, args...), onArgs...)...); err != nil {
		return err
	}
	return e.cleanOrderColumns(ctx, a, nil, ids)
}

func (e *Engine) deletePreviousAnyToOne(ctx context.Context, id any, a *metadata.Attribute, relID any) error {
	jt := a.JoinTable
	on, onArgs := onCond(e, jt.On)
	sib, sibArgs := e.siblingIDs(jt.InverseJoinColumn.ReferencedTable, relID)
	if isManyToAny(a) {
		rows, err := e.DB.Query(ctx, e.placeholders("select "+e.q(jt.InverseJoinColumn.Name)+" from "+e.q(jt.Name)+" where "+
			e.q(jt.JoinColumn.Name)+" = ? and "+e.q(jt.InverseJoinColumn.Name)+" not in ("+sib+")"+on), append(append([]any{id}, sibArgs...), onArgs...)...)
		if err != nil {
			return err
		}
		var del []any
		for rows.Next() {
			var v any
			if err := rows.Scan(&v); err != nil {
				_ = rows.Close()
				return err
			}
			del = append(del, normID(v))
		}
		if err := rows.Close(); err != nil {
			return err
		}
		cond, args := inList(e, jt.InverseJoinColumn.Name, del, false)
		if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ? and "+cond+on, append(append([]any{id}, args...), onArgs...)...); err != nil {
			return err
		}
		return e.cleanOrderColumns(ctx, a, nil, del)
	}
	_, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ? and "+e.q(jt.InverseJoinColumn.Name)+" not in ("+sib+")"+on,
		append(append([]any{id}, sibArgs...), onArgs...)...)
	return err
}

// deleteRelationLinks removes some or all links of an entry, renumbering
// the order columns, Strapi's deleteRelations.
func (e *Engine) deleteRelationLinks(ctx context.Context, id any, a *metadata.Attribute, all bool, toDelete, keep []any) error {
	jt := a.JoinTable
	on, onArgs := onCond(e, jt.On)
	if !hasOrderColumn(a) && !hasInverseOrderColumn(a) {
		notIn, nArgs := inList(e, jt.InverseJoinColumn.Name, keep, true)
		q := "delete from " + e.q(jt.Name) + " where " + e.q(jt.JoinColumn.Name) + " = ? and " + notIn
		args := append([]any{id}, nArgs...)
		if !all {
			in, iArgs := inList(e, jt.InverseJoinColumn.Name, toDelete, false)
			q += " and " + in
			args = append(args, iArgs...)
		}
		_, err := e.sqlExec(ctx, q+on, append(args, onArgs...)...)
		return err
	}
	var lastID int64
	for {
		notIn, nArgs := inList(e, jt.InverseJoinColumn.Name, keep, true)
		q := "select " + e.q("id") + ", " + e.q(jt.InverseJoinColumn.Name) + " from " + e.q(jt.Name) + " where " + e.q(jt.JoinColumn.Name) + " = ? and " + e.q("id") + " > ? and " + notIn
		args := append([]any{id, lastID}, nArgs...)
		if !all {
			in, iArgs := inList(e, jt.InverseJoinColumn.Name, toDelete, false)
			q += " and " + in
			args = append(args, iArgs...)
		}
		rows, err := e.DB.Query(ctx, e.placeholders(q+on+" order by "+e.q("id")+" limit 100"), append(args, onArgs...)...)
		if err != nil {
			return err
		}
		var batch []any
		n := 0
		for rows.Next() {
			var rid int64
			var inv any
			if err := rows.Scan(&rid, &inv); err != nil {
				_ = rows.Close()
				return err
			}
			lastID = rid
			batch = append(batch, normID(inv))
			n++
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if n == 0 {
			lastID = 0
		}
		in, iArgs := inList(e, jt.InverseJoinColumn.Name, batch, false)
		if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ? and "+in+on, append(append([]any{id}, iArgs...), onArgs...)...); err != nil {
			return err
		}
		if err := e.cleanOrderColumns(ctx, a, id, batch); err != nil {
			return err
		}
		if n < 100 {
			return nil
		}
	}
}

// cleanOrderColumns renumbers the order columns 1, 2, 3... after links moved.
func (e *Engine) cleanOrderColumns(ctx context.Context, a *metadata.Attribute, id any, inverseIDs []any) error {
	jt := a.JoinTable
	if hasOrderColumn(a) && id != nil && idKey(id) != "" {
		q := "update " + e.q(jt.Name) + " as a set " + e.q(jt.OrderColumnName) + " = b.src_order from (select " + e.q("id") +
			", row_number() over (partition by " + e.q(jt.JoinColumn.Name) + " order by " + e.q(jt.OrderColumnName) + ") as src_order from " +
			e.q(jt.Name) + " where " + e.q(jt.JoinColumn.Name) + " = ?) as b where b.id = a.id"
		if _, err := e.sqlExec(ctx, q, id); err != nil {
			return err
		}
	}
	if hasInverseOrderColumn(a) && len(inverseIDs) > 0 {
		cond, args := inList(e, jt.InverseJoinColumn.Name, inverseIDs, false)
		q := "update " + e.q(jt.Name) + " as a set " + e.q(jt.InverseOrderColumnName) + " = b.inv_order from (select " + e.q("id") +
			", row_number() over (partition by " + e.q(jt.InverseJoinColumn.Name) + " order by " + e.q(jt.InverseOrderColumnName) + ") as inv_order from " +
			e.q(jt.Name) + " where " + cond + ") as b where b.id = a.id"
		if _, err := e.sqlExec(ctx, q, args...); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) linkRows(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := e.DB.Query(ctx, e.placeholders(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols, _ := rows.Columns()
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			m[c] = normalizeInt(v)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func numberOf(v any) (float64, bool) {
	switch x := lodashToNumber(asNumberish(v)).(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func (e *Engine) updateRelations(ctx context.Context, m *metadata.Model, id any, data Data) error {
	for name, a := range m.Attributes.All() {
		v, present := data[name]
		if a.Type != "relation" || !present {
			continue
		}
		rel, err := toAssocs(v)
		if err != nil {
			return err
		}
		switch a.Relation {
		case schema.MorphOne, schema.MorphMany:
			if err := e.updateMorphTarget(ctx, m, id, name, a, rel); err != nil {
				return err
			}
			continue
		case schema.MorphToOne:
			continue
		case schema.MorphToMany:
			if err := e.updateMorphToMany(ctx, m, id, name, a, rel); err != nil {
				return err
			}
			continue
		}
		if a.JoinColumn != nil && a.Owner {
			continue
		}
		if a.JoinColumn != nil && !a.Owner {
			target, _ := e.MD.Model(a.Target)
			ref := a.JoinColumn.ReferencedColumn
			if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(ref)+" = null where "+e.q(ref)+" = ?", id); err != nil {
				return err
			}
			if !rel.setNull {
				cond, args := inList(e, "id", itemIDs(rel.set), false)
				if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(ref)+" = ? where "+cond, append([]any{id}, args...)...); err != nil {
					return err
				}
			}
		}
		if a.JoinTable == nil {
			continue
		}
		if err := e.updateJoinTable(ctx, id, a, rel); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) updateJoinTable(ctx context.Context, id any, a *metadata.Attribute, rel assocs) error {
	jt := a.JoinTable
	if rel.setNull {
		return e.deleteRelationLinks(ctx, id, a, true, nil, nil)
	}
	var moving []any
	if !rel.hasSet {
		moving = itemIDs(rel.connect)
		connectIDs := map[string]bool{}
		for _, c := range rel.connect {
			connectIDs[idKey(c.ID)] = true
		}
		var toDelete []any
		for _, d := range rel.disconnect {
			if !connectIDs[idKey(d.ID)] {
				toDelete = append(toDelete, d.ID)
			}
		}
		connect, err := e.resolveDeletedAnchors(ctx, id, a, rel.connect, toDelete)
		if err != nil {
			return err
		}
		if len(toDelete) > 0 {
			if err := e.deleteRelationLinks(ctx, id, a, false, toDelete, nil); err != nil {
				return err
			}
		}
		if len(rel.connect) == 0 {
			return nil
		}
		var current []map[string]any
		if hasOrderColumn(a) || hasInverseOrderColumn(a) {
			cond, args := inList(e, jt.InverseJoinColumn.Name, moving, false)
			on, onArgs := onCond(e, jt.On)
			current, err = e.linkRows(ctx, "select "+e.q(jt.InverseJoinColumn.Name)+" from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ? and "+cond+on,
				append(append([]any{id}, args...), onArgs...)...)
			if err != nil {
				return err
			}
		}
		var insert []*schema.OrderedMap[any]
		for _, it := range uniqByID(rel.connect) {
			r := rowOf(jt.JoinColumn.Name, id, jt.InverseJoinColumn.Name, it.ID)
			addOn(r, jt.On)
			addMap(r, it.Pivot)
			insert = append(insert, r)
		}
		if hasOrderColumn(a) {
			var anchors []any
			for _, c := range connect {
				if c.Position != nil {
					if c.Position.After != nil && idKey(c.Position.After) != "" {
						anchors = append(anchors, c.Position.After)
					} else if c.Position.Before != nil && idKey(c.Position.Before) != "" {
						anchors = append(anchors, c.Position.Before)
					}
				}
			}
			on, onArgs := onCond(e, jt.On)
			in, inArgs := inList(e, jt.InverseJoinColumn.Name, anchors, false)
			jc, oc := e.q(jt.JoinColumn.Name), e.q(jt.OrderColumnName)
			sub := func(fn string) string {
				return "(select " + fn + "(" + oc + ") from " + e.q(jt.Name) + " where " + jc + " = ?" + on + ")"
			}
			q := "select * from " + e.q(jt.Name) + " where ((" + jc + " = ? and " + in + ") or (" + jc + " = ? and " + oc + " = " + sub("min") +
				") or (" + jc + " = ? and " + oc + " = " + sub("max") + "))" + on + " order by " + oc + " asc"
			args := []any{id}
			args = append(args, inArgs...)
			args = append(args, id, id)
			args = append(args, onArgs...)
			args = append(args, id, id)
			args = append(args, onArgs...)
			args = append(args, onArgs...)
			adjacent, err := e.linkRows(ctx, q, args...)
			if err != nil {
				return err
			}
			var initial []*Link
			for _, r := range adjacent {
				order, ok := numberOf(r[jt.OrderColumnName])
				if !ok || r[jt.OrderColumnName] == nil {
					order = 1
				}
				initial = append(initial, &Link{ID: r[jt.InverseJoinColumn.Name], Order: order})
			}
			// undefined takes the default parameter: strict.
			strict := true
			if rel.strict != nil {
				strict = *rel.strict
			}
			o := NewOrderer(initial, strict)
			links := make([]*Link, len(connect))
			for i, c := range connect {
				links[i] = &Link{ID: c.ID, Position: c.Position}
			}
			if err := o.Connect(links); err != nil {
				return err
			}
			orders := o.OrderMap()
			for _, r := range insert {
				v, _ := r.Get(jt.InverseJoinColumn.Name)
				if ord, ok := orders[idKey(v)]; ok {
					r.Set(jt.OrderColumnName, ord)
				} else {
					r.Set(jt.OrderColumnName, nil)
				}
			}
		}
		if hasInverseOrderColumn(a) {
			existing := map[string]bool{}
			for _, r := range current {
				existing[idKey(r[jt.InverseJoinColumn.Name])] = true
			}
			var nonExisting []any
			for _, mid := range moving {
				if !existing[idKey(mid)] {
					nonExisting = append(nonExisting, mid)
				}
			}
			maxes, err := e.maxInverseOrder(ctx, a, nonExisting)
			if err != nil {
				return err
			}
			for _, r := range insert {
				v, _ := r.Get(jt.InverseJoinColumn.Name)
				r.Set(jt.InverseOrderColumnName, maxes[idKey(v)]+1)
			}
		}
		var merge []string
		if hasOrderColumn(a) {
			merge = []string{jt.OrderColumnName}
		}
		if err := e.insertRows(ctx, jt.Name, insert, jt.PivotColumns, merge, !hasOrderColumn(a)); err != nil {
			return err
		}
		if err := e.cleanOrderColumns(ctx, a, id, nil); err != nil {
			return err
		}
	} else {
		moving = itemIDs(rel.set)
		if err := e.deleteRelationLinks(ctx, id, a, true, nil, moving); err != nil {
			return err
		}
		if len(rel.set) == 0 {
			return nil
		}
		var insert []*schema.OrderedMap[any]
		for i, it := range uniqByID(rel.set) {
			r := rowOf(jt.JoinColumn.Name, id, jt.InverseJoinColumn.Name, it.ID)
			addOn(r, jt.On)
			addMap(r, it.Pivot)
			if hasOrderColumn(a) {
				r.Set(jt.OrderColumnName, float64(i+1))
			}
			insert = append(insert, r)
		}
		if hasInverseOrderColumn(a) {
			cond, args := inList(e, jt.InverseJoinColumn.Name, moving, false)
			on, onArgs := onCond(e, jt.On)
			existingRows, err := e.linkRows(ctx, "select "+e.q(jt.InverseJoinColumn.Name)+" from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ? and "+cond+on,
				append(append([]any{id}, args...), onArgs...)...)
			if err != nil {
				return err
			}
			existing := map[string]bool{}
			for _, r := range existingRows {
				existing[idKey(r[jt.InverseJoinColumn.Name])] = true
			}
			var nonExisting []any
			for _, mid := range moving {
				if !existing[idKey(mid)] {
					nonExisting = append(nonExisting, mid)
				}
			}
			maxes, err := e.maxInverseOrder(ctx, a, nonExisting)
			if err != nil {
				return err
			}
			for _, r := range insert {
				v, _ := r.Get(jt.InverseJoinColumn.Name)
				r.Set(jt.InverseOrderColumnName, maxes[idKey(v)]+1)
			}
		}
		var merge []string
		if hasOrderColumn(a) {
			merge = []string{jt.OrderColumnName}
		}
		if err := e.insertRows(ctx, jt.Name, insert, jt.PivotColumns, merge, !hasOrderColumn(a)); err != nil {
			return err
		}
	}
	return e.afterRelationWrite(ctx, id, a, moving)
}

func (e *Engine) afterRelationWrite(ctx context.Context, id any, a *metadata.Attribute, ids []any) error {
	if isBidirectional(a) && isOneToAny(a) {
		if err := e.deletePreviousOneToAny(ctx, id, a, ids); err != nil {
			return err
		}
	}
	if isAnyToOne(a) {
		var first any
		if len(ids) > 0 {
			first = ids[0]
		}
		return e.deletePreviousAnyToOne(ctx, id, a, first)
	}
	return nil
}

// resolveDeletedAnchors moves a connect positioned against a relation that
// the same payload disconnects to that relation's nearest surviving
// neighbour, like Strapi.
func (e *Engine) resolveDeletedAnchors(ctx context.Context, id any, a *metadata.Attribute, connect []relItem, toDelete []any) ([]relItem, error) {
	deleted := map[string]bool{}
	for _, d := range toDelete {
		deleted[idKey(d)] = true
	}
	needs := false
	for _, c := range connect {
		if adj := c.Position.adjacent(); adj != "" && deleted[adj] {
			needs = true
		}
	}
	if !hasOrderColumn(a) || len(toDelete) == 0 || !needs {
		return connect, nil
	}
	jt := a.JoinTable
	on, onArgs := onCond(e, jt.On)
	current, err := e.linkRows(ctx, "select "+e.q(jt.InverseJoinColumn.Name)+", "+e.q(jt.OrderColumnName)+" from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ?"+on+" order by "+e.q(jt.OrderColumnName)+" asc",
		append([]any{id}, onArgs...)...)
	if err != nil {
		return nil, err
	}
	var ordered []string
	var orderedIDs []any
	for _, r := range current {
		ordered = append(ordered, idKey(r[jt.InverseJoinColumn.Name]))
		orderedIDs = append(orderedIDs, r[jt.InverseJoinColumn.Name])
	}
	connectIDs := map[string]bool{}
	for _, c := range connect {
		connectIDs[idKey(c.ID)] = true
	}
	deletedInOrder := map[string]bool{}
	for _, k := range ordered {
		if deleted[k] {
			deletedInOrder[k] = true
		}
	}
	neighbour := func(target string, dir int) any {
		i := slices.Index(ordered, target)
		for i != -1 {
			i += dir
			if i < 0 || i >= len(ordered) {
				return nil
			}
			if !deleted[ordered[i]] && !connectIDs[ordered[i]] {
				return orderedIDs[i]
			}
		}
		return nil
	}
	type group struct {
		target        string
		before, after []int
	}
	var groups []*group
	byTarget := map[string]*group{}
	for i, c := range connect {
		var adj any
		if c.Position != nil && c.Position.before() != "" {
			adj = c.Position.Before
		} else if c.Position != nil {
			adj = c.Position.After
		}
		k := idKey(adj)
		if k == "" || !deleted[k] || !deletedInOrder[k] {
			continue
		}
		g := byTarget[k]
		if g == nil {
			g = &group{target: k}
			byTarget[k] = g
			groups = append(groups, g)
		}
		if c.Position.before() != "" {
			g.before = append(g.before, i)
		} else {
			g.after = append(g.after, i)
		}
	}
	out := append([]relItem(nil), connect...)
	for _, g := range groups {
		prev, next := neighbour(g.target, -1), neighbour(g.target, 1)
		for _, i := range append(append([]int{}, g.before...), g.after...) {
			switch {
			case prev != nil:
				out[i].Position = &Position{After: prev}
			case next != nil:
				out[i].Position = &Position{Before: next}
			default:
				out[i].Position = &Position{Start: true}
			}
			prev = out[i].ID
		}
	}
	return out, nil
}

func (e *Engine) deleteRelations(ctx context.Context, m *metadata.Model, id any) error {
	for name, a := range m.Attributes.All() {
		if a.Type != "relation" {
			continue
		}
		switch a.Relation {
		case schema.MorphOne, schema.MorphMany:
			target, _ := e.MD.Model(a.Target)
			inv, _ := target.Attributes.Get(a.MorphBy)
			switch {
			case inv != nil && inv.Relation == schema.MorphToOne:
				mc := inv.MorphColumn
				if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(mc.IDColumn.Name)+" = null, "+e.q(mc.TypeColumn)+" = null where "+
					e.q(mc.IDColumn.Name)+" = ? and "+e.q(mc.TypeColumn)+" = ?", id, m.UID); err != nil {
					return err
				}
			case inv != nil && inv.Relation == schema.MorphToMany:
				jt := inv.JoinTable
				on, onArgs := onCond(e, jt.On)
				if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(jt.MorphColumn.IDColumn.Name)+" = ? and "+e.q(jt.MorphColumn.TypeColumn)+
					" = ?"+on+" and "+e.q("field")+" = ?", append(append([]any{id, m.UID}, onArgs...), name)...); err != nil {
					return err
				}
			}
			continue
		case schema.MorphToOne:
			continue
		case schema.MorphToMany:
			jt := a.JoinTable
			on, onArgs := onCond(e, jt.On)
			if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ?"+on, append([]any{id}, onArgs...)...); err != nil {
				return err
			}
			continue
		}
		if e.DB.Dialect.Name() == "postgres" {
			continue
		}
		if a.JoinColumn != nil && a.Owner {
			continue
		}
		if a.JoinColumn != nil && !a.Owner {
			target, _ := e.MD.Model(a.Target)
			ref := a.JoinColumn.ReferencedColumn
			if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(ref)+" = null where "+e.q(ref)+" = ?", id); err != nil {
				return err
			}
		}
		if a.JoinTable != nil {
			if err := e.deleteRelationLinks(ctx, id, a, true, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// Table returns the raw rows of a table by id, for tests and the bench.
func (e *Engine) Table(ctx context.Context, table string) ([]map[string]any, error) {
	rows, err := e.linkRows(ctx, "select * from "+e.q(table)+" order by "+e.q("id"))
	for _, r := range rows {
		for k, v := range r {
			if t, ok := v.(time.Time); ok {
				r[k] = t.UTC().Format("2006-01-02T15:04:05.000Z")
			}
		}
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, err
}

// Load reads relations of one entry, Strapi's db.query(uid).load: the value
// of field, or of each field when several are asked.
func (e *Engine) Load(ctx context.Context, uid string, id any, fields []string, params map[string]any) (map[string]any, error) {
	populate := map[string]any{}
	for _, f := range fields {
		if params != nil {
			populate[f] = params
		} else {
			populate[f] = true
		}
	}
	row, err := e.FindOne(ctx, uid, Params{Where: map[string]any{"id": id}, Select: []string{"id"}, Populate: populate})
	if err != nil || row == nil {
		return nil, err
	}
	out := map[string]any{}
	for _, f := range fields {
		v, _ := row.Get(f)
		out[f] = v
	}
	return out, nil
}
