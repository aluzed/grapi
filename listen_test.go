package grapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aluzed/grapi/internal/config"
	"github.com/aluzed/grapi/internal/project"
)

func TestHTTPServerTimeouts(t *testing.T) {
	for conf, want := range map[string][3]time.Duration{
		"":                    {5 * time.Minute, 10 * time.Second, 5 * time.Second},
		"requestTimeout: 0\n": {0, 10 * time.Second, 5 * time.Second},
		"requestTimeout: 60000\n    headersTimeout: 2000\n    keepAliveTimeout: 1000\n": {time.Minute, 2 * time.Second, time.Second},
	} {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config", "server.yaml"), []byte("http:\n  serverOptions:\n    "+conf), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := config.Load(config.Options{Dir: dir, Env: config.Env{}})
		if err != nil {
			t.Fatal(err)
		}
		srv := (&Server{project: &project.Project{Conf: c}}).httpServer(nil)
		if got := [3]time.Duration{srv.ReadTimeout, srv.ReadHeaderTimeout, srv.IdleTimeout}; got != want {
			t.Errorf("%q: %v, want %v", conf, got, want)
		}
	}
}
