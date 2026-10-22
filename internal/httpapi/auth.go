package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/db"
)

// Auth is who a request acts as: the public role, an API token or (later)
// an end user. allowed answers Strapi's auth.verify(auth, {scope}).
type Auth struct {
	Strategy  string
	TokenType string
	scopes    map[string]bool
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

type authenticator struct {
	db   *db.DB
	salt string
}

func (a *authenticator) hash(key string) string {
	m := hmac.New(sha512.New, []byte(a.salt))
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))
}

// authenticate tries the API token strategy, then users-permissions: a
// request without credentials gets the public role.
func (a *authenticator) authenticate(ctx context.Context, r *http.Request) (*Auth, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return a.role(ctx, "public")
	}
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			token = parts[1]
		} else {
			return nil, errUnauthorized
		}
	}
	auth, err := a.apiToken(ctx, strings.TrimSpace(token))
	if err != nil || auth != nil {
		return auth, err
	}
	return nil, errUnauthorized
}

func (a *authenticator) apiToken(ctx context.Context, token string) (*Auth, error) {
	if token == "" {
		return nil, nil
	}
	var (
		id      int64
		typ     string
		expires sql.NullString
	)
	err := a.db.QueryRow(ctx, `select "id", "type", cast("expires_at" as varchar) from "strapi_api_tokens" where "access_key" = `+a.db.Dialect.Placeholder(1), a.hash(token)).Scan(&id, &typ, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expires.Valid && expires.String != "" {
		if t, ok := parseStoredTime(expires.String); ok && t.Before(time.Now()) {
			return nil, newError(http.StatusUnauthorized, "UnauthorizedError", "Token expired")
		}
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

func parseStoredTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// role loads the permissions of a users-permissions role by type.
func (a *authenticator) role(ctx context.Context, roleType string) (*Auth, error) {
	rows, err := a.db.Query(ctx, `select p."action" from "up_permissions" p
		join "up_permissions_role_lnk" l on l."permission_id" = p."id"
		join "up_roles" r on r."id" = l."role_id"
		where r."type" = `+a.db.Dialect.Placeholder(1), roleType)
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
