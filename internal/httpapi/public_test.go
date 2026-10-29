package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicHidesDotfiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"robots.txt", ".env", "uploads/.upload-123", ".well-known/x.txt"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{cfg: Config{Upload: UploadConfig{PublicDir: dir}}, log: slog.Default()}
	for path, want := range map[string]int{"/robots.txt": 200, "/.env": 404, "/uploads/.upload-123": 404, "/.well-known/x.txt": 404} {
		w := httptest.NewRecorder()
		s.route(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Errorf("%s = %d, want %d", path, w.Code, want)
		}
	}
}
