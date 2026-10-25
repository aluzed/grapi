package adminapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
)

func (s *Server) adminRoutes() {
	s.allowed("GET /users", "admin::users.read", s.listUsers)
	s.allowed("GET /users/{id}", "admin::users.read", s.getUser)
	s.allowed("POST /users", "admin::users.create", s.createUser)
	s.allowed("PUT /users/{id}", "admin::users.update", s.updateUser)
	s.allowed("DELETE /users/{id}", "admin::users.delete", s.deleteUser)
	s.allowed("POST /users/batch-delete", "admin::users.delete", s.batchDeleteUsers)
}

// Pagination is the page info of a list.
type Pagination struct {
	Page      int   `json:"page"`
	PageSize  int   `json:"pageSize"`
	PageCount int   `json:"pageCount"`
	Total     int64 `json:"total"`
}

func pageParams(r *http.Request) (page, size int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	size, _ = strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

var userSorts = map[string]bool{"id": true, "firstname": true, "lastname": true, "email": true, "username": true, "isActive": true, "createdAt": true}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	page, size := pageParams(r)
	where := map[string]any{}
	if q := strings.TrimSpace(r.URL.Query().Get("search")); q != "" {
		var or []any
		for _, f := range []string{"firstname", "lastname", "email", "username"} {
			or = append(or, map[string]any{f: map[string]any{"$containsi": q}})
		}
		where["$or"] = or
	}
	sortField, dir, _ := strings.Cut(r.URL.Query().Get("sort"), ":")
	if !userSorts[sortField] {
		sortField = "id"
	}
	if dir != "desc" {
		dir = "asc"
	}
	ctx := r.Context()
	total, err := s.e.Count(ctx, userUID, query.Params{Where: where})
	if err != nil {
		return err
	}
	offset := (page - 1) * size
	list, err := s.e.FindMany(ctx, userUID, query.Params{Where: where, Limit: &size, Offset: &offset, OrderBy: []any{map[string]any{sortField: dir}}, Populate: map[string]any{"roles": true}})
	if err != nil {
		return err
	}
	out := make([]*User, len(list))
	for i, row := range list {
		out[i] = userFromRow(row)
	}
	pages := int((total + int64(size) - 1) / int64(size))
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"pagination": Pagination{Page: page, PageSize: size, PageCount: pages, Total: total}}})
	return nil
}

func (s *Server) userByPath(r *http.Request) (*User, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errNotFound
	}
	u, err := s.findUser(r.Context(), map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "User does not exist"}
	}
	return u, nil
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	u, err := s.userByPath(r)
	if err != nil {
		return err
	}
	return data(w, u)
}

// roleIDs checks that every id names a role and returns them as int64.
func (s *Server) roleIDs(ctx context.Context, field string, ids []int64) ([]any, error) {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		n, err := s.e.Count(ctx, roleUID, query.Params{Where: map[string]any{"id": id}})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, validation([]FieldError{{Path: []string{field}, Message: fmt.Sprintf("Role %d does not exist", id)}})
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, me *User, _ *permissions.Ability) error {
	var b struct {
		Firstname string  `json:"firstname"`
		Lastname  string  `json:"lastname"`
		Email     string  `json:"email"`
		Roles     []int64 `json:"roles"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	b.Email = normalizeEmail(b.Email)
	var errs []FieldError
	if strings.TrimSpace(b.Firstname) == "" {
		errs = append(errs, required("firstname"))
	}
	errs = append(errs, checkEmail("email", b.Email)...)
	if len(b.Roles) == 0 {
		errs = append(errs, FieldError{Path: []string{"roles"}, Message: "roles must have at least 1 items"})
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	ctx := r.Context()
	if other, err := s.findUser(ctx, map[string]any{"email": b.Email}); err != nil {
		return err
	} else if other != nil {
		return validation([]FieldError{{Path: []string{"email"}, Message: "Email already taken"}})
	}
	roles, err := s.roleIDs(ctx, "roles", b.Roles)
	if err != nil {
		return err
	}
	tok := token()
	row, err := s.e.Create(ctx, userUID, query.Data{
		"firstname": b.Firstname, "lastname": nullable(b.Lastname), "email": b.Email,
		"isActive": false, "blocked": false, "registrationToken": tok, "roles": roles,
	}, query.Params{})
	if err != nil {
		return err
	}
	u, err := s.findUser(ctx, map[string]any{"id": get(row, "id")})
	if err != nil {
		return err
	}
	s.audit(r, me, "user.create", map[string]any{"id": u.ID, "email": u.Email, "roles": b.Roles})
	link := strings.TrimRight(s.cfg.AdminURL, "/") + "/auth/register?registrationToken=" + tok
	s.mail(ctx, u.Email, "You have been invited to an admin panel", "You have been invited to join an administration panel.\n\nFollow this link to create your account:\n"+link)
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"user": u, "registrationToken": tok, "registrationUrl": link}})
	return nil
}

func (s *Server) superAdminRoleID(ctx context.Context) (any, error) {
	row, err := s.e.FindOne(ctx, roleUID, query.Params{Where: map[string]any{"code": SuperAdminCode}})
	if err != nil || row == nil {
		return nil, err
	}
	return get(row, "id"), nil
}

// activeSuperAdmins counts the active Super Admins other than except.
func (s *Server) activeSuperAdmins(ctx context.Context, except any) (int64, error) {
	id, err := s.superAdminRoleID(ctx)
	if err != nil || id == nil {
		return 0, err
	}
	where := map[string]any{"isActive": true, "blocked": map[string]any{"$ne": true}, "roles": map[string]any{"id": id}}
	if except != nil {
		where["id"] = map[string]any{"$ne": except}
	}
	return s.e.Count(ctx, userUID, query.Params{Where: where})
}

var errLastSuperAdmin = appError("You must have at least one active super admin")

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, me *User, _ *permissions.Ability) error {
	target, err := s.userByPath(r)
	if err != nil {
		return err
	}
	var b struct {
		profileBody
		IsActive *bool    `json:"isActive"`
		Blocked  *bool    `json:"blocked"`
		Roles    *[]int64 `json:"roles"`
		Password string   `json:"password"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	ctx := r.Context()
	set := query.Data{}
	errs, err := s.setProfile(ctx, b.profileBody, target.Email, set)
	if err != nil {
		return err
	}
	if b.Password != "" {
		errs = append(errs, checkPassword("password", b.Password)...)
	}
	if b.Roles != nil && len(*b.Roles) == 0 {
		errs = append(errs, FieldError{Path: []string{"roles"}, Message: "roles must have at least 1 items"})
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	if b.IsActive != nil {
		set["isActive"] = *b.IsActive
	}
	if b.Blocked != nil {
		set["blocked"] = *b.Blocked
	}
	if b.Roles != nil {
		roles, err := s.roleIDs(ctx, "roles", *b.Roles)
		if err != nil {
			return err
		}
		set["roles"] = roles
	}
	if b.Password != "" {
		hash, err := hashPassword(b.Password)
		if err != nil {
			return err
		}
		set["password"] = hash
	}
	if target.IsSuperAdmin() && target.IsActive && !target.Blocked {
		stillSuper := true
		if b.Roles != nil {
			super, err := s.superAdminRoleID(ctx)
			if err != nil {
				return err
			}
			stillSuper = false
			for _, id := range *b.Roles {
				if fmt.Sprint(id) == fmt.Sprint(super) {
					stillSuper = true
				}
			}
		}
		losing := !stillSuper || (b.IsActive != nil && !*b.IsActive) || (b.Blocked != nil && *b.Blocked)
		if losing {
			n, err := s.activeSuperAdmins(ctx, target.ID)
			if err != nil {
				return err
			}
			if n == 0 {
				return errLastSuperAdmin
			}
		}
	}
	if len(set) > 0 {
		if _, err := s.e.Update(ctx, userUID, map[string]any{"id": target.ID}, set, query.Params{}); err != nil {
			return err
		}
	}
	if (b.IsActive != nil && !*b.IsActive) || (b.Blocked != nil && *b.Blocked) || b.Password != "" {
		if err := s.sessions.Logout(ctx, fmt.Sprint(target.ID), ""); err != nil {
			return err
		}
	}
	if b.Password != "" {
		s.audit(r, me, "user.password.update", map[string]any{"id": target.ID})
	}
	if fields := changedFields(withoutPassword(set)); len(fields) > 0 {
		s.audit(r, me, "user.update", map[string]any{"id": target.ID, "fields": fields})
	}
	u, err := s.findUser(ctx, map[string]any{"id": target.ID})
	if err != nil {
		return err
	}
	return data(w, u)
}

func (s *Server) deleteUsers(ctx context.Context, ids []int64) ([]*User, error) {
	var targets []*User
	removesSuper := false
	exclude := []any{}
	for _, id := range ids {
		u, err := s.findUser(ctx, map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		if u == nil {
			continue
		}
		targets = append(targets, u)
		exclude = append(exclude, u.ID)
		if u.IsSuperAdmin() && u.IsActive && !u.Blocked {
			removesSuper = true
		}
	}
	if removesSuper {
		super, err := s.superAdminRoleID(ctx)
		if err != nil {
			return nil, err
		}
		n, err := s.e.Count(ctx, userUID, query.Params{Where: map[string]any{
			"isActive": true, "blocked": map[string]any{"$ne": true},
			"roles": map[string]any{"id": super}, "id": map[string]any{"$notIn": exclude},
		}})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, errLastSuperAdmin
		}
	}
	for _, u := range targets {
		if err := s.sessions.Logout(ctx, fmt.Sprint(u.ID), ""); err != nil {
			return nil, err
		}
		if _, err := s.e.Delete(ctx, userUID, map[string]any{"id": u.ID}, query.Params{}); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, me *User, _ *permissions.Ability) error {
	target, err := s.userByPath(r)
	if err != nil {
		return err
	}
	if _, err := s.deleteUsers(r.Context(), []int64{toInt64(target.ID)}); err != nil {
		return err
	}
	s.audit(r, me, "user.delete", map[string]any{"id": target.ID, "email": target.Email})
	return data(w, target)
}

func (s *Server) batchDeleteUsers(w http.ResponseWriter, r *http.Request, me *User, _ *permissions.Ability) error {
	var b struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	if len(b.IDs) == 0 {
		return validation([]FieldError{{Path: []string{"ids"}, Message: "ids must have at least 1 items"}})
	}
	deleted, err := s.deleteUsers(r.Context(), b.IDs)
	if err != nil {
		return err
	}
	if deleted == nil {
		deleted = []*User{}
	}
	for _, d := range deleted {
		s.audit(r, me, "user.delete", map[string]any{"id": d.ID, "email": d.Email})
	}
	return data(w, deleted)
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64)
	return n
}
