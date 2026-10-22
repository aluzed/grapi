package openapi

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/aluzed/grapi/schema"
)

func writable(name string, a *schema.Attribute) bool {
	switch name {
	case "id", schema.AttrDocumentID, schema.AttrCreatedAt, schema.AttrUpdatedAt, schema.AttrCreatedBy, schema.AttrUpdatedBy:
		return false
	}
	return a.IsWritable()
}

// input is mapAttributeToInputSchema.
func (g *generator) input(name string, a *schema.Attribute) (obj, bool) {
	required := a.IsRequired()
	var s obj
	desc := ""
	uuid := obj{"type": "string", "format": "uuid", "pattern": uuidPattern}
	switch a.Type {
	case schema.TypeBoolean:
		s, desc = obj{"anyOf": []any{obj{"type": "string", "enum": []any{"0", "1", "t", "true", "f", "false"}}, obj{"type": "null"}}}, "A boolean field"
	case schema.TypeComponent:
		desc = "A component field"
		s = obj{}
		if a.IsRepeatable() {
			s = obj{"type": "array", "items": obj{}}
			if v, ok := numberOf(a.Min); ok {
				s["minItems"] = v
			}
			if v, ok := numberOf(a.Max); ok {
				s["maxItems"] = v
			}
		}
	case schema.TypeInteger:
		// Strapi describes integer inputs as float fields.
		s, desc = obj{"type": "integer", "minimum": float64(-maxSafe), "maximum": float64(maxSafe)}, "A float field"
		if l, okl := numberOf(a.Min); okl {
			if h, okh := numberOf(a.Max); okh {
				s["minimum"], s["maximum"] = l, h
			}
		}
	case schema.TypeMedia:
		desc = "A media field"
		s = obj{}
		if a.IsMultiple() {
			s = obj{"type": "array", "items": obj{}}
		}
	case schema.TypeRelation:
		desc = "A relational field"
		s = uuid
		if toMany(a) {
			s = obj{"type": "array", "items": uuid}
		}
	default:
		out, req := g.attribute(name, a)
		delete(out, "readOnly")
		return out, req
	}
	if a.Type == schema.TypeBoolean || a.Type == schema.TypeInteger {
		if v, ok := defaultOf(name, a, g.now); ok {
			s = withKey(s, "default", v)
			required = true
		}
	}
	return withKey(s, "description", desc), required
}

func (g *generator) data(ct *schema.ContentType, partial bool) obj {
	scalars, populatable := fields(&ct.Attributes)
	var props []string
	schemas := map[string]obj{}
	required := map[string]bool{}
	for _, f := range append(scalars, populatable...) {
		if !writable(f.name, f.a) {
			continue
		}
		s, req := g.input(f.name, f.a)
		props = append(props, f.name)
		schemas[f.name] = s
		required[f.name] = req && !partial
	}
	return object(props, schemas, required)
}

func root(s obj) obj { return withKey(s, "$schema", jsonSchema) }

func wrapData(s obj) obj {
	return obj{"type": "object", "properties": obj{"data": s}, "required": []any{"data"}, "additionalProperties": false}
}

func enumOf(values []any) obj { return obj{"type": "string", "enum": values} }

func queryParam(name string, s obj) obj {
	return obj{"name": name, "in": "query", "required": false, "schema": s, "x-strapi-serialize": "querystring"}
}

func (g *generator) queryParams(ct *schema.ContentType, list []string) []any {
	scalars, populatable := fields(&ct.Attributes)
	sf, pf := names(scalars), names(populatable)
	var out []any
	for _, p := range list {
		switch p {
		case "fields":
			out = append(out, queryParam(p, root(obj{"readOnly": true, "description": "The fields to return, this doesn't include populatable fields like relations, components, files, or dynamic zones", "type": "array", "items": enumOf(sf)})))
		case "filters":
			q := queryParam(p, root(obj{"type": "object", "propertyNames": enumOf(sf), "additionalProperties": obj{}, "description": "Filters to apply to the query"}))
			q["style"], q["explode"] = "deepObject", true
			out = append(out, q)
		case "_q":
			out = append(out, queryParam(p, root(obj{"type": "string"})))
		case "pagination":
			out = append(out,
				queryParam("pagination[withCount]", obj{"description": "Include total count in response", "type": "boolean"}),
				queryParam("pagination[page]", obj{"type": "integer", "exclusiveMinimum": 0, "maximum": maxSafe, "description": "Page number (1-based)"}),
				queryParam("pagination[pageSize]", obj{"type": "integer", "exclusiveMinimum": 0, "maximum": maxSafe, "description": "Number of entries per page"}),
				queryParam("pagination[start]", obj{"type": "integer", "minimum": 0, "maximum": maxSafe, "description": "Number of entries to skip"}),
				queryParam("pagination[limit]", obj{"type": "integer", "exclusiveMinimum": 0, "maximum": maxSafe, "description": "Maximum number of entries to return"}),
			)
		case "sort":
			dir := enumOf([]any{"asc", "desc"})
			record := obj{"type": "object", "propertyNames": enumOf(sf), "additionalProperties": dir}
			out = append(out, queryParam(p, root(obj{"anyOf": []any{
				enumOf(sf), obj{"type": "array", "items": enumOf(sf)}, record, obj{"type": "array", "items": record},
			}, "description": "Sort the result"})))
		case "populate":
			out = append(out, queryParam(p, root(obj{"anyOf": []any{
				obj{"readOnly": true, "description": "Populate all the first level relations, components, files, and dynamic zones for the entry", "type": "string", "const": "*"},
				obj{"readOnly": true, "description": "Populate a single relation, component, file, or dynamic zone", "type": "string", "enum": pf},
				obj{"type": "array", "items": enumOf(pf), "description": "Populate a selection of multiple relations, components, files, or dynamic zones"},
			}})))
		case "locale":
			out = append(out, queryParam(p, root(obj{"type": "string", "description": "Select a locale"})))
		case "status":
			out = append(out, queryParam(p, root(obj{"type": "string", "enum": []any{"draft", "published"}, "description": "Fetch documents based on their status. Default to \"published\" if not specified."})))
		case "publicationFilter":
			values := []any{"never-published", "has-published-version", "modified", "unmodified", "never-published-document", "has-published-version-document", "published-without-draft", "published-with-draft"}
			out = append(out, queryParam(p, root(obj{"type": "string", "enum": values, "description": "Derived publication cohort: pair-scoped (per documentId+locale), document-scoped variants (-document), or published-slice diagnostics (published-without-draft / published-with-draft)"})))
		case "hasPublishedVersion":
			out = append(out, queryParam(p, root(obj{"anyOf": []any{obj{"type": "boolean"}, enumOf([]any{"true", "false"})}, "description": "[Deprecated: prefer publicationFilter] Filter documents by whether they have a published version. Use with status=draft to find documents that have never been published"})))
		}
	}
	return out
}

var pathParam = regexp.MustCompile(`^\{(\w+)\}$`)

// operationID is the OperationIDAssembler's origin/method/path.
func operationID(origin, method, path string) string {
	id := origin + "/" + strings.ToLower(method)
	appendix := "/"
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		var p string
		if m := pathParam.FindStringSubmatch(part); m != nil {
			p = "by_" + m[1]
		} else {
			p = regexp.MustCompile(`\W`).ReplaceAllString(part, "_")
		}
		if !strings.HasSuffix(appendix, "_") && !strings.HasSuffix(appendix, "/") {
			p = "_" + p
		}
		appendix += p
	}
	return id + appendix
}

var errorResponses = obj{
	"400": obj{"description": "Bad request"}, "401": obj{"description": "Unauthorized"}, "403": obj{"description": "Forbidden"},
	"404": obj{"description": "Not found"}, "500": obj{"description": "Internal server error"},
}

func responses(data obj) obj {
	out := obj{}
	for k, v := range errorResponses {
		out[k] = v
	}
	out["200"] = obj{"description": "OK", "content": obj{"application/json": obj{"schema": root(wrapData(data))}}}
	return out
}

func (g *generator) contentTypePaths(paths obj, ct *schema.ContentType) {
	api := ct.UID.Origin()
	var conditional []string
	if ct.Localized() {
		conditional = append(conditional, "locale")
	}
	if ct.DraftAndPublish() {
		conditional = append(conditional, "status", "publicationFilter", "hasPublishedVersion")
	}
	params := func(base ...string) []string { return append(base, conditional...) }
	doc := g.document(ct)
	idParam := obj{"name": "id", "in": "path", "required": true, "schema": root(documentIDSchema())}
	op := func(method, path string, query []string, withID bool, body obj, data obj) obj {
		ps := []any{}
		if withID {
			ps = append(ps, idParam)
		}
		ps = append(ps, g.queryParams(ct, query)...)
		o := obj{"operationId": operationID(api, method, path), "tags": []any{api}, "parameters": ps, "responses": responses(data)}
		if body != nil {
			o["requestBody"] = obj{"content": obj{"application/json": obj{"schema": root(wrapData(body))}}}
		}
		return o
	}
	add := func(path, method string, o obj) {
		if paths[path] == nil {
			paths[path] = obj{}
		}
		paths[path].(obj)[strings.ToLower(method)] = o
	}
	list := obj{"type": "array", "items": doc}
	if ct.IsSingleType() {
		p := "/" + ct.Info.SingularName
		add(p, "GET", op("GET", p, params("fields", "populate", "filters"), false, nil, doc))
		add(p, "PUT", op("PUT", p, params("fields", "populate"), false, g.data(ct, true), doc))
		add(p, "DELETE", op("DELETE", p, params("fields", "populate"), false, nil, doc))
		return
	}
	p := "/" + ct.Info.PluralName
	one := p + "/{id}"
	add(p, "GET", op("GET", p, params("fields", "filters", "_q", "pagination", "sort", "populate"), false, nil, list))
	add(p, "POST", op("POST", p, params("fields", "populate"), false, g.data(ct, false), doc))
	add(one, "GET", op("GET", one, params("fields", "populate", "filters", "sort"), true, nil, doc))
	add(one, "PUT", op("PUT", one, params("fields", "populate"), true, g.data(ct, true), doc))
	add(one, "DELETE", op("DELETE", one, params("fields", "populate", "filters"), true, nil, doc))
}

// JSON is the document as JSON.
func JSON(doc map[string]any) ([]byte, error) { return json.Marshal(doc) }
