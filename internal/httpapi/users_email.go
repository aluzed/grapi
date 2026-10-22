package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/plugins/email"
)

type upEmailTemplate struct {
	Options struct {
		From struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"from"`
		ResponseEmail string `json:"response_email"`
		Object        string `json:"object"`
		Message       string `json:"message"`
	} `json:"options"`
}

type upEmailSettings struct {
	ResetPassword     upEmailTemplate `json:"reset_password"`
	EmailConfirmation upEmailTemplate `json:"email_confirmation"`
}

type upAdvancedFull struct {
	upAdvanced
	EmailResetPassword           string `json:"email_reset_password"`
	EmailConfirmationRedirection string `json:"email_confirmation_redirection"`
}

func (s *Server) addUsersEmailRoutes() {
	base := strings.TrimPrefix(s.cfg.Prefix, "/")
	auth := "plugin::users-permissions.auth"
	s.routes = append(s.routes,
		route{"POST", []string{base, "auth", "forgot-password"}, auth, "forgotPassword", s.authForgotPassword, "", false, false, nil},
		route{"POST", []string{base, "auth", "reset-password"}, auth, "resetPassword", s.authResetPassword, "", false, false, nil},
		route{"GET", []string{base, "auth", "email-confirmation"}, auth, "emailConfirmation", s.authEmailConfirmation, "", false, false, nil},
		route{"POST", []string{base, "auth", "send-email-confirmation"}, auth, "sendEmailConfirmation", s.authSendEmailConfirmation, "", false, false, nil},
	)
	if s.cfg.Email != nil {
		s.routes = append(s.routes, route{"POST", []string{base, "email"}, "plugin::email.email", "send", s.emailSend, "", false, false, nil})
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// plain turns a sanitized row into template data.
func plain(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func (s *Server) sendMail(ctx context.Context, m email.Message) error {
	if s.cfg.Email == nil {
		return newError(http.StatusInternalServerError, "InternalServerError", "Internal Server Error")
	}
	return s.cfg.Email.Send(ctx, m)
}

func (s *Server) authForgotPassword(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	if err := (bodySchema{noUnknown: true, fields: []bodyField{{name: "email", required: true, email: true}}}).validate(body); err != nil {
		return err
	}
	var settings upEmailSettings
	var advanced upAdvancedFull
	if err := s.auth.store(ctx, "email", &settings); err != nil {
		return err
	}
	if err := s.auth.store(ctx, "advanced", &advanced); err != nil {
		return err
	}
	user, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"email": strings.ToLower(body["email"].(string))}})
	if err != nil {
		return err
	}
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return nil
	}
	if blocked, _ := user.Get("blocked"); blocked == true {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return nil
	}
	info := plain(s.sanitizeOutput(user, s.schemaOf(userUID), rc.auth))
	token := randomHex(64)
	opts := settings.ResetPassword.Options
	msg := email.Template(opts.Message, map[string]any{
		"URL": advanced.EmailResetPassword, "SERVER_URL": s.cfg.ServerURL, "ADMIN_URL": s.cfg.AdminURL, "USER": info, "TOKEN": token,
	})
	subject := email.Template(opts.Object, map[string]any{"USER": info})
	from := ""
	if opts.From.Email != "" || opts.From.Name != "" {
		from = opts.From.Name + " <" + opts.From.Email + ">"
	}
	if _, err := s.editUser(ctx, user, map[string]any{"resetPasswordToken": token}); err != nil {
		return err
	}
	to, _ := user.Get("email")
	if err := s.sendMail(ctx, email.Message{To: toString(to), From: from, ReplyTo: opts.ResponseEmail, Subject: subject, Text: msg, HTML: msg}); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func (s *Server) authResetPassword(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	err = bodySchema{noUnknown: true, fields: []bodyField{
		{name: "password", required: true, tests: []func(any, map[string]any) string{maxPasswordBytes}},
		{name: "passwordConfirmation", required: true, tests: []func(any, map[string]any) string{matchesPassword}},
		{name: "code", required: true},
	}}.validate(body)
	if err != nil {
		return err
	}
	user, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"resetPasswordToken": body["code"]}})
	if err != nil {
		return err
	}
	if user == nil {
		return validationError("Incorrect code provided", nil)
	}
	if _, err := s.editUser(ctx, user, map[string]any{"resetPasswordToken": nil, "password": body["password"]}); err != nil {
		return err
	}
	return s.sendAuth(w, user, rc.auth)
}

func (s *Server) authEmailConfirmation(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	q := map[string]any{}
	if v, ok := rc.query["confirmation"]; ok {
		q["confirmation"] = v
	}
	if err := (bodySchema{fields: []bodyField{{name: "confirmation", required: true}}}).validate(q); err != nil {
		return err
	}
	users, err := s.docs.E.FindMany(ctx, userUID, query.Params{Where: map[string]any{"confirmationToken": q["confirmation"]}, Limit: ptr(1)})
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return validationError("Invalid token", nil)
	}
	if _, err := s.editUser(ctx, users[0], map[string]any{"confirmed": true, "confirmationToken": nil}); err != nil {
		return err
	}
	var advanced upAdvancedFull
	if err := s.auth.store(ctx, "advanced", &advanced); err != nil {
		return err
	}
	to := advanced.EmailConfirmationRedirection
	if to == "" {
		to = "/"
	}
	http.Redirect(w, r, to, http.StatusFound)
	return nil
}

func ptr(n int) *int { return &n }

func (s *Server) authSendEmailConfirmation(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	ctx := r.Context()
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	if err := (bodySchema{fields: []bodyField{{name: "email", required: true, email: true}}}).validate(body); err != nil {
		return err
	}
	addr := body["email"].(string)
	user, err := s.docs.E.FindOne(ctx, userUID, query.Params{Where: map[string]any{"email": strings.ToLower(addr)}})
	if err != nil {
		return err
	}
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{"email": addr, "sent": true})
		return nil
	}
	if confirmed, _ := user.Get("confirmed"); confirmed == true {
		return appError("Already confirmed")
	}
	if blocked, _ := user.Get("blocked"); blocked == true {
		return appError("User blocked")
	}
	if err := s.sendConfirmationEmail(ctx, user); err != nil {
		return err
	}
	to, _ := user.Get("email")
	writeJSON(w, http.StatusOK, map[string]any{"email": to, "sent": true})
	return nil
}

// sendConfirmationEmail is the user service's: a new token, then the
// email_confirmation template.
func (s *Server) sendConfirmationEmail(ctx context.Context, user query.Row) error {
	var settings upEmailSettings
	if err := s.auth.store(ctx, "email", &settings); err != nil {
		return err
	}
	info := plain(s.sanitizeOutput(user, s.schemaOf(userUID), nil))
	token := randomHex(20)
	if _, err := s.editUser(ctx, user, map[string]any{"confirmationToken": token}); err != nil {
		return err
	}
	opts := settings.EmailConfirmation.Options
	msg := email.Template(opts.Message, map[string]any{
		"URL":        strings.TrimRight(s.cfg.ServerURL, "/") + "/" + strings.Trim(s.cfg.Prefix, "/") + "/auth/email-confirmation",
		"SERVER_URL": s.cfg.ServerURL, "ADMIN_URL": s.cfg.AdminURL, "USER": info, "CODE": token,
	})
	subject := email.Template(opts.Object, map[string]any{"USER": info})
	from := ""
	if opts.From.Email != "" && opts.From.Name != "" {
		from = opts.From.Name + " <" + opts.From.Email + ">"
	}
	to, _ := user.Get("email")
	return s.sendMail(ctx, email.Message{To: toString(to), From: from, ReplyTo: opts.ResponseEmail, Subject: subject, Text: msg, HTML: msg})
}

// emailSend is the email plugin's content API route.
func (s *Server) emailSend(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	body, _, err := readBody(r)
	if err != nil {
		return err
	}
	m := email.Message{}
	for k, dst := range map[string]*string{"to": &m.To, "from": &m.From, "cc": &m.Cc, "bcc": &m.Bcc, "replyTo": &m.ReplyTo, "subject": &m.Subject, "text": &m.Text, "html": &m.HTML} {
		*dst = toString(body[k])
	}
	if err := s.cfg.Email.Send(r.Context(), m); err != nil {
		s.log.Error("email send failed", "error", err)
		return newError(http.StatusInternalServerError, "InternalServerError", "Internal Server Error")
	}
	writeJSON(w, http.StatusOK, map[string]any{})
	return nil
}
