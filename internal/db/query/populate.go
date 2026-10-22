package query

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/schema"
)

// Populate: one query per relation and per level, rows matched in memory,
// like Strapi's applyPopulate.

const joinColPrefix = "__strapi"

// processPopulate normalizes the populate param and adds the columns the
// populate needs to the select.
func (b *builder) processPopulate(populate any) error {
	m := b.model
	var pm map[string]any
	switch p := populate.(type) {
	case nil:
		return nil
	case bool:
		if !p {
			return nil
		}
		pm = map[string]any{}
		for name, a := range m.Attributes.All() {
			if a.Type == "relation" {
				pm[name] = true
			}
		}
	case []any:
		pm = map[string]any{}
		for _, k := range p {
			key, _ := k.(string)
			root, rest, nested := strings.Cut(key, ".")
			if !nested {
				if _, ok := pm[root]; !ok {
					pm[root] = true
				}
				continue
			}
			switch cur := pm[root].(type) {
			case map[string]any:
				list, _ := cur["populate"].([]any)
				cur["populate"] = append([]any{rest}, list...)
			default:
				pm[root] = map[string]any{"populate": []any{rest}}
			}
		}
	case []string:
		list := make([]any, len(p))
		for i, s := range p {
			list[i] = s
		}
		return b.processPopulate(list)
	case map[string]any:
		pm = p
	default:
		return errors.New("Populate must be an object") //nolint:revive,staticcheck // Strapi's message
	}
	b.populate = map[string]any{}
	keys := make([]string, 0, len(pm))
	for k := range pm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a, ok := m.Attributes.Get(k)
		if !ok || a.Type != "relation" {
			continue
		}
		if a.JoinColumn != nil {
			b.addSelect(a.JoinColumn.Name)
		}
		if _, ok := m.Attributes.Get("id"); ok {
			b.addSelect("id")
		}
		b.populate[k] = pm[k]
		b.popOrder = append(b.popOrder, k)
	}
	return nil
}

func (b *builder) addSelect(cols ...string) {
	for _, c := range cols {
		if !contains(b.selects, c) {
			b.selects = append(b.selects, c)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// populateValue is Strapi's getPopulateValue: the root filters, then the
// params a relation's populate may set.
func populateValue(v any, filters any) map[string]any {
	out := map[string]any{"filters": filters}
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	picked := []string{"select", "count", "where", "populate", "orderBy", "filters", "ordering", "on"}
	if c, _ := m["count"].(bool); !c {
		picked = append(picked, "limit", "offset")
	}
	for _, k := range picked {
		if val, ok := m[k]; ok {
			out[k] = val
		}
	}
	if on, ok := out["on"].(map[string]any); ok {
		for t, val := range on {
			if vm, ok := val.(map[string]any); ok {
				vm["filters"] = filters
				on[t] = vm
			}
		}
	}
	return out
}

func paramsOf(pv map[string]any) Params {
	p := Params{Where: pv["where"], Filters: pv["filters"], OrderBy: pv["orderBy"], Populate: pv["populate"]}
	if sel, ok := pv["select"].([]any); ok {
		for _, s := range sel {
			p.Select = append(p.Select, fmt.Sprint(s))
		}
	} else if s, ok := pv["select"].(string); ok {
		p.Select = []string{s}
	}
	if n, ok := asInt(pv["limit"]); ok {
		p.Limit = &n
	}
	if n, ok := asInt(pv["offset"]); ok {
		p.Offset = &n
	}
	return p
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	}
	return 0, false
}

// hasSort is @strapi/utils' hasSort: a non empty sort.
func hasSort(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func joinOrder(pv map[string]any, jt *metadata.JoinTable, alias string) []orderTerm {
	explicit := pv["orderBy"]
	if explicit == nil {
		explicit = pv["sort"]
	}
	if hasSort(explicit) || jt.OrderBy == "" {
		return nil
	}
	dir := "asc"
	if o, ok := pv["ordering"].(string); ok && o != "" {
		dir = o
	}
	return []orderTerm{{column: alias + "." + jt.OrderBy, order: dir}}
}

// sub starts the query of a populated relation on uid.
func (b *builder) sub(uid string) (*builder, error) {
	nb, err := b.e.newBuilder(uid, "select")
	if err != nil {
		return nil, err
	}
	return nb, nil
}

// subInit initializes a populate query, with a where added on top of the
// populate's own params.
func (b *builder) subInit(pv map[string]any, extra map[string]any) error {
	p := paramsOf(pv)
	if p.Where != nil {
		if _, ok := p.Where.(map[string]any); !ok {
			return errors.New("Where must be an object") //nolint:revive,staticcheck // Strapi's message
		}
	}
	wheres := []any{}
	if p.Where != nil {
		wheres = append(wheres, p.Where)
	}
	if extra != nil {
		wheres = append(wheres, extra)
	}
	p.Where = nil
	return b.initWith(p, wheres)
}

func values(results []Row, col string) []any {
	seen := map[string]bool{}
	var out []any
	for _, r := range results {
		v, _ := r.Get(col)
		if v == nil {
			continue
		}
		k := idKey(v)
		if v != nil && k == "" {
			k = "0"
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

func groupRows(rows []Row, col string) map[string][]Row {
	out := map[string][]Row{}
	for _, r := range rows {
		v, _ := r.Get(col)
		k := groupKey(v)
		out[k] = append(out[k], r)
	}
	return out
}

func groupKey(v any) string {
	if v == nil {
		return "null"
	}
	if k := idKey(v); k != "" {
		return k
	}
	return "0"
}

func rowsFrom(m *metadata.Model, rows []Row) []any {
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(m, r))
	}
	return out
}

func firstFrom(m *metadata.Model, rows []Row) any {
	if len(rows) == 0 {
		return nil
	}
	return fromRow(m, rows[0])
}

func setAll(results []Row, name string, v func(Row) any) {
	for _, r := range results {
		r.Set(name, v(r))
	}
}

func (b *builder) applyPopulate(ctx context.Context, results []Row) error {
	for _, name := range b.popOrder {
		a, _ := b.model.Attributes.Get(name)
		pv := populateValue(b.populate[name], b.filters)
		isCount, _ := pv["count"].(bool)
		var err error
		switch a.Relation {
		case schema.OneToOne, schema.ManyToOne:
			err = b.populateLinked(ctx, results, name, a, pv, isCount, false)
		case schema.OneToMany, schema.ManyToMany:
			err = b.populateLinked(ctx, results, name, a, pv, isCount, true)
		case schema.MorphOne, schema.MorphMany:
			err = b.populateMorphTarget(ctx, results, name, a, pv)
		case schema.MorphToMany:
			err = b.populateMorphToMany(ctx, results, name, a, pv, isCount)
		case schema.MorphToOne:
			err = b.populateMorphToOne(ctx, results, name, a, pv, isCount)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) populateLinked(ctx context.Context, results []Row, name string, a *metadata.Attribute, pv map[string]any, isCount, many bool) error {
	target, ok := b.e.MD.Model(a.Target)
	if !ok {
		return fmt.Errorf("metadata for %q not found", a.Target)
	}
	if jc := a.JoinColumn; jc != nil {
		vals := values(results, jc.Name)
		if len(vals) == 0 {
			setAll(results, name, func(Row) any { return nil })
			return nil
		}
		nb, err := b.sub(target.UID)
		if err != nil {
			return err
		}
		extra := map[string]any{jc.ReferencedColumn: vals}
		if a.Virtual && name == schema.AttrLocalizations {
			extra["id"] = map[string]any{"$notIn": values(results, "id")}
		}
		if err := nb.subInit(pv, extra); err != nil {
			return err
		}
		nb.addSelect(nb.alias + "." + jc.ReferencedColumn)
		rows, err := nb.runRaw(ctx)
		if err != nil {
			return err
		}
		groups := groupRows(rows, jc.ReferencedColumn)
		setAll(results, name, func(r Row) any {
			v, _ := r.Get(jc.Name)
			if many {
				return rowsFrom(target, groups[groupKey(v)])
			}
			return firstFrom(target, groups[groupKey(v)])
		})
		return nil
	}
	jt := a.JoinTable
	if jt == nil {
		return nil
	}
	nb, err := b.sub(target.UID)
	if err != nil {
		return err
	}
	alias := nb.nextAlias()
	joinCol := alias + "." + jt.JoinColumn.Name
	rename := joinColPrefix + jt.JoinColumn.Name
	vals := values(results, jt.JoinColumn.ReferencedColumn)
	var on []entry
	for _, k := range sortedStringKeys(jt.On) {
		on = append(on, entry{k, jt.On[k]})
	}
	j := join{alias: alias, table: jt.Name, column: jt.InverseJoinColumn.Name, rootAlias: nb.alias, rootColumn: jt.InverseJoinColumn.ReferencedColumn, on: on}
	if isCount {
		if len(vals) == 0 {
			setAll(results, name, func(Row) any { return map[string]any{"count": int64(0)} })
			return nil
		}
		nb.joins = append(nb.joins, j)
		if err := nb.subInit(pv, map[string]any{joinCol: vals}); err != nil {
			return err
		}
		key := jt.JoinColumn.Name
		sel := joinCol
		// oneToMany selects the renamed column, the others the bare one.
		if a.Relation == schema.OneToMany {
			sel = joinCol + " as " + rename
			key = rename
		}
		nb.selects = []string{sel, rawSelect + "count(*) AS count"}
		nb.groupBy = []string{joinCol}
		nb.orderBy = nil
		nb.limit, nb.offset = nil, nil
		rows, err := nb.runRaw(ctx)
		if err != nil {
			return err
		}
		counts := map[string]int64{}
		for _, r := range rows {
			k, _ := r.Get(key)
			c, _ := r.Get("count")
			n, _ := numberOf(c)
			counts[groupKey(k)] = int64(n)
		}
		setAll(results, name, func(r Row) any {
			v, _ := r.Get(jt.JoinColumn.ReferencedColumn)
			return map[string]any{"count": counts[groupKey(v)]}
		})
		return nil
	}
	if len(vals) == 0 {
		setAll(results, name, func(Row) any {
			if many {
				return []any{}
			}
			return nil
		})
		return nil
	}
	j.orderBy = joinOrder(pv, jt, alias)
	nb.joins = append(nb.joins, j)
	if err := nb.subInit(pv, map[string]any{joinCol: vals}); err != nil {
		return err
	}
	nb.addSelect(joinCol + " as " + rename)
	rows, err := nb.runRaw(ctx)
	if err != nil {
		return err
	}
	groups := groupRows(rows, rename)
	setAll(results, name, func(r Row) any {
		v, _ := r.Get(jt.JoinColumn.ReferencedColumn)
		if many {
			return rowsFrom(target, groups[groupKey(v)])
		}
		return firstFrom(target, groups[groupKey(v)])
	})
	return nil
}

func (b *builder) populateMorphTarget(ctx context.Context, results []Row, name string, a *metadata.Attribute, pv map[string]any) error {
	target, ok := b.e.MD.Model(a.Target)
	if !ok {
		return fmt.Errorf("metadata for %q not found", a.Target)
	}
	inv, _ := target.Attributes.Get(a.MorphBy)
	if inv == nil {
		return nil
	}
	empty := func(Row) any {
		if a.Relation == schema.MorphOne {
			return nil
		}
		return []any{}
	}
	pick := func(rows []Row) any {
		if a.Relation == schema.MorphOne {
			return firstFrom(target, rows)
		}
		return rowsFrom(target, rows)
	}
	switch inv.Relation {
	case schema.MorphToOne:
		mc := inv.MorphColumn
		vals := values(results, mc.IDColumn.ReferencedColumn)
		if len(vals) == 0 {
			setAll(results, name, empty)
			return nil
		}
		nb, err := b.sub(target.UID)
		if err != nil {
			return err
		}
		if err := nb.subInit(pv, map[string]any{mc.IDColumn.Name: vals, mc.TypeColumn: b.model.UID}); err != nil {
			return err
		}
		rows, err := nb.runRaw(ctx)
		if err != nil {
			return err
		}
		groups := groupRows(rows, mc.IDColumn.Name)
		setAll(results, name, func(r Row) any {
			v, _ := r.Get(mc.IDColumn.ReferencedColumn)
			return pick(groups[groupKey(v)])
		})
	case schema.MorphToMany:
		jt := inv.JoinTable
		mc := jt.MorphColumn
		vals := values(results, mc.IDColumn.ReferencedColumn)
		if len(vals) == 0 {
			setAll(results, name, empty)
			return nil
		}
		nb, err := b.sub(target.UID)
		if err != nil {
			return err
		}
		alias := nb.nextAlias()
		var on []entry
		ons := map[string]string{"field": name}
		for k, v := range jt.On {
			ons[k] = v
		}
		ons["field"] = name
		for _, k := range sortedStringKeys(ons) {
			on = append(on, entry{k, ons[k]})
		}
		nb.joins = append(nb.joins, join{
			alias: alias, table: jt.Name, column: jt.JoinColumn.Name, rootAlias: nb.alias,
			rootColumn: jt.JoinColumn.ReferencedColumn, on: on, orderBy: joinOrder(pv, jt, alias),
		})
		if err := nb.subInit(pv, map[string]any{alias + "." + mc.IDColumn.Name: vals, alias + "." + mc.TypeColumn: b.model.UID}); err != nil {
			return err
		}
		nb.addSelect(alias+"."+mc.IDColumn.Name, alias+"."+mc.TypeColumn)
		rows, err := nb.runRaw(ctx)
		if err != nil {
			return err
		}
		groups := groupRows(rows, mc.IDColumn.Name)
		setAll(results, name, func(r Row) any {
			v, _ := r.Get(mc.IDColumn.ReferencedColumn)
			return pick(groups[groupKey(v)])
		})
	}
	return nil
}

func withoutOn(pv map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range pv {
		if k != "on" {
			out[k] = v
		}
	}
	return out
}

func (b *builder) typeRows(ctx context.Context, idsByType map[string][]any, types []string, pv map[string]any, idCol string, allowed func(string) bool) (map[string]map[string][]Row, error) {
	out := map[string]map[string][]Row{}
	on, _ := pv["on"].(map[string]any)
	for _, t := range types {
		if allowed != nil && !allowed(t) {
			out[t] = map[string][]Row{}
			continue
		}
		if _, ok := b.e.MD.Model(t); !ok {
			out[t] = map[string][]Row{}
			continue
		}
		nb, err := b.sub(t)
		if err != nil {
			return nil, err
		}
		tp := withoutOn(pv)
		if v, ok := on[t].(map[string]any); ok {
			tp = v
		}
		if err := nb.subInit(tp, map[string]any{idCol: idsByType[t]}); err != nil {
			return nil, err
		}
		nb.addSelect(nb.alias + "." + idCol)
		rows, err := nb.runRaw(ctx)
		if err != nil {
			return nil, err
		}
		out[t] = groupRows(rows, idCol)
	}
	return out, nil
}

func (b *builder) populateMorphToMany(ctx context.Context, results []Row, name string, a *metadata.Attribute, pv map[string]any, isCount bool) error {
	jt := a.JoinTable
	mc := jt.MorphColumn
	typeField := mc.TypeField
	if typeField == "" {
		typeField = "__type"
	}
	vals := values(results, jt.JoinColumn.ReferencedColumn)
	in, inArgs := inList(b.e, jt.JoinColumn.Name, vals, false)
	on, onArgs := onCond(b.e, jt.On)
	joinRows, err := b.e.linkRowsOrdered(ctx, "select * from "+b.e.q(jt.Name)+" where "+in+on+" order by "+b.e.q(jt.JoinColumn.Name)+" asc, "+b.e.q("order")+" asc", append(inArgs, onArgs...)...)
	if err != nil {
		return err
	}
	if onTypes, ok := pv["on"].(map[string]any); ok {
		var kept []Row
		for _, r := range joinRows {
			t, _ := r.Get(mc.TypeColumn)
			if ts, ok := t.(string); ok {
				if _, allowed := onTypes[ts]; allowed {
					kept = append(kept, r)
				}
			}
		}
		joinRows = kept
	}
	joinMap := groupRows(joinRows, jt.JoinColumn.Name)
	if isCount {
		setAll(results, name, func(r Row) any {
			v, _ := r.Get(jt.JoinColumn.ReferencedColumn)
			return map[string]any{"count": int64(len(joinMap[groupKey(v)]))}
		})
		return nil
	}
	idsByType := map[string][]any{}
	var types []string
	for _, r := range joinRows {
		id, _ := r.Get(mc.IDColumn.Name)
		t, _ := r.Get(mc.TypeColumn)
		ts, _ := t.(string)
		if id == nil || idKey(id) == "" || ts == "" {
			continue
		}
		if _, ok := idsByType[ts]; !ok {
			types = append(types, ts)
		}
		idsByType[ts] = append(idsByType[ts], id)
	}
	byType, err := b.typeRows(ctx, idsByType, types, pv, mc.IDColumn.ReferencedColumn, nil)
	if err != nil {
		return err
	}
	setAll(results, name, func(r Row) any {
		v, _ := r.Get(jt.JoinColumn.ReferencedColumn)
		out := []any{}
		for _, jr := range joinMap[groupKey(v)] {
			id, _ := jr.Get(mc.IDColumn.Name)
			t, _ := jr.Get(mc.TypeColumn)
			ts, _ := t.(string)
			tm, _ := b.e.MD.Model(ts)
			for _, row := range byType[ts][groupKey(id)] {
				if tm == nil {
					continue
				}
				fr := fromRow(tm, row)
				fr.Set(typeField, ts)
				out = append(out, fr)
			}
		}
		return out
	})
	return nil
}

func (b *builder) populateMorphToOne(ctx context.Context, results []Row, name string, a *metadata.Attribute, pv map[string]any, isCount bool) error {
	mc := a.MorphColumn
	typeField := mc.TypeField
	if typeField == "" {
		typeField = "__type"
	}
	idsByType := map[string][]any{}
	var types []string
	for _, r := range results {
		id, _ := r.Get(mc.IDColumn.Name)
		t, _ := r.Get(mc.TypeColumn)
		ts, _ := t.(string)
		if id == nil || idKey(id) == "" || ts == "" {
			continue
		}
		if _, ok := idsByType[ts]; !ok {
			types = append(types, ts)
		}
		idsByType[ts] = append(idsByType[ts], id)
	}
	on, hasOn := pv["on"].(map[string]any)
	allowed := func(t string) bool {
		if !hasOn {
			return true
		}
		_, ok := on[t]
		return ok
	}
	if isCount {
		setAll(results, name, func(r Row) any {
			id, _ := r.Get(mc.IDColumn.Name)
			t, _ := r.Get(mc.TypeColumn)
			ts, _ := t.(string)
			_, known := idsByType[ts]
			if id != nil && idKey(id) != "" && ts != "" && known && allowed(ts) {
				return map[string]any{"count": int64(1)}
			}
			return map[string]any{"count": int64(0)}
		})
		return nil
	}
	byType, err := b.typeRows(ctx, idsByType, types, pv, mc.IDColumn.ReferencedColumn, allowed)
	if err != nil {
		return err
	}
	setAll(results, name, func(r Row) any {
		id, _ := r.Get(mc.IDColumn.Name)
		t, _ := r.Get(mc.TypeColumn)
		ts, _ := t.(string)
		if ts == "" || id == nil || idKey(id) == "" {
			return nil
		}
		tm, ok := b.e.MD.Model(ts)
		rows := byType[ts][groupKey(id)]
		if !ok || len(rows) == 0 {
			return nil
		}
		fr := fromRow(tm, rows[0])
		fr.Set(typeField, ts)
		return fr
	})
	return nil
}

// linkRowsOrdered reads raw rows in their query order.
func (e *Engine) linkRowsOrdered(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := e.DB.Query(ctx, e.placeholders(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanRaw(rows)
}
