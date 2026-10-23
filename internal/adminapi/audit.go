package adminapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/permissions"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/schema"
)

// The audit log (a grapi design, Strapi's is Enterprise): one immutable
// row per admin write, in a table of grapi's own.
const auditTable = "grapi_audit_logs"

func (s *Server) ensureAuditTable(ctx context.Context) error {
	id := "id integer primary key autoincrement"
	if s.e.DB.Dialect.Name() == "postgres" {
		id = "id serial primary key"
	}
	q := s.e.DB.Dialect.Quote
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`create table if not exists %s (%s, action varchar(255) not null, date varchar(64) not null,
		user_id integer, user_name varchar(255), user_email varchar(255), origin varchar(64) not null, payload text not null,
		uid varchar(255), document_id varchar(255), locale varchar(255))`, q(auditTable), id)); err != nil {
		return err
	}
	rows, err := s.e.DB.Query(ctx, fmt.Sprintf(`select * from %s where 1 = 0`, q(auditTable)))
	if err != nil {
		return err
	}
	cols, err := rows.Columns()
	_ = rows.Close()
	if err != nil {
		return err
	}
	// Tables from before these columns get them on start.
	for _, col := range []string{"uid", "document_id", "locale"} {
		if slices.Contains(cols, col) {
			continue
		}
		if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`alter table %s add column %s varchar(255)`, q(auditTable), col)); err != nil {
			return err
		}
	}
	for name, cols := range map[string]string{"grapi_audit_logs_date_idx": "date", "grapi_audit_logs_document_idx": "uid, document_id"} {
		if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`create index if not exists %s on %s (%s)`, q(name), q(auditTable), cols)); err != nil {
			return err
		}
	}
	return nil
}

// audit records action by u (nil for an anonymous one, a failed login).
// The payload never holds secrets: callers pass what is safe to keep.
func (s *Server) audit(r *http.Request, u *User, action string, payload map[string]any) {
	if !s.cfg.AuditEnabled {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["ip"], payload["userAgent"], payload["method"], payload["route"] = s.cfg.Proxy.IP(r), r.UserAgent(), r.Method, r.URL.Path
	s.writeAudit(context.WithoutCancel(r.Context()), u, "admin-panel", action, payload)
}

func (s *Server) writeAudit(ctx context.Context, u *User, origin, action string, payload map[string]any) {
	if !s.cfg.AuditEnabled {
		return
	}
	b, _ := json.Marshal(payload)
	var id, name, email any
	if u != nil {
		id, email = u.ID, u.Email
		name = strings.TrimSpace(u.Firstname + " " + u.Lastname)
	}
	str := func(k string) any {
		if v, ok := payload[k]; ok && v != nil && v != "" {
			return fmt.Sprint(v)
		}
		return nil
	}
	q := fmt.Sprintf(`insert into %s (action, date, user_id, user_name, user_email, origin, payload, uid, document_id, locale) values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`,
		s.e.DB.Dialect.Quote(auditTable), s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6), s.ph(7), s.ph(8), s.ph(9), s.ph(10))
	now := s.now().UTC()
	// The write itself succeeded: an audit failure is logged, not returned.
	if _, err := s.e.DB.Exec(ctx, q, action, now.Format(storedTime), id, name, email, origin, string(b), str("uid"), str("documentId"), str("locale")); err != nil {
		s.log.Error("audit log", "action", action, "error", err)
	}
}

// AuditCLI logs an action of the grapi CLI (origin cli, no user) in the
// audit table the server created.
func AuditCLI(ctx context.Context, d *db.DB, action string, payload map[string]any) error {
	b, _ := json.Marshal(payload)
	ph := d.Dialect.Placeholder
	_, err := d.Exec(ctx, fmt.Sprintf(`insert into %s (action, date, origin, payload) values (%s, %s, 'cli', %s)`, d.Dialect.Quote(auditTable), ph(1), ph(2), ph(3)),
		action, time.Now().UTC().Format(storedTime), string(b))
	return err
}

// purgeAudit drops the entries older than the retention, at most once an
// hour; it is the only deletion of the audit log.
func (s *Server) purgeAudit(ctx context.Context) {
	days := s.cfg.AuditRetentionDays
	if days <= 0 {
		return
	}
	now := s.now().UTC()
	if now.Sub(s.auditPurgedAt) < time.Hour {
		return
	}
	s.auditPurgedAt = now
	cut := now.Add(-time.Duration(days) * 24 * time.Hour).Format(storedTime)
	if _, err := s.e.DB.Exec(ctx, fmt.Sprintf(`delete from %s where date < %s`, s.e.DB.Dialect.Quote(auditTable), s.ph(1)), cut); err != nil {
		s.log.Error("audit purge", "error", err)
	}
}

// urlHost keeps the scheme and host of a URL: its path and query may hold
// secrets.
func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// auditEntryUpdate logs the fields an update changed, with their values
// before and after; private and password attributes are left out.
func (s *Server) auditEntryUpdate(r *http.Request, u *User, ct *schema.ContentType, documentID string, locale any, before map[string]any) {
	if !s.cfg.AuditEnabled {
		return
	}
	after, _ := s.snapshot(r.Context(), ct, documentID, locale, false)
	for name, a := range ct.Attributes.All() {
		if a.IsPrivate() || a.Type == schema.TypePassword {
			delete(before, name)
			delete(after, name)
		}
	}
	changes := s.diff(before, after, &ct.Attributes, "")
	fields := []string{}
	seen := map[string]bool{}
	for _, c := range changes {
		top, _, _ := strings.Cut(c.Path, ".")
		if !seen[top] {
			seen[top] = true
			fields = append(fields, top)
		}
	}
	s.audit(r, u, "entry.update", map[string]any{"uid": string(ct.UID), "documentId": documentID, "locale": locale, "fields": fields, "changes": changes})
}

// changedFields lists the keys of an update body, never their values.
func changedFields(data map[string]any) []string {
	out := make([]string, 0, len(data))
	for k := range data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *Server) auditRoutes() {
	s.allowed("GET /audit-logs", "admin::audit-logs.read", s.listAuditLogs)
	s.allowed("GET /audit-logs/export.csv", "admin::audit-logs.export", s.exportAuditLogs)
	s.allowed("GET /audit-logs/{id}", "admin::audit-logs.read", s.getAuditLog)
}

// AuditEntry is one row of the audit log.
type AuditEntry struct {
	ID      int64          `json:"id"`
	Action  string         `json:"action"`
	Date    string         `json:"date"`
	User    map[string]any `json:"user"`
	Origin  string         `json:"origin"`
	Payload map[string]any `json:"payload,omitempty"`
}

// auditWhere builds the filters of a listing or an export.
func (s *Server) auditWhere(r *http.Request) (string, []any) {
	q := r.URL.Query()
	var parts []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		parts = append(parts, fmt.Sprintf(cond, s.ph(len(args))))
	}
	if actions := q["action"]; len(actions) > 0 {
		var ph []string
		for _, a := range actions {
			args = append(args, a)
			ph = append(ph, s.ph(len(args)))
		}
		parts = append(parts, "action in ("+strings.Join(ph, ", ")+")")
	}
	if u := q.Get("user"); u != "" {
		if n, err := strconv.ParseInt(u, 10, 64); err == nil {
			add("user_id = %s", n)
		}
	}
	for param, col := range map[string]string{"origin": "origin", "uid": "uid", "documentId": "document_id", "locale": "locale"} {
		if v := q.Get(param); v != "" {
			add(col+" = %s", v)
		}
	}
	if from := q.Get("from"); from != "" {
		add("date >= %s", from)
	}
	if to := q.Get("to"); to != "" {
		add("date <= %s", to)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " where " + strings.Join(parts, " and "), args
}

func scanAudit(scan func(...any) error, withPayload bool) (AuditEntry, error) {
	var e AuditEntry
	var uid *int64
	var name, email *string
	var payload string
	if err := scan(&e.ID, &e.Action, &e.Date, &uid, &name, &email, &e.Origin, &payload); err != nil {
		return e, err
	}
	if uid != nil {
		e.User = map[string]any{"id": *uid, "name": deref(name), "email": deref(email)}
	}
	if withPayload {
		_ = json.Unmarshal([]byte(payload), &e.Payload)
	}
	return e, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

const auditColumns = "id, action, date, user_id, user_name, user_email, origin, payload"

func (s *Server) listAuditLogs(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	page, size := pageParams(r)
	where, args := s.auditWhere(r)
	ctx := r.Context()
	var total int64
	if err := s.e.DB.QueryRow(ctx, `select count(*) from `+s.e.DB.Dialect.Quote(auditTable)+where, args...).Scan(&total); err != nil {
		return err
	}
	q := fmt.Sprintf(`select %s from %s%s order by date desc, id desc limit %d offset %d`, auditColumns, s.e.DB.Dialect.Quote(auditTable), where, size, (page-1)*size)
	rows, err := s.e.DB.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	out := []AuditEntry{}
	for rows.Next() {
		e, err := scanAudit(rows.Scan, false)
		if err != nil {
			return err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	pages := int((total + int64(size) - 1) / int64(size))
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"pagination": Pagination{Page: page, PageSize: size, PageCount: pages, Total: total}}})
	return nil
}

func (s *Server) getAuditLog(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return errNotFound
	}
	q := fmt.Sprintf(`select %s from %s where id = %s`, auditColumns, s.e.DB.Dialect.Quote(auditTable), s.ph(1))
	e, err := scanAudit(s.e.DB.QueryRow(r.Context(), q, id).Scan, true)
	if err != nil {
		return &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Audit log not found"}
	}
	return data(w, e)
}

// exportAuditLogs streams the filtered rows as CSV, and is audited itself.
func (s *Server) exportAuditLogs(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	where, args := s.auditWhere(r)
	q := fmt.Sprintf(`select %s from %s%s order by date desc, id desc limit %d`, auditColumns, s.e.DB.Dialect.Quote(auditTable), where, s.cfg.AuditExportMaxRows)
	rows, err := s.e.DB.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "action", "origin", "user_id", "user_email", "uid", "document_id", "locale", "payload_json"})
	for rows.Next() {
		e, err := scanAudit(rows.Scan, true)
		if err != nil {
			return err
		}
		userID, email := "", ""
		if e.User != nil {
			userID, email = fmt.Sprint(e.User["id"]), fmt.Sprint(e.User["email"])
		}
		str := func(k string) string {
			if v, ok := e.Payload[k]; ok && v != nil {
				return fmt.Sprint(v)
			}
			return ""
		}
		payload, _ := json.Marshal(e.Payload)
		_ = cw.Write([]string{e.Date, e.Action, e.Origin, userID, email, str("uid"), str("documentId"), str("locale"), string(payload)})
	}
	cw.Flush()
	s.audit(r, u, "audit-log.export", map[string]any{"filters": r.URL.RawQuery})
	return rows.Err()
}
