package grapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

// Requests keep being answered while develop reloads a changed schema: by
// the old version or the new one, or 503 with Retry-After, never an error.
func TestDevelopReloadUnderLoad(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "0")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.OnBootstrap(func(ctx context.Context) error {
		role, err := app.Documents().FindFirst(ctx, "plugin::users-permissions.role", document.Params{"filters": map[string]any{"type": "public"}})
		if err != nil || role == nil {
			return err
		}
		id, _ := role.Get("id")
		found, err := app.Documents().FindFirst(ctx, "plugin::users-permissions.permission", document.Params{"filters": map[string]any{"action": "api::article.article.find"}})
		if err != nil || found != nil {
			return err
		}
		_, err = app.Documents().Create(ctx, "plugin::users-permissions.permission", document.Params{"data": map[string]any{"action": "api::article.article.find", "role": id}})
		return err
	})
	addr := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- app.Develop(func(a string) { addr <- a }) }()
	var base string
	select {
	case a := <-addr:
		base = "http://" + strings.TrimPrefix(strings.TrimPrefix(a, "http://"), "https://")
	case err := <-done:
		t.Fatalf("develop: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("develop did not listen")
	}
	defer func() {
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		<-done
	}()

	stop := make(chan struct{})
	var mu sync.Mutex
	counts := map[string]int{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				res, err := http.Get(base + "/api/articles")
				key := ""
				switch {
				case err != nil:
					key = "error: " + err.Error()
				case res.StatusCode == 503 && res.Header.Get("Retry-After") != "":
					key = "503"
				default:
					key = res.Status
				}
				if res != nil {
					_, _ = io.Copy(io.Discard, res.Body)
					_ = res.Body.Close()
				}
				mu.Lock()
				counts[key]++
				mu.Unlock()
			}
		}()
	}
	time.Sleep(300 * time.Millisecond)
	path := filepath.Join(dir, "src", "api", "article", "content-types", "article", "schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	_ = json.Unmarshal(raw, &s)
	s["attributes"].(map[string]any)["subtitle"] = map[string]any{"type": "string"}
	out, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	reloaded := false
	for time.Now().Before(deadline) && !reloaded {
		time.Sleep(200 * time.Millisecond)
		res, err := http.Get(base + "/api/articles?fields[0]=subtitle")
		if err == nil {
			_ = res.Body.Close()
			reloaded = res.StatusCode == 200
		}
	}
	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
	if !reloaded {
		t.Fatal("the schema change was not picked up")
	}
	for key, n := range counts {
		if key != "200 OK" && key != "503" {
			t.Errorf("%d responses %s", n, key)
		}
	}
	t.Logf("responses during the reload: %v", counts)
}
