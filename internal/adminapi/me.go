package adminapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
)

// Languages of the admin interface.
var Languages = []string{"en", "fr"}

func (s *Server) meRoutes() {
	s.authed("GET /users/me", func(w http.ResponseWriter, _ *http.Request, u *User) error { return data(w, u) })
	s.authed("PUT /users/me", s.updateMe)
	s.authed("GET /users/me/permissions", s.myPermissions)
	s.authed("GET /users/me/sessions", s.mySessions)
	s.authed("DELETE /users/me/sessions/{id}", s.revokeSession)
	s.authed("DELETE /users/me/sessions", s.revokeOtherSessions)
	s.authed("GET /information", s.information)
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request, u *User) error {
	var b struct {
		Firstname        *string `json:"firstname"`
		Lastname         *string `json:"lastname"`
		Username         *string `json:"username"`
		Email            *string `json:"email"`
		PreferedLanguage *string `json:"preferedLanguage"`
		CurrentPassword  string  `json:"currentPassword"`
		Password         string  `json:"password"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	set := query.Data{}
	var errs []FieldError
	if b.Firstname != nil {
		if strings.TrimSpace(*b.Firstname) == "" {
			errs = append(errs, FieldError{Path: []string{"firstname"}, Message: "firstname is a required field"})
		}
		set["firstname"] = *b.Firstname
	}
	if b.Lastname != nil {
		set["lastname"] = nullable(*b.Lastname)
	}
	if b.Username != nil {
		set["username"] = nullable(*b.Username)
	}
	ctx := r.Context()
	if b.Email != nil {
		e := normalizeEmail(*b.Email)
		if fe := checkEmail("email", e); fe != nil {
			errs = append(errs, fe...)
		} else if e != u.Email {
			other, err := s.findUser(ctx, map[string]any{"email": e})
			if err != nil {
				return err
			}
			if other != nil {
				errs = append(errs, FieldError{Path: []string{"email"}, Message: "Email already taken"})
			}
		}
		set["email"] = e
	}
	if b.PreferedLanguage != nil {
		if *b.PreferedLanguage != "" && !contains(Languages, *b.PreferedLanguage) {
			errs = append(errs, FieldError{Path: []string{"preferedLanguage"}, Message: "preferedLanguage must be one of " + strings.Join(Languages, ", ")})
		}
		set["preferedLanguage"] = nullable(*b.PreferedLanguage)
	}
	if b.Password != "" {
		errs = append(errs, checkPassword("password", b.Password)...)
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	if b.Password != "" {
		if !passwordMatches(b.CurrentPassword, u.password) {
			return validation([]FieldError{{Path: []string{"currentPassword"}, Message: "Current password is invalid"}})
		}
		hash, err := hashPassword(b.Password)
		if err != nil {
			return err
		}
		set["password"] = hash
	}
	if len(set) > 0 {
		if _, err := s.e.Update(ctx, userUID, map[string]any{"id": u.ID}, set, query.Params{}); err != nil {
			return err
		}
	}
	if b.Password != "" {
		if err := s.revokeOthers(r, u); err != nil {
			return err
		}
	}
	nu, err := s.findUser(ctx, map[string]any{"id": u.ID})
	if err != nil {
		return err
	}
	return data(w, nu)
}

func contains(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

func (s *Server) myPermissions(w http.ResponseWriter, r *http.Request, u *User) error {
	perms, err := s.permissionsOf(r.Context(), u)
	if err != nil {
		return err
	}
	if perms == nil {
		perms = []Permission{}
	}
	return data(w, perms)
}

func (s *Server) mySessions(w http.ResponseWriter, r *http.Request, u *User) error {
	list, err := s.sessions.List(r.Context(), fmt.Sprint(u.ID))
	if err != nil {
		return err
	}
	current := ""
	if cur, err := s.sessions.Get(r.Context(), u.sessionID); err == nil && cur != nil {
		current = cur.DeviceID
	}
	out := make([]map[string]any, 0, len(list))
	for _, sess := range list {
		out = append(out, map[string]any{
			"id": sess.SessionID, "deviceId": sess.DeviceID, "type": sess.Type, "createdAt": sess.CreatedAt,
			"expiresAt": sess.ExpiresAt, "metadata": sess.Metadata, "isCurrent": sess.DeviceID == current,
		})
	}
	return data(w, out)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request, u *User) error {
	ok, err := s.sessions.Revoke(r.Context(), fmt.Sprint(u.ID), r.PathValue("id"))
	if err != nil {
		return err
	}
	if !ok {
		return errNotFound
	}
	return data(w, map[string]any{})
}

// revokeOthers ends every session of u but the current one's device.
func (s *Server) revokeOthers(r *http.Request, u *User) error {
	ctx := r.Context()
	cur, err := s.sessions.Get(ctx, u.sessionID)
	if err != nil {
		return err
	}
	list, err := s.sessions.List(ctx, fmt.Sprint(u.ID))
	if err != nil {
		return err
	}
	for _, sess := range list {
		if cur != nil && sess.DeviceID == cur.DeviceID {
			continue
		}
		if err := s.sessions.Logout(ctx, fmt.Sprint(u.ID), sess.DeviceID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request, u *User) error {
	if err := s.revokeOthers(r, u); err != nil {
		return err
	}
	return data(w, map[string]any{})
}

// information is what the shell needs besides the user: menu entries and
// the content types it can show.
func (s *Server) information(w http.ResponseWriter, _ *http.Request, _ *User) error {
	return data(w, map[string]any{"apiVersion": Version, "develop": s.cfg.Develop, "languages": Languages})
}
