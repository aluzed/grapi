package grapi_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/aluzed/grapi"
)

// TestShutdownChild is the server TestGracefulShutdown runs in a process of
// its own.
func TestShutdownChild(t *testing.T) {
	dir := os.Getenv("GRAPI_SHUTDOWN_DIR")
	if dir == "" {
		t.Skip("run by TestGracefulShutdown")
	}
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.PublicRoute("GET", "/slow", func(c *grapi.Request) error {
		time.Sleep(1500 * time.Millisecond)
		_, err := c.W.Write([]byte("done"))
		return err
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
}

func TestGracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	var stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run", "^TestShutdownChild$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), "GRAPI_SHUTDOWN_DIR="+dir, "HOST=127.0.0.1", fmt.Sprintf("PORT=%d", port),
		"DATABASE_CLIENT=sqlite", "DATABASE_FILENAME="+filepath.Join(dir, "data.db"), "JWT_SECRET=test")
	cmd.Stdout, cmd.Stderr = &stderr, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	var health *http.Response
	for i := 0; ; i++ {
		if health, err = http.Head(base + "/_health"); err == nil {
			_ = health.Body.Close()
			break
		}
		if i > 100 {
			t.Fatalf("no server: %v\n%s", err, stderr.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if health.StatusCode != 204 {
		t.Fatalf("HEAD /_health: %d", health.StatusCode)
	}
	for _, h := range []string{"Server", "X-Powered-By"} {
		if v := health.Header.Get(h); v != "" {
			t.Errorf("%s names the server: %q", h, v)
		}
	}

	type result struct {
		status int
		body   string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		res, err := http.Get(base + "/api/slow")
		if err != nil {
			done <- result{err: err}
			return
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		done <- result{status: res.StatusCode, body: string(b)}
	}()
	time.Sleep(400 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil || r.status != 200 || r.body != "done" {
		t.Fatalf("the running request ends normally: %+v", r)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit after SIGTERM: %v\n%s", err, stderr.String())
	}
}

func TestUnixSocket(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	// A short path: Unix socket names are limited to about 100 bytes.
	sockDir, err := os.MkdirTemp("/tmp", "grapi")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(sockDir) }()
	socket := filepath.Join(sockDir, "s.sock")
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	conf := "socket: " + socket + "\nhttp:\n  serverOptions:\n    keepAliveTimeout: 1000\n    headersTimeout: 5000\n"
	if err := os.WriteFile(filepath.Join(dir, "config", "server.yaml"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run", "^TestShutdownChild$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), "GRAPI_SHUTDOWN_DIR="+dir,
		"DATABASE_CLIENT=sqlite", "DATABASE_FILENAME="+filepath.Join(dir, "data.db"), "JWT_SECRET=test")
	cmd.Stdout, cmd.Stderr = &stderr, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	c := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	for i := 0; ; i++ {
		res, err := c.Get("http://grapi/_health")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode != 204 {
				t.Fatalf("/_health on the socket: %d", res.StatusCode)
			}
			break
		}
		if i > 100 {
			t.Fatalf("nothing on %s: %v\n%s", socket, err, stderr.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
}
