package adminapi

import (
	"net/http"

	"github.com/aluzed/grapi/internal/auth/permissions"
)

var errForbidden = &Error{Status: http.StatusForbidden, Name: "ForbiddenError", Message: "Forbidden"}

type permittedHandler func(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error

// allowed registers a route that needs action (an admin:: or plugin::
// action id, subject-less).
func (s *Server) allowed(pattern, action string, h permittedHandler) {
	s.authed(pattern, func(w http.ResponseWriter, r *http.Request, u *User) error {
		a, err := s.Ability(r.Context(), u)
		if err != nil {
			return err
		}
		if !a.Can(action, "", "") {
			return errForbidden
		}
		return h(w, r, u, a)
	})
}
