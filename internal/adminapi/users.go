package adminapi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
)

const (
	userUID       = "admin::user"
	roleUID       = "admin::role"
	permissionUID = "admin::permission"
	// SuperAdminCode is the code of the Super Admin role.
	SuperAdminCode = "strapi-super-admin"
	editorCode     = "strapi-editor"
	authorCode     = "strapi-author"
)

// Role is an admin role.
type Role struct {
	ID          any    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

// User is an admin user, as the API shows it.
type User struct {
	ID               any    `json:"id"`
	DocumentID       string `json:"documentId,omitempty"`
	Firstname        string `json:"firstname"`
	Lastname         string `json:"lastname"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	IsActive         bool   `json:"isActive"`
	Blocked          bool   `json:"blocked"`
	PreferedLanguage string `json:"preferedLanguage"`
	Roles            []Role `json:"roles"`
	CreatedAt        any    `json:"createdAt"`
	UpdatedAt        any    `json:"updatedAt"`

	password  string
	sessionID string
}

// IsSuperAdmin reports the Super Admin role.
func (u *User) IsSuperAdmin() bool {
	for _, r := range u.Roles {
		if r.Code == SuperAdminCode {
			return true
		}
	}
	return false
}

func get(r query.Row, k string) any {
	v, _ := r.Get(k)
	return v
}

func getString(r query.Row, k string) string {
	switch v := get(r, k).(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func getBool(r query.Row, k string) bool {
	switch v := get(r, k).(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case float64:
		return v != 0
	}
	return false
}

func rows(v any) []query.Row {
	switch x := v.(type) {
	case []query.Row:
		return x
	case []any:
		var out []query.Row
		for _, e := range x {
			if r, ok := e.(query.Row); ok {
				out = append(out, r)
			}
		}
		return out
	}
	return nil
}

func roleFromRow(r query.Row) Role {
	return Role{ID: get(r, "id"), Name: getString(r, "name"), Code: getString(r, "code"), Description: getString(r, "description")}
}

func userFromRow(r query.Row) *User {
	u := &User{
		ID: get(r, "id"), DocumentID: getString(r, "documentId"), Firstname: getString(r, "firstname"), Lastname: getString(r, "lastname"),
		Username: getString(r, "username"), Email: getString(r, "email"), IsActive: getBool(r, "isActive"), Blocked: getBool(r, "blocked"),
		PreferedLanguage: getString(r, "preferedLanguage"), CreatedAt: get(r, "createdAt"), UpdatedAt: get(r, "updatedAt"),
		password: getString(r, "password"), Roles: []Role{},
	}
	for _, role := range rows(get(r, "roles")) {
		u.Roles = append(u.Roles, roleFromRow(role))
	}
	return u
}

func (s *Server) findUser(ctx context.Context, where map[string]any) (*User, error) {
	r, err := s.e.FindOne(ctx, userUID, query.Params{Where: where, Populate: map[string]any{"roles": true}})
	if err != nil || r == nil {
		return nil, err
	}
	return userFromRow(r), nil
}

func (s *Server) countUsers(ctx context.Context) (int64, error) {
	return s.e.Count(ctx, userUID, query.Params{})
}

func jsonValue(v any, out any) {
	switch x := v.(type) {
	case nil:
	case string:
		_ = json.Unmarshal([]byte(x), out)
	default:
		b, _ := json.Marshal(x)
		_ = json.Unmarshal(b, out)
	}
}

// Permission is an admin permission, as the API shows it.
type Permission struct {
	ID         any            `json:"id,omitempty"`
	Action     string         `json:"action"`
	Subject    *string        `json:"subject"`
	Properties map[string]any `json:"properties"`
	Conditions []string       `json:"conditions"`
}

func permissionFromRow(r query.Row) Permission {
	p := Permission{ID: get(r, "id"), Action: getString(r, "action"), Properties: map[string]any{}, Conditions: []string{}}
	if subj := getString(r, "subject"); subj != "" {
		p.Subject = &subj
	}
	jsonValue(get(r, "properties"), &p.Properties)
	jsonValue(get(r, "conditions"), &p.Conditions)
	if p.Properties == nil {
		p.Properties = map[string]any{}
	}
	if p.Conditions == nil {
		p.Conditions = []string{}
	}
	return p
}

func stringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (p Permission) engine() permissions.Permission {
	ep := permissions.Permission{Action: p.Action, Conditions: p.Conditions}
	if p.Subject != nil {
		ep.Subject = *p.Subject
	}
	if f, ok := p.Properties["fields"]; ok && f != nil {
		ep.Fields = stringList(f)
	}
	if l, ok := p.Properties["locales"]; ok && l != nil {
		ep.Locales = stringList(l)
	}
	return ep
}

func (s *Server) rolePermissions(ctx context.Context, roleIDs []any) ([]Permission, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	list, err := s.e.FindMany(ctx, permissionUID, query.Params{Where: map[string]any{"role": map[string]any{"id": map[string]any{"$in": roleIDs}}}, OrderBy: []any{map[string]any{"id": "asc"}}})
	if err != nil {
		return nil, err
	}
	out := make([]Permission, len(list))
	for i, r := range list {
		out[i] = permissionFromRow(r)
	}
	return out, nil
}

// permissionsOf are the permissions of u; the Super Admin gets every
// registered action.
func (s *Server) permissionsOf(ctx context.Context, u *User) ([]Permission, error) {
	if u.IsSuperAdmin() {
		return s.allPermissions(), nil
	}
	ids := make([]any, len(u.Roles))
	for i, r := range u.Roles {
		ids[i] = r.ID
	}
	return s.rolePermissions(ctx, ids)
}

// Ability is what u may do.
func (s *Server) Ability(ctx context.Context, u *User) (*permissions.Ability, error) {
	if u.IsSuperAdmin() {
		return permissions.SuperAdmin(), nil
	}
	perms, err := s.permissionsOf(ctx, u)
	if err != nil {
		return nil, err
	}
	ep := make([]permissions.Permission, len(perms))
	for i, p := range perms {
		ep[i] = p.engine()
	}
	pu := permissions.User{ID: u.ID}
	for _, r := range u.Roles {
		pu.RoleIDs = append(pu.RoleIDs, r.ID)
	}
	return permissions.Build(ep, pu, s.conditions), nil
}
