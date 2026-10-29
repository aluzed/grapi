package httpapi

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// maxBodyRead bounds what a handler reads; strapi::body has already
// refused the bodies over their limit.
const maxBodyRead = 256 << 20

// BodyLimits are strapi::body's koa-body limits, in bytes. MaxFileSize is
// formidable's maxFileSize: the file data of one multipart request.
type BodyLimits struct{ JSON, Form, Text, MaxFileSize int64 }

// DefaultBodyLimits are koa-body's: 1mb of JSON, 56kb of form or text.
var DefaultBodyLimits = BodyLimits{JSON: 1 << 20, Form: 56 << 10, Text: 56 << 10, MaxFileSize: 200 << 20}

// parseBytes reads the bytes package's sizes: a number of bytes, or
// "56kb", "1mb", "1.5gb".
func parseBytes(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case float64:
		return int64(x), true
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		units := []struct {
			suffix string
			mult   float64
		}{{"tb", 1 << 40}, {"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10}, {"b", 1}}
		for _, u := range units {
			if strings.HasSuffix(s, u.suffix) {
				f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
				return int64(f * u.mult), err == nil
			}
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	}
	return 0, false
}

func parseBodyLimits(cfg map[string]any) *BodyLimits {
	l := DefaultBodyLimits
	for key, dst := range map[string]*int64{"jsonLimit": &l.JSON, "formLimit": &l.Form, "textLimit": &l.Text} {
		if n, ok := parseBytes(cfg[key]); ok {
			*dst = n
		}
	}
	if f, ok := cfg["formidable"].(map[string]any); ok {
		if n, ok := parseBytes(f["maxFileSize"]); ok && n > 0 {
			l.MaxFileSize = n
		}
	}
	return &l
}

func isJSON(mt string) bool {
	return mt == "application/json" || mt == "application/json-patch+json" || mt == "application/vnd.api+json" || mt == "application/csp-report" || strings.HasSuffix(mt, "+json")
}

// readJSONBody reads the body only when it is declared as JSON, as koa-body
// parses it: any other body is left unread, so it never escapes the JSON
// limit.
func readJSONBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if !isJSON(mt) {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(r.Body, maxBodyRead))
}

func (l BodyLimits) of(contentType string) int64 {
	mt, _, _ := mime.ParseMediaType(contentType)
	switch {
	case isJSON(mt):
		return l.JSON
	case mt == "application/x-www-form-urlencoded":
		return l.Form
	case strings.HasPrefix(mt, "text/"):
		return l.Text
	}
	return -1
}

// limitBody answers koa-body's 413 when a JSON, form or text body is over
// its limit, before routing and authentication.
func (l BodyLimits) limitBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return false
	}
	limit := l.of(r.Header.Get("Content-Type"))
	if limit < 0 {
		return false
	}
	tooLarge := r.ContentLength > limit
	if !tooLarge {
		raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
		if err != nil {
			return false
		}
		tooLarge = int64(len(raw)) > limit
		r.Body = io.NopCloser(bytes.NewReader(raw))
	}
	if tooLarge {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"data": nil, "error": map[string]any{
			"status": http.StatusRequestEntityTooLarge, "name": "PayloadTooLargeError", "message": "request entity too large",
		}})
		return true
	}
	return false
}

type maxFileSizeKey struct{}

func withMaxFileSize(r *http.Request, n int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), maxFileSizeKey{}, n))
}

func maxFileSizeOf(ctx context.Context) int64 {
	if n, ok := ctx.Value(maxFileSizeKey{}).(int64); ok {
		return n
	}
	return DefaultBodyLimits.MaxFileSize
}

// errFileTooBig is Strapi's answer when formidable's maxFileSize is
// exceeded while the files stream in.
var errFileTooBig = newError(http.StatusRequestEntityTooLarge, "PayloadTooLargeError", "FileTooBig")
