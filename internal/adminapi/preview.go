package adminapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/httpapi"
)

// PreviewRequest is what a Go preview handler gets: the document as the
// edit view shows it, and a token that reads it from the content API.
type PreviewRequest struct {
	UID, DocumentID, Locale, Status string
	Document                        map[string]any
	Token                           string
}

// PreviewFunc builds a preview URL; "" means no preview for this document.
type PreviewFunc func(ctx context.Context, req PreviewRequest) (string, error)

// Preview is admin.preview: URL templates by content type, Go handlers
// first, and the origins the admin may frame.
type Preview struct {
	Enabled        bool
	AllowedOrigins []string
	Handlers       map[string]string
	Funcs          map[string]PreviewFunc
	// Secret signs the preview tokens (the admin JWT secret).
	Secret string
}

func (s *Server) previewRoutes() {
	s.authed("GET /preview/url/{uid}", s.previewURL)
}

var previewPlaceholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func scalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool, float64, float32, int, int64, int32:
		return fmt.Sprint(x), true
	case fmt.Stringer:
		return x.String(), true
	}
	return "", false
}

// previewURL answers the preview URL of a document, 204 when its content
// type has none.
func (s *Server) previewURL(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, _, filter, err := s.cmAccess(r, u, actionRead)
	if err != nil {
		return err
	}
	p := s.cfg.Preview
	uid := string(ct.UID)
	fn, template := p.Funcs[uid], p.Handlers[uid]
	if !p.Enabled || (fn == nil && template == "") {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	q := r.URL.Query()
	id, status := q.Get("documentId"), q.Get("status")
	if status != "published" {
		status = "draft"
	}
	params := localeParam(ct, q.Get("locale"))
	locale, _ := params["locale"].(string)
	doc := map[string]any{}
	if ct.IsSingleType() || id != "" {
		if id != "" {
			ok, err := s.allows(r.Context(), ct, id, params["locale"], filter)
			if err != nil {
				return err
			}
			if !ok {
				return errNotFound
			}
		}
		if ct.DraftAndPublish() {
			params["status"] = status
		}
		var row any
		if id != "" {
			row, err = s.docs.FindOne(r.Context(), uid, id, params)
		} else {
			row, err = s.docs.FindFirst(r.Context(), uid, params)
		}
		if err != nil {
			return asValidation(err)
		}
		if row == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		doc = toMap(row)
		if v, ok := doc["documentId"].(string); ok {
			id = v
		}
	}
	// The token is signed for a 10 minute window, not the instant: the URL
	// stays the same between two saves, and the frame gets a message
	// instead of a reload. It lives 50 minutes at least.
	token, err := httpapi.SignPreviewToken(p.Secret, uid, id, time.Now().Truncate(10*time.Minute))
	if err != nil {
		return err
	}
	var out string
	if fn != nil {
		if out, err = fn(r.Context(), PreviewRequest{UID: uid, DocumentID: id, Locale: locale, Status: status, Document: doc, Token: token}); err != nil {
			return err
		}
	} else {
		known := map[string]string{"documentId": id, "locale": locale, "status": status, "uid": uid, "previewToken": token}
		out = previewPlaceholder.ReplaceAllStringFunc(template, func(m string) string {
			name := m[1 : len(m)-1]
			if v, ok := known[name]; ok {
				return url.PathEscape(v)
			}
			if v, ok := scalar(doc[name]); ok {
				return url.PathEscape(v)
			}
			return ""
		})
	}
	if out == "" {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	if !originAllowed(out, p.AllowedOrigins) {
		return appError(fmt.Sprintf("The preview URL is not on an origin of admin.preview.allowedOrigins: %s", origin(out)))
	}
	return data(w, map[string]any{"url": out, "origin": origin(out)})
}

func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func originAllowed(raw string, allowed []string) bool {
	o := origin(raw)
	if o == "" {
		return false
	}
	for _, a := range allowed {
		if strings.TrimRight(a, "/") == o {
			return true
		}
	}
	return false
}

// FrameSources is the CSP frame-src the preview needs: the admin itself
// (relations open in a dialog) and the preview origins.
func (s *Server) FrameSources() []string {
	if !s.cfg.Preview.Enabled || len(s.cfg.Preview.AllowedOrigins) == 0 {
		return nil
	}
	return append([]string{"'self'"}, s.cfg.Preview.AllowedOrigins...)
}
