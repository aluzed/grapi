package adminapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/schema"
)

const (
	upRoleUID       = "plugin::users-permissions.role"
	upPermissionUID = "plugin::users-permissions.permission"
	upUserUID       = "plugin::users-permissions.user"
	up              = "plugin::users-permissions."
)

var upScope = store.Scope{Type: "plugin", Name: "users-permissions"}

func (s *Server) usersPermissionsRoutes() {
	if _, ok := s.set.ContentType(upRoleUID); !ok {
		return
	}
	s.allowed("GET /users-permissions/roles", up+"roles.read", s.upRoles)
	s.allowed("GET /users-permissions/actions", up+"roles.read", s.tokenActions)
	s.allowed("GET /users-permissions/routes", up+"roles.read", s.upRoutes)
	s.allowed("GET /users-permissions/roles/{id}", up+"roles.read", s.upRole)
	s.allowed("POST /users-permissions/roles", up+"roles.create", s.upCreateRole)
	s.allowed("PUT /users-permissions/roles/{id}", up+"roles.update", s.upUpdateRole)
	s.allowed("DELETE /users-permissions/roles/{id}", up+"roles.delete", s.upDeleteRole)
	for _, k := range []struct{ key, path, read, update string }{
		{"advanced", "advanced", "advanced-settings.read", "advanced-settings.update"},
		{"email", "email-templates", "email-templates.read", "email-templates.update"},
		{"grant", "providers", "providers.read", "providers.update"},
	} {
		s.allowed("GET /users-permissions/"+k.path, up+k.read, s.upSettingsGet(k.key))
		s.allowed("PUT /users-permissions/"+k.path, up+k.update, s.upSettingsPut(k.key))
	}
}

// UPRole is a users-permissions role.
type UPRole struct {
	ID          any      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	UsersCount  int64    `json:"usersCount"`
	Permissions []string `json:"permissions,omitempty"`
}

func (s *Server) upRoleOut(ctx context.Context, row query.Row, withPerms bool) (*UPRole, error) {
	r := &UPRole{ID: get(row, "id"), Name: getString(row, "name"), Description: getString(row, "description"), Type: getString(row, "type")}
	n, err := s.e.Count(ctx, upUserUID, query.Params{Where: map[string]any{"role": map[string]any{"id": r.ID}}})
	if err != nil {
		return nil, err
	}
	r.UsersCount = n
	if withPerms {
		perms, err := s.e.FindMany(ctx, upPermissionUID, query.Params{Where: map[string]any{"role": map[string]any{"id": r.ID}}, OrderBy: []any{map[string]any{"action": "asc"}}})
		if err != nil {
			return nil, err
		}
		r.Permissions = []string{}
		for _, p := range perms {
			r.Permissions = append(r.Permissions, getString(p, "action"))
		}
	}
	return r, nil
}

func (s *Server) upRoles(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	rows, err := s.e.FindMany(r.Context(), upRoleUID, query.Params{OrderBy: []any{map[string]any{"id": "asc"}}})
	if err != nil {
		return err
	}
	out := make([]*UPRole, 0, len(rows))
	for _, row := range rows {
		ro, err := s.upRoleOut(r.Context(), row, false)
		if err != nil {
			return err
		}
		out = append(out, ro)
	}
	return data(w, out)
}

func (s *Server) upRoleByPath(r *http.Request) (query.Row, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errNotFound
	}
	row, err := s.e.FindOne(r.Context(), upRoleUID, query.Params{Where: map[string]any{"id": id}})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Role not found"}
	}
	return row, nil
}

func (s *Server) upRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.upRoleByPath(r)
	if err != nil {
		return err
	}
	ro, err := s.upRoleOut(r.Context(), row, true)
	if err != nil {
		return err
	}
	return data(w, ro)
}

type upRoleBody struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
}

func (s *Server) upCheck(r *http.Request, b *upRoleBody, creating bool, except any) error {
	errs, err := s.checkName(r.Context(), b.Name, creating, upRoleUID, func(n string) map[string]any {
		return map[string]any{"name": map[string]any{"$eqi": n}}
	}, except, "A role with this name already exists")
	if err != nil {
		return err
	}
	errs = append(errs, s.unknownActions(b.Permissions)...)
	if len(errs) > 0 {
		return validation(errs)
	}
	return nil
}

// setUPPermissions makes the role's permissions exactly actions, keeping
// the rows of the actions that stay.
func (s *Server) setUPPermissions(ctx context.Context, id any, actions []string) error {
	rows, err := s.e.FindMany(ctx, upPermissionUID, query.Params{Where: map[string]any{"role": map[string]any{"id": id}}})
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, a := range actions {
		wanted[a] = true
	}
	have := map[string]bool{}
	for _, p := range rows {
		a := getString(p, "action")
		if !wanted[a] || have[a] {
			if _, err := s.e.Delete(ctx, upPermissionUID, map[string]any{"id": get(p, "id")}, query.Params{}); err != nil {
				return err
			}
			continue
		}
		have[a] = true
	}
	for _, a := range actions {
		if have[a] {
			continue
		}
		have[a] = true
		if _, err := s.e.Create(ctx, upPermissionUID, query.Data{"action": a, "role": id}, query.Params{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) upCreateRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	var b upRoleBody
	if err := decode(r, &b); err != nil {
		return err
	}
	if err := s.upCheck(r, &b, true, nil); err != nil {
		return err
	}
	ctx := r.Context()
	d := query.Data{"name": *b.Name, "type": schema.SnakeCase(strings.ToLower(*b.Name))}
	if b.Description != nil {
		d["description"] = *b.Description
	}
	row, err := s.e.Create(ctx, upRoleUID, d, query.Params{})
	if err != nil {
		return err
	}
	if err := s.setUPPermissions(ctx, get(row, "id"), b.Permissions); err != nil {
		return err
	}
	ro, err := s.upRoleOut(ctx, row, true)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": ro})
	return nil
}

func (s *Server) upUpdateRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.upRoleByPath(r)
	if err != nil {
		return err
	}
	var b upRoleBody
	if err := decode(r, &b); err != nil {
		return err
	}
	id := get(row, "id")
	if err := s.upCheck(r, &b, false, id); err != nil {
		return err
	}
	ctx := r.Context()
	set := query.Data{}
	if b.Name != nil {
		set["name"] = *b.Name
	}
	if b.Description != nil {
		set["description"] = *b.Description
	}
	if len(set) > 0 {
		if row, err = s.e.Update(ctx, upRoleUID, map[string]any{"id": id}, set, query.Params{}); err != nil {
			return err
		}
	}
	if b.Permissions != nil {
		if err := s.setUPPermissions(ctx, id, b.Permissions); err != nil {
			return err
		}
	}
	ro, err := s.upRoleOut(ctx, row, true)
	if err != nil {
		return err
	}
	return data(w, ro)
}

// upDeleteRole moves the role's users to the Public role, as Strapi does;
// Public and Authenticated cannot be deleted.
func (s *Server) upDeleteRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.upRoleByPath(r)
	if err != nil {
		return err
	}
	if t := getString(row, "type"); t == "public" || t == "authenticated" {
		return appError("The " + t + " role cannot be deleted")
	}
	ctx := r.Context()
	public, err := s.e.FindOne(ctx, upRoleUID, query.Params{Where: map[string]any{"type": "public"}})
	if err != nil {
		return err
	}
	id := get(row, "id")
	users, err := s.e.FindMany(ctx, upUserUID, query.Params{Where: map[string]any{"role": map[string]any{"id": id}}, Select: []string{"id"}})
	if err != nil {
		return err
	}
	for _, u := range users {
		if _, err := s.e.Update(ctx, upUserUID, map[string]any{"id": get(u, "id")}, query.Data{"role": get(public, "id")}, query.Params{}); err != nil {
			return err
		}
	}
	if err := s.setUPPermissions(ctx, id, nil); err != nil {
		return err
	}
	if _, err := s.e.Delete(ctx, upRoleUID, map[string]any{"id": id}, query.Params{}); err != nil {
		return err
	}
	return data(w, map[string]any{"id": id})
}

func (s *Server) upSettingsGet(key string) permittedHandler {
	return func(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
		var v map[string]any
		if _, err := s.store.Get(r.Context(), upScope, key, &v); err != nil {
			return err
		}
		if v == nil {
			v = map[string]any{}
		}
		return data(w, v)
	}
}

// upSettingsPut updates the known entries of a settings object (the keys
// already in the store), leaving the others as they are.
func (s *Server) upSettingsPut(key string) permittedHandler {
	return func(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
		var body map[string]any
		if err := decode(r, &body); err != nil {
			return err
		}
		ctx := r.Context()
		var current map[string]any
		if _, err := s.store.Get(ctx, upScope, key, &current); err != nil {
			return err
		}
		if current == nil {
			current = map[string]any{}
		}
		for k, v := range body {
			if _, ok := current[k]; !ok {
				return validation([]FieldError{{Path: []string{k}, Message: "Unknown setting " + k}})
			}
			if old, isMap := current[k].(map[string]any); isMap {
				nv, ok := v.(map[string]any)
				if !ok {
					return validation([]FieldError{{Path: []string{k}, Message: k + " must be an object"}})
				}
				for kk, vv := range nv {
					old[kk] = vv
				}
				continue
			}
			current[k] = v
		}
		if key == "advanced" {
			if role, _ := current["default_role"].(string); role != "" {
				if n, err := s.e.Count(ctx, upRoleUID, query.Params{Where: map[string]any{"type": role}}); err != nil {
					return err
				} else if n == 0 {
					return validation([]FieldError{{Path: []string{"default_role"}, Message: "Unknown role " + role}})
				}
			}
		}
		if err := s.store.Set(ctx, upScope, key, current); err != nil {
			return err
		}
		return data(w, current)
	}
}

// upRoutes maps each content API action to the routes it opens.
func (s *Server) upRoutes(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	out := map[string][]map[string]string{}
	if s.cfg.Routes != nil {
		for _, r := range s.cfg.Routes() {
			if r.Action == "" {
				continue
			}
			out[r.Action] = append(out[r.Action], map[string]string{"method": r.Method, "path": r.Path})
		}
	}
	return data(w, out)
}
