package adminapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
)

func (s *Server) roleRoutes() {
	s.allowed("GET /roles", "admin::roles.read", s.listRoles)
	s.allowed("GET /roles/{id}", "admin::roles.read", s.getRole)
	s.allowed("POST /roles", "admin::roles.create", s.createRole)
	s.allowed("PUT /roles/{id}", "admin::roles.update", s.updateRole)
	s.allowed("DELETE /roles/{id}", "admin::roles.delete", s.deleteRole)
	s.allowed("GET /roles/{id}/permissions", "admin::roles.read", s.getRolePermissions)
	s.allowed("PUT /roles/{id}/permissions", "admin::roles.update", s.setRolePermissions)
	s.allowed("GET /permissions", "admin::roles.read", s.catalog)
}

type roleOut struct {
	Role
	UsersCount int64 `json:"usersCount"`
}

func (s *Server) roleOut(ctx context.Context, row query.Row) (roleOut, error) {
	r := roleFromRow(row)
	n, err := s.e.Count(ctx, userUID, query.Params{Where: map[string]any{"roles": map[string]any{"id": r.ID}}})
	return roleOut{Role: r, UsersCount: n}, err
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	ctx := r.Context()
	list, err := s.e.FindMany(ctx, roleUID, query.Params{OrderBy: []any{map[string]any{"id": "asc"}}})
	if err != nil {
		return err
	}
	out := make([]roleOut, 0, len(list))
	for _, row := range list {
		ro, err := s.roleOut(ctx, row)
		if err != nil {
			return err
		}
		out = append(out, ro)
	}
	return data(w, out)
}

func (s *Server) roleByPath(r *http.Request) (query.Row, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errNotFound
	}
	row, err := s.e.FindOne(r.Context(), roleUID, query.Params{Where: map[string]any{"id": id}})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Role does not exist"}
	}
	return row, nil
}

func (s *Server) getRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.roleByPath(r)
	if err != nil {
		return err
	}
	ro, err := s.roleOut(r.Context(), row)
	if err != nil {
		return err
	}
	return data(w, ro)
}

type roleBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (s *Server) checkRoleName(ctx context.Context, name string, except any) []FieldError {
	name = strings.TrimSpace(name)
	if name == "" {
		return []FieldError{{Path: []string{"name"}, Message: "name is a required field"}}
	}
	if len(name) > 255 {
		return []FieldError{{Path: []string{"name"}, Message: "name must be at most 255 characters"}}
	}
	where := map[string]any{"name": map[string]any{"$eqi": name}}
	if except != nil {
		where["id"] = map[string]any{"$ne": except}
	}
	if n, err := s.e.Count(ctx, roleUID, query.Params{Where: where}); err == nil && n > 0 {
		return []FieldError{{Path: []string{"name"}, Message: fmt.Sprintf("The name must be unique and a role with name `%s` already exists.", name)}}
	}
	return nil
}

func roleCode(name string, now time.Time) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	suffix := "-" + strconv.FormatInt(now.UnixMilli(), 36)
	prefix := strings.TrimRight(b.String(), "-")
	if len(prefix) > 255-len(suffix) {
		prefix = strings.TrimRight(prefix[:255-len(suffix)], "-")
	}
	return prefix + suffix
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	var b roleBody
	if err := decode(r, &b); err != nil {
		return err
	}
	ctx := r.Context()
	name := ""
	if b.Name != nil {
		name = strings.TrimSpace(*b.Name)
	}
	if errs := s.checkRoleName(ctx, name, nil); errs != nil {
		return validation(errs)
	}
	d := query.Data{"name": name, "code": roleCode(name, s.now())}
	if b.Description != nil {
		d["description"] = *b.Description
	}
	row, err := s.e.Create(ctx, roleUID, d, query.Params{})
	if err != nil {
		return err
	}
	ro, err := s.roleOut(ctx, row)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": ro})
	return nil
}

func (s *Server) updateRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.roleByPath(r)
	if err != nil {
		return err
	}
	var b roleBody
	if err := decode(r, &b); err != nil {
		return err
	}
	ctx := r.Context()
	id := get(row, "id")
	set := query.Data{}
	if b.Name != nil {
		if errs := s.checkRoleName(ctx, *b.Name, id); errs != nil {
			return validation(errs)
		}
		set["name"] = strings.TrimSpace(*b.Name)
	}
	if b.Description != nil {
		set["description"] = *b.Description
	}
	if len(set) > 0 {
		if row, err = s.e.Update(ctx, roleUID, map[string]any{"id": id}, set, query.Params{}); err != nil {
			return err
		}
	}
	ro, err := s.roleOut(ctx, row)
	if err != nil {
		return err
	}
	return data(w, ro)
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.roleByPath(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	ro, err := s.roleOut(ctx, row)
	if err != nil {
		return err
	}
	if ro.Code == SuperAdminCode {
		return appError("You cannot delete the super admin role")
	}
	if ro.UsersCount > 0 {
		return appError("Some roles are still assigned to some users")
	}
	if _, err := s.e.DeleteMany(ctx, permissionUID, query.Params{Where: map[string]any{"role": map[string]any{"id": ro.ID}}}); err != nil {
		return err
	}
	if _, err := s.e.Delete(ctx, roleUID, map[string]any{"id": ro.ID}, query.Params{}); err != nil {
		return err
	}
	return data(w, ro)
}

func (s *Server) getRolePermissions(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.roleByPath(r)
	if err != nil {
		return err
	}
	if getString(row, "code") == SuperAdminCode {
		return data(w, s.allPermissions())
	}
	perms, err := s.rolePermissions(r.Context(), []any{get(row, "id")})
	if err != nil {
		return err
	}
	if perms == nil {
		perms = []Permission{}
	}
	return data(w, perms)
}

// checkPermission validates p against the registries.
func (s *Server) checkPermission(i int, p Permission) []FieldError {
	at := func(field, msg string) FieldError {
		return FieldError{Path: []string{"permissions", strconv.Itoa(i), field}, Message: msg}
	}
	a, ok := s.actions.Get(p.Action)
	if !ok {
		return []FieldError{at("action", fmt.Sprintf("Unknown action %q", p.Action))}
	}
	var errs []FieldError
	switch {
	case len(a.Subjects) == 0 && p.Subject != nil:
		errs = append(errs, at("subject", "This action takes no subject"))
	case len(a.Subjects) > 0 && (p.Subject == nil || !slices.Contains(a.Subjects, *p.Subject)):
		errs = append(errs, at("subject", "Invalid subject for this action"))
	}
	for k := range p.Properties {
		if !slices.Contains(a.ApplyToProperties, k) {
			errs = append(errs, at("properties", fmt.Sprintf("This action does not take %s", k)))
		}
	}
	for _, c := range p.Conditions {
		if _, ok := s.conditions.Get(c); !ok {
			errs = append(errs, at("conditions", fmt.Sprintf("Unknown condition %q", c)))
		}
	}
	return errs
}

// setRolePermissions replaces the permissions of a role.
func (s *Server) setRolePermissions(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.roleByPath(r)
	if err != nil {
		return err
	}
	if getString(row, "code") == SuperAdminCode {
		return appError("Super admin permissions can't be edited")
	}
	var b struct {
		Permissions []Permission `json:"permissions"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	var errs []FieldError
	for i, p := range b.Permissions {
		errs = append(errs, s.checkPermission(i, p)...)
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	ctx := r.Context()
	id := get(row, "id")
	if _, err := s.e.DeleteMany(ctx, permissionUID, query.Params{Where: map[string]any{"role": map[string]any{"id": id}}}); err != nil {
		return err
	}
	for _, p := range b.Permissions {
		props := map[string]any{}
		for k, v := range p.Properties {
			props[k] = v
		}
		conds := make([]any, len(p.Conditions))
		for i, c := range p.Conditions {
			conds[i] = c
		}
		var subject any
		if p.Subject != nil {
			subject = *p.Subject
		}
		if _, err := s.e.Create(ctx, permissionUID, query.Data{
			"action": p.Action, "actionParameters": map[string]any{}, "subject": subject,
			"properties": props, "conditions": conds, "role": id,
		}, query.Params{}); err != nil {
			return err
		}
	}
	perms, err := s.rolePermissions(ctx, []any{id})
	if err != nil {
		return err
	}
	if perms == nil {
		perms = []Permission{}
	}
	return data(w, perms)
}

type actionOut struct {
	ActionID          string   `json:"actionId"`
	DisplayName       string   `json:"displayName"`
	Plugin            string   `json:"plugin"`
	Category          string   `json:"category,omitempty"`
	Subjects          []string `json:"subjects,omitempty"`
	ApplyToProperties []string `json:"applyToProperties,omitempty"`
}

type conditionOut struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Category    string `json:"category"`
}

// catalog lists the registered actions by section, and the conditions.
func (s *Server) catalog(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	sections := map[string][]actionOut{permissions.SectionContentTypes: {}, permissions.SectionPlugins: {}, permissions.SectionSettings: {}}
	for _, a := range s.actions.All() {
		if a.Section == permissions.SectionInternal {
			continue
		}
		sections[a.Section] = append(sections[a.Section], actionOut{
			ActionID: a.ActionID(), DisplayName: a.DisplayName, Plugin: a.PluginName,
			Category: a.Category, Subjects: a.Subjects, ApplyToProperties: a.ApplyToProperties,
		})
	}
	var conds []conditionOut
	for _, c := range s.conditions.All() {
		conds = append(conds, conditionOut{ID: c.ID, DisplayName: c.DisplayName, Category: c.Category})
	}
	return data(w, map[string]any{"sections": sections, "conditions": conds})
}
