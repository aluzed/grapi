package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// isFramework reports a framework project: a Go module whose main package
// sits next to src/ (grapi new --framework).
func isFramework(dir string) bool {
	for _, f := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return false
		}
	}
	return true
}

// goSnapshot is the modification state of the module's Go sources.
func goSnapshot(dir string) string {
	var sum int64
	var n int
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a vanished file is a change, seen next time
		}
		if d.IsDir() {
			switch d.Name() {
			case ".grapi", "node_modules", "vendor", ".git", "public", "src", "config":
				if path != dir {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if name := d.Name(); strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum" {
			if info, err := d.Info(); err == nil {
				sum += info.ModTime().UnixNano() + info.Size()
				n++
			}
		}
		return nil
	})
	return fmt.Sprintf("%d:%d", n, sum)
}

// devProcess is the running build of a framework project.
type devProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *devProcess) stop() {
	if p == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// superviseFramework builds the project's main, runs it with
// GRAPI_DEVELOP=1 (it then reloads src/ and config/ in process), and builds
// and restarts it when its Go code changes. A failed build keeps the
// running version.
func superviseFramework(dir, configDir string, stderr io.Writer, onListen func(addr string)) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	stop := make(chan struct{})
	go func() { <-sig; close(stop) }()
	return supervise(dir, configDir, stderr, onListen, stop, 500*time.Millisecond)
}

func supervise(dir, configDir string, stderr io.Writer, onListen func(addr string), stop <-chan struct{}, every time.Duration) error {
	bin, err := filepath.Abs(filepath.Join(dir, ".grapi", "develop-bin"))
	if err != nil {
		return err
	}
	build := func() error {
		cmd := exec.Command("go", "build", "-o", bin, ".") //nolint:gosec // bin is under the project's .grapi directory
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, stderr, stderr
		return cmd.Run()
	}
	run := func() (*devProcess, error) {
		var args []string
		if configDir != "" {
			args = append(args, "-config", configDir)
		}
		cmd := exec.Command(bin, args...) //nolint:gosec // the project's own build
		cmd.Dir, cmd.Stdout, cmd.Stderr = dir, stderr, stderr
		cmd.Env = append(os.Environ(), "GRAPI_DEVELOP=1")
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		p := &devProcess{cmd: cmd, done: make(chan struct{})}
		go func() { _ = cmd.Wait(); close(p.done) }()
		return p, nil
	}
	if err := build(); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	proc, err := run()
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "grapi develop: framework mode, watching the Go code of %s\n", dir)
	if onListen != nil {
		port := os.Getenv("PORT")
		if port == "" {
			port = "1337"
		}
		onListen(":" + port)
	}
	last := goSnapshot(dir)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			proc.stop()
			return nil
		case <-tick.C:
			now := goSnapshot(dir)
			if now == last {
				continue
			}
			last = now
			if err := build(); err != nil {
				fmt.Fprintln(stderr, "grapi develop: build failed, keeping the running version:", err)
				continue
			}
			proc.stop()
			if proc, err = run(); err != nil {
				return err
			}
			fmt.Fprintln(stderr, "grapi develop: rebuilt and restarted")
		}
	}
}
