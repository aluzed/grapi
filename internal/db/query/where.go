package query

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/metadata"
)

// Operators of Strapi's filters, by family (@strapi/utils operators).
var (
	groupOperators = []string{"$and", "$or"}
	whereOperators = []string{
		"$not", "$in", "$notIn", "$eq", "$eqi", "$ne", "$nei", "$gt", "$gte", "$lt", "$lte", "$null",
		"$notNull", "$between", "$startsWith", "$endsWith", "$startsWithi", "$endsWithi", "$contains",
		"$notContains", "$containsi", "$notContainsi", "$jsonSupersetOf",
	}
	castOperators  = []string{"$not", "$in", "$notIn", "$eq", "$ne", "$gt", "$gte", "$lt", "$lte", "$between"}
	arrayOperators = []string{"$in", "$notIn", "$between"}
)

// IsOperator reports whether key is a filter operator.
func IsOperator(key string) bool {
	return slices.Contains(groupOperators, key) || slices.Contains(whereOperators, key)
}

// filter is a processed where: column references (aliased) or $and, $or and
// $not, with values in the shape Strapi's applyWhere reads.
type filter []entry

type entry struct {
	key string
	val any
}

// opMap is the processed where of one column: operator to value.
type opMap []entry

// sortedKeys gives a deterministic order to the keys of a decoded object.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// processWhere resolves attribute names to aliased columns, adds the joins
// relation filters need and converts values, like Strapi's processWhere.
func (b *builder) processWhere(where any, model *metadata.Model, alias string) (filter, error) {
	switch w := where.(type) {
	case []any:
		var subs []filter
		for _, sub := range w {
			f, err := b.processWhere(sub, model, alias)
			if err != nil {
				return nil, err
			}
			subs = append(subs, f)
		}
		return filter{{key: "$and", val: subs}}, nil
	case map[string]any:
		return b.processObject(w, model, alias)
	}
	return nil, fmt.Errorf("Where must be an array or an object") //nolint:revive,staticcheck // Strapi's message
}

func (b *builder) processNested(v any, model *metadata.Model, alias string) (any, error) {
	if m, ok := v.(map[string]any); ok {
		return b.processObject(m, model, alias)
	}
	return v, nil
}

func (b *builder) processObject(where map[string]any, model *metadata.Model, alias string) (filter, error) {
	var out filter
	for _, key := range sortedKeys(where) {
		value := where[key]
		switch {
		case slices.Contains(groupOperators, key):
			list, ok := value.([]any)
			if !ok {
				return nil, fmt.Errorf("Operator %s must be an array", key) //nolint:revive,staticcheck // Strapi's message
			}
			subs := make([]any, 0, len(list))
			for _, sub := range list {
				n, err := b.processNested(sub, model, alias)
				if err != nil {
					return nil, err
				}
				subs = append(subs, n)
			}
			out = append(out, entry{key, subs})
			continue
		case key == "$not":
			n, err := b.processNested(value, model, alias)
			if err != nil {
				return nil, err
			}
			out = append(out, entry{key, n})
			continue
		case slices.Contains(whereOperators, key):
			return nil, fmt.Errorf("Only $and, $or and $not can only be used as root level operators. Found %s.", key) //nolint:revive,staticcheck // Strapi's message
		}

		attr, ok := model.Attributes.Get(key)
		if !ok {
			// Strapi lets internal callers filter on raw columns; grapi only
			// accepts the columns the table has.
			// Keys qualified by an alias come from the engine itself (populate).
			if !hasColumn(model, key) && !strings.Contains(key, ".") {
				return nil, fmt.Errorf("undefined attribute %s on %s", key, model.UID)
			}
			v, err := b.processAttributeWhere("", value, "$eq")
			if err != nil {
				return nil, err
			}
			out = append(out, entry{b.aliasColumn(key, alias), v})
			continue
		}
		switch {
		case attr.Type == "relation" && attr.Target != "":
			sub, err := b.createJoin(alias, model, key, attr)
			if err != nil {
				return nil, err
			}
			target, ok := b.e.MD.Model(attr.Target)
			if !ok {
				return nil, fmt.Errorf("metadata for %q not found", attr.Target)
			}
			nested, err := b.processRelationWhere(value, target, sub)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
		case scalarType(attr.Type):
			v, err := b.processAttributeWhere(attr.Type, value, "$eq")
			if err != nil {
				return nil, err
			}
			out = append(out, entry{b.aliasColumn(columnName(model, key), alias), v})
		default:
			return nil, fmt.Errorf("You cannot filter on %s types", attr.Type) //nolint:revive,staticcheck // Strapi's message
		}
	}
	return out, nil
}

func (b *builder) processRelationWhere(where any, target *metadata.Model, alias string) (filter, error) {
	idAlias := b.aliasColumn("id", alias)
	m, ok := where.(map[string]any)
	if !ok {
		return filter{{idAlias, where}}, nil
	}
	var ops []string
	for k := range m {
		if IsOperator(k) {
			ops = append(ops, k)
		}
	}
	if len(ops) > 0 && len(ops) != len(m) {
		return nil, fmt.Errorf("Operator and non-operator keys cannot be mixed in a relation where clause") //nolint:revive,staticcheck // Strapi's message
	}
	if len(ops) > 1 {
		sort.Strings(ops)
		return nil, fmt.Errorf("Only one operator key is allowed in a relation where clause, but found: %s", strings.Join(ops, ",")) //nolint:revive,staticcheck // Strapi's message
	}
	if len(ops) == 1 {
		op := ops[0]
		if slices.Contains(groupOperators, op) {
			return b.processObject(m, target, alias)
		}
		nested, err := b.processNested(m[op], target, alias)
		if err != nil {
			return nil, err
		}
		return filter{{idAlias, opMap{{op, nested}}}}, nil
	}
	return b.processObject(m, target, alias)
}

// hasColumn reports whether the model's table has the column: an attribute's
// column, a join column or a morph column.
func hasColumn(m *metadata.Model, col string) bool {
	if _, ok := m.ColumnToAttribute[col]; ok {
		return true
	}
	for _, a := range m.Attributes.All() {
		if a.JoinColumn != nil && a.Owner && a.JoinColumn.Name == col {
			return true
		}
		if a.MorphColumn != nil && a.Owner && (a.MorphColumn.IDColumn.Name == col || a.MorphColumn.TypeColumn == col) {
			return true
		}
	}
	return false
}

// processAttributeWhere converts the values of one column's filter.
// attrType is empty for raw columns, whose values are kept as given.
func (b *builder) processAttributeWhere(attrType string, where any, operator string) (any, error) {
	if list, ok := where.([]any); ok {
		out := make([]any, len(list))
		for i, sub := range list {
			v, err := b.processSingleAttributeWhere(attrType, sub, operator)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	return b.processSingleAttributeWhere(attrType, where, operator)
}

func (b *builder) processSingleAttributeWhere(attrType string, where any, operator string) (any, error) {
	m, ok := where.(map[string]any)
	if !ok {
		if attrType != "" && slices.Contains(castOperators, operator) && where != nil {
			return toDB(attrType, where, b.sqlite())
		}
		return where, nil
	}
	var out opMap
	for _, key := range sortedKeys(m) {
		if !slices.Contains(whereOperators, key) {
			return nil, fmt.Errorf("Undefined attribute level operator %s", key) //nolint:revive,staticcheck // Strapi's message
		}
		v, err := b.processAttributeWhere(attrType, m[key], key)
		if err != nil {
			return nil, err
		}
		out = append(out, entry{key, v})
	}
	return out, nil
}

// applyWhere renders a processed filter as SQL conditions joined by AND.
func (b *builder) applyWhere(f filter, w *sqlWriter) error {
	var parts []string
	for _, e := range f {
		var sub sqlWriter
		sub.d = w.d
		switch e.key {
		case "$and", "$or":
			var items []string
			list, _ := e.val.([]any)
			if subs, ok := e.val.([]filter); ok {
				for _, s := range subs {
					list = append(list, s)
				}
			}
			for _, item := range list {
				f, ok := item.(filter)
				if !ok {
					return fmt.Errorf("Where must be an array or an object") //nolint:revive,staticcheck // Strapi's message
				}
				var s sqlWriter
				s.d = w.d
				if err := b.applyWhere(f, &s); err != nil {
					return err
				}
				if s.empty() {
					continue
				}
				items = append(items, s.sql.String())
				sub.args = append(sub.args, s.args...)
			}
			if len(items) == 0 {
				continue
			}
			sep := " and "
			if e.key == "$or" {
				sep = " or "
				for i := range items {
					items[i] = "(" + items[i] + ")"
				}
			}
			sub.sql.WriteString("(" + strings.Join(items, sep) + ")")
		case "$not":
			f, ok := e.val.(filter)
			if !ok {
				return fmt.Errorf("Where must be an array or an object") //nolint:revive,staticcheck // Strapi's message
			}
			var s sqlWriter
			s.d = w.d
			if err := b.applyWhere(f, &s); err != nil {
				return err
			}
			if s.empty() {
				continue
			}
			sub.sql.WriteString("not (" + s.sql.String() + ")")
			sub.args = append(sub.args, s.args...)
		default:
			if err := b.applyColumn(e.key, e.val, &sub); err != nil {
				return err
			}
		}
		if !sub.empty() {
			parts = append(parts, sub.sql.String())
			w.args = append(w.args, sub.args...)
		}
	}
	w.sql.WriteString(strings.Join(parts, " and "))
	return nil
}

func (b *builder) applyColumn(column string, where any, w *sqlWriter) error {
	ops, ok := where.(opMap)
	if !ok {
		if f, isFilter := where.(filter); isFilter {
			// A nested object reached a column: its keys are not operators.
			return fmt.Errorf("Undefined attribute level operator %s", f[0].key) //nolint:revive,staticcheck // Strapi's message
		}
		if list, isList := where.([]any); isList {
			return b.applyOperator(column, "$in", list, w)
		}
		return b.applyOperator(column, "$eq", where, w)
	}
	var parts []string
	for _, e := range ops {
		var s sqlWriter
		s.d = w.d
		if err := b.applyOperator(column, e.key, e.val, &s); err != nil {
			return err
		}
		if !s.empty() {
			parts = append(parts, s.sql.String())
			w.args = append(w.args, s.args...)
		}
	}
	w.sql.WriteString(strings.Join(parts, " and "))
	return nil
}

func (b *builder) lower(col string) string {
	if b.sqlite() {
		return "LOWER(" + col + ")"
	}
	return "LOWER(CAST(" + col + " AS VARCHAR))"
}

func (b *builder) likeEscape() string {
	if b.sqlite() {
		return ` ESCAPE '\'`
	}
	return ""
}

func escapeQuery(s, chars string) string {
	var out strings.Builder
	for _, r := range s {
		if strings.ContainsRune(chars, r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}

func escapeLike(v any) string { return escapeQuery(jsString(v), `%_\`) }

// truthy is JavaScript truthiness, which $null and $notNull use.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && x == x
	case int64:
		return x != 0
	case int:
		return x != 0
	}
	return true
}

func (b *builder) applyOperator(column, op string, value any, w *sqlWriter) error {
	col := w.d.QuoteRef(column)
	if list, ok := value.([]any); ok && !slices.Contains(arrayOperators, op) {
		var parts []string
		for _, v := range list {
			var s sqlWriter
			s.d = w.d
			if err := b.applyOperator(column, op, v, &s); err != nil {
				return err
			}
			if !s.empty() {
				parts = append(parts, "("+s.sql.String()+")")
				w.args = append(w.args, s.args...)
			}
		}
		if len(parts) > 0 {
			w.sql.WriteString("(" + strings.Join(parts, " or ") + ")")
		}
		return nil
	}
	switch op {
	case "$not":
		var s sqlWriter
		s.d = w.d
		if err := b.applyColumn(column, value, &s); err != nil {
			return err
		}
		if !s.empty() {
			w.sql.WriteString("not (" + s.sql.String() + ")")
			w.args = append(w.args, s.args...)
		}
	case "$in", "$notIn":
		list := castArray(value)
		if len(list) == 0 {
			if op == "$in" {
				w.sql.WriteString("1 = 0")
			} else {
				w.sql.WriteString("1 = 1")
			}
			return nil
		}
		not := ""
		if op == "$notIn" {
			not = "not "
		}
		w.sql.WriteString(col + " " + not + "in (" + strings.TrimSuffix(strings.Repeat("?, ", len(list)), ", ") + ")")
		w.args = append(w.args, list...)
	case "$eq", "$ne":
		if value == nil {
			if op == "$eq" {
				w.sql.WriteString(col + " is null")
			} else {
				w.sql.WriteString(col + " is not null")
			}
			return nil
		}
		cmp := " = ?"
		if op == "$ne" {
			cmp = " <> ?"
		}
		w.sql.WriteString(col + cmp)
		w.args = append(w.args, value)
	case "$eqi", "$nei":
		if value == nil {
			if op == "$eqi" {
				w.sql.WriteString(col + " is null")
			} else {
				w.sql.WriteString(col + " is not null")
			}
			return nil
		}
		cmp := " = LOWER(?)"
		if op == "$nei" {
			cmp = " <> LOWER(?)"
		}
		w.sql.WriteString(b.lower(col) + cmp)
		w.args = append(w.args, jsString(value))
	case "$gt", "$gte", "$lt", "$lte":
		sym := map[string]string{"$gt": ">", "$gte": ">=", "$lt": "<", "$lte": "<="}[op]
		w.sql.WriteString(col + " " + sym + " ?")
		w.args = append(w.args, value)
	case "$null", "$notNull":
		isNull := truthy(value)
		if op == "$notNull" {
			isNull = !isNull
		}
		if isNull {
			w.sql.WriteString(col + " is null")
		} else {
			w.sql.WriteString(col + " is not null")
		}
	case "$between":
		list, ok := value.([]any)
		if !ok || len(list) != 2 {
			return fmt.Errorf("$between expects an array of two values")
		}
		w.sql.WriteString(col + " between ? and ?")
		w.args = append(w.args, list...)
	case "$startsWith", "$endsWith", "$contains", "$notContains":
		pattern := map[string]string{"$startsWith": "%s%%", "$endsWith": "%%%s", "$contains": "%%%s%%", "$notContains": "%%%s%%"}[op]
		like := " LIKE ?"
		if op == "$notContains" {
			like = " NOT LIKE ?"
		}
		w.sql.WriteString(col + like + b.likeEscape())
		w.args = append(w.args, fmt.Sprintf(pattern, escapeLike(value)))
	case "$startsWithi", "$endsWithi", "$containsi", "$notContainsi":
		pattern := map[string]string{"$startsWithi": "%s%%", "$endsWithi": "%%%s", "$containsi": "%%%s%%", "$notContainsi": "%%%s%%"}[op]
		like := " LIKE LOWER(?)"
		if op == "$notContainsi" {
			like = " NOT LIKE LOWER(?)"
		}
		w.sql.WriteString(b.lower(col) + like + b.likeEscape())
		w.args = append(w.args, fmt.Sprintf(pattern, escapeLike(value)))
	case "$jsonSupersetOf":
		if b.sqlite() {
			return fmt.Errorf("Undefined attribute level operator %s", op) //nolint:revive,staticcheck // Strapi's message
		}
		raw, err := jsonString(value)
		if err != nil {
			return err
		}
		w.sql.WriteString(col + " @> ?")
		w.args = append(w.args, raw)
	default:
		return fmt.Errorf("Undefined attribute level operator %s", op) //nolint:revive,staticcheck // Strapi's message
	}
	return nil
}

func castArray(v any) []any {
	if list, ok := v.([]any); ok {
		return list
	}
	return []any{v}
}
