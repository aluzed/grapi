package document

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// Publish and discardDraft delete a version and create its replacement, so
// the links other rows held on the old version go with it (cascade). They
// are captured before and pointed at the new version after, as Strapi's
// unidirectional, bidirectional and self-referential relation utilities do.

type version struct{ id, locale any }

func versionsOf(rows []query.Row) []version {
	out := make([]version, len(rows))
	for i, r := range rows {
		out[i].id, _ = r.Get("id")
		out[i].locale, _ = r.Get(schema.AttrLocale)
	}
	return out
}

func idsOf(vs []version) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v.id
	}
	return out
}

type joinRow map[string]any

type linkBatch struct {
	jt   *metadata.JoinTable
	rows []joinRow
	// entityCol and relatedCol: the bidirectional order restore.
	entityCol, relatedCol string
	// remapSource and remapTarget: the self-referential remap.
	remapSource, remapTarget bool
}

type relinks struct{ uni, bi, self []linkBatch }

// loadRelinks captures the links to restore when oldVersions are replaced
// by copies of newVersions, which become targetStatus.
func (s *Service) loadRelinks(ctx context.Context, uid string, oldVersions, newVersions []version, targetStatus string) (*relinks, error) {
	r := &relinks{}
	var err error
	if r.uni, err = s.loadUnidirectional(ctx, uid, oldVersions, newVersions); err != nil {
		return nil, err
	}
	if r.bi, err = s.loadBidirectional(ctx, uid, oldVersions, newVersions); err != nil {
		return nil, err
	}
	if r.self, err = s.loadSelfReferential(ctx, uid, newVersions, targetStatus); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Service) syncRelinks(ctx context.Context, r *relinks, oldEntries []version, newEntries []query.Row) error {
	created := versionsOf(newEntries)
	idMap := map[string]any{}
	byLocale := map[string]any{}
	for _, e := range created {
		byLocale[localeString(e.locale)] = e.id
	}
	for _, e := range oldEntries {
		if id, ok := byLocale[localeString(e.locale)]; ok {
			idMap[fmt.Sprint(e.id)] = id
		}
	}
	if err := s.syncUnidirectional(ctx, idMap, r.uni); err != nil {
		return err
	}
	if err := s.syncBidirectional(ctx, idMap, r.bi); err != nil {
		return err
	}
	return s.syncSelfReferential(ctx, idMap, r.self)
}

// relationModels are the content types and components, in Strapi's order.
func (s *Service) relationModels() []*metadata.Model {
	var out []*metadata.Model
	for uid := range s.Set.ContentTypes.All() {
		if m, ok := s.E.MD.Model(uid); ok {
			out = append(out, m)
		}
	}
	for uid := range s.Set.Components.All() {
		if m, ok := s.E.MD.Model(uid); ok {
			out = append(out, m)
		}
	}
	return out
}

func linkTable(a *metadata.Attribute) *metadata.JoinTable {
	if a.Type != schema.TypeRelation || a.JoinTable == nil || a.MorphBy != "" || a.JoinTable.MorphColumn != nil {
		return nil
	}
	return a.JoinTable
}

func (s *Service) loadUnidirectional(ctx context.Context, uid string, oldVersions, newVersions []version) ([]linkBatch, error) {
	var out []linkBatch
	oldIDs, newIDs := idsOf(oldVersions), idsOf(newVersions)
	for _, m := range s.relationModels() {
		for _, a := range m.Attributes.All() {
			jt := linkTable(a)
			if jt == nil || a.Target != uid || a.InversedBy != "" || a.MappedBy != "" {
				continue
			}
			src, tgt := jt.JoinColumn.Name, jt.InverseJoinColumn.Name
			w := s.where().in(tgt, oldIDs)
			// Self links between republished entries are the self-referential pass's.
			if m.UID == uid && len(oldIDs) > 0 {
				w.notIn(src, oldIDs)
			}
			old, err := s.selectLinks(ctx, jt.Name, w)
			if err != nil {
				return nil, err
			}
			if len(old) > 0 {
				out = append(out, linkBatch{jt: jt, rows: old})
			}
			if s.hasDP(m.UID) {
				continue
			}
			// A model without draft and publish links one version: carry
			// those links over when the other version had none.
			rows, err := s.selectLinks(ctx, jt.Name, s.where().in(tgt, newIDs))
			if err != nil {
				return nil, err
			}
			if _, isComponent := s.Set.Component(schema.UID(m.UID)); isComponent && len(rows) > 0 {
				if rows, err = s.withoutDPOwnedComponents(ctx, m.UID, src, rows); err != nil {
					return nil, err
				}
			}
			had := map[string]bool{}
			for _, o := range old {
				had[fmt.Sprint(o[src])] = true
			}
			var add []joinRow
			for _, r := range rows {
				if !had[fmt.Sprint(r[src])] {
					delete(r, "id")
					add = append(add, r)
				}
			}
			if len(add) > 0 {
				out = append(out, linkBatch{jt: jt, rows: add})
			}
		}
	}
	return out, nil
}

func (s *Service) syncUnidirectional(ctx context.Context, idMap map[string]any, batches []linkBatch) error {
	for _, b := range batches {
		col := b.jt.InverseJoinColumn.Name
		for _, r := range b.rows {
			id, ok := idMap[fmt.Sprint(r[col])]
			if !ok {
				continue
			}
			r[col] = id
			if err := s.insertLink(ctx, b.jt.Name, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) loadBidirectional(ctx context.Context, uid string, oldVersions, newVersions []version) ([]linkBatch, error) {
	var out []linkBatch
	for _, m := range s.relationModels() {
		for _, a := range m.Attributes.All() {
			jt := linkTable(a)
			if jt == nil || (a.InversedBy == "" && a.MappedBy == "") {
				continue
			}
			owning := a.InversedBy != "" && m.UID == uid && a.Relation == schema.ManyToMany && m.UID != a.Target
			inverse := a.Target == uid && m.UID != uid
			if !owning && !inverse {
				continue
			}
			entityCol, relatedCol, relatedUID := jt.InverseJoinColumn.Name, jt.JoinColumn.Name, m.UID
			if owning {
				entityCol, relatedCol, relatedUID = jt.JoinColumn.Name, jt.InverseJoinColumn.Name, a.Target
			}
			// The owning side's links are written again in draft order by
			// the copy: restoring the old order would revert a reorder.
			if !owning && len(oldVersions) > 0 {
				rows, err := s.selectLinks(ctx, jt.Name, s.where().in(entityCol, idsOf(oldVersions)))
				if err != nil {
					return nil, err
				}
				if len(rows) > 0 {
					out = append(out, linkBatch{jt: jt, rows: rows, entityCol: entityCol, relatedCol: relatedCol})
				}
			}
			if _, ok := s.Set.ContentType(schema.UID(m.UID)); !ok {
				continue
			}
			oldLocales := map[string]bool{}
			for _, v := range oldVersions {
				oldLocales[localeString(v.locale)] = true
			}
			var firsts []any
			for _, v := range newVersions {
				if !oldLocales[localeString(v.locale)] {
					firsts = append(firsts, v.id)
				}
			}
			if len(firsts) == 0 {
				continue
			}
			rows, err := s.selectLinks(ctx, jt.Name, s.where().in(entityCol, firsts))
			if err != nil || len(rows) == 0 {
				if err != nil {
					return nil, err
				}
				continue
			}
			if s.hasDP(relatedUID) {
				rm, ok := s.E.MD.Model(relatedUID)
				if ok {
					related := make([]any, len(rows))
					for i, r := range rows {
						related[i] = r[relatedCol]
					}
					counterparts, err := s.counterparts(ctx, rm.TableName, related, "published")
					if err != nil {
						return nil, err
					}
					for _, r := range rows {
						if id, ok := counterparts[fmt.Sprint(r[relatedCol])]; ok {
							r[relatedCol] = id
						}
					}
				}
			}
			out = append(out, linkBatch{jt: jt, rows: rows, entityCol: entityCol, relatedCol: relatedCol})
		}
	}
	return out, nil
}

// syncBidirectional restores the order the related entries had the old
// version in. Strapi also inserts the missing links of sources that are
// not republished, but every mapped source is a republished entry, so it
// never does.
func (s *Service) syncBidirectional(ctx context.Context, idMap map[string]any, batches []linkBatch) error {
	for _, b := range batches {
		order := b.jt.OrderColumnName
		if order == "" || b.entityCol == "" || b.relatedCol == "" {
			continue
		}
		for _, r := range b.rows {
			id, ok := idMap[fmt.Sprint(r[b.entityCol])]
			if !ok {
				continue
			}
			if err := s.updateLink(ctx, b.jt.Name, b.entityCol, id, b.relatedCol, r[b.relatedCol], map[string]any{order: r[order]}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) loadSelfReferential(ctx context.Context, uid string, sources []version, targetStatus string) ([]linkBatch, error) {
	m, ok := s.E.MD.Model(uid)
	if !ok {
		return nil, nil
	}
	var out []linkBatch
	ids := idsOf(sources)
	for _, a := range m.Attributes.All() {
		jt := linkTable(a)
		// The mappedBy side shares the owning side's table.
		if jt == nil || a.Target != uid || a.MappedBy != "" {
			continue
		}
		src, tgt := jt.JoinColumn.Name, jt.InverseJoinColumn.Name
		both, err := s.selectLinks(ctx, jt.Name, s.where().in(src, ids).in(tgt, ids))
		if err != nil {
			return nil, err
		}
		if len(both) > 0 {
			out = append(out, linkBatch{jt: jt, rows: both, remapSource: true, remapTarget: true})
		}
		// The links of the copied state are the truth for one-sided rows:
		// their other end moves to its counterpart of targetStatus, or the
		// link is dropped when there is none.
		for _, side := range []struct{ mine, other string }{{src, tgt}, {tgt, src}} {
			rows, err := s.selectLinks(ctx, jt.Name, s.where().in(side.mine, ids).notIn(side.other, ids))
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				continue
			}
			others := make([]any, len(rows))
			for i, r := range rows {
				others[i] = r[side.other]
			}
			counterparts, err := s.counterparts(ctx, m.TableName, others, targetStatus)
			if err != nil {
				return nil, err
			}
			var kept []joinRow
			for _, r := range rows {
				if id, ok := counterparts[fmt.Sprint(r[side.other])]; ok {
					r[side.other] = id
					kept = append(kept, r)
				}
			}
			if len(kept) > 0 {
				out = append(out, linkBatch{jt: jt, rows: kept, remapSource: side.mine == src, remapTarget: side.mine == tgt})
			}
		}
	}
	return out, nil
}

func (s *Service) syncSelfReferential(ctx context.Context, idMap map[string]any, batches []linkBatch) error {
	for _, b := range batches {
		src, tgt := b.jt.JoinColumn.Name, b.jt.InverseJoinColumn.Name
		seen := map[string]bool{}
		var links []joinRow
		for _, r := range b.rows {
			out := joinRow{}
			for k, v := range r {
				if k != "id" {
					out[k] = v
				}
			}
			if b.remapSource {
				id, ok := idMap[fmt.Sprint(r[src])]
				if !ok {
					continue
				}
				out[src] = id
			}
			if b.remapTarget {
				id, ok := idMap[fmt.Sprint(r[tgt])]
				if !ok {
					continue
				}
				out[tgt] = id
			}
			key := fmt.Sprint(out[src]) + ":" + fmt.Sprint(out[tgt])
			if !seen[key] {
				seen[key] = true
				links = append(links, out)
			}
		}
		for _, l := range links {
			exists, err := s.linkExists(ctx, b.jt.Name, src, l[src], tgt, l[tgt])
			if err != nil {
				return err
			}
			if !exists {
				if err := s.insertLink(ctx, b.jt.Name, l); err != nil {
					return err
				}
				continue
			}
			// The copy already wrote the link: restore its saved order.
			restore := map[string]any{}
			for _, c := range []string{b.jt.OrderColumnName, b.jt.InverseOrderColumnName} {
				if v, ok := l[c]; ok && c != "" {
					restore[c] = v
				}
			}
			if len(restore) > 0 {
				if err := s.updateLink(ctx, b.jt.Name, src, l[src], tgt, l[tgt], restore); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// counterparts maps entry ids of table to the id of the same document and
// locale in status.
func (s *Service) counterparts(ctx context.Context, table string, ids []any, status string) (map[string]any, error) {
	out := map[string]any{}
	if len(ids) == 0 {
		return out, nil
	}
	entries, err := s.selectLinks(ctx, table, s.where().in("id", ids), "id", "document_id", "locale")
	if err != nil || len(entries) == 0 {
		return out, err
	}
	docs := make([]any, len(entries))
	for i, e := range entries {
		docs[i] = e["document_id"]
	}
	w := s.where().in("document_id", docs)
	if status == "published" {
		w.raw(s.q("published_at") + " is not null")
	} else {
		w.raw(s.q("published_at") + " is null")
	}
	others, err := s.selectLinks(ctx, table, w, "id", "document_id", "locale")
	if err != nil {
		return nil, err
	}
	byKey := map[string]any{}
	for _, o := range others {
		byKey[fmt.Sprint(o["document_id"])+":"+localeString(o["locale"])] = o["id"]
	}
	for _, e := range entries {
		if id, ok := byKey[fmt.Sprint(e["document_id"])+":"+localeString(e["locale"])]; ok {
			out[fmt.Sprint(e["id"])] = id
		}
	}
	return out, nil
}

// withoutDPOwnedComponents drops the links of component instances owned,
// directly or through other components, by a draft and publish entry: the
// copy of that entry rewrites them.
func (s *Service) withoutDPOwnedComponents(ctx context.Context, componentUID, idCol string, rows []joinRow) ([]joinRow, error) {
	ids := make([]any, len(rows))
	for i, r := range rows {
		ids[i] = r[idCol]
	}
	owned, err := s.dpOwnedComponents(ctx, componentUID, ids)
	if err != nil {
		return nil, err
	}
	var out []joinRow
	for _, r := range rows {
		if !owned[fmt.Sprint(r[idCol])] {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) dpOwnedComponents(ctx context.Context, componentUID string, ids []any) (map[string]bool, error) {
	owned := map[string]bool{}
	type parent struct {
		uid string
		id  any
	}
	parents := map[string]parent{}
	pending := ids
	for _, m := range s.relationModels() {
		if len(pending) == 0 {
			break
		}
		var jt *metadata.JoinTable
		for _, a := range m.Attributes.All() {
			if a.JoinTable != nil && a.JoinTable.On["field"] != "" && len(a.JoinTable.PivotColumns) == 4 && (a.Target == componentUID || a.JoinTable.MorphColumn != nil) {
				jt = a.JoinTable
				break
			}
		}
		if jt == nil {
			continue
		}
		entity, cmpCol, typeCol := jt.JoinColumn.Name, jt.InverseJoinColumn.Name, jt.PivotColumns[3]
		rows, err := s.selectLinks(ctx, jt.Name, s.where().eq(typeCol, componentUID).in(cmpCol, pending), entity, cmpCol)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			k := fmt.Sprint(r[cmpCol])
			if _, ok := parents[k]; !ok {
				parents[k] = parent{uid: m.UID, id: r[entity]}
			}
		}
		var rest []any
		for _, id := range pending {
			if _, ok := parents[fmt.Sprint(id)]; !ok {
				rest = append(rest, id)
			}
		}
		pending = rest
	}
	nested := map[string]map[string][]string{}
	for cmp, p := range parents {
		if _, ok := s.Set.Component(schema.UID(p.uid)); ok {
			if nested[p.uid] == nil {
				nested[p.uid] = map[string][]string{}
			}
			k := fmt.Sprint(p.id)
			nested[p.uid][k] = append(nested[p.uid][k], cmp)
			continue
		}
		if s.hasDP(p.uid) {
			owned[cmp] = true
		}
	}
	for uid, byParent := range nested {
		parentIDs := make([]any, 0, len(byParent))
		for k := range byParent {
			parentIDs = append(parentIDs, k)
		}
		up, err := s.dpOwnedComponents(ctx, uid, parentIDs)
		if err != nil {
			return nil, err
		}
		for k := range up {
			for _, cmp := range byParent[k] {
				owned[cmp] = true
			}
		}
	}
	return owned, nil
}

func (s *Service) q(id string) string { return s.E.DB.Dialect.Quote(id) }

type whereSQL struct {
	s     *Service
	conds []string
	args  []any
	none  bool
}

func (s *Service) where() *whereSQL { return &whereSQL{s: s} }

func (w *whereSQL) ph(v any) string {
	w.args = append(w.args, v)
	return w.s.E.DB.Dialect.Placeholder(len(w.args))
}

func (w *whereSQL) eq(col string, v any) *whereSQL {
	w.conds = append(w.conds, w.s.q(col)+" = "+w.ph(v))
	return w
}

func (w *whereSQL) list(vals []any) string {
	ph := make([]string, len(vals))
	for i, v := range vals {
		ph[i] = w.ph(v)
	}
	return strings.Join(ph, ", ")
}

func (w *whereSQL) in(col string, vals []any) *whereSQL {
	if len(vals) == 0 {
		w.none = true
		return w
	}
	w.conds = append(w.conds, w.s.q(col)+" in ("+w.list(vals)+")")
	return w
}

func (w *whereSQL) notIn(col string, vals []any) *whereSQL {
	if len(vals) > 0 {
		w.conds = append(w.conds, w.s.q(col)+" not in ("+w.list(vals)+")")
	}
	return w
}

func (w *whereSQL) raw(cond string) *whereSQL {
	w.conds = append(w.conds, cond)
	return w
}

func (s *Service) selectLinks(ctx context.Context, table string, w *whereSQL, cols ...string) ([]joinRow, error) {
	if w.none {
		return nil, nil
	}
	sel := "*"
	if len(cols) > 0 {
		quoted := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = s.q(c)
		}
		sel = strings.Join(quoted, ", ")
	}
	stmt := "select " + sel + " from " + s.q(table)
	if len(w.conds) > 0 {
		stmt += " where " + strings.Join(w.conds, " and ")
	}
	rows, err := s.E.DB.Query(ctx, stmt+" order by "+s.q("id"), w.args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanLinks(rows)
}

func scanLinks(rows *sql.Rows) ([]joinRow, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []joinRow
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := joinRow{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// insertLink writes r as is: a restored link keeps the id it had, as in
// Strapi.
func (s *Service) insertLink(ctx context.Context, table string, r joinRow) error {
	cols := make([]string, 0, len(r))
	for c := range r {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	w := s.where()
	quoted := make([]string, len(cols))
	ph := make([]string, len(cols))
	for i, c := range cols {
		quoted[i], ph[i] = s.q(c), w.ph(r[c])
	}
	_, err := s.E.DB.Exec(ctx, "insert into "+s.q(table)+" ("+strings.Join(quoted, ", ")+") values ("+strings.Join(ph, ", ")+")", w.args...)
	return err
}

func (s *Service) updateLink(ctx context.Context, table, srcCol string, src any, tgtCol string, tgt any, set map[string]any) error {
	cols := make([]string, 0, len(set))
	for c := range set {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	w := s.where()
	sets := make([]string, len(cols))
	for i, c := range cols {
		sets[i] = s.q(c) + " = " + w.ph(set[c])
	}
	w.eq(srcCol, src).eq(tgtCol, tgt)
	_, err := s.E.DB.Exec(ctx, "update "+s.q(table)+" set "+strings.Join(sets, ", ")+" where "+strings.Join(w.conds, " and "), w.args...)
	return err
}

func (s *Service) linkExists(ctx context.Context, table, srcCol string, src any, tgtCol string, tgt any) (bool, error) {
	rows, err := s.selectLinks(ctx, table, s.where().eq(srcCol, src).eq(tgtCol, tgt), srcCol)
	return len(rows) > 0, err
}
