package document

import (
	"fmt"
	"strings"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

var publicationModes = []string{
	"never-published",
	"has-published-version",
	"modified",
	"unmodified",
	"never-published-document",
	"has-published-version-document",
	"published-without-draft",
	"published-with-draft",
}

// ParsePublicationFilter validates a publicationFilter value; "" when absent.
func ParsePublicationFilter(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	if s, ok := v.(string); ok {
		for _, m := range publicationModes {
			if s == m {
				return s, nil
			}
		}
	}
	return "", validation("Invalid value for 'publicationFilter'. Expected one of: %s.", strings.Join(publicationModes, ", "))
}

func parseHasPublishedVersion(v any) (string, error) {
	switch v {
	case nil:
		return "", nil
	case true, "true":
		return "has-published-version-document", nil
	case false, "false":
		return "never-published-document", nil
	}
	return "", validation("Invalid value for 'hasPublishedVersion'. Expected boolean or 'true'/'false' string.")
}

// publicationCondition is buildPublicationFilterWhere: an id IN (subquery)
// condition, nil for models without draft and publish.
func (s *Service) publicationCondition(m *metadata.Model, mode, status string) (any, error) {
	ct, ok := s.Set.ContentType(schema.UID(m.UID))
	if !ok || !ct.DraftAndPublish() {
		return nil, nil
	}
	col := func(attr string) (string, error) {
		a, ok := m.Attributes.Get(attr)
		if !ok {
			return "", &appError{fmt.Sprintf("Cannot build publicationFilter SQL: attribute '%s' is missing from metadata for table '%s'.", attr, m.TableName)}
		}
		if a.ColumnName == "" {
			return "", &appError{fmt.Sprintf("Cannot build publicationFilter SQL: attribute '%s' on table '%s' has no resolved columnName.", attr, m.TableName)}
		}
		return s.E.DB.Dialect.Quote(a.ColumnName), nil
	}
	cols := map[string]string{}
	for _, attr := range []string{"id", schema.AttrDocumentID, schema.AttrPublishedAt, schema.AttrUpdatedAt} {
		c, err := col(attr)
		if err != nil {
			return nil, err
		}
		cols[attr] = c
	}
	localeCol := ""
	if _, ok := m.Attributes.Get(schema.AttrLocale); ok {
		c, err := col(schema.AttrLocale)
		if err != nil {
			return nil, err
		}
		localeCol = c
	}
	table := s.E.DB.Dialect.Quote(m.TableName)
	id, doc, pub, upd := cols["id"], cols[schema.AttrDocumentID], cols[schema.AttrPublishedAt], cols[schema.AttrUpdatedAt]
	pairOn := func(a, b string) string {
		on := a + "." + doc + " = " + b + "." + doc
		if localeCol != "" {
			on += " and (" + a + "." + localeCol + " = " + b + "." + localeCol + " or (" + a + "." + localeCol + " is null and " + b + "." + localeCol + " is null))"
		}
		return on
	}
	documentOn := func(a, b string) string { return a + "." + doc + " = " + b + "." + doc }
	empty := query.Subquery{SQL: "select " + table + "." + id + " from " + table + " where 0 = 1"}
	// rows of alias that match (or not, with exists false) a row of other.
	sub := func(alias string, aliasPublished bool, exists bool, other string, on string, extra string) query.Subquery {
		state := " is null"
		if aliasPublished {
			state = " is not null"
		}
		otherState := " is not null"
		if aliasPublished {
			otherState = " is null"
		}
		kw := "exists"
		if !exists {
			kw = "not exists"
		}
		inner := "select 1 from " + table + " as " + other + " where " + on + " and " + other + "." + pub + otherState
		if extra != "" {
			inner += " and " + extra
		}
		return query.Subquery{SQL: "select " + alias + "." + id + " from " + table + " as " + alias + " where " + alias + "." + pub + state + " and " + kw + " (" + inner + ")"}
	}
	draft := status == "draft"
	var q query.Subquery
	switch mode {
	case "never-published":
		if !draft {
			q = empty
		} else {
			q = sub("d", false, false, "p", pairOn("p", "d"), "")
		}
	case "has-published-version":
		if draft {
			q = sub("d", false, true, "p", pairOn("p", "d"), "")
		} else {
			q = sub("p", true, true, "d", pairOn("d", "p"), "")
		}
	case "modified", "unmodified":
		cmp := " > "
		if mode == "unmodified" {
			cmp = " <= "
		}
		cond := "d." + upd + cmp + "p." + upd
		if draft {
			q = sub("d", false, true, "p", pairOn("p", "d"), cond)
		} else {
			q = sub("p", true, true, "d", pairOn("d", "p"), cond)
		}
	case "never-published-document":
		if !draft {
			q = empty
		} else {
			q = sub("d", false, false, "p", documentOn("p", "d"), "")
		}
	case "has-published-version-document":
		if draft {
			q = sub("d", false, true, "p", documentOn("p", "d"), "")
		} else {
			q = sub("p", true, true, "d", documentOn("d", "p"), "")
		}
	case "published-without-draft", "published-with-draft":
		if draft {
			q = empty
		} else {
			q = sub("p", true, mode == "published-with-draft", "d", pairOn("d", "p"), "")
		}
	default:
		return nil, nil
	}
	return map[string]any{"id": map[string]any{"$in": q}}, nil
}

// withPublicationFilter wraps the query filters with the publication
// cohort of every queried model, populated ones included.
func (s *Service) withPublicationFilter(q *query.Params, p Params) error {
	mode, err := ParsePublicationFilter(p["publicationFilter"])
	if err != nil {
		return err
	}
	if mode == "" {
		if mode, err = parseHasPublishedVersion(p["hasPublishedVersion"]); err != nil {
			return err
		}
	}
	if mode == "" {
		return nil
	}
	status := "draft"
	if p["status"] == "published" {
		status = "published"
	}
	existing := q.Filters
	q.Filters = query.FilterFunc(func(m *metadata.Model) any {
		var base any = map[string]any{}
		switch f := existing.(type) {
		case query.FilterFunc:
			if r := f(m); r != nil {
				base = r
			}
		case nil:
		default:
			base = f
		}
		// The columns always exist on draft and publish models, so the
		// error of publicationCondition cannot happen here.
		cond, _ := s.publicationCondition(m, mode, status)
		if cond == nil {
			return base
		}
		conds := []any{}
		if bm, ok := base.(map[string]any); !ok || len(bm) > 0 {
			conds = append(conds, base)
		}
		return map[string]any{"$and": append(conds, cond)}
	})
	return nil
}
