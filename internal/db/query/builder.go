// Package query is grapi's query engine: Strapi's db.query(uid) on top of the
// database metadata. Filters, sorting and search produce the same rows, in
// the same order, as Strapi 5's query builder on the same database.
package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/schema"
)

// Engine runs queries on a database described by its metadata.
type Engine struct {
	DB *db.DB
	MD *metadata.Metadata
}

// Params of a query, as Strapi's query builder init() takes them. Where,
// Filters and OrderBy are decoded JSON (objects, arrays and scalars).
type Params struct {
	Select  []string
	Where   any
	Filters any
	OrderBy any
	Limit   *int
	Offset  *int
	Search  string
	// Populate is Strapi's populate: true, a list of paths, or an object
	// mapping relations to true or to their own params.
	Populate any
}

// FilterFunc gives the filters of a model: Strapi's query `filters` function,
// applied to the root model and to every populated one (status filters).
type FilterFunc func(m *metadata.Model) any

// Row is a row read back, keyed by attribute name in column order.
type Row = *schema.OrderedMap[any]

type join struct {
	alias, table, column, rootAlias, rootColumn string
	on                                          []entry
	// orderBy is the link table's order column (populate keeps link order).
	orderBy []orderTerm
}

type orderTerm struct {
	column string
	order  string
	// status sorts on the draft/modified/published rank (raw expression).
	status bool
	i18n   bool
}

type builder struct {
	e        *Engine
	model    *metadata.Model
	alias    string
	counter  int
	joins    []join
	orderBy  []orderTerm
	where    filter
	search   string
	selects  []string
	limit    *int
	offset   *int
	first    bool
	kind     string
	groupBy  []string
	populate map[string]any
	popOrder []string
	// filters are given to every populated relation, like Strapi's state.filters.
	filters any
}

// ErrNotFound is returned for an unknown model.
var ErrNotFound = errors.New("model not found")

func (e *Engine) newBuilder(uid, kind string) (*builder, error) {
	m, ok := e.MD.Model(uid)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, uid)
	}
	b := &builder{e: e, model: m, kind: kind}
	b.alias = b.nextAlias()
	return b, nil
}

func (b *builder) nextAlias() string {
	a := "t" + strconv.Itoa(b.counter)
	b.counter++
	return a
}

func (b *builder) sqlite() bool { return b.e.DB.Dialect.Name() == "sqlite" }

func (b *builder) mustUseAlias() bool { return b.kind == "select" || b.kind == "count" }

func (b *builder) aliasColumn(key, alias string) string {
	if strings.Contains(key, ".") {
		return key
	}
	if alias != "" {
		return alias + "." + key
	}
	if b.mustUseAlias() {
		return b.alias + "." + key
	}
	return key
}

func columnName(m *metadata.Model, name string) string {
	if a, ok := m.Attributes.Get(name); ok && a.ColumnName != "" {
		return a.ColumnName
	}
	return name
}

func scalarType(t string) bool {
	switch t {
	case "increments", schema.TypePassword, schema.TypeEmail, schema.TypeString, schema.TypeUID, schema.TypeRichText,
		schema.TypeText, schema.TypeJSON, schema.TypeEnumeration, schema.TypeInteger, schema.TypeBigInteger,
		schema.TypeFloat, schema.TypeDecimal, schema.TypeDate, schema.TypeTime, schema.TypeDateTime,
		schema.TypeTimestamp, schema.TypeBoolean, schema.TypeBlocks:
		return true
	}
	return false
}

func (b *builder) addJoin(j join) { b.joins = append(b.joins, j) }

// createJoin joins the target of a relation and returns its alias.
func (b *builder) createJoin(alias string, model *metadata.Model, name string, attr *metadata.Attribute) (string, error) {
	if alias == "" {
		alias = b.alias
	}
	target, ok := b.e.MD.Model(attr.Target)
	if !ok {
		return "", fmt.Errorf("metadata for %q not found", attr.Target)
	}
	if attr.Relation == schema.MorphOne || attr.Relation == schema.MorphMany {
		inverse, ok := target.Attributes.Get(attr.MorphBy)
		if !ok {
			return alias, nil
		}
		switch {
		case inverse.MorphColumn != nil:
			sub := b.nextAlias()
			b.addJoin(join{
				alias: sub, table: target.TableName, column: inverse.MorphColumn.IDColumn.Name,
				rootAlias: alias, rootColumn: inverse.MorphColumn.IDColumn.ReferencedColumn,
				on: []entry{{inverse.MorphColumn.TypeColumn, model.UID}},
			})
			return sub, nil
		case inverse.JoinTable != nil && inverse.JoinTable.MorphColumn != nil:
			jt := inverse.JoinTable
			pivot := b.nextAlias()
			b.addJoin(join{
				alias: pivot, table: jt.Name, column: jt.MorphColumn.IDColumn.Name,
				rootAlias: alias, rootColumn: jt.MorphColumn.IDColumn.ReferencedColumn,
				on: []entry{{jt.MorphColumn.TypeColumn, model.UID}, {"field", name}},
			})
			sub := b.nextAlias()
			b.addJoin(join{
				alias: sub, table: target.TableName, column: jt.JoinColumn.ReferencedColumn,
				rootAlias: pivot, rootColumn: jt.JoinColumn.Name,
			})
			return sub, nil
		}
		return alias, nil
	}
	if jc := attr.JoinColumn; jc != nil {
		sub := b.nextAlias()
		b.addJoin(join{alias: sub, table: target.TableName, column: jc.ReferencedColumn, rootAlias: alias, rootColumn: jc.Name})
		return sub, nil
	}
	if jt := attr.JoinTable; jt != nil {
		pivot := b.nextAlias()
		var on []entry
		for _, k := range sortedStringKeys(jt.On) {
			on = append(on, entry{k, jt.On[k]})
		}
		b.addJoin(join{
			alias: pivot, table: jt.Name, column: jt.JoinColumn.Name,
			rootAlias: alias, rootColumn: jt.JoinColumn.ReferencedColumn, on: on,
		})
		sub := b.nextAlias()
		b.addJoin(join{
			alias: sub, table: target.TableName, column: jt.InverseJoinColumn.ReferencedColumn,
			rootAlias: pivot, rootColumn: jt.InverseJoinColumn.Name,
		})
		return sub, nil
	}
	return alias, nil
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// processOrderBy is Strapi's processOrderBy: attribute names to aliased
// columns, with a join for each relation sorted on.
func (b *builder) processOrderBy(orderBy any, model *metadata.Model, alias string) ([]orderTerm, error) {
	switch o := orderBy.(type) {
	case nil:
		return nil, nil
	case string:
		if o == "status" {
			return b.statusOrder(model, "")
		}
		if _, ok := model.Attributes.Get(o); !ok {
			return nil, fmt.Errorf("Attribute %s not found on model %s", o, model.UID) //nolint:revive,staticcheck // Strapi's message
		}
		return []orderTerm{{column: b.aliasColumn(columnName(model, o), alias)}}, nil
	case []any:
		var out []orderTerm
		for _, v := range o {
			terms, err := b.processOrderBy(v, model, alias)
			if err != nil {
				return nil, err
			}
			out = append(out, terms...)
		}
		return out, nil
	case map[string]any:
		var out []orderTerm
		for _, key := range sortedKeys(o) {
			value := o[key]
			if key == "status" {
				dir, _ := value.(string)
				terms, err := b.statusOrder(model, dir)
				if err != nil {
					return nil, err
				}
				out = append(out, terms...)
				continue
			}
			attr, ok := model.Attributes.Get(key)
			if !ok {
				return nil, fmt.Errorf("Attribute %s not found on model %s", key, model.UID) //nolint:revive,staticcheck // Strapi's message
			}
			switch {
			case scalarType(attr.Type):
				dir, _ := value.(string)
				out = append(out, orderTerm{column: b.aliasColumn(columnName(model, key), alias), order: dir})
			case attr.Type == "relation" && attr.Target != "":
				sub, err := b.createJoin(alias, model, key, attr)
				if err != nil {
					return nil, err
				}
				target, _ := b.e.MD.Model(attr.Target)
				terms, err := b.processOrderBy(value, target, sub)
				if err != nil {
					return nil, err
				}
				out = append(out, terms...)
			default:
				return nil, fmt.Errorf("You cannot order on %s types", attr.Type) //nolint:revive,staticcheck // Strapi's message
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("Invalid orderBy syntax") //nolint:revive,staticcheck // Strapi's message
}

func (b *builder) statusOrder(model *metadata.Model, dir string) ([]orderTerm, error) {
	_, hasPublished := model.Attributes.Get(schema.AttrPublishedAt)
	_, hasDocument := model.Attributes.Get(schema.AttrDocumentID)
	if !hasPublished || !hasDocument {
		return nil, fmt.Errorf("Cannot order by status on model %s: missing publishedAt or documentId", model.UID) //nolint:revive,staticcheck // Strapi's message
	}
	_, i18n := model.Attributes.Get(schema.AttrLocale)
	return []orderTerm{{status: true, i18n: i18n, order: dir}}, nil
}

func (b *builder) statusExpression(alias string, i18n bool) string {
	q := b.e.DB.Dialect.Quote
	locale := ""
	if i18n {
		locale = " AND sub.locale = " + alias + ".locale"
	}
	table := q(b.model.TableName)
	return "CASE WHEN NOT EXISTS(SELECT 1 FROM " + table + " sub WHERE sub.document_id = " + alias + ".document_id AND sub.published_at IS NOT NULL" + locale +
		") THEN 0 WHEN " + alias + ".updated_at > (SELECT MAX(sub.updated_at) FROM " + table + " sub WHERE sub.document_id = " + alias +
		".document_id AND sub.published_at IS NOT NULL" + locale + ") THEN 1 ELSE 2 END"
}

// search is Strapi's applySearch: id, the string columns, and the number
// columns when the term is a number, matched as text.
func (b *builder) searchSQL(w *sqlWriter) {
	m := b.model
	cols := []string{"id"}
	searchable := func(a *metadata.Attribute) bool { return a.Searchable == nil || *a.Searchable }
	for name, a := range m.Attributes.All() {
		if stringType(a.Type) && searchable(a) {
			cols = append(cols, name)
		}
	}
	if n, ok := lodashToNumber(b.search).(float64); !ok || n == n {
		for name, a := range m.Attributes.All() {
			if numberType(a.Type) && searchable(a) {
				cols = append(cols, name)
			}
		}
	}
	pattern := "%" + escapeQuery(b.search, `*%\`) + "%"
	var parts []string
	for _, name := range cols {
		col := w.d.QuoteRef(b.aliasColumn(columnName(m, name), ""))
		if b.sqlite() {
			parts = append(parts, col+` LIKE ? ESCAPE '\'`)
		} else {
			parts = append(parts, col+"::text ILIKE ?")
		}
		w.args = append(w.args, pattern)
	}
	w.sql.WriteString("(" + strings.Join(parts, " or ") + ")")
}

func stringType(t string) bool {
	switch t {
	case schema.TypeString, schema.TypeText, schema.TypeUID, schema.TypeEmail, schema.TypeEnumeration, schema.TypeRichText:
		return true
	}
	return false
}

func numberType(t string) bool {
	switch t {
	case schema.TypeBigInteger, schema.TypeInteger, schema.TypeDecimal, schema.TypeFloat:
		return true
	}
	return false
}

func (b *builder) init(p Params) error {
	var wheres []any
	if p.Where != nil {
		if _, ok := p.Where.(map[string]any); !ok {
			return fmt.Errorf("Where must be an object") //nolint:revive,staticcheck // Strapi's message
		}
		wheres = append(wheres, p.Where)
	}
	return b.initWith(p, wheres)
}

// initWith is Strapi's init then processState: the sort, then the filters
// (wheres, then p.Filters), then the populate.
func (b *builder) initWith(p Params, wheres []any) error {
	if p.Select != nil {
		b.selects = slices.Compact(append([]string(nil), p.Select...))
	} else {
		b.selects = []string{"*"}
	}
	b.limit, b.offset = p.Limit, p.Offset
	b.search = p.Search
	b.filters = p.Filters
	terms, err := b.processOrderBy(p.OrderBy, b.model, "")
	if err != nil {
		return err
	}
	b.orderBy = terms
	switch f := p.Filters.(type) {
	case nil:
	case FilterFunc:
		if w := f(b.model); w != nil {
			if m, ok := w.(map[string]any); !ok || len(m) > 0 {
				wheres = append(wheres, w)
			}
		}
	default:
		wheres = append(wheres, f)
	}
	if len(wheres) > 0 {
		f, err := b.processWhere(wheres, b.model, "")
		if err != nil {
			return err
		}
		b.where = f
	}
	if err := b.processPopulate(p.Populate); err != nil {
		return err
	}
	b.ensurePaginationOrderStability()
	return nil
}

func (b *builder) deepSort() bool {
	for _, t := range b.orderBy {
		if t.status || !strings.Contains(t.column, ".") {
			continue
		}
		parts := strings.Split(t.column, ".")
		for _, el := range parts[:len(parts)-1] {
			if a, ok := b.model.Attributes.Get(el); ok && a.Type == "relation" {
				return true
			}
			for _, j := range b.joins {
				if j.alias == el {
					return true
				}
			}
		}
	}
	return false
}

// ensurePaginationOrderStability appends id to the sort of a paginated
// select, so that pages neither repeat nor skip rows.
func (b *builder) ensurePaginationOrderStability() {
	if b.kind != "select" || b.first || (b.limit == nil && b.offset == nil) {
		return
	}
	if b.limit != nil && *b.limit == -1 || b.deepSort() {
		return
	}
	if _, ok := b.model.Attributes.Get("id"); !ok {
		return
	}
	id := b.aliasColumn(columnName(b.model, "id"), "")
	if n := len(b.orderBy); n > 0 && b.orderBy[n-1].column == id {
		return
	}
	b.orderBy = append(b.orderBy, orderTerm{column: id, order: "asc"})
}

func (b *builder) distinct() bool { return len(b.joins) > 0 && len(b.groupBy) == 0 }

// rawSelect marks a select item written as SQL.
const rawSelect = "\x00raw:"

func (b *builder) selectColumn(s string) string {
	if strings.HasPrefix(s, rawSelect) || strings.Contains(s, " as ") {
		return s
	}
	return b.aliasColumn(columnName(b.model, s), "")
}

func renderSelect(d dialect, c string) string {
	if raw, ok := strings.CutPrefix(c, rawSelect); ok {
		return raw
	}
	if col, alias, ok := strings.Cut(c, " as "); ok {
		return d.QuoteRef(col) + " as " + d.Quote(alias)
	}
	return d.QuoteRef(c)
}

// sqlWriter collects SQL with "?" placeholders and their arguments.
type sqlWriter struct {
	d    dialect
	sql  strings.Builder
	args []any
}

func (w *sqlWriter) empty() bool { return w.sql.Len() == 0 }

type dialect struct{ db.Dialect }

// QuoteRef quotes "alias.column" or "column"; "*" stays bare.
func (d dialect) QuoteRef(ref string) string {
	parts := strings.Split(ref, ".")
	for i, p := range parts {
		if p != "*" {
			parts[i] = d.Quote(p)
		}
	}
	return strings.Join(parts, ".")
}

func (b *builder) renderJoins(w *sqlWriter) {
	for _, j := range b.joins {
		w.sql.WriteString(" left join " + w.d.Quote(j.table) + " as " + w.d.Quote(j.alias) +
			" on " + w.d.QuoteRef(j.rootAlias+"."+j.rootColumn) + " = " + w.d.QuoteRef(j.alias+"."+j.column))
		for _, o := range j.on {
			w.sql.WriteString(" and " + w.d.QuoteRef(j.alias+"."+o.key) + " = ?")
			w.args = append(w.args, o.val)
		}
	}
}

func (b *builder) renderWhere(w *sqlWriter) error {
	var conds []string
	if len(b.where) > 0 {
		var s sqlWriter
		s.d = w.d
		if err := b.applyWhere(b.where, &s); err != nil {
			return err
		}
		if !s.empty() {
			conds = append(conds, s.sql.String())
			w.args = append(w.args, s.args...)
		}
	}
	if b.search != "" {
		var s sqlWriter
		s.d = w.d
		b.searchSQL(&s)
		conds = append(conds, s.sql.String())
		w.args = append(w.args, s.args...)
	}
	if len(conds) > 0 {
		w.sql.WriteString(" where " + strings.Join(conds, " and "))
	}
	return nil
}

func (b *builder) orderSQL(t orderTerm, d dialect, alias string) string {
	dir := strings.ToLower(t.order)
	if dir != "desc" {
		dir = "asc"
	}
	if t.status {
		return b.statusExpression(d.Quote(alias), t.i18n) + " " + dir
	}
	return d.QuoteRef(t.column) + " " + dir
}

// renderPagination follows knex: a zero limit or offset is no clause.
func (b *builder) renderPagination(w *sqlWriter) {
	limit := 0
	if b.limit != nil {
		limit = *b.limit
	}
	if b.first {
		limit = 1
	}
	offset := 0
	if b.offset != nil {
		offset = *b.offset
	}
	if limit > 0 {
		w.sql.WriteString(" limit " + strconv.Itoa(limit))
	}
	if offset > 0 {
		if limit <= 0 {
			w.sql.WriteString(" limit " + b.noLimit())
		}
		w.sql.WriteString(" offset " + strconv.Itoa(offset))
	}
}

func (b *builder) noLimit() string {
	if b.sqlite() {
		return "-1"
	}
	return "all"
}

// selectSQL renders the select query.
func (b *builder) selectSQL() (string, []any, error) {
	w := &sqlWriter{d: dialect{b.e.DB.Dialect}}
	d := w.d
	var cols []string
	if b.distinct() {
		for _, j := range b.joins {
			for _, t := range j.orderBy {
				cols = append(cols, t.column)
			}
		}
		for _, t := range b.orderBy {
			if !t.status {
				cols = append(cols, t.column)
			}
		}
	}
	for _, s := range b.selects {
		cols = append(cols, b.selectColumn(s))
	}
	cols = dedupe(cols)
	rendered := make([]string, len(cols))
	for i, c := range cols {
		rendered[i] = renderSelect(d, c)
	}
	if b.distinct() {
		for _, t := range b.orderBy {
			if t.status {
				rendered = append(rendered, b.statusExpression(d.Quote(b.alias), t.i18n))
			}
		}
	}
	w.sql.WriteString("select ")
	if b.distinct() {
		w.sql.WriteString("distinct ")
	}
	w.sql.WriteString(strings.Join(rendered, ", "))
	w.sql.WriteString(" from " + d.Quote(b.model.TableName) + " as " + d.Quote(b.alias))
	b.renderJoins(w)
	if err := b.renderWhere(w); err != nil {
		return "", nil, err
	}
	if len(b.groupBy) > 0 {
		g := make([]string, len(b.groupBy))
		for i, c := range b.groupBy {
			g[i] = d.QuoteRef(c)
		}
		w.sql.WriteString(" group by " + strings.Join(g, ", "))
	}
	if b.deepSort() {
		return b.wrapDeepSort(w, cols)
	}
	var terms []string
	// Join orders (link tables' order columns) come before the root sort.
	for _, j := range b.joins {
		for _, t := range j.orderBy {
			terms = append(terms, b.orderSQL(t, d, b.alias))
		}
	}
	for _, t := range b.orderBy {
		terms = append(terms, b.orderSQL(t, d, b.alias))
	}
	if len(terms) > 0 {
		w.sql.WriteString(" order by " + strings.Join(terms, ", "))
	}
	b.renderPagination(w)
	return w.sql.String(), w.args, nil
}

func dedupe(list []string) []string {
	seen := map[string]bool{}
	out := list[:0]
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func orderAlias(column string) string {
	return "__strapi_order_by__" + strings.ReplaceAll(column, ".", "_")
}

// wrapDeepSort is Strapi's wrapWithDeepSort: sorting on a relation's column
// joins several rows per entry, so the filtered rows are numbered per id in
// sort order and only the first one of each id is kept.
func (b *builder) wrapDeepSort(base *sqlWriter, selected []string) (string, []any, error) {
	d := base.d
	var columnTerms []orderTerm
	var statusTerms []orderTerm
	for _, t := range b.orderBy {
		if t.status {
			statusTerms = append(statusTerms, t)
		} else {
			columnTerms = append(columnTerms, t)
		}
	}
	result, baseAlias, part := b.nextAlias(), b.nextAlias(), b.nextAlias()

	// The base query: same joins and filters, selecting the id and the sort columns.
	inner := &sqlWriter{d: d}
	cols := []string{d.QuoteRef(b.alias + ".id")}
	for _, t := range columnTerms {
		cols = append(cols, d.QuoteRef(t.column)+" as "+d.Quote(orderAlias(t.column)))
	}
	inner.sql.WriteString("select " + strings.Join(cols, ", ") + " from " + d.Quote(b.model.TableName) + " as " + d.Quote(b.alias))
	b.renderJoins(inner)
	if err := b.renderWhere(inner); err != nil {
		return "", nil, err
	}

	var partCols, rowOrder []string
	partCols = append(partCols, d.QuoteRef(baseAlias+".id"))
	for _, t := range columnTerms {
		c := d.QuoteRef(baseAlias + "." + orderAlias(t.column))
		partCols = append(partCols, c)
		// Strapi asks knex for NULLS LAST here, but knex drops it inside a
		// window function: each dialect's default NULL order applies.
		rowOrder = append(rowOrder, c+" "+direction(t.order))
	}
	rowNumber := "row_number() over (partition by " + d.QuoteRef(baseAlias+".id")
	if len(rowOrder) > 0 {
		rowNumber += " order by " + strings.Join(rowOrder, ", ")
	}
	rowNumber += ") as " + d.Quote("__strapi_row_number")
	partitioned := "(select " + strings.Join(append(partCols, rowNumber), ", ") + " from (" + inner.sql.String() + ") as " + d.Quote(baseAlias) + ") as " + d.Quote(part)

	var outCols []string
	for _, c := range selected {
		isOrder := false
		for _, t := range columnTerms {
			if t.column == c {
				isOrder = true
			}
		}
		if !isOrder {
			name := c
			if i := strings.LastIndex(c, "."); i >= 0 {
				name = c[i+1:]
			}
			outCols = append(outCols, d.QuoteRef(result+"."+name))
		}
	}
	w := &sqlWriter{d: d, args: inner.args}
	w.sql.WriteString("select " + strings.Join(outCols, ", ") + " from " + d.Quote(b.model.TableName) + " as " + d.Quote(result) +
		" inner join " + partitioned + " on " + d.QuoteRef(part+".id") + " = " + d.QuoteRef(result+".id") +
		" and " + d.QuoteRef(part+".__strapi_row_number") + " = ?")
	w.args = append(w.args, 1)
	var order []string
	for _, t := range columnTerms {
		order = append(order, d.QuoteRef(part+"."+orderAlias(t.column))+" "+direction(t.order))
	}
	for _, t := range statusTerms {
		order = append(order, b.statusExpression(d.Quote(result), t.i18n)+" "+direction(t.order))
	}
	order = append(order, d.QuoteRef(part+".id")+" asc")
	w.sql.WriteString(" order by " + strings.Join(order, ", "))
	b.renderPagination(w)
	return w.sql.String(), w.args, nil
}

func direction(o string) string {
	if strings.EqualFold(o, "desc") {
		return "desc"
	}
	return "asc"
}

// FindMany returns the rows matching p.
func (e *Engine) FindMany(ctx context.Context, uid string, p Params) ([]Row, error) {
	b, err := e.newBuilder(uid, "select")
	if err != nil {
		return nil, err
	}
	if err := b.init(p); err != nil {
		return nil, err
	}
	return b.run(ctx)
}

// FindOne returns the first row matching p, or nil.
func (e *Engine) FindOne(ctx context.Context, uid string, p Params) (Row, error) {
	b, err := e.newBuilder(uid, "select")
	if err != nil {
		return nil, err
	}
	b.first = true
	if err := b.init(p); err != nil {
		return nil, err
	}
	rows, err := b.run(ctx)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

// Count counts the entries matching p (distinct ids when relations are joined).
func (e *Engine) Count(ctx context.Context, uid string, p Params) (int64, error) {
	b, err := e.newBuilder(uid, "count")
	if err != nil {
		return 0, err
	}
	p.OrderBy, p.Limit, p.Offset = nil, nil, nil
	if err := b.init(p); err != nil {
		return 0, err
	}
	w := &sqlWriter{d: dialect{e.DB.Dialect}}
	id := w.d.QuoteRef(b.aliasColumn(columnName(b.model, "id"), ""))
	if b.distinct() {
		w.sql.WriteString("select count(distinct " + id + ") as " + w.d.Quote("count"))
	} else {
		w.sql.WriteString("select count(" + id + ") as " + w.d.Quote("count"))
	}
	w.sql.WriteString(" from " + w.d.Quote(b.model.TableName) + " as " + w.d.Quote(b.alias))
	b.renderJoins(w)
	if err := b.renderWhere(w); err != nil {
		return 0, err
	}
	var n int64
	err = e.DB.QueryRow(ctx, e.placeholders(w.sql.String()), w.args...).Scan(&n)
	return n, err
}

// placeholders turns "?" into the dialect's placeholders. The SQL is built
// by this package only: a "?" never appears inside a literal.
func (e *Engine) placeholders(s string) string {
	if e.DB.Dialect.Name() != "postgres" {
		return s
	}
	var out strings.Builder
	n := 0
	for _, r := range s {
		if r == '?' {
			n++
			out.WriteString(e.DB.Dialect.Placeholder(n))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func (b *builder) run(ctx context.Context) ([]Row, error) {
	raws, err := b.runRaw(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Row, len(raws))
	for i, r := range raws {
		out[i] = fromRow(b.model, r)
	}
	return out, nil
}

// runRaw returns rows keyed by column, populated relations included under
// their attribute name: Strapi's execute with mapResults false.
func (b *builder) runRaw(ctx context.Context) ([]Row, error) {
	query, args, err := b.selectSQL()
	if err != nil {
		return nil, err
	}
	rows, err := b.e.DB.Query(ctx, b.e.placeholders(query), args...)
	if err != nil {
		return nil, err
	}
	raws, err := scanRaw(rows)
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(b.populate) > 0 && len(raws) > 0 {
		if err := b.applyPopulate(ctx, raws); err != nil {
			return nil, err
		}
	}
	return raws, nil
}

func scanRaw(rows *sql.Rows) ([]Row, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []Row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := &schema.OrderedMap[any]{}
		for i, c := range cols {
			v := vals[i]
			if b, isBytes := v.([]byte); isBytes {
				v = string(b)
			}
			r.Set(c, normalizeInt(v))
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// fromRow keeps the columns that map to an attribute, converted like
// Strapi's fromRow: scalars through their field, relation values (join
// columns or populated relations) as they are.
func fromRow(m *metadata.Model, raw Row) Row {
	if raw == nil {
		return nil
	}
	row := &schema.OrderedMap[any]{}
	for col, v := range raw.All() {
		name, ok := m.ColumnToAttribute[col]
		if !ok {
			continue
		}
		attr, _ := m.Attributes.Get(name)
		switch {
		case scalarType(attr.Type):
			if v == nil {
				row.Set(name, nil)
			} else {
				row.Set(name, fromDB(attr.Type, v))
			}
		case attr.Type == "relation":
			row.Set(name, v)
		}
	}
	return row
}

func normalizeInt(v any) any {
	switch x := v.(type) {
	case int32:
		return int64(x)
	case int:
		return int64(x)
	}
	return v
}

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
