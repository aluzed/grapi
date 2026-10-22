package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/graphql"
	"github.com/aluzed/grapi/internal/jsnum"
	"github.com/aluzed/grapi/internal/plugins/email"
	"github.com/aluzed/grapi/schema"
)

// Config of the REST API.
type Config struct {
	// Prefix of the content API (api.rest.prefix), "/api" by default.
	Prefix string
	// APITokenSalt is admin.apiToken.salt, used to hash API tokens.
	APITokenSalt string
	// DefaultLimit and MaxLimit are api.rest.defaultLimit and maxLimit.
	DefaultLimit int
	MaxLimit     int
	// WithCount is api.rest.withCount (true by default).
	WithCount *bool
	Users     UsersConfig
	Upload    UploadConfig
	// Middlewares defaults to a generated project's stack.
	Middlewares *Middlewares
	// Email is the email plugin; nil when no provider is configured.
	Email *email.Service
	// ServerURL and AdminURL are server.absoluteUrl and admin.absoluteUrl.
	ServerURL, AdminURL string
	// TrustProxy is server.proxy: the client IP comes from X-Forwarded-For.
	TrustProxy bool
	// GraphQL is the graphql plugin, nil when it is not enabled.
	GraphQL *GraphQLConfig
	// OpenAPI serves the content API's document, nil when access is disabled.
	OpenAPI *OpenAPIConfig
}

// Server serves the content API of every api:: content type.
type Server struct {
	cfg     Config
	set     *schema.Set
	docs    *document.Service
	auth    *authenticator
	log     *slog.Logger
	routes  []route
	handler http.Handler
	limiter limiter
	gql     *graphql.Schema
}

type route struct {
	method  string
	path    []string
	uid     string
	action  string
	handler func(w http.ResponseWriter, r *http.Request, rc *requestContext) error
	// scope overrides uid.action; public routes skip authentication
	// (config.auth: false).
	scope  string
	public bool
	// limited routes go through the users-permissions rateLimit middleware.
	limited  bool
	policies []Policy
}

type requestContext struct {
	auth   *Auth
	params map[string]string
	query  map[string]any
	model  *schemaModel
}

// New builds the API of the content types of set.
func New(cfg Config, d *db.DB, set *schema.Set, docs *document.Service, log *slog.Logger) *Server {
	if cfg.Prefix == "" {
		cfg.Prefix = "/api"
	}
	if cfg.DefaultLimit == 0 {
		cfg.DefaultLimit = 25
	}
	if log == nil {
		log = slog.Default()
	}
	if cfg.Users.JWTExpiresIn == "" {
		cfg.Users.JWTExpiresIn = "30d"
	}
	s := &Server{cfg: cfg, set: set, docs: docs, auth: &authenticator{db: d, engine: docs.E, salt: cfg.APITokenSalt, users: cfg.Users}, log: log}
	for uid, ct := range set.ContentTypes.All() {
		if !strings.HasPrefix(uid, "api::") {
			continue
		}
		s.addContentType(uid, ct)
	}
	if cfg.Upload.Breakpoints == nil {
		s.cfg.Upload.Breakpoints = DefaultBreakpoints
	}
	if cfg.Upload.SizeLimit == 0 {
		s.cfg.Upload.SizeLimit = 1000000000
	}
	s.addUsersRoutes()
	if _, ok := set.ContentType(userUID); ok {
		s.addUsersEmailRoutes()
		limited := []string{"auth/local", "auth/local/register", "auth/forgot-password", "auth/reset-password", "auth/send-email-confirmation", "auth/change-password"}
		for i := range s.routes {
			p := strings.Join(s.routes[i].path[1:], "/")
			for _, l := range limited {
				if p == l {
					s.routes[i].limited = true
				}
			}
		}
	}
	s.addUploadRoutes()
	s.addOpenAPI()
	if err := s.addGraphQL(); err != nil {
		log.Error("graphql plugin disabled", "error", err)
	}
	mw := DefaultMiddlewares()
	if cfg.Middlewares != nil {
		mw = *cfg.Middlewares
	}
	s.handler = s.logRequests(mw.wrap(http.HandlerFunc(s.route)))
	if _, ok := set.ContentType(localeUID); ok {
		base := strings.TrimPrefix(cfg.Prefix, "/")
		s.routes = append(s.routes, route{"GET", []string{base, "i18n", "locales"}, "plugin::i18n.locales", "listLocales", s.listLocales, "", false, false, nil})
	}
	return s
}

const localeUID = "plugin::i18n.locale"

// listLocales is the i18n plugin's content API: the raw list, not wrapped
// in data, with isDefault.
func (s *Server) listLocales(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	rows, err := s.docs.E.FindMany(r.Context(), localeUID, query.Params{})
	if err != nil {
		return err
	}
	def := s.docs.DefaultLocale(r.Context())
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		m, _ := s.sanitizeOutput(row, s.schemaOf(localeUID), nil).(*schema.OrderedMap[any])
		if m == nil {
			continue
		}
		code, _ := m.Get("code")
		m.Set("isDefault", def == code)
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func splitPath(p string) []string {
	return strings.Split(strings.Trim(p, "/"), "/")
}

func (s *Server) addContentType(uid string, ct *schema.ContentType) {
	base := strings.TrimPrefix(s.cfg.Prefix, "/")
	if ct.IsSingleType() {
		p := []string{base, ct.Info.SingularName}
		s.routes = append(s.routes,
			route{"GET", p, uid, "find", s.singleFind, "", false, false, nil},
			route{"PUT", p, uid, "update", s.singleUpdate, "", false, false, nil},
			route{"DELETE", p, uid, "delete", s.singleDelete, "", false, false, nil},
		)
		return
	}
	list := []string{base, ct.Info.PluralName}
	one := []string{base, ct.Info.PluralName, ":id"}
	s.routes = append(s.routes,
		route{"GET", list, uid, "find", s.find, "", false, false, nil},
		route{"GET", one, uid, "findOne", s.findOne, "", false, false, nil},
		route{"POST", list, uid, "create", s.create, "", false, false, nil},
		route{"PUT", one, uid, "update", s.update, "", false, false, nil},
		route{"DELETE", one, uid, "delete", s.remove, "", false, false, nil},
	)
}

func matchPath(pattern, path []string) (map[string]string, bool) {
	if len(pattern) != len(path) {
		return nil, false
	}
	params := map[string]string{}
	for i, seg := range pattern {
		if strings.HasPrefix(seg, ":") {
			if path[i] == "" {
				return nil, false
			}
			params[seg[1:]] = path[i]
			continue
		}
		if seg != path[i] {
			return nil, false
		}
	}
	return params, true
}

// ServeHTTP runs the global middlewares, then the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// route routes a request, with Strapi's 404 and 405.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/_health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	path := splitPath(r.URL.Path)
	var allowed []string
	for i := range s.routes {
		rt := &s.routes[i]
		params, ok := matchPath(rt.path, path)
		if !ok {
			continue
		}
		if rt.method != r.Method {
			allowed = append(allowed, rt.method)
			continue
		}
		s.serve(w, r, rt, params)
		return
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && s.servePublic(w, r) {
		return
	}
	// Strapi serves public files on GET /*, so any other method on an
	// unknown path is a 405.
	if len(allowed) == 0 && r.Method != http.MethodGet && r.Method != http.MethodHead {
		allowed = []string{"HEAD", "GET"}
	}
	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "Method Not Allowed")
		return
	}
	writeError(w, errNotFound, s.log)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request, rt *route, params map[string]string) {
	ctx := r.Context()
	if rt.public {
		rc := &requestContext{auth: &Auth{scopes: map[string]bool{}}, params: params, query: ParseQuery(r.URL.RawQuery), model: s.schemaOf(rt.uid)}
		if err := s.runPolicies(r, rt, rc); err != nil {
			writeError(w, err, s.log)
			return
		}
		if err := rt.handler(w, r, rc); err != nil {
			writeError(w, err, s.log)
		}
		return
	}
	auth, err := s.auth.authenticate(ctx, r)
	if err != nil {
		writeError(w, err, s.log)
		return
	}
	scope := rt.uid + "." + rt.action
	if rt.scope != "" {
		scope = rt.scope
	}
	if !auth.Allowed(scope) {
		if auth.Strategy == "users-permissions" {
			writeError(w, errForbidden, s.log)
		} else {
			writeError(w, errForbidden, s.log)
		}
		return
	}
	if rt.limited && s.rateLimited(w, r) {
		return
	}
	rc := &requestContext{auth: auth, params: params, query: ParseQuery(r.URL.RawQuery), model: s.schemaOf(rt.uid)}
	if err := s.runPolicies(r, rt, rc); err != nil {
		writeError(w, err, s.log)
		return
	}
	if err := rt.handler(w, r, rc); err != nil {
		writeError(w, err, s.log)
	}
}

// sanitizedQuery is the controller's validateQuery then sanitizeQuery.
func (s *Server) sanitizedQuery(rc *requestContext) (document.Params, error) {
	if err := s.validateQuery(rc.query, rc.model, rc.auth); err != nil {
		return nil, err
	}
	q := document.Params{}
	for k, v := range rc.query {
		q[k] = v
	}
	if p, ok := q["populate"]; ok && truthy(p) {
		q["populate"] = s.sanitizePopulate(p, rc.model, rc.auth)
	}
	return q, nil
}

// fetchParams is the core service's getFetchParams: published by default.
func fetchParams(q document.Params) document.Params {
	out := document.Params{"status": "published"}
	for k, v := range q {
		out[k] = v
	}
	return out
}

func (s *Server) respond(w http.ResponseWriter, status int, data any, meta any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	writeJSON(w, status, map[string]any{"data": data, "meta": meta})
	return nil
}

func (s *Server) find(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	ctx := context.Background()
	fp := fetchParams(q)
	pag, _ := fp["pagination"].(map[string]any)
	info, err := s.paginationInfo(pag)
	if err != nil {
		return err
	}
	isPaged := isPagedPagination(pag)
	findParams := document.Params{}
	for k, v := range fp {
		findParams[k] = v
	}
	findParams["start"] = info.start
	findParams["limit"] = info.limit
	rows, err := s.docs.FindMany(ctx, rc.model.uid, findParams)
	if err != nil {
		return err
	}
	data := s.sanitizeOutput(rows, rc.model, rc.auth)
	if data == nil {
		data = []any{}
	}
	countIt, err := s.shouldCount(pag)
	if err != nil {
		return err
	}
	var total *int64
	if countIt {
		n, err := s.docs.Count(ctx, rc.model.uid, findParams)
		if err != nil {
			return err
		}
		total = &n
	}
	return s.respond(w, http.StatusOK, data, map[string]any{"pagination": paginationResponse(info, total, isPaged)})
}

func (s *Server) findOne(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	row, err := s.docs.FindOne(context.Background(), rc.model.uid, rc.params["id"], fetchParams(q))
	if err != nil {
		return err
	}
	if row == nil {
		return errNotFound
	}
	return s.respond(w, http.StatusOK, s.sanitizeOutput(row, rc.model, rc.auth), nil)
}

// readData returns the body's data object and its keys in body order, the
// order Strapi's validators report the first invalid key in.
func readData(r *http.Request) (map[string]any, []string, error) {
	missing := validationError("Missing \"data\" payload in the request body", nil)
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, nil, missing
	}
	var body map[string]json.RawMessage
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &body) != nil {
		return nil, nil, missing
	}
	var data map[string]any
	if json.Unmarshal(body["data"], &data) != nil || data == nil {
		return nil, nil, missing
	}
	return data, objectKeys(body["data"]), nil
}

func objectKeys(raw json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil
	}
	var keys []string
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return keys
		}
		k, _ := t.(string)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}

func (s *Server) writeParams(r *http.Request, rc *requestContext) (document.Params, error) {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return nil, err
	}
	data, keys, err := readData(r)
	if err != nil {
		return nil, err
	}
	if err := s.validateInput(data, keys, rc.model, rc.auth); err != nil {
		return nil, err
	}
	p := fetchParams(q)
	p["data"] = s.sanitizeInput(data, rc.model, rc.auth)
	return p, nil
}

func (s *Server) create(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	p, err := s.writeParams(r, rc)
	if err != nil {
		return err
	}
	row, err := s.docs.Create(context.Background(), rc.model.uid, p)
	if err != nil {
		return err
	}
	return s.respond(w, http.StatusCreated, s.sanitizeOutput(row, rc.model, rc.auth), nil)
}

func (s *Server) update(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	p, err := s.writeParams(r, rc)
	if err != nil {
		return err
	}
	row, err := s.docs.Update(context.Background(), rc.model.uid, rc.params["id"], p)
	if err != nil {
		return err
	}
	if row == nil {
		return errNotFound
	}
	return s.respond(w, http.StatusOK, s.sanitizeOutput(row, rc.model, rc.auth), nil)
}

func (s *Server) remove(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	if _, err := s.docs.Delete(context.Background(), rc.model.uid, rc.params["id"], fetchParams(q)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) singleDocumentID(ctx context.Context, uid string) (string, error) {
	row, err := s.docs.E.FindOne(ctx, uid, query.Params{})
	if err != nil || row == nil {
		return "", err
	}
	v, _ := row.Get(schema.AttrDocumentID)
	str, _ := v.(string)
	return str, nil
}

func (s *Server) singleFind(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	row, err := s.docs.FindFirst(context.Background(), rc.model.uid, fetchParams(q))
	if err != nil {
		return err
	}
	if row == nil {
		return errNotFound
	}
	return s.respond(w, http.StatusOK, s.sanitizeOutput(row, rc.model, rc.auth), nil)
}

func (s *Server) singleUpdate(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	p, err := s.writeParams(r, rc)
	if err != nil {
		return err
	}
	ctx := context.Background()
	id, err := s.singleDocumentID(ctx, rc.model.uid)
	if err != nil {
		return err
	}
	var row query.Row
	if id != "" {
		row, err = s.docs.Update(ctx, rc.model.uid, id, p)
	} else {
		row, err = s.docs.Create(ctx, rc.model.uid, p)
	}
	if err != nil {
		return err
	}
	return s.respond(w, http.StatusOK, s.sanitizeOutput(row, rc.model, rc.auth), nil)
}

func (s *Server) singleDelete(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	ctx := context.Background()
	id, err := s.singleDocumentID(ctx, rc.model.uid)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := s.docs.Delete(ctx, rc.model.uid, id, fetchParams(q)); err != nil {
			return err
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type paginationInfo struct {
	start, limit float64
	page         *float64
	pageSize     *float64
}

func isPagedPagination(p map[string]any) bool {
	_, hasStart := p["start"]
	_, hasLimit := p["limit"]
	_, hasPage := p["page"]
	_, hasPageSize := p["pageSize"]
	return hasPage || hasPageSize || !hasStart && !hasLimit
}

// jsMax is Math.max: NaN wins.
func jsMax(a, b float64) float64 {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.NaN()
	}
	return math.Max(a, b)
}

func isMinusOne(v any) bool {
	f, ok := v.(float64)
	return ok && f == -1
}

// paginationInfo is the core service's getPaginationInfo
// (withDefaultPagination). Query values are strings, so the strict -1
// checks never match them: limit=-1 gives a limit of 1, as in Strapi.
func (s *Server) paginationInfo(p map[string]any) (paginationInfo, error) {
	def := float64(s.cfg.DefaultLimit)
	maxLimit := float64(-1)
	if s.cfg.MaxLimit > 0 {
		maxLimit = float64(s.cfg.MaxLimit)
	}
	ensure := func(start, limit any) (float64, float64) {
		st := jsMax(jsnum.Number(start), 0)
		var l float64
		if isMinusOne(limit) {
			l = -1
		} else {
			l = jsMax(jsnum.Number(limit), 1)
		}
		if maxLimit != -1 && !(l < maxLimit) {
			l = maxLimit
		}
		return st, l
	}
	given := func(k string) bool { return p[k] != nil }
	usePage := given("page") || given("pageSize")
	useOffset := given("start") || given("limit")
	if !usePage && !useOffset {
		st, l := ensure(float64(0), def)
		return paginationInfo{start: st, limit: l}, nil
	}
	if usePage && useOffset {
		return paginationInfo{}, &document.PaginationError{Msg: "Cannot use both page & offset pagination in the same query"}
	}
	var start, limit any
	var info paginationInfo
	if useOffset {
		start, limit = float64(0), def
		if given("start") {
			start = p["start"]
		}
		if given("limit") {
			limit = p["limit"]
		}
	} else {
		var page any = float64(1)
		if given("page") {
			page = p["page"]
		}
		var ps any = def
		if given("pageSize") {
			ps = jsMax(1, jsnum.Number(p["pageSize"]))
		}
		_, pl := ensure(float64(0), ps)
		start, limit = (jsnum.Number(page)-1)*pl, pl
		pg := jsnum.Number(page)
		info.page, info.pageSize = &pg, &pl
	}
	if isMinusOne(limit) {
		limit = maxLimit
	}
	info.start, info.limit = ensure(start, limit)
	return info, nil
}

func (s *Server) shouldCount(p map[string]any) (bool, error) {
	if v, ok := p["withCount"]; ok {
		switch x := v.(type) {
		case bool:
			return x, nil
		case nil:
			return false, nil
		case string:
			switch x {
			case "true", "t", "1":
				return true, nil
			case "false", "f", "0":
				return false, nil
			}
		case float64:
			if x == 1 {
				return true, nil
			}
			if x == 0 {
				return false, nil
			}
		}
		return false, validationError(`Invalid withCount parameter. Expected "t","1","true","false","0","f"`, nil)
	}
	if s.cfg.WithCount != nil {
		return *s.cfg.WithCount, nil
	}
	return true, nil
}

func paginationResponse(pi paginationInfo, total *int64, paged bool) map[string]any {
	info := struct{ start, limit int }{int(pi.start), int(pi.limit)}
	out := map[string]any{}
	t := int64(0)
	if total != nil {
		t = *total
	}
	if paged {
		limit := info.limit
		if limit < 0 {
			limit = int(t)
		}
		page := info.start/max(limit, 1) + 1
		if limit == 0 {
			page = 1
		}
		pageCount := 0
		if limit > 0 {
			pageCount = int(math.Ceil(float64(t) / float64(limit)))
		}
		out["page"] = page
		out["pageSize"] = limit
		if total != nil {
			out["pageCount"] = pageCount
			out["total"] = t
		}
		return out
	}
	limit := info.limit
	if limit < 0 {
		limit = int(t)
	}
	out["start"] = info.start
	out["limit"] = limit
	if total != nil {
		out["total"] = t
	}
	return out
}
