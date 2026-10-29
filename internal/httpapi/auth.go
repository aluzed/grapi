package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/session"
	"github.com/aluzed/grapi/internal/auth/users"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/store"
)

// Auth is who a request acts as: the public role, an API token or an end
// user. Allowed answers Strapi's auth.verify(auth, {scope}).
type Auth struct {
	Strategy  string
	TokenType string
	// User is the authenticated end user (users-permissions), with its role.
	User query.Row
	// SessionID is the session of a refresh-mode access token.
	SessionID string
	scopes    map[string]bool
	preview   *previewGrant
}

// Allowed reports whether the request may run the action scope
// ("api::article.article.find").
func (a *Auth) Allowed(scope string) bool {
	switch a.TokenType {
	case "full-access":
		return true
	case "read-only":
		return strings.HasSuffix(scope, ".find") || strings.HasSuffix(scope, ".findOne")
	}
	return a.scopes[scope]
}

func (a *Auth) userID() any {
	if a == nil || a.User == nil {
		return nil
	}
	id, _ := a.User.Get("id")
	return id
}

type authenticator struct {
	db     *db.DB
	engine *query.Engine
	salt   string
	users  UsersConfig
	// up verifies the access tokens in jwtManagement "refresh".
	up *session.Manager
	// previewSecret verifies the admin's preview tokens.
	previewSecret string
}

func (a *authenticator) hash(key string) string {
	m := hmac.New(sha512.New, []byte(a.salt))
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))
}

var spaces = regexp.MustCompile(`\s+`)

// bearer is how both strategies read the header: exactly "bearer <token>".
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := spaces.Split(h, -1)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}

// authenticate runs the content API strategies in Strapi's order: API
// token, then users-permissions (JWT, or the public role without token).
func (a *Auth) origin() string {
	if a != nil && a.Strategy == "api-token" {
		return "admin-token"
	}
	return "content-api"
}

func (a *authenticator) authenticate(ctx context.Context, r *http.Request) (*Auth, error) {
	if grant, ok, err := a.previewToken(previewTokenOf(r)); ok {
		if err != nil {
			return nil, err
		}
		// Beyond its one document, a preview token reads what the public
		// role reads: a populate cannot reach further.
		auth, err := a.role(ctx, "public")
		if err != nil {
			return nil, err
		}
		auth.Strategy, auth.preview = "preview", grant
		return auth, nil
	}
	token := bearer(r)
	if token != "" {
		auth, err := a.apiToken(ctx, token)
		if err != nil || auth != nil {
			return auth, err
		}
		return a.jwtUser(ctx, token)
	}
	auth, err := a.role(ctx, "public")
	if err != nil {
		return nil, err
	}
	if len(auth.scopes) == 0 {
		return nil, errUnauthorized
	}
	return auth, nil
}

func (a *authenticator) jwtUser(ctx context.Context, token string) (*Auth, error) {
	var id any
	sessionID := ""
	if a.up != nil {
		// Refresh mode takes only the access tokens of a live session.
		userID, sid, err := a.up.ValidateAccess(ctx, token)
		if err != nil {
			return nil, errUnauthorized
		}
		id, sessionID = userID, sid
		if n, err := strconv.ParseInt(userID, 10, 64); err == nil {
			id = n
		}
	} else {
		payload, err := users.Verify(token, a.users.JWTSecret, a.users.JWTAlgorithm, time.Now())
		if err != nil {
			return nil, errUnauthorized
		}
		var ok bool
		if id, ok = payload["id"]; !ok {
			return nil, errUnauthorized
		}
	}
	invalid := newError(http.StatusUnauthorized, "UnauthorizedError", "Invalid credentials")
	user, err := a.engine.FindOne(ctx, userUID, query.Params{Where: map[string]any{"id": id}, Populate: []any{"role"}})
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, invalid
	}
	var advanced upAdvanced
	if err := a.store(ctx, "advanced", &advanced); err != nil {
		return nil, err
	}
	if confirmed, _ := user.Get("confirmed"); advanced.EmailConfirmation && confirmed != true {
		return nil, invalid
	}
	if blocked, _ := user.Get("blocked"); blocked == true {
		return nil, invalid
	}
	role, _ := user.Get("role")
	roleRow, _ := role.(query.Row)
	if roleRow == nil {
		return nil, errUnauthorized
	}
	roleID, _ := roleRow.Get("id")
	auth, err := a.roleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	auth.User = user
	auth.SessionID = sessionID
	return auth, nil
}

func (a *authenticator) apiToken(ctx context.Context, token string) (*Auth, error) {
	var (
		id               int64
		typ              string
		expires, lastUse sql.NullString
	)
	err := a.db.QueryRow(ctx, `select "id", "type", cast("expires_at" as varchar), cast("last_used_at" as varchar) from "strapi_api_tokens" where "access_key" = `+a.db.Dialect.Placeholder(1), a.hash(token)).Scan(&id, &typ, &expires, &lastUse)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expires.Valid && expires.String != "" {
		if t, ok := parseStoredTime(expires.String); !ok || t.Before(time.Now()) {
			return nil, newError(http.StatusUnauthorized, "UnauthorizedError", "Token expired")
		}
	}
	if err := a.touchToken(ctx, id, lastUse); err != nil {
		return nil, err
	}
	auth := &Auth{Strategy: "api-token", TokenType: typ, scopes: map[string]bool{}}
	if typ == "custom" {
		rows, err := a.db.Query(ctx, `select p."action" from "strapi_api_token_permissions" p
			join "strapi_api_token_permissions_token_lnk" l on l."api_token_permission_id" = p."id"
			where l."api_token_id" = `+a.db.Dialect.Placeholder(1), id)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var action string
			if err := rows.Scan(&action); err != nil {
				return nil, err
			}
			auth.scopes[action] = true
		}
	}
	return auth, nil
}

// touchToken is updateLastUsedAt: lastUsedAt is written at most once an
// hour.
func (a *authenticator) touchToken(ctx context.Context, id int64, last sql.NullString) error {
	now := time.Now()
	if last.Valid && last.String != "" {
		if t, ok := parseStoredTime(last.String); ok && now.Sub(t) < time.Hour {
			return nil
		}
	}
	_, err := a.engine.Update(ctx, "admin::api-token", map[string]any{"id": id}, query.Data{"lastUsedAt": now.UTC().Format("2006-01-02T15:04:05.000Z")}, query.Params{})
	return err
}

// parseStoredTime reads expires_at: an ISO string under Postgres, epoch
// milliseconds under SQLite.
func parseStoredTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05.999999-07", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	ms := json.Number(s)
	if n, err := ms.Int64(); err == nil {
		return time.UnixMilli(n), true
	}
	return time.Time{}, false
}

const rolePermissions = `select p."action" from "up_permissions" p
	join "up_permissions_role_lnk" l on l."permission_id" = p."id"
	join "up_roles" r on r."id" = l."role_id" where `

func (a *authenticator) role(ctx context.Context, roleType string) (*Auth, error) {
	return a.permissions(ctx, rolePermissions+`r."type" = `+a.db.Dialect.Placeholder(1), roleType)
}

func (a *authenticator) roleByID(ctx context.Context, id any) (*Auth, error) {
	return a.permissions(ctx, rolePermissions+`r."id" = `+a.db.Dialect.Placeholder(1), id)
}

func (a *authenticator) permissions(ctx context.Context, q string, arg any) (*Auth, error) {
	rows, err := a.db.Query(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	auth := &Auth{Strategy: "users-permissions", scopes: map[string]bool{}}
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			return nil, err
		}
		auth.scopes[action] = true
	}
	return auth, rows.Err()
}

// store reads a users-permissions setting from the core store.
func (a *authenticator) store(ctx context.Context, key string, v any) error {
	_, err := store.Store{DB: a.db}.Get(ctx, store.Scope{Type: "plugin", Name: "users-permissions"}, key, v)
	return err
}

type upAdvanced struct {
	UniqueEmail       bool   `json:"unique_email"`
	AllowRegister     bool   `json:"allow_register"`
	EmailConfirmation bool   `json:"email_confirmation"`
	DefaultRole       string `json:"default_role"`
}
