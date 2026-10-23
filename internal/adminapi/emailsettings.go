package adminapi

import (
	"net/http"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/plugins/email"
)

func (s *Server) emailRoutes() {
	s.allowed("GET /email/settings", "plugin::email.settings.read", s.emailSettings)
	s.allowed("POST /email/test", "plugin::email.settings.read", s.emailTest)
}

func (s *Server) emailSettings(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	out := map[string]any{"provider": s.cfg.EmailProvider, "configured": s.cfg.Email != nil, "defaultFrom": "", "defaultReplyTo": ""}
	if s.cfg.Email != nil {
		out["defaultFrom"], out["defaultReplyTo"] = s.cfg.Email.Settings.DefaultFrom, s.cfg.Email.Settings.DefaultReplyTo
	}
	return data(w, out)
}

// emailTest sends the email plugin's test message; the provider's error is
// shown, it is what the admin wants to read.
func (s *Server) emailTest(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	var b struct {
		To string `json:"to"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	if errs := checkEmail("to", normalizeEmail(b.To)); errs != nil {
		return validation(errs)
	}
	if s.cfg.Email == nil {
		return appError("No email provider is configured")
	}
	err := s.cfg.Email.Send(r.Context(), email.Message{To: b.To, Subject: "grapi Email Test", Text: "This is a test email sent from the grapi admin.\nYour email provider works."})
	if err != nil {
		return appError(err.Error())
	}
	return data(w, map[string]any{"sent": true})
}
