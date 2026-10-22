// Package runner sends a corpus to one target and records normalized responses.
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/compat/corpus"
	"github.com/aluzed/grapi/compat/normalize"
)

// Result is what gets compared between targets for one case.
type Result struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType,omitempty"`
	Body        any    `json:"body"`
	Error       string `json:"error,omitempty"`
	Skipped     string `json:"skipped,omitempty"`
}

// Target is one engine under test.
type Target struct {
	BaseURL string
	Tokens  map[string]string
	Client  *http.Client
}

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// Run executes files in order and writes <out>/<file>/<NNN>-<case>.json
// (normalized) and <out>/raw/... (as received, for debugging).
func Run(t Target, files []corpus.File, out string) error {
	if t.Client == nil {
		t.Client = &http.Client{Timeout: 30 * time.Second}
	}
	norm := normalize.New()
	for _, f := range files {
		vars := map[string]string{}
		for i, c := range f.Cases {
			res, raw := runCase(t, c, vars)
			if res.Body != nil && res.Skipped == "" {
				res.Body = norm.Value(res.Body)
			}
			name := fmt.Sprintf("%03d-%s.json", i+1, c.Name)
			if err := writeJSON(filepath.Join(out, f.Name, name), res); err != nil {
				return err
			}
			if raw != nil {
				if err := os.WriteFile(filepath.Join(mkdir(filepath.Join(out, "raw", f.Name)), name), raw, 0o600); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func runCase(t Target, c corpus.Case, vars map[string]string) (Result, []byte) {
	if c.Skip != "" {
		return Result{Skipped: c.Skip}, nil
	}
	path, err := expand(c.Path, vars)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	query, err := expand(c.Query, vars)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	target := strings.TrimRight(t.BaseURL, "/") + path
	if query != "" {
		target += "?" + encodeQuery(query)
	}
	var body io.Reader
	contentType := ""
	switch {
	case len(c.Files) > 0:
		b, ct, err := multipartBody(c.Files)
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		body, contentType = bytes.NewReader(b), ct
	case c.Body != nil:
		expanded, err := expandValue(c.Body, vars)
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		if m, ok := expanded.(map[string]any); ok && c.Pad > 0 {
			m["padding"] = strings.Repeat("x", c.Pad)
		}
		b, err := json.Marshal(expanded)
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		body, contentType = bytes.NewReader(b), "application/json"
	}
	req, err := http.NewRequest(c.Method, target, body)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != "" {
		key, ok := t.Tokens[c.Token]
		if !ok {
			return Result{Error: fmt.Sprintf("unknown token %q", c.Token)}, nil
		}
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if c.Bearer != "" {
		bearer, err := expand(c.Bearer, vars)
		if err != nil {
			return Result{Error: err.Error()}, nil
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if c.Method != "" && c.Method != http.MethodGet {
		// Writes in the same millisecond tie on createdAt, and the tie order
		// is the engine's: keep the timestamps distinct.
		defer time.Sleep(2 * time.Millisecond)
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{Status: resp.StatusCode, Error: err.Error()}, nil
	}
	res := Result{Status: resp.StatusCode, ContentType: mediaType(resp.Header.Get("Content-Type"))}
	if len(raw) > 0 {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			res.Body = string(raw)
		} else {
			res.Body = decoded
			for name, p := range c.Save {
				if v, ok := lookup(decoded, p); ok {
					vars[name] = v
				}
			}
		}
	}
	return res, raw
}

// encodeQuery escapes the values of a qs-style query written in clear in the
// corpus; brackets in keys stay literal, as browsers and qs clients send them.
func encodeQuery(q string) string {
	pairs := strings.Split(q, "&")
	for i, p := range pairs {
		k, v, found := strings.Cut(p, "=")
		k = strings.ReplaceAll(url.QueryEscape(k), "%5B", "[")
		k = strings.ReplaceAll(k, "%5D", "]")
		k = strings.ReplaceAll(k, "%24", "$")
		if found {
			pairs[i] = k + "=" + url.QueryEscape(v)
		} else {
			pairs[i] = k
		}
	}
	return strings.Join(pairs, "&")
}

func multipartBody(files []corpus.Upload) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range files {
		field := f.Field
		if field == "" {
			field = "files"
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, f.Name))
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if err := png.Encode(part, gradient(f.Width, f.Height)); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// gradient is a deterministic image; a flat color would make every resized
// format collapse to a few bytes, which hides size handling.
func gradient(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / width), G: uint8(y * 255 / height), B: 128, A: 255}) //nolint:gosec // x < width, y < height: always below 255
		}
	}
	return img
}

func mediaType(ct string) string {
	mt, _, _ := strings.Cut(ct, ";")
	return strings.TrimSpace(mt)
}

func expand(s string, vars map[string]string) (string, error) {
	var missing string
	out := placeholder.ReplaceAllStringFunc(s, func(m string) string {
		name := placeholder.FindStringSubmatch(m)[1]
		v, ok := vars[name]
		if !ok {
			missing = name
		}
		return v
	})
	if missing != "" {
		return "", fmt.Errorf("undefined variable %q", missing)
	}
	return out, nil
}

func expandValue(v any, vars map[string]string) (any, error) {
	switch t := v.(type) {
	case string:
		// A value that is exactly one placeholder of a numeric var stays a number.
		if m := placeholder.FindStringSubmatch(t); m != nil && m[0] == t {
			if val, ok := vars[m[1]]; ok {
				if n, err := strconv.ParseInt(val, 10, 64); err == nil {
					return n, nil
				}
			}
		}
		return expand(t, vars)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			e, err := expandValue(child, vars)
			if err != nil {
				return nil, err
			}
			out[k] = e
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			e, err := expandValue(child, vars)
			if err != nil {
				return nil, err
			}
			out[i] = e
		}
		return out, nil
	}
	return v, nil
}

// lookup resolves a dotted path such as "data.0.documentId".
func lookup(v any, path string) (string, bool) {
	for _, part := range strings.Split(path, ".") {
		switch t := v.(type) {
		case map[string]any:
			v = t[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(t) {
				return "", false
			}
			v = t[i]
		default:
			return "", false
		}
	}
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	default:
		return fmt.Sprint(t), true
	}
}

func mkdir(dir string) string {
	_ = os.MkdirAll(dir, 0o750)
	return dir
}

func writeJSON(path string, v any) error {
	mkdir(filepath.Dir(path))
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
