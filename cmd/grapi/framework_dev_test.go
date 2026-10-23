package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const devMain = `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	_ = os.WriteFile("started", []byte("VERSION:"+os.Getenv("GRAPI_DEVELOP")), 0o600)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
}
`

func TestSuperviseRebuildsOnGoChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go programs")
	}
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module devtest\n\ngo 1.22\n")
	write("main.go", strings.Replace(devMain, "VERSION", "v1", 1))
	if !isFramework(dir) {
		t.Fatal("a module with a main.go is a framework project")
	}
	started := func() string {
		b, _ := os.ReadFile(filepath.Join(dir, "started"))
		return string(b)
	}
	wait := func(want string) {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		for started() != want {
			if time.Now().After(deadline) {
				t.Fatalf("waiting for %q, got %q", want, started())
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() { done <- supervise(dir, "", &out, nil, stop, 100*time.Millisecond) }()
	wait("v1:1")
	write("main.go", strings.Replace(devMain, "VERSION", "v2", 1))
	wait("v2:1")
	write("broken.go", "package main\nfunc broken( {\n")
	time.Sleep(2 * time.Second)
	if started() != "v2:1" || !strings.Contains(out.String(), "build failed") {
		t.Fatalf("a failed build keeps the running version: %q %s", started(), out.String())
	}
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
