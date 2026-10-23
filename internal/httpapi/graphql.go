package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/graphql"
	"github.com/aluzed/grapi/schema"
)

// GraphQLConfig is the graphql plugin's configuration.
type GraphQLConfig struct {
	Endpoint string
	// Introspection is allowed outside production (Apollo's default).
	Introspection bool
	// DefaultLimit and MaxLimit are the plugin's defaultLimit and maxLimit.
	DefaultLimit int
	MaxLimit     int
	// Options hold the shadow CRUD, the extensions and depthLimit.
	Options graphql.Options
}

func (s *Server) addGraphQL() error {
	if s.cfg.GraphQL == nil {
		return nil
	}
	sch, err := graphql.Build(s.set, s.cfg.GraphQL.Options)
	if err != nil {
		return err
	}
	s.gql = sch
	var unbounded []string
	if s.cfg.GraphQL.Options.DepthLimit <= 0 {
		unbounded = append(unbounded, "depthLimit")
	}
	if s.cfg.GraphQL.MaxLimit <= 0 {
		unbounded = append(unbounded, "maxLimit")
	}
	if len(unbounded) > 0 {
		s.log.Warn("Built-in GraphQL operation limits are unbounded or invalid for: " + strings.Join(unbounded, ", ") +
			". Configure these limits (for example: defaultLimit: 25, maxLimit: 100, depthLimit: 10). Custom Apollo validation rules may independently enforce limits. See https://docs.strapi.io/cms/configurations/plugins.")
	}
	if s.cfg.GraphQL.Endpoint == "" {
		s.cfg.GraphQL.Endpoint = "/graphql"
	}
	path := splitPath(s.cfg.GraphQL.Endpoint)
	for _, m := range []string{"GET", "POST"} {
		s.routes = append(s.routes, route{m, path, "plugin::graphql", "graphql", s.graphqlHandler, "", true, false, nil})
	}
	return nil
}

type gqlBody struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
}

func (s *Server) graphqlHandler(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	auth, err := s.auth.authenticate(r.Context(), r)
	if err != nil {
		return err
	}
	r = r.WithContext(document.WithOrigin(r.Context(), auth.origin()))
	if csrfBlocked(r) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []any{map[string]any{
			"message":    "This operation has been blocked as a potential Cross-Site Request Forgery (CSRF). Please either specify a 'content-type' header (with a type that is not one of application/x-www-form-urlencoded, multipart/form-data, text/plain) or provide a non-empty value for one of the following headers: x-apollo-operation-name, apollo-require-preflight\n",
			"extensions": map[string]any{"code": "BAD_REQUEST"},
		}}})
		return nil
	}
	var body gqlBody
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		body.Query, body.OperationName = q.Get("query"), q.Get("operationName")
		if v := q.Get("variables"); v != "" {
			_ = json.Unmarshal([]byte(v), &body.Variables)
		}
	} else {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyRead))
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []any{map[string]any{"message": "POST body sent invalid JSON.", "extensions": map[string]any{"code": "BAD_REQUEST"}}}})
			return nil
		}
	}
	if strings.TrimSpace(body.Query) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []any{map[string]any{
			"message": "GraphQL operations must contain a non-empty `query` or a `persistedQuery` extension.", "extensions": map[string]any{"code": "BAD_REQUEST"},
		}}})
		return nil
	}
	res := s.gql.Execute(r.Context(), graphql.Request{
		Query: body.Query, OperationName: body.OperationName, Variables: body.Variables, Introspection: s.cfg.GraphQL.Introspection,
	}, &gqlResolver{s: s, auth: auth, r: r})
	out := map[string]any{}
	if res.Status == http.StatusOK {
		out["data"] = res.Data
	}
	var errs []any
	for _, e := range res.Errors {
		errs = append(errs, s.formatGraphQLError(e, res))
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	writeJSON(w, res.Status, out)
	return nil
}

// csrfBlocked is Apollo's csrfPrevention: a request a browser could send
// without a preflight needs a header that forces one.
func csrfBlocked(r *http.Request) bool {
	if r.Header.Get("X-Apollo-Operation-Name") != "" || r.Header.Get("Apollo-Require-Preflight") != "" {
		return false
	}
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return true
	}
	mt, _, _ := strings.Cut(strings.ToLower(ct), ";")
	switch strings.TrimSpace(mt) {
	case "application/x-www-form-urlencoded", "multipart/form-data", "text/plain":
		return true
	}
	return false
}

// formatGraphQLError is the plugin's formatGraphqlError on Apollo's format.
func (s *Server) formatGraphQLError(e *graphql.Error, res *graphql.Result) map[string]any {
	out := map[string]any{"message": e.Message}
	if len(e.Locations) > 0 {
		var locs []any
		for _, l := range e.Locations {
			locs = append(locs, map[string]any{"line": l.Line, "column": l.Column})
		}
		out["locations"] = locs
	}
	if e.Path != nil {
		out["path"] = e.Path
	}
	ext := map[string]any{}
	for k, v := range res.Extensions {
		ext[k] = v
	}
	out["extensions"] = ext
	if e.Err == nil {
		ext["code"] = res.Code
		if res.Code == "" {
			ext["code"] = "INTERNAL_SERVER_ERROR"
		}
		return out
	}
	api := toAPIError(e.Err)
	if api == nil {
		s.log.Error("graphql resolver failed", "error", e.Err)
		out["message"] = "Internal Server Error"
		ext["code"] = "INTERNAL_SERVER_ERROR"
		ext["error"] = map[string]any{"name": "InternalServerError", "message": "Internal Server Error"}
		return out
	}
	delete(out, "locations")
	details := api.Details
	if details == nil {
		details = map[string]any{}
	}
	out["message"] = api.Message
	ext["error"] = map[string]any{"name": api.Name, "message": api.Message, "details": details}
	switch api.Name {
	case "ForbiddenError", "UnauthorizedError", "PolicyError":
		ext["code"] = "FORBIDDEN"
	case "ValidationError":
		ext["code"] = "BAD_USER_INPUT"
	default:
		ext["code"] = "STRAPI_" + strings.ToUpper(schema.SnakeCase(api.Name))
	}
	return out
}

type gqlResolver struct {
	s    *Server
	auth *Auth
	r    *http.Request
}

var errGQLForbidden = newError(http.StatusForbidden, "ForbiddenError", "Forbidden access")

// authorize is the plugin's wrap-resolvers: root fields always go through
// auth.verify, other fields only when they have a scope.
func (g *gqlResolver) authorize(fc *graphql.FieldContext) error {
	f := fc.Field
	root := fc.Type.Name == "Query" || fc.Type.Name == "Mutation"
	if f.NoAuth || !root && f.Scope == "" {
		return nil
	}
	if f.Scope == "" {
		if g.auth.User == nil && g.auth.TokenType == "" {
			return errGQLForbidden
		}
		return nil
	}
	if !g.auth.Allowed(f.Scope) {
		return errGQLForbidden
	}
	return nil
}

var errPolicy = newError(http.StatusForbidden, "PolicyError", "Policy Failed")

func (g *gqlResolver) policies(ctx context.Context, fc *graphql.FieldContext) error {
	for _, p := range fc.Field.Policies {
		ok, err := p(ctx, fc)
		if err != nil {
			return err
		}
		if !ok {
			return errPolicy
		}
	}
	return nil
}

func (g *gqlResolver) TypeOf(union *graphql.Def, v any) string {
	if union.Name == "GenericMorph" || strings.HasSuffix(union.Name, "Morph") {
		uid, _ := valueAt(v, "__type").(string)
		return g.s.gql.TypeName(schema.UID(uid))
	}
	uid, _ := valueAt(v, "__component").(string)
	if uid == "" {
		return "Error"
	}
	return g.s.gql.TypeName(schema.UID(uid))
}

func valueAt(v any, k string) any {
	switch x := v.(type) {
	case map[string]any:
		return x[k]
	case query.Row:
		val, _ := x.Get(k)
		return val
	}
	return nil
}

// jsonValue gives the values the JSON decoding of a REST body gives.
func jsonValue(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// gqlPagination is withDefaultPagination.
func (g *gqlResolver) gqlPagination(p map[string]any) (start, limit int, err error) {
	cfg := g.s.cfg.GraphQL
	maxLimit := cfg.MaxLimit
	if maxLimit == 0 {
		maxLimit = -1
	}
	def := 10
	if cfg.DefaultLimit > 0 {
		def = cfg.DefaultLimit
	}
	num := func(k string) (int, bool) {
		v, ok := p[k]
		if !ok || v == nil {
			return 0, false
		}
		f, _ := toFloat64(v)
		return int(f), true
	}
	withMax := func(l int) int {
		if maxLimit == -1 || l < maxLimit {
			return l
		}
		return maxLimit
	}
	valid := func(s, l int) (int, int) {
		s = max(s, 0)
		if l != -1 {
			l = max(l, 1)
		}
		return s, withMax(l)
	}
	page, hasPage := num("page")
	pageSize, hasPageSize := num("pageSize")
	st, hasStart := num("start")
	lim, hasLimit := num("limit")
	paged, offset := hasPage || hasPageSize, hasStart || hasLimit
	if !paged && !offset {
		start, limit = valid(0, def)
		return start, limit, nil
	}
	if paged && offset {
		return 0, 0, &APIError{Status: 400, Name: "PaginationError", Message: "Cannot use both page & offset pagination in the same query"}
	}
	if offset {
		if !hasLimit {
			lim = def
		}
		start, limit = st, lim
	} else {
		if !hasPage {
			page = 1
		}
		if !hasPageSize {
			pageSize = def
		}
		_, pl := valid(0, max(1, pageSize))
		start, limit = (page-1)*pl, pl
	}
	if limit == -1 {
		limit = maxLimit
	}
	start, limit = valid(start, limit)
	return start, limit, nil
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// gqlFilters is graphQLFiltersToStrapiQuery.
func (g *gqlResolver) gqlFilters(v any, uid schema.UID) any {
	if v == nil {
		return map[string]any{}
	}
	if list, ok := v.([]any); ok {
		out := make([]any, len(list))
		for i, x := range list {
			out[i] = g.gqlFilters(x, uid)
		}
		return out
	}
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	attrs := g.s.attrsOf(uid)
	out := map[string]any{}
	for k, val := range m {
		if k == "id" || k == schema.AttrDocumentID {
			out[k] = replaceOperators(val)
			continue
		}
		if a, ok := attrs.Get(k); ok {
			switch {
			case a.IsScalar():
				out[k] = replaceOperators(val)
			case a.Type == schema.TypeRelation || a.Type == schema.TypeMedia:
				out[k] = g.gqlFilters(val, schema.UID(a.Target))
			case a.Type == schema.TypeComponent:
				out[k] = g.gqlFilters(val, schema.UID(a.Component))
			}
			continue
		}
		if k == "and" || k == "or" || k == "not" {
			out["$"+k] = g.gqlFilters(val, uid)
		}
	}
	return out
}

func replaceOperators(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = replaceOperators(e)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if op, ok := graphql.FilterOperators[k]; ok {
				k = op
			}
			out[k] = replaceOperators(e)
		}
		return out
	}
	return v
}

func (s *Server) attrsOf(uid schema.UID) *schema.OrderedMap[*schema.Attribute] {
	if ct, ok := s.set.ContentType(uid); ok {
		return &ct.Attributes
	}
	if c, ok := s.set.Component(uid); ok {
		return &c.Attributes
	}
	return &schema.OrderedMap[*schema.Attribute]{}
}

// transformArgs is the plugin's transformArgs: pagination flattened, filters
// mapped, an empty sort dropped. It returns REST-like params.
func (g *gqlResolver) transformArgs(args map[string]any, uid schema.UID, paginate bool) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range args {
		if k != "pagination" && k != "filters" {
			out[k] = v
		}
	}
	if paginate {
		p, _ := args["pagination"].(map[string]any)
		start, limit, err := g.gqlPagination(p)
		if err != nil {
			return nil, err
		}
		out["start"], out["limit"] = float64(start), float64(limit)
	}
	if f, ok := args["filters"]; ok && f != nil {
		out["filters"] = jsonValue(g.gqlFilters(f, uid))
	}
	if sort, ok := out["sort"].([]any); ok && len(sort) == 0 {
		delete(out, "sort")
	} else if out["sort"] == nil {
		delete(out, "sort")
	}
	return out, nil
}

// documentParams is the queries' mergeDocumentListParams.
func documentParams(q map[string]any) document.Params {
	p := document.Params{}
	for k, v := range q {
		switch k {
		case "documentId", "hasPublishedVersion", "publicationFilter":
			continue
		}
		p[k] = v
	}
	if p["status"] == nil {
		p["status"] = "published"
	}
	if v, ok := q["publicationFilter"]; ok && v != nil {
		p["publicationFilter"] = v
	} else if v, ok := q["hasPublishedVersion"].(bool); ok {
		p["publicationFilter"] = map[bool]string{true: "has-published-version-document", false: "never-published-document"}[v]
	}
	if p["locale"] == nil {
		delete(p, "locale")
	}
	return p
}

type entityCollection struct {
	nodes []query.Row
	uid   schema.UID
	args  map[string]any
}

func (c *entityCollection) Get(k string) (any, bool) {
	if k == "nodes" {
		return rowsAsAny(c.nodes), true
	}
	return nil, false
}

func rowsAsAny(rows []query.Row) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = r
	}
	return out
}

func (g *gqlResolver) Resolve(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	if err := g.authorize(fc); err != nil {
		return nil, err
	}
	// wrap-resolvers: the middlewares, then the policies, then the resolver.
	mws := fc.Field.Middlewares
	var run func(i int) (any, error)
	run = func(i int) (any, error) {
		if i < len(mws) {
			return mws[i](ctx, fc, func() (any, error) { return run(i + 1) })
		}
		if err := g.policies(ctx, fc); err != nil {
			return nil, err
		}
		return g.resolve(ctx, fc)
	}
	return run(0)
}

func (g *gqlResolver) resolve(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	f := fc.Field
	switch f.Kind {
	case graphql.FieldExtension:
		return f.Resolve(ctx, fc)
	case graphql.FieldFind, graphql.FieldFindConnection, graphql.FieldFindOne, graphql.FieldSingleFind:
		return g.find(ctx, fc)
	case graphql.FieldNodes:
		if c, ok := fc.Parent.(*entityCollection); ok {
			return rowsAsAny(c.nodes), nil
		}
		return valueAt(fc.Parent, "nodes"), nil
	case graphql.FieldPageInfo:
		return g.pageInfo(ctx, fc.Parent.(*entityCollection))
	case graphql.FieldEntityData:
		return valueAt(fc.Parent, "data"), nil
	case graphql.FieldRelation, graphql.FieldRelationList, graphql.FieldRelationConnection, graphql.FieldMorph:
		return g.association(ctx, fc)
	case graphql.FieldComponent:
		return g.component(ctx, fc)
	case graphql.FieldDynamicZone:
		return g.load(ctx, fc.Field.UID, fc.Parent, fc.Field.Attr, nil)
	case graphql.FieldCreate, graphql.FieldUpdate, graphql.FieldDelete, graphql.FieldSingleUpdate, graphql.FieldSingleDelete:
		return g.mutate(ctx, fc)
	case graphql.FieldCustom:
		return g.custom(ctx, fc)
	}
	return valueAt(fc.Parent, f.Name), nil
}

func (g *gqlResolver) find(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	f := fc.Field
	model := g.s.schemaOf(string(f.UID))
	list := f.Kind == graphql.FieldFind || f.Kind == graphql.FieldFindConnection
	args, err := g.transformArgs(fc.Args, f.UID, list)
	if err != nil {
		return nil, err
	}
	if err := g.s.validateQuery(args, model, g.auth); err != nil {
		return nil, err
	}
	p := documentParams(args)
	switch f.Kind {
	case graphql.FieldFindOne:
		id, _ := fc.Args["documentId"].(string)
		row, err := g.s.docs.FindOne(ctx, string(f.UID), id, p)
		if err != nil || row == nil {
			return nil, err
		}
		return row, nil
	case graphql.FieldSingleFind:
		row, err := g.s.docs.FindFirst(ctx, string(f.UID), p)
		if err != nil || row == nil {
			return nil, err
		}
		return row, nil
	}
	rows, err := g.s.docs.FindMany(ctx, string(f.UID), p)
	if err != nil {
		return nil, err
	}
	if f.Kind == graphql.FieldFind {
		return rowsAsAny(rows), nil
	}
	return &entityCollection{nodes: rows, uid: f.UID, args: args}, nil
}

func (g *gqlResolver) pageInfo(ctx context.Context, c *entityCollection) (any, error) {
	start, _ := toFloat64(c.args["start"])
	limit, _ := toFloat64(c.args["limit"])
	safe := math.Max(limit, 1)
	p := documentParams(c.args)
	delete(p, "start")
	delete(p, "limit")
	delete(p, "sort")
	total, err := g.s.docs.Count(ctx, string(c.uid), p)
	if err != nil {
		return nil, err
	}
	t := float64(total)
	info := map[string]any{"total": t}
	if limit == -1 {
		info["pageSize"], info["pageCount"], info["page"] = t-start, safe, safe
	} else {
		info["pageSize"], info["pageCount"], info["page"] = safe, math.Ceil(t/safe), math.Floor(start/safe)+1
	}
	return info, nil
}

// load is db.query(uid).load(parent, attr, params).
func (g *gqlResolver) load(ctx context.Context, uid schema.UID, parent any, attr string, params map[string]any) (any, error) {
	id := valueAt(parent, "id")
	if id == nil {
		return nil, nil
	}
	out, err := g.s.docs.E.Load(ctx, string(uid), id, []string{attr}, params)
	if err != nil || out == nil {
		return nil, err
	}
	return out[attr], nil
}

func loadParams(q query.Params) map[string]any {
	p := map[string]any{}
	if q.Where != nil {
		p["where"] = q.Where
	}
	if q.Filters != nil {
		p["filters"] = q.Filters
	}
	if q.OrderBy != nil {
		p["orderBy"] = q.OrderBy
	}
	if q.Limit != nil {
		p["limit"] = *q.Limit
	}
	if q.Offset != nil {
		p["offset"] = *q.Offset
	}
	if q.Select != nil {
		sel := make([]any, len(q.Select))
		for i, s := range q.Select {
			sel[i] = s
		}
		p["select"] = sel
	}
	return p
}

func isRootQuery(f *graphql.Field) bool {
	if f == nil {
		return false
	}
	switch f.Kind {
	case graphql.FieldFind, graphql.FieldFindConnection, graphql.FieldFindOne, graphql.FieldSingleFind:
		return true
	}
	return false
}

func (g *gqlResolver) association(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	f := fc.Field
	target := f.Target
	if f.Kind == graphql.FieldMorph {
		v, err := g.load(ctx, f.UID, fc.Parent, f.Attr, nil)
		if err != nil {
			return nil, err
		}
		return g.s.sanitizePolymorphic(v, g.auth, false), nil
	}
	args, err := g.transformArgs(fc.Args, target, true)
	if err != nil {
		return nil, err
	}
	model := g.s.schemaOf(string(target))
	if err := g.s.validateQuery(args, model, g.auth); err != nil {
		return nil, err
	}
	tq, err := g.s.docs.TransformQuery(string(target), document.Params(args))
	if err != nil {
		return nil, err
	}
	params := loadParams(tq)
	status, _ := fc.Args["status"].(string)
	if status == "" && isRootQuery(fc.Root) {
		status, _ = fc.RootArgs["status"].(string)
		if status == "" {
			status = "published"
		}
	}
	if ct, ok := g.s.set.ContentType(target); ok && ct.DraftAndPublish() {
		if err := g.inheritPublicationFilter(fc, string(target), status, params); err != nil {
			return nil, err
		}
	}
	if ct, ok := g.s.set.ContentType(target); ok && ct.DraftAndPublish() && status != "" {
		cond := map[string]any{"$notNull": true}
		if status != "published" {
			cond = map[string]any{"$null": true}
		}
		where := map[string]any{"publishedAt": cond}
		if w, ok := params["where"].(map[string]any); ok {
			for k, v := range w {
				where[k] = v
			}
		} else if params["where"] != nil {
			where = map[string]any{"$and": []any{where, params["where"]}}
		}
		params["where"] = where
	}
	v, err := g.load(ctx, f.UID, fc.Parent, f.Attr, params)
	if err != nil {
		return nil, err
	}
	switch f.Kind {
	case graphql.FieldRelationConnection:
		return map[string]any{"nodes": nonNilList(v)}, nil
	case graphql.FieldRelationList:
		return nonNilList(v), nil
	}
	return v, nil
}

// inheritPublicationFilter is the association resolver's: the
// publicationFilter (or hasPublishedVersion) of a built-in root query
// applies to its relations.
func (g *gqlResolver) inheritPublicationFilter(fc *graphql.FieldContext, target, status string, params map[string]any) error {
	if !isRootQuery(fc.Root) {
		return nil
	}
	mode, _ := fc.RootArgs["publicationFilter"].(string)
	if mode == "" {
		if v, ok := fc.RootArgs["hasPublishedVersion"].(bool); ok {
			mode = map[bool]string{true: "has-published-version-document", false: "never-published-document"}[v]
		}
	}
	if mode == "" {
		return nil
	}
	if status != "published" {
		status = "draft"
	}
	cond, err := g.s.docs.PublicationWhere(target, mode, status)
	if err != nil || cond == nil {
		return err
	}
	if w, ok := params["where"].(map[string]any); ok {
		merged := map[string]any{}
		for k, v := range cond.(map[string]any) {
			merged[k] = v
		}
		for k, v := range w {
			merged[k] = v
		}
		params["where"] = merged
	} else if params["where"] != nil {
		params["where"] = map[string]any{"$and": []any{cond, params["where"]}}
	} else {
		params["where"] = cond
	}
	return nil
}

func nonNilList(v any) any {
	if v == nil {
		return []any{}
	}
	return v
}

func (g *gqlResolver) component(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	f := fc.Field
	args, err := g.transformArgs(fc.Args, f.Target, true)
	if err != nil {
		return nil, err
	}
	if err := g.s.validateQuery(args, g.s.schemaOf(string(f.Target)), g.auth); err != nil {
		return nil, err
	}
	tq, err := g.s.docs.TransformQuery(string(f.Target), document.Params(args))
	if err != nil {
		return nil, err
	}
	return g.load(ctx, f.UID, fc.Parent, f.Attr, loadParams(tq))
}

// inputData converts the dynamic zone inputs (__typename) to __component.
func (g *gqlResolver) inputData(uid schema.UID, data map[string]any) map[string]any {
	attrs := g.s.attrsOf(uid)
	for k, v := range data {
		a, ok := attrs.Get(k)
		if !ok || a.Type != schema.TypeDynamicZone {
			continue
		}
		list, _ := v.([]any)
		for i, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typename, _ := m["__typename"].(string)
			comp, _ := g.s.gql.ComponentOf(typename)
			out := map[string]any{"__component": string(comp)}
			for mk, mv := range m {
				if mk != "__typename" {
					out[mk] = mv
				}
			}
			list[i] = out
		}
	}
	return data
}

func (g *gqlResolver) mutate(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	f := fc.Field
	uid := string(f.UID)
	model := g.s.schemaOf(uid)
	p := document.Params{}
	for _, k := range []string{"status", "locale"} {
		if v, ok := fc.Args[k]; ok && v != nil {
			p[k] = v
		}
	}
	if d, ok := fc.Args["data"].(map[string]any); ok {
		data, _ := jsonValue(g.inputData(f.UID, d)).(map[string]any)
		p["data"] = g.s.sanitizeInput(data, model, g.auth)
	}
	id, _ := fc.Args["documentId"].(string)
	switch f.Kind {
	case graphql.FieldCreate:
		return nilRow(g.s.docs.Create(ctx, uid, p))
	case graphql.FieldUpdate:
		p["documentId"] = id
		return nilRow(g.s.docs.Update(ctx, uid, id, p))
	case graphql.FieldDelete:
		if _, err := g.s.docs.Delete(ctx, uid, id, document.Params{}); err != nil {
			return nil, err
		}
		return map[string]any{"documentId": id}, nil
	}
	existing, err := g.s.docs.E.FindOne(ctx, uid, query.Params{})
	if err != nil {
		return nil, err
	}
	if f.Kind == graphql.FieldSingleDelete {
		if existing == nil {
			return nil, newError(http.StatusNotFound, "NotFoundError", "Document not found")
		}
		docID, _ := existing.Get(schema.AttrDocumentID)
		ds, _ := docID.(string)
		if _, err := g.s.docs.Delete(ctx, uid, ds, p); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if existing != nil {
		docID, _ := existing.Get(schema.AttrDocumentID)
		ds, _ := docID.(string)
		return nilRow(g.s.docs.Update(ctx, uid, ds, p))
	}
	return nilRow(g.s.docs.Create(ctx, uid, p))
}

func nilRow(row query.Row, err error) (any, error) {
	if err != nil || row == nil {
		return nil, err
	}
	return row, nil
}

// rest runs a REST handler of the same plugin and decodes its answer.
// With limited, the call goes through the rateLimit middleware of that
// route first, as the plugin's createRateLimitRunner does.
func (g *gqlResolver) rest(h func(http.ResponseWriter, *http.Request, *requestContext) error, method string, body any, params map[string]string, limited string, uid string) (any, error) {
	b, _ := json.Marshal(body)
	target := "/"
	if limited != "" {
		target = "/" + strings.Trim(g.s.cfg.Prefix, "/") + limited
	}
	r := httptest.NewRequestWithContext(g.r.Context(), method, target, bytes.NewReader(b))
	r.RemoteAddr = g.r.RemoteAddr
	r.Header.Set("Content-Type", "application/json")
	for _, k := range []string{"Authorization", "X-Forwarded-For"} {
		if v := g.r.Header.Get(k); v != "" {
			r.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	if limited != "" && g.s.rateLimited(rec, r) {
		return nil, errRateLimit
	}
	rc := &requestContext{auth: g.auth, params: params, query: ParseQuery(r.URL.RawQuery), model: g.s.schemaOf(uid)}
	if err := h(rec, r, rc); err != nil {
		return nil, err
	}
	var out any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 400 {
		e, _ := valueAt(out, "error").(map[string]any)
		api := &APIError{Status: rec.Code, Name: "ApplicationError", Message: "Internal Server Error"}
		if e != nil {
			api.Name, _ = e["name"].(string)
			api.Message, _ = e["message"].(string)
			api.Details = e["details"]
		}
		return nil, api
	}
	return out, nil
}

func (g *gqlResolver) custom(ctx context.Context, fc *graphql.FieldContext) (any, error) {
	a := fc.Args
	input, _ := a["input"].(map[string]any)
	id, _ := a["id"].(string)
	switch fc.Field.Name {
	case "me":
		if g.auth.User == nil {
			return nil, errors.New("Authentication requested") //nolint:staticcheck // the plugin's message
		}
		uid, _ := g.auth.User.Get("id")
		return g.s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"id": uid}, Populate: map[string]any{"role": true}})
	case "login":
		body := map[string]any{"identifier": input["identifier"], "password": input["password"]}
		return g.rest(g.s.authCallback, "POST", body, map[string]string{"provider": toString(input["provider"])}, "/auth/local", userUID)
	case "register":
		return g.rest(g.s.authRegister, "POST", input, nil, "/auth/local/register", userUID)
	case "forgotPassword":
		return g.rest(g.s.authForgotPassword, "POST", map[string]any{"email": a["email"]}, nil, "/auth/forgot-password", userUID)
	case "resetPassword":
		return g.rest(g.s.authResetPassword, "POST", map[string]any{"password": a["password"], "passwordConfirmation": a["passwordConfirmation"], "code": a["code"]}, nil, "/auth/reset-password", userUID)
	case "changePassword":
		return g.rest(g.s.authChangePassword, "POST", map[string]any{"currentPassword": a["currentPassword"], "password": a["password"], "passwordConfirmation": a["passwordConfirmation"]}, nil, "/auth/change-password", userUID)
	case "createUsersPermissionsUser", "updateUsersPermissionsUser", "deleteUsersPermissionsUser":
		h, method := g.s.usersCreate, "POST"
		switch fc.Field.Name {
		case "updateUsersPermissionsUser":
			h, method = g.s.usersUpdate, "PUT"
		case "deleteUsersPermissionsUser":
			h, method = g.s.usersDestroy, "DELETE"
		}
		user, err := g.rest(h, method, a["data"], map[string]string{"id": id}, "", userUID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"data": user}, nil
	case "emailConfirmation":
		h := func(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
			rc.query = map[string]any{"confirmation": a["confirmation"]}
			return g.s.emailConfirmation(w, r, rc, true)
		}
		return g.rest(h, "GET", nil, nil, "", userUID)
	case "createUsersPermissionsRole":
		return g.rest(g.s.roleCreate, "POST", a["data"], nil, "", roleUID)
	case "updateUsersPermissionsRole":
		return g.rest(g.s.roleUpdate, "PUT", a["data"], map[string]string{"role": id}, "", roleUID)
	case "deleteUsersPermissionsRole":
		return g.rest(g.s.roleDelete, "DELETE", nil, map[string]string{"role": id}, "", roleUID)
	case "updateUploadFile":
		info, _ := a["info"].(map[string]any)
		row, err := g.s.applyFileInfo(ctx, id, info)
		if errors.Is(err, errNotFound) {
			return nil, newError(http.StatusNotFound, "NotFoundError", "Entity not found")
		}
		return nilRow(row, err)
	case "deleteUploadFile":
		return g.rest(g.s.uploadDestroy, "DELETE", nil, map[string]string{"id": id}, "", fileUID)
	}
	return nil, newError(http.StatusNotImplemented, "NotImplementedError", fc.Field.Name+" is not implemented by grapi yet")
}
