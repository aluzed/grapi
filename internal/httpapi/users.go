package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aluzed/grapi/internal/auth/users"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

const (
	userUID = "plugin::users-permissions.user"
	roleUID = "plugin::users-permissions.role"
)

// UsersConfig is the users-permissions plugin config.
type UsersConfig struct {
	JWTSecret    string
	JWTExpiresIn string
	JWTAlgorithm string
	// AllowedFields are register.allowedFields.
	AllowedFields []string
}

func (s *Server) addUsersRoutes() {
	if _, ok := s.set.ContentType(userUID); !ok {
		return
	}
	base := strings.TrimPrefix(s.cfg.Prefix, "/")
	auth := "plugin::users-permissions.auth"
	s.routes = append(s.routes,
		route{"POST", []string{base, "auth", "local"}, auth, "callback", s.authCallback, "", false},
		route{"POST", []string{base, "auth", "local", "register"}, auth, "register", s.authRegister, "", false},
		route{"POST", []string{base, "auth", "change-password"}, auth, "changePassword", s.authChangePassword, "", false},
		route{"GET", []string{base, "users", "count"}, userUID, "count", s.usersCount, "", false},
		route{"GET", []string{base, "users"}, userUID, "find", s.usersFind, "", false},
		route{"GET", []string{base, "users", "me"}, userUID, "me", s.usersMe, "", false},
		route{"GET", []string{base, "users", ":id"}, userUID, "findOne", s.usersFindOne, "", false},
		route{"POST", []string{base, "users"}, userUID, "create", s.usersCreate, "", false},
		route{"PUT", []string{base, "users", ":id"}, userUID, "update", s.usersUpdate, "", false},
		route{"DELETE", []string{base, "users", ":id"}, userUID, "destroy", s.usersDestroy, "", false},
	)
}

// readBody is the raw JSON body (users-permissions routes take no data
// wrapper) and its keys in body order.
func readBody(r *http.Request) (map[string]any, []string, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, nil, err
	}
	body := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return body, nil, nil
	}
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return map[string]any{}, nil, nil
	}
	return body, objectKeys(raw), nil
}

func (s *Server) issueJWT(id any) (string, error) {
	return users.Sign([]users.KV{{Key: "id", Value: id}}, s.cfg.Users.JWTSecret, s.cfg.Users.JWTAlgorithm, s.cfg.Users.JWTExpiresIn, time.Now())
}

func (s *Server) sendAuth(w http.ResponseWriter, user query.Row, auth *Auth) error {
	id, _ := user.Get("id")
	jwt, err := s.issueJWT(id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"jwt": jwt, "user": s.sanitizeOutput(user, s.schemaOf(userUID), auth)})
	return nil
}

func appError(msg string) *APIError { return newError(http.StatusBadRequest, "ApplicationError", msg) }

func checkPassword(password, hash any) bool {
	p, ok1 := password.(string)
	h, ok2 := hash.(string)
	return ok1 && ok2 && h != "" && bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) == nil
}

func (s *Server) authCallback(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	var grant map[string]struct {
		Enabled bool `json:"enabled"`
	}
	if err := s.auth.store(ctx, "grant", &grant); err != nil {
		return err
	}
	if !grant["email"].Enabled {
		return appError("This provider is disabled")
	}
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	err = bodySchema{fields: []bodyField{
		{name: "identifier", required: true},
		{name: "password", required: true},
	}}.validate(body)
	if err != nil {
		return err
	}
	identifier := body["identifier"].(string)
	user, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{
		"provider": "local",
		"$or":      []any{map[string]any{"email": strings.ToLower(identifier)}, map[string]any{"username": identifier}},
	}})
	if err != nil {
		return err
	}
	invalid := validationError("Invalid identifier or password", nil)
	if user == nil {
		return invalid
	}
	hash, _ := user.Get("password")
	if !checkPassword(body["password"], hash) {
		return invalid
	}
	var advanced upAdvanced
	if err := s.auth.store(ctx, "advanced", &advanced); err != nil {
		return err
	}
	if confirmed, _ := user.Get("confirmed"); advanced.EmailConfirmation && confirmed != true {
		return appError("Your account email is not confirmed")
	}
	if blocked, _ := user.Get("blocked"); blocked == true {
		return appError("Your account has been blocked by an administrator")
	}
	return s.sendAuth(w, user, rc.auth)
}

func (s *Server) authRegister(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	var advanced upAdvanced
	if err := s.auth.store(ctx, "advanced", &advanced); err != nil {
		return err
	}
	if !advanced.AllowRegister {
		return appError("Register action is currently disabled")
	}
	body, keys, err := readBody(r)
	if err != nil {
		return err
	}
	allowed := []string{"username", "password", "email"}
	for _, f := range s.cfg.Users.AllowedFields {
		if f != "" {
			allowed = append(allowed, f)
		}
	}
	var invalid []string
	for _, k := range keys {
		if !contains(allowed, k) {
			invalid = append(invalid, k)
		}
	}
	if len(invalid) > 0 {
		return validationError("Invalid parameters: "+strings.Join(invalid, ", "), nil)
	}
	params := map[string]any{}
	for _, k := range allowed {
		if v, ok := body[k]; ok {
			params[k] = v
		}
	}
	params["provider"] = "local"
	err = bodySchema{fields: []bodyField{
		{name: "email", required: true, email: true},
		{name: "username", required: true},
		{name: "password", required: true, tests: []func(any, map[string]any) string{maxPasswordBytes}},
	}}.validate(params)
	if err != nil {
		return err
	}
	role, err := s.docs.E.FindOne(ctx, roleUID, query.Params{Where: map[string]any{"type": advanced.DefaultRole}})
	if err != nil {
		return err
	}
	if role == nil {
		return appError("Impossible to find the default role")
	}
	email := strings.ToLower(params["email"].(string))
	username := params["username"].(string)
	identifiers := []any{
		map[string]any{"email": email},
		map[string]any{"username": email},
		map[string]any{"username": username},
		map[string]any{"email": username},
	}
	n, err := s.docs.E.Count(ctx, userUID, query.Params{Where: map[string]any{"$or": identifiers, "provider": "local"}})
	if err != nil {
		return err
	}
	if n > 0 {
		return appError("Email or Username are already taken")
	}
	if advanced.UniqueEmail {
		n, err := s.docs.E.Count(ctx, userUID, query.Params{Where: map[string]any{"$or": identifiers}})
		if err != nil {
			return err
		}
		if n > 0 {
			return appError("Email or Username are already taken")
		}
	}
	roleID, _ := role.Get("id")
	params["role"] = roleID
	params["email"] = email
	params["confirmed"] = !advanced.EmailConfirmation
	user, err := s.docs.Create(ctx, userUID, document.Params{"data": params, "populate": []any{"role"}})
	if err != nil {
		return err
	}
	if advanced.EmailConfirmation {
		if err := s.sendConfirmationEmail(ctx, user); err != nil {
			s.log.Error("sending the confirmation email failed", "error", err)
			return appError("Error sending confirmation email")
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": s.sanitizeOutput(user, s.schemaOf(userUID), rc.auth)})
		return nil
	}
	return s.sendAuth(w, user, rc.auth)
}

func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	if rc.auth.User == nil {
		return appError("You must be authenticated to reset your password")
	}
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	err = bodySchema{noUnknown: true, fields: []bodyField{
		{name: "password", required: true, tests: []func(any, map[string]any) string{maxPasswordBytes}},
		{name: "passwordConfirmation", required: true, tests: []func(any, map[string]any) string{matchesPassword}},
		{name: "currentPassword", required: true},
	}}.validate(body)
	if err != nil {
		return err
	}
	user, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"id": rc.auth.userID()}})
	if err != nil {
		return err
	}
	if user == nil {
		return errNotFound
	}
	hash, _ := user.Get("password")
	if !checkPassword(body["currentPassword"], hash) {
		return validationError("The provided current password is invalid", nil)
	}
	if body["currentPassword"] == body["password"] {
		return validationError("Your new password must be different than your current password", nil)
	}
	if _, err := s.editUser(ctx, user, map[string]any{"password": body["password"]}); err != nil {
		return err
	}
	return s.sendAuth(w, user, rc.auth)
}

// editUser is the user service's edit: a document update by documentId.
func (s *Server) editUser(ctx context.Context, user query.Row, data map[string]any) (query.Row, error) {
	doc, _ := user.Get(schema.AttrDocumentID)
	return s.docs.Update(ctx, userUID, fmt.Sprint(doc), document.Params{"data": data, "populate": []any{"role"}})
}

// fetchUser is the user service's fetch: the id and the query's where.
func (s *Server) fetchUser(ctx context.Context, id any, q document.Params) (query.Row, error) {
	qp, err := s.docs.TransformQuery(userUID, userQuery(q))
	if err != nil {
		return nil, err
	}
	var where any = map[string]any{}
	if qp.Where != nil {
		where = qp.Where
	}
	qp.Where = map[string]any{"$and": []any{map[string]any{"id": id}, where}}
	qp.Limit, qp.Offset = nil, nil
	return s.docs.E.FindOne(ctx, userUID, qp)
}

func userQuery(q document.Params) document.Params {
	out := document.Params{}
	for _, k := range []string{"filters", "sort", "fields", "populate", "status", "locale", "page", "pageSize", "start", "limit", "_q", "publicationFilter", "hasPublishedVersion", "pagination", "count", "ordering"} {
		if v, ok := q[k]; ok {
			out[k] = v
		}
	}
	return out
}

func (s *Server) usersMe(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	if rc.auth.User == nil {
		return newError(http.StatusUnauthorized, "UnauthorizedError", "Unauthorized")
	}
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	user, err := s.fetchUser(r.Context(), rc.auth.userID(), q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.sanitizeOutput(user, rc.model, rc.auth))
	return nil
}

func (s *Server) usersFind(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	qp, err := s.docs.TransformQuery(userUID, userQuery(q))
	if err != nil {
		return err
	}
	rows, err := s.docs.E.FindMany(r.Context(), userUID, qp)
	if err != nil {
		return err
	}
	out := s.sanitizeOutput(rows, rc.model, rc.auth)
	if out == nil {
		out = []any{}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) usersFindOne(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	user, err := s.fetchUser(r.Context(), rc.params["id"], q)
	if err != nil {
		return err
	}
	if user == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	writeJSON(w, http.StatusOK, s.sanitizeOutput(user, rc.model, rc.auth))
	return nil
}

func (s *Server) usersCount(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	qp, err := s.docs.TransformQuery(userUID, userQuery(q))
	if err != nil {
		return err
	}
	n, err := s.docs.E.Count(r.Context(), userUID, query.Params{Where: qp.Where, Filters: qp.Filters, Search: qp.Search})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, n)
	return nil
}

func (s *Server) usersCreate(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	var advanced upAdvanced
	if err := s.auth.store(ctx, "advanced", &advanced); err != nil {
		return err
	}
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	err = bodySchema{fields: []bodyField{
		{name: "email", required: true, email: true},
		{name: "username", required: true, min: 1},
		{name: "password", required: true, min: 1},
		{name: "role", required: true, mixed: true},
	}}.validate(body)
	if err != nil {
		return err
	}
	email := strings.ToLower(body["email"].(string))
	if advanced.UniqueEmail {
		same, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"email": email}})
		if err != nil {
			return err
		}
		if same != nil {
			return appError("Email already taken")
		}
	}
	data := map[string]any{}
	for k, v := range body {
		data[k] = v
	}
	data["email"] = email
	data["provider"] = "local"
	user, err := s.docs.Create(ctx, userUID, document.Params{"data": data, "populate": []any{"role"}})
	if err != nil {
		if api := toAPIError(err); api != nil && api.Status < 500 {
			return appError(api.Message)
		}
		return err
	}
	writeJSON(w, http.StatusCreated, s.sanitizeOutput(user, rc.model, rc.auth))
	return nil
}

func (s *Server) usersUpdate(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	var advanced upAdvanced
	if err := s.auth.store(ctx, "advanced", &advanced); err != nil {
		return err
	}
	id := rc.params["id"]
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	user, err := s.fetchUser(ctx, id, document.Params{})
	if err != nil {
		return err
	}
	if user == nil {
		return newError(http.StatusNotFound, "NotFoundError", "User not found")
	}
	err = bodySchema{fields: []bodyField{
		{name: "email", email: true, min: 1},
		{name: "username", min: 1},
		{name: "password", mixed: true, tests: []func(any, map[string]any) string{func(v any, _ map[string]any) string {
			if v == nil || v == "" {
				return ""
			}
			if s, ok := v.(string); ok && s != "" {
				return ""
			}
			return "Password must be at least 1 character"
		}}},
	}}.validate(body)
	if err != nil {
		return err
	}
	if provider, _ := user.Get("provider"); provider == "local" {
		if p, has := body["password"]; has && (p == nil || p == "" || p == false || p == float64(0)) {
			return validationError("password.notNull", nil)
		}
	}
	if username, has := body["username"]; has {
		same, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"username": username}})
		if err != nil {
			return err
		}
		if same != nil {
			if sameID, _ := same.Get("id"); fmt.Sprint(sameID) != fmt.Sprint(id) {
				return appError("Username already taken")
			}
		}
	}
	if email, has := body["email"].(string); has && advanced.UniqueEmail {
		same, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"email": strings.ToLower(email)}})
		if err != nil {
			return err
		}
		if same != nil {
			if sameID, _ := same.Get("id"); fmt.Sprint(sameID) != fmt.Sprint(id) {
				return appError("Email already taken")
			}
		}
		body["email"] = strings.ToLower(email)
	}
	updated, err := s.editUser(ctx, user, body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.sanitizeOutput(updated, rc.model, rc.auth))
	return nil
}

func (s *Server) usersDestroy(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	row, err := s.docs.E.Delete(r.Context(), userUID, map[string]any{"id": rc.params["id"]}, query.Params{})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.sanitizeOutput(row, rc.model, rc.auth))
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
