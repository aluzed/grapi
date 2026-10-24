package adminapi

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // logos: DecodeConfig must know these formats
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/store"
)

var adminScope = store.Scope{Type: "core", Name: "admin"}

var logoInputs = []string{"menuLogo", "authLogo"}

const (
	maxLogoSize = 1024 * 1024
	maxLogoSide = 750
)

// logoTypes give the stored file its extension: never the uploaded name's,
// which could make a logo an HTML or a script file served by the admin.
var logoTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/svg+xml": ".svg"}

// logoContentMatches checks the file is what its type says: a decodable
// PNG or JPEG, or a document with an <svg> element.
func logoContentMatches(path, mime string) (image.Config, bool) {
	f, err := os.Open(path) //nolint:gosec // a temporary upload file
	if err != nil {
		return image.Config{}, false
	}
	defer func() { _ = f.Close() }()
	if mime == "image/svg+xml" {
		head := make([]byte, 4096)
		n, _ := io.ReadFull(f, head)
		return image.Config{}, strings.Contains(strings.ToLower(string(head[:n])), "<svg")
	}
	cfg, format, err := image.DecodeConfig(f)
	return cfg, err == nil && "image/"+format == mime
}

func (s *Server) projectRoutes() {
	if s.cfg.Media == nil {
		return
	}
	s.allowed("GET /project-settings", "admin::project-settings.read", s.getProjectSettings)
	s.allowed("POST /project-settings", "admin::project-settings.update", s.updateProjectSettings)
}

func (s *Server) projectSettings(ctx context.Context) (map[string]map[string]any, error) {
	stored := map[string]map[string]any{}
	if _, err := s.store.Get(ctx, adminScope, "project-settings", &stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// publicLogos is getProjectSettings: each logo's name, url, width, height,
// ext and size, or null.
func publicLogos(stored map[string]map[string]any) map[string]any {
	out := map[string]any{}
	for _, in := range logoInputs {
		f := stored[in]
		if f == nil {
			out[in] = nil
			continue
		}
		pick := map[string]any{}
		for _, k := range []string{"name", "url", "width", "height", "ext", "size"} {
			pick[k] = f[k]
		}
		out[in] = pick
	}
	return out
}

func (s *Server) getProjectSettings(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	stored, err := s.projectSettings(r.Context())
	if err != nil {
		return err
	}
	return data(w, publicLogos(stored))
}

// logoURLs are the logos the login page and the menu show, before sign in.
func (s *Server) logoURLs(ctx context.Context) map[string]any {
	out := map[string]any{"menuLogo": nil, "authLogo": nil}
	stored, err := s.projectSettings(ctx)
	if err != nil {
		return out
	}
	for _, in := range logoInputs {
		if f := stored[in]; f != nil {
			out[in] = f["url"]
		}
	}
	return out
}

// updateProjectSettings takes the logos as multipart files; a text value
// for a logo resets it. The checks are Strapi's: JPEG, PNG or SVG, 1 MB
// and 750 x 750 at most.
func (s *Server) updateProjectSettings(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	r.Body = http.MaxBytesReader(w, r.Body, 3*maxLogoSize)
	if err := r.ParseMultipartForm(3 * maxLogoSize); err != nil { //nolint:gosec // the body is bounded by MaxBytesReader above
		return validation([]FieldError{{Path: []string{"files"}, Message: "the logos must be sent as multipart form data of 2 MB at most"}})
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	ctx := r.Context()
	previous, err := s.projectSettings(ctx)
	if err != nil {
		return err
	}
	next := map[string]map[string]any{}
	for k, v := range previous {
		next[k] = v
	}
	var errs []FieldError
	type upload struct{ name, path, mime, filename string }
	var uploads []upload
	for _, in := range logoInputs {
		if fhs := r.MultipartForm.File[in]; len(fhs) > 0 {
			fh := fhs[0]
			mime := fh.Header.Get("Content-Type")
			ext, known := logoTypes[mime]
			if !known {
				errs = append(errs, FieldError{Path: []string{in, "mimetype"}, Message: "Invalid enum value. Expected 'image/jpeg' | 'image/png' | 'image/svg+xml'"})
				continue
			}
			if fh.Size > maxLogoSize {
				errs = append(errs, FieldError{Path: []string{in, "size"}, Message: fmt.Sprintf("Number must be less than or equal to %d", maxLogoSize)})
				continue
			}
			src, err := fh.Open()
			if err != nil {
				return err
			}
			tmp, err := os.CreateTemp("", "grapi-logo-*")
			if err != nil {
				_ = src.Close()
				return err
			}
			_, err = io.Copy(tmp, src)
			_ = src.Close()
			_ = tmp.Close()
			defer func() { _ = os.Remove(tmp.Name()) }()
			if err != nil {
				return err
			}
			cfg, ok := logoContentMatches(tmp.Name(), mime)
			if !ok {
				errs = append(errs, FieldError{Path: []string{in, "mimetype"}, Message: "The file content does not match its type"})
				continue
			}
			if cfg.Width > maxLogoSide || cfg.Height > maxLogoSide {
				errs = append(errs, FieldError{Path: []string{in, "width"}, Message: fmt.Sprintf("Number must be less than or equal to %d", maxLogoSide)})
				continue
			}
			name := strings.TrimSuffix(filepath.Base(fh.Filename), filepath.Ext(fh.Filename)) + ext
			uploads = append(uploads, upload{name: in, path: tmp.Name(), mime: mime, filename: name})
		} else if _, reset := r.MultipartForm.Value[in]; reset {
			next[in] = nil
		}
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	for _, u := range uploads {
		f, err := s.cfg.Media.StoreProjectFile(ctx, u.path, u.filename, u.mime)
		if err != nil {
			return err
		}
		next[u.name] = f
	}
	for _, in := range logoInputs {
		old := previous[in]
		if old != nil && (next[in] == nil || next[in]["hash"] != old["hash"]) {
			if err := s.cfg.Media.DeleteProjectFile(ctx, old); err != nil {
				s.log.Warn("project settings: the previous logo could not be deleted", "input", in, "error", err)
			}
		}
	}
	if err := s.store.Set(ctx, adminScope, "project-settings", next); err != nil {
		return err
	}
	return data(w, publicLogos(next))
}
