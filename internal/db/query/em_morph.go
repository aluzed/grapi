package query

import (
	"context"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/schema"
)

// encodeMorphID merges an id and its type into one key, Strapi's
// encodePolymorphicId.
func encodeMorphID(id any, typ string) string { return jsString(id) + ":::" + typ }

func encodeMorphLink(it relItem) *Link {
	l := &Link{ID: encodeMorphID(it.ID, it.Type)}
	if it.Position != nil {
		p := *it.Position
		if p.before() != "" {
			p.Before = encodeMorphID(p.Before, it.Type)
		}
		if p.after() != "" {
			p.After = encodeMorphID(p.After, it.Type)
		}
		l.Position = &p
	}
	return l
}

func (e *Engine) morphInverse(a *metadata.Attribute) (*metadata.Model, *metadata.Attribute) {
	target, ok := e.MD.Model(a.Target)
	if !ok {
		return nil, nil
	}
	inv, _ := target.Attributes.Get(a.MorphBy)
	return target, inv
}

// attachMorphTarget writes a morphOne or morphMany: the link lives on the
// target side, in its morph column or morph link table.
func (e *Engine) attachMorphTarget(ctx context.Context, m *metadata.Model, id any, name string, a *metadata.Attribute, rel assocs) error {
	target, inv := e.morphInverse(a)
	if inv == nil {
		return nil
	}
	switch inv.Relation {
	case schema.MorphToOne:
		var first any
		if len(rel.set) > 0 {
			first = rel.set[0].ID
		}
		relID, err := toID(first)
		if err != nil {
			return err
		}
		mc := inv.MorphColumn
		_, err = e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(mc.IDColumn.Name)+" = ?, "+e.q(mc.TypeColumn)+" = ? where "+e.q("id")+" = ?", id, m.UID, relID)
		return err
	case schema.MorphToMany:
		if len(rel.set) == 0 {
			return nil
		}
		jt := inv.JoinTable
		rows := make([]*schema.OrderedMap[any], 0, len(rel.set))
		for i, it := range rel.set {
			r := rowOf(jt.JoinColumn.Name, it.ID, jt.MorphColumn.IDColumn.Name, id, jt.MorphColumn.TypeColumn, m.UID)
			addOn(r, jt.On)
			addMap(r, it.Pivot)
			r.Set(orderColumn(), float64(i+1))
			r.Set("field", name)
			rows = append(rows, r)
		}
		return e.insertRows(ctx, jt.Name, rows, nil, nil, false)
	}
	return nil
}

func orderColumn() string { return "order" }

func (e *Engine) attachMorphToMany(ctx context.Context, m *metadata.Model, id any, name string, a *metadata.Attribute, rel assocs) error {
	if len(rel.set) == 0 && len(rel.connect) == 0 {
		return nil
	}
	jt := a.JoinTable
	mc := jt.MorphColumn
	dataset := rel.set
	if len(dataset) == 0 {
		dataset = rel.connect
	}
	dataset = withType(dataset, mc.TypeField)
	o := NewOrderer(nil, true)
	links := make([]*Link, len(dataset))
	for i, it := range dataset {
		links[i] = encodeMorphLink(it)
	}
	if err := o.Connect(links); err != nil {
		return err
	}
	pos := map[string]float64{}
	for i, l := range o.Relations() {
		pos[idKey(l.ID)] = float64(i + 1)
	}
	rows := make([]*schema.OrderedMap[any], 0, len(dataset))
	for _, it := range dataset {
		r := rowOf(jt.JoinColumn.Name, id, mc.IDColumn.Name, it.ID, mc.TypeColumn, nilIfEmpty(it.Type))
		addOn(r, jt.On)
		addMap(r, it.Pivot)
		r.Set(orderColumn(), pos[encodeMorphID(it.ID, it.Type)])
		rows = append(rows, r)
	}
	if err := e.deleteMorphOneAfterMorphToMany(ctx, rows, m.UID, name, jt); err != nil {
		return err
	}
	return e.insertRows(ctx, jt.Name, rows, nil, nil, false)
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// deleteMorphOneAfterMorphToMany removes the links a morphOne owner had to
// another entry, now that the morphToMany side points it elsewhere.
func (e *Engine) deleteMorphOneAfterMorphToMany(ctx context.Context, rows []*schema.OrderedMap[any], uid, name string, jt *metadata.JoinTable) error {
	mc := jt.MorphColumn
	groups := map[string]map[string][]any{}
	for _, r := range rows {
		typ, _ := r.Get(mc.TypeColumn)
		field, _ := r.Get("field")
		ts, _ := typ.(string)
		fs, _ := field.(string)
		model, ok := e.MD.Model(ts)
		if !ok {
			continue
		}
		ta, ok := model.Attributes.Get(fs)
		if !ok || ta.Target != uid || ta.MorphBy != name || ta.Relation != schema.MorphOne {
			continue
		}
		if groups[ts] == nil {
			groups[ts] = map[string][]any{}
		}
		v, _ := r.Get(mc.IDColumn.Name)
		groups[ts][fs] = append(groups[ts][fs], v)
	}
	var parts []string
	var args []any
	types := make([]string, 0, len(groups))
	for t := range groups {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		fields := make([]string, 0, len(groups[t]))
		for f := range groups[t] {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, f := range fields {
			in, inArgs := inList(e, mc.IDColumn.Name, groups[t][f], false)
			parts = append(parts, "("+e.q(mc.TypeColumn)+" = ? and "+e.q("field")+" = ? and "+in+")")
			args = append(append(args, t, f), inArgs...)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	_, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+strings.Join(parts, " or "), args...)
	return err
}

func (e *Engine) updateMorphTarget(ctx context.Context, m *metadata.Model, id any, name string, a *metadata.Attribute, rel assocs) error {
	target, inv := e.morphInverse(a)
	if inv == nil {
		return nil
	}
	switch inv.Relation {
	case schema.MorphToOne:
		mc := inv.MorphColumn
		if _, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(mc.IDColumn.Name)+" = null, "+e.q(mc.TypeColumn)+" = null where "+
			e.q(mc.IDColumn.Name)+" = ? and "+e.q(mc.TypeColumn)+" = ?", id, m.UID); err != nil {
			return err
		}
		if rel.setNull || len(rel.set) == 0 {
			return nil
		}
		_, err := e.sqlExec(ctx, "update "+e.q(target.TableName)+" set "+e.q(mc.IDColumn.Name)+" = ?, "+e.q(mc.TypeColumn)+" = ? where "+e.q("id")+" in (?)", id, m.UID, rel.set[0].ID)
		return err
	case schema.MorphToMany:
		jt := inv.JoinTable
		mc := jt.MorphColumn
		on, onArgs := onCond(e, jt.On)
		hasSet, hasConnect, hasDisconnect := len(rel.set) > 0, len(rel.connect) > 0, len(rel.disconnect) > 0
		if !hasSet && (hasConnect || hasDisconnect) {
			toDelete := append(append([]relItem{}, rel.disconnect...), rel.connect...)
			var parts []string
			var args []any
			for _, it := range toDelete {
				parts = append(parts, "("+e.q(mc.IDColumn.Name)+" = ? and "+e.q(mc.TypeColumn)+" = ? and "+e.q(jt.JoinColumn.Name)+" = ?"+on+" and "+e.q("field")+" = ?)")
				args = append(append(append(args, id, m.UID, it.ID), onArgs...), name)
			}
			if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+strings.Join(parts, " or "), args...); err != nil {
				return err
			}
			if !hasConnect {
				return nil
			}
			rows, err := e.linkRows(ctx, "select max("+e.q("order")+") as "+e.q("max")+" from "+e.q(jt.Name)+" where "+e.q(mc.IDColumn.Name)+" = ? and "+e.q(mc.TypeColumn)+" = ?"+on,
				append([]any{id, m.UID}, onArgs...)...)
			if err != nil {
				return err
			}
			start := 0.0
			if len(rows) > 0 {
				start, _ = numberOf(rows[0]["max"])
			}
			insert := make([]*schema.OrderedMap[any], 0, len(rel.connect))
			for i, it := range rel.connect {
				r := rowOf(jt.JoinColumn.Name, it.ID, mc.IDColumn.Name, id, mc.TypeColumn, m.UID)
				addOn(r, jt.On)
				addMap(r, it.Pivot)
				r.Set(orderColumn(), start+float64(i+1))
				r.Set("field", name)
				insert = append(insert, r)
			}
			return e.insertRows(ctx, jt.Name, insert, nil, nil, false)
		}
		if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(mc.IDColumn.Name)+" = ? and "+e.q(mc.TypeColumn)+" = ?"+on+" and "+e.q("field")+" = ?",
			append(append([]any{id, m.UID}, onArgs...), name)...); err != nil {
			return err
		}
		if !hasSet {
			return nil
		}
		insert := make([]*schema.OrderedMap[any], 0, len(rel.set))
		for i, it := range rel.set {
			r := rowOf(jt.JoinColumn.Name, it.ID, mc.IDColumn.Name, id, mc.TypeColumn, m.UID)
			addOn(r, jt.On)
			addMap(r, it.Pivot)
			r.Set(orderColumn(), float64(i+1))
			r.Set("field", name)
			insert = append(insert, r)
		}
		return e.insertRows(ctx, jt.Name, insert, nil, nil, false)
	}
	return nil
}

func (e *Engine) updateMorphToMany(ctx context.Context, m *metadata.Model, id any, name string, a *metadata.Attribute, rel assocs) error {
	jt := a.JoinTable
	mc := jt.MorphColumn
	rel.set = withType(rel.set, mc.TypeField)
	rel.connect = withType(rel.connect, mc.TypeField)
	rel.disconnect = withType(rel.disconnect, mc.TypeField)
	on, onArgs := onCond(e, jt.On)
	hasSet, hasConnect, hasDisconnect := len(rel.set) > 0, len(rel.connect) > 0, len(rel.disconnect) > 0
	if !hasSet && (hasConnect || hasDisconnect) {
		var toDeleteRows []*schema.OrderedMap[any]
		for i, it := range append(append([]relItem{}, rel.disconnect...), rel.connect...) {
			r := rowOf(jt.JoinColumn.Name, id, mc.IDColumn.Name, it.ID, mc.TypeColumn, nilIfEmpty(it.Type))
			addOn(r, jt.On)
			addMap(r, it.Pivot)
			r.Set(orderColumn(), float64(i+1))
			toDeleteRows = append(toDeleteRows, r)
		}
		var anchors []any
		for _, c := range rel.connect {
			if c.Position != nil {
				if c.Position.after() != "" {
					anchors = append(anchors, c.Position.After)
				} else if c.Position.before() != "" {
					anchors = append(anchors, c.Position.Before)
				}
			}
		}
		jc, oc := e.q(jt.JoinColumn.Name), e.q("order")
		in, inArgs := inList(e, mc.IDColumn.Name, anchors, false)
		sub := func(fn string) string {
			return "(select " + fn + "(" + oc + ") from " + e.q(jt.Name) + " where " + jc + " = ?" + on + ")"
		}
		args := append([]any{id}, inArgs...)
		args = append(append(append(args, id, id), onArgs...), id, id)
		args = append(append(args, onArgs...), onArgs...)
		adjacent, err := e.linkRows(ctx, "select * from "+e.q(jt.Name)+" where (("+jc+" = ? and "+in+") or ("+jc+" = ? and "+oc+" = "+sub("min")+") or ("+jc+" = ? and "+oc+" = "+sub("max")+"))"+on+" order by "+oc+" asc", args...)
		if err != nil {
			return err
		}
		toDelete := append(append([]relItem{}, rel.disconnect...), rel.connect...)
		if len(toDelete) > 0 {
			var parts []string
			var dargs []any
			for _, it := range toDelete {
				parts = append(parts, "("+e.q(mc.IDColumn.Name)+" = ? and "+e.q(mc.TypeColumn)+" = ? and "+jc+" = ?"+on+")")
				dargs = append(append(dargs, it.ID, nilIfEmpty(it.Type), id), onArgs...)
			}
			if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+strings.Join(parts, " or "), dargs...); err != nil {
				return err
			}
			if err := e.deleteMorphOneAfterMorphToMany(ctx, toDeleteRows, m.UID, name, jt); err != nil {
				return err
			}
		}
		if !hasConnect {
			return nil
		}
		var initial []*Link
		for _, r := range adjacent {
			order, ok := numberOf(r["order"])
			if !ok || r["order"] == nil {
				order = 1
			}
			typ, _ := r[mc.TypeColumn].(string)
			initial = append(initial, &Link{ID: encodeMorphID(r[mc.IDColumn.Name], typ), Order: order})
		}
		strict := true
		if rel.strict != nil {
			strict = *rel.strict
		}
		o := NewOrderer(initial, strict)
		links := make([]*Link, len(rel.connect))
		for i, it := range rel.connect {
			links[i] = encodeMorphLink(it)
		}
		if err := o.Connect(links); err != nil {
			return err
		}
		orders := o.OrderMap()
		insert := make([]*schema.OrderedMap[any], 0, len(rel.connect))
		for _, it := range rel.connect {
			r := rowOf(jt.JoinColumn.Name, id, mc.IDColumn.Name, it.ID, mc.TypeColumn, nilIfEmpty(it.Type))
			addOn(r, jt.On)
			addMap(r, it.Pivot)
			r.Set("field", name)
			r.Set(orderColumn(), orders[encodeMorphID(it.ID, it.Type)])
			insert = append(insert, r)
		}
		return e.insertRows(ctx, jt.Name, insert, nil, nil, false)
	}
	if !hasSet {
		return nil
	}
	if _, err := e.sqlExec(ctx, "delete from "+e.q(jt.Name)+" where "+e.q(jt.JoinColumn.Name)+" = ?"+on, append([]any{id}, onArgs...)...); err != nil {
		return err
	}
	rows := make([]*schema.OrderedMap[any], 0, len(rel.set))
	for i, it := range rel.set {
		r := rowOf(jt.JoinColumn.Name, id, mc.IDColumn.Name, it.ID, mc.TypeColumn, nilIfEmpty(it.Type))
		r.Set("field", name)
		addOn(r, jt.On)
		addMap(r, it.Pivot)
		r.Set(orderColumn(), float64(i+1))
		rows = append(rows, r)
	}
	if err := e.deleteMorphOneAfterMorphToMany(ctx, rows, m.UID, name, jt); err != nil {
		return err
	}
	return e.insertRows(ctx, jt.Name, rows, nil, nil, false)
}
