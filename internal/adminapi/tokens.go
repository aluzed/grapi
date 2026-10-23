package adminapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/apitoken"
	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
)

const (
	apiTokenUID     = "admin::api-token"
	apiTokenPermUID = "admin::api-token-permission"
)

func (s *Server) tokenRoutes() {
	s.allowed("GET /api-tokens", "admin::api-tokens.read", s.listTokens)
	s.allowed("GET /api-tokens/actions", "admin::api-tokens.read", s.tokenActions)
	s.allowed("GET /api-tokens/{id}", "admin::api-tokens.read", s.getToken)
	s.allowed("POST /api-tokens", "admin::api-tokens.create", s.createToken)
	s.allowed("PUT /api-tokens/{id}", "admin::api-tokens.update", s.updateToken)
	s.allowed("POST /api-tokens/{id}/regenerate", "admin::api-tokens.regenerate", s.regenerateToken)
	s.allowed("DELETE /api-tokens/{id}", "admin::api-tokens.delete", s.deleteToken)
}

// Token is an API token without its key.
type Token struct {
	ID          any      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Lifespan    any      `json:"lifespan"`
	ExpiresAt   any      `json:"expiresAt"`
	LastUsedAt  any      `json:"lastUsedAt"`
	CreatedAt   any      `json:"createdAt"`
	Permissions []string `json:"permissions"`
	// AccessKey is the key itself: on creation and regeneration, or when
	// it is stored encrypted.
	AccessKey string `json:"accessKey,omitempty"`
}

func (s *Server) tokenOut(r *http.Request, row query.Row, withPerms bool) (*Token, error) {
	t := &Token{
		ID: get(row, "id"), Name: getString(row, "name"), Description: getString(row, "description"), Type: getString(row, "type"),
		Lifespan: get(row, "lifespan"), ExpiresAt: get(row, "expiresAt"), LastUsedAt: get(row, "lastUsedAt"), CreatedAt: get(row, "createdAt"), Permissions: []string{},
	}
	if withPerms {
		perms, err := s.e.FindMany(r.Context(), apiTokenPermUID, query.Params{Where: map[string]any{"token": map[string]any{"id": t.ID}}, OrderBy: []any{map[string]any{"action": "asc"}}})
		if err != nil {
			return nil, err
		}
		for _, p := range perms {
			t.Permissions = append(t.Permissions, getString(p, "action"))
		}
		if enc := getString(row, "encryptedKey"); enc != "" && s.cfg.EncryptionKey != "" {
			if key, err := apitoken.Decrypt(s.cfg.EncryptionKey, enc); err == nil {
				t.AccessKey = key
			}
		}
	}
	return t, nil
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	rows, err := s.e.FindMany(r.Context(), apiTokenUID, query.Params{Where: map[string]any{"kind": "content-api"}, OrderBy: []any{map[string]any{"createdAt": "desc"}}})
	if err != nil {
		return err
	}
	out := make([]*Token, 0, len(rows))
	for _, row := range rows {
		t, err := s.tokenOut(r, row, false)
		if err != nil {
			return err
		}
		out = append(out, t)
	}
	return data(w, out)
}

func (s *Server) tokenActions(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	var out []string
	if s.cfg.ContentActions != nil {
		out = s.cfg.ContentActions()
	}
	if out == nil {
		out = []string{}
	}
	return data(w, out)
}

func (s *Server) tokenByPath(r *http.Request) (query.Row, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errNotFound
	}
	row, err := s.e.FindOne(r.Context(), apiTokenUID, query.Params{Where: map[string]any{"id": id, "kind": "content-api"}})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "API Token not found"}
	}
	return row, nil
}

func (s *Server) getToken(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.tokenByPath(r)
	if err != nil {
		return err
	}
	t, err := s.tokenOut(r, row, true)
	if err != nil {
		return err
	}
	return data(w, t)
}

type tokenBody struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Type        *string  `json:"type"`
	Lifespan    *int     `json:"lifespan"`
	Permissions []string `json:"permissions"`
}

// check validates the body; name is required on creation.
func (s *Server) check(r *http.Request, b *tokenBody, creating bool, except any) error {
	var errs []FieldError
	if b.Name != nil {
		*b.Name = strings.TrimSpace(*b.Name)
	}
	if creating && (b.Name == nil || *b.Name == "") || b.Name != nil && *b.Name == "" {
		errs = append(errs, FieldError{Path: []string{"name"}, Message: "name is a required field"})
	} else if b.Name != nil {
		where := map[string]any{"name": *b.Name, "kind": "content-api"}
		if except != nil {
			where["id"] = map[string]any{"$ne": except}
		}
		if n, err := s.e.Count(r.Context(), apiTokenUID, query.Params{Where: where}); err != nil {
			return err
		} else if n > 0 {
			errs = append(errs, FieldError{Path: []string{"name"}, Message: "Name already taken"})
		}
	}
	if creating && b.Type == nil {
		errs = append(errs, FieldError{Path: []string{"type"}, Message: "type is a required field"})
	}
	if b.Type != nil && *b.Type != "read-only" && *b.Type != "full-access" && *b.Type != "custom" {
		errs = append(errs, FieldError{Path: []string{"type"}, Message: "type must be one of read-only, full-access, custom"})
	}
	if b.Lifespan != nil && !apitoken.Lifespans[*b.Lifespan] {
		errs = append(errs, FieldError{Path: []string{"lifespan"}, Message: "lifespan must be 7, 30, 90 days or unlimited"})
	}
	known := map[string]bool{}
	if s.cfg.ContentActions != nil {
		for _, a := range s.cfg.ContentActions() {
			known[a] = true
		}
	}
	for i, a := range b.Permissions {
		if !known[a] {
			errs = append(errs, FieldError{Path: []string{"permissions", strconv.Itoa(i)}, Message: "Unknown action " + a})
		}
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	return nil
}

func (s *Server) setTokenPermissions(r *http.Request, id any, typ string, actions []string) error {
	ctx := r.Context()
	if _, err := s.e.DeleteMany(ctx, apiTokenPermUID, query.Params{Where: map[string]any{"token": map[string]any{"id": id}}}); err != nil {
		return err
	}
	if typ != "custom" {
		return nil
	}
	seen := map[string]bool{}
	for _, a := range actions {
		if seen[a] {
			continue
		}
		seen[a] = true
		if _, err := s.e.Create(ctx, apiTokenPermUID, query.Data{"action": a, "token": id}, query.Params{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) (err error) {
	defer func() {
		if err == nil {
			s.audit(r, u, "token.create", map[string]any{"id": r.PathValue("id")})
		}
	}()
	if s.cfg.APITokenSalt == "" {
		return appError("admin.apiToken.salt (API_TOKEN_SALT) is not set")
	}
	var b tokenBody
	if err := decode(r, &b); err != nil {
		return err
	}
	if err := s.check(r, &b, true, nil); err != nil {
		return err
	}
	key, cols, err := apitoken.New(s.cfg.APITokenSalt, s.cfg.EncryptionKey)
	if err != nil {
		return err
	}
	d := query.Data{"name": *b.Name, "type": *b.Type, "kind": "content-api"}
	for k, v := range cols {
		d[k] = v
	}
	if b.Description != nil {
		d["description"] = *b.Description
	}
	if b.Lifespan != nil && *b.Lifespan > 0 {
		life := time.Duration(*b.Lifespan) * apitoken.Day
		d["lifespan"] = strconv.FormatInt(life.Milliseconds(), 10)
		d["expiresAt"] = s.now().Add(life).UTC()
	}
	row, err := s.e.Create(r.Context(), apiTokenUID, d, query.Params{})
	if err != nil {
		return err
	}
	if err := s.setTokenPermissions(r, get(row, "id"), *b.Type, b.Permissions); err != nil {
		return err
	}
	t, err := s.tokenOut(r, row, true)
	if err != nil {
		return err
	}
	t.AccessKey = key
	writeJSON(w, http.StatusCreated, map[string]any{"data": t})
	return nil
}

func (s *Server) updateToken(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) (err error) {
	defer func() {
		if err == nil {
			s.audit(r, u, "token.update", map[string]any{"id": r.PathValue("id")})
		}
	}()
	row, err := s.tokenByPath(r)
	if err != nil {
		return err
	}
	var b tokenBody
	if err := decode(r, &b); err != nil {
		return err
	}
	id := get(row, "id")
	if err := s.check(r, &b, false, id); err != nil {
		return err
	}
	set := query.Data{}
	if b.Name != nil {
		set["name"] = *b.Name
	}
	if b.Description != nil {
		set["description"] = *b.Description
	}
	typ := getString(row, "type")
	if b.Type != nil {
		set["type"], typ = *b.Type, *b.Type
	}
	if len(set) > 0 {
		if row, err = s.e.Update(r.Context(), apiTokenUID, map[string]any{"id": id}, set, query.Params{}); err != nil {
			return err
		}
	}
	if b.Permissions != nil || b.Type != nil {
		perms := b.Permissions
		if perms == nil && typ == "custom" {
			t, err := s.tokenOut(r, row, true)
			if err != nil {
				return err
			}
			perms = t.Permissions
		}
		if err := s.setTokenPermissions(r, id, typ, perms); err != nil {
			return err
		}
	}
	t, err := s.tokenOut(r, row, true)
	if err != nil {
		return err
	}
	return data(w, t)
}

func (s *Server) regenerateToken(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) (err error) {
	defer func() {
		if err == nil {
			s.audit(r, u, "token.regenerate", map[string]any{"id": r.PathValue("id")})
		}
	}()
	row, err := s.tokenByPath(r)
	if err != nil {
		return err
	}
	key, cols, err := apitoken.New(s.cfg.APITokenSalt, s.cfg.EncryptionKey)
	if err != nil {
		return err
	}
	if _, err := s.e.Update(r.Context(), apiTokenUID, map[string]any{"id": get(row, "id")}, query.Data(cols), query.Params{}); err != nil {
		return err
	}
	return data(w, map[string]any{"accessKey": key})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) (err error) {
	defer func() {
		if err == nil {
			s.audit(r, u, "token.delete", map[string]any{"id": r.PathValue("id")})
		}
	}()
	row, err := s.tokenByPath(r)
	if err != nil {
		return err
	}
	id := get(row, "id")
	if _, err := s.e.DeleteMany(r.Context(), apiTokenPermUID, query.Params{Where: map[string]any{"token": map[string]any{"id": id}}}); err != nil {
		return err
	}
	if _, err := s.e.Delete(r.Context(), apiTokenUID, map[string]any{"id": id}, query.Params{}); err != nil {
		return err
	}
	t, err := s.tokenOut(r, row, false)
	if err != nil {
		return err
	}
	return data(w, t)
}
