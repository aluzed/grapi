// Package bootstrap is what the built-in plugins write at start-up on a
// database: default settings, roles and permissions, the default locale.
// It only adds what is missing: unlike Strapi's users-permissions, it never
// deletes the permissions of actions it does not know, since the actions of
// JS controllers are invisible to grapi.
package bootstrap

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"reflect"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/schema"
)

//go:embed defaults.json
var defaultsJSON []byte

// Defaults captured from a fresh Strapi 5.59 database.
type defaults struct {
	UsersPermissions struct {
		Grant    map[string]any `json:"grant"`
		Email    any            `json:"email"`
		Advanced any            `json:"advanced"`
	} `json:"users-permissions"`
	Upload struct {
		Settings          map[string]any `json:"settings"`
		ViewConfiguration map[string]any `json:"view_configuration"`
	} `json:"upload"`
}

// Options of Run.
type Options struct {
	// InitLocale is STRAPI_PLUGIN_I18N_INIT_LOCALE_CODE, "en" by default.
	InitLocale string
	Log        *slog.Logger
}

const (
	userUID       = "plugin::users-permissions.user"
	roleUID       = "plugin::users-permissions.role"
	permissionUID = "plugin::users-permissions.permission"
	localeUID     = "plugin::i18n.locale"
)

var defaultPermissions = []struct{ action, role string }{
	{"plugin::users-permissions.auth.callback", "public"},
	{"plugin::users-permissions.auth.connect", "public"},
	{"plugin::users-permissions.auth.forgotPassword", "public"},
	{"plugin::users-permissions.auth.resetPassword", "public"},
	{"plugin::users-permissions.auth.register", "public"},
	{"plugin::users-permissions.auth.emailConfirmation", "public"},
	{"plugin::users-permissions.auth.sendEmailConfirmation", "public"},
	{"plugin::users-permissions.auth.refresh", "public"},
	{"plugin::users-permissions.auth.logout", "authenticated"},
	{"plugin::users-permissions.auth.getSessions", "authenticated"},
	{"plugin::users-permissions.auth.revokeSession", "authenticated"},
	{"plugin::users-permissions.user.me", "authenticated"},
	{"plugin::users-permissions.auth.changePassword", "authenticated"},
}

// Run applies the bootstrap of users-permissions, upload and i18n.
func Run(ctx context.Context, e *query.Engine, set *schema.Set, opts Options) error {
	var d defaults
	if err := json.Unmarshal(defaultsJSON, &d); err != nil {
		return err
	}
	st := store.Store{DB: e.DB}
	if _, ok := set.ContentType(userUID); ok {
		if err := usersPermissions(ctx, e, st, d); err != nil {
			return err
		}
	}
	if _, ok := set.ContentType("plugin::upload.file"); ok {
		scope := store.Scope{Type: "plugin", Name: "upload"}
		for key, value := range map[string]map[string]any{"settings": d.Upload.Settings, "view_configuration": d.Upload.ViewConfiguration} {
			if err := fillMissing(ctx, st, scope, key, value); err != nil {
				return err
			}
		}
	}
	if _, ok := set.ContentType(localeUID); ok {
		if err := defaultLocale(ctx, e, st, opts.InitLocale); err != nil {
			return err
		}
	}
	return nil
}

// fillMissing is the upload bootstrap: the stored value gets the default
// keys it lacks.
func fillMissing(ctx context.Context, st store.Store, scope store.Scope, key string, def map[string]any) error {
	var current map[string]any
	found, err := st.Get(ctx, scope, key, &current)
	if err != nil {
		return err
	}
	complete := found && current != nil
	for k := range def {
		if _, ok := current[k]; !ok {
			complete = false
		}
	}
	if complete {
		return nil
	}
	merged := map[string]any{}
	for k, v := range def {
		merged[k] = v
	}
	for k, v := range current {
		merged[k] = v
	}
	return st.Set(ctx, scope, key, merged)
}

func merge(dst, src any) any {
	d, ok1 := dst.(map[string]any)
	s, ok2 := src.(map[string]any)
	if !ok1 || !ok2 {
		if src == nil {
			return dst
		}
		return src
	}
	out := map[string]any{}
	for k, v := range d {
		out[k] = v
	}
	for k, v := range s {
		out[k] = merge(out[k], v)
	}
	return out
}

func usersPermissions(ctx context.Context, e *query.Engine, st store.Store, d defaults) error {
	scope := store.Scope{Type: "plugin", Name: "users-permissions"}
	var prev map[string]any
	if _, err := st.Get(ctx, scope, "grant", &prev); err != nil {
		return err
	}
	grant := map[string]any{}
	for k, v := range d.UsersPermissions.Grant {
		grant[k] = v
		if p, ok := prev[k]; ok {
			grant[k] = merge(v, p)
		}
	}
	// Strapi rewrites the value whenever it differs from the defaults;
	// grapi only when the merge changes it, so a Strapi database stays as is.
	if !reflect.DeepEqual(prev, grant) {
		if err := st.Set(ctx, scope, "grant", grant); err != nil {
			return err
		}
	}
	for key, value := range map[string]any{"email": d.UsersPermissions.Email, "advanced": d.UsersPermissions.Advanced} {
		var current any
		found, err := st.Get(ctx, scope, key, &current)
		if err != nil {
			return err
		}
		if !found || current == nil {
			if err := st.Set(ctx, scope, key, value); err != nil {
				return err
			}
		}
	}
	n, err := e.Count(ctx, roleUID, query.Params{})
	if err != nil {
		return err
	}
	if n == 0 {
		for _, r := range []query.Data{
			{"name": "Authenticated", "description": "Default role given to authenticated user.", "type": "authenticated"},
			{"name": "Public", "description": "Default role given to unauthenticated user.", "type": "public"},
		} {
			if _, err := e.Create(ctx, roleUID, r, query.Params{}); err != nil {
				return err
			}
		}
	}
	perms, err := e.Count(ctx, permissionUID, query.Params{})
	if err != nil || perms > 0 {
		return err
	}
	roles, err := e.FindMany(ctx, roleUID, query.Params{OrderBy: map[string]any{"id": "asc"}})
	if err != nil {
		return err
	}
	for _, role := range roles {
		id, _ := role.Get("id")
		typ, _ := role.Get("type")
		for _, p := range defaultPermissions {
			if p.role != typ {
				continue
			}
			if _, err := e.Create(ctx, permissionUID, query.Data{"action": p.action, "role": id}, query.Params{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultLocale(ctx context.Context, e *query.Engine, st store.Store, code string) error {
	if code == "" {
		code = "en"
	}
	n, err := e.Count(ctx, localeUID, query.Params{})
	if err != nil || n > 0 {
		return err
	}
	name := code
	if code == "en" {
		name = "English (en)"
	}
	if _, err := e.Create(ctx, localeUID, query.Data{"name": name, "code": code}, query.Params{}); err != nil {
		return err
	}
	return st.Set(ctx, store.Scope{Type: "plugin", Name: "i18n"}, "default_locale", code)
}
