// Package query holds the params of the Query Engine (Strapi's
// strapi.db.query), the layer under the Document Service: rows by id, no
// draft and publish or locale logic, lifecycles run.
package query

// Params of a query, as Strapi's query builder takes them. Where, Filters
// and OrderBy are decoded JSON (objects, arrays and scalars).
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

// Data is a row to write, by attribute name.
type Data = map[string]any
