package grapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
)

// Snapshot is the modification state of a project's src/ and config/.
func Snapshot(dir string) string {
	var sum int64
	var n int
	for _, sub := range []string{"src", "config"} {
		_ = filepath.WalkDir(filepath.Join(dir, sub), func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // a vanished file is a change, seen next time
			}
			if info, err := d.Info(); err == nil {
				sum += info.ModTime().UnixNano() + info.Size()
				n++
			}
			return nil
		})
	}
	return fmt.Sprintf("%d:%d", n, sum)
}

// Develop serves the App and sets it up again, in process, when src/ or
// config/ changes; the previous version keeps serving when the new one
// fails. The same App is reused, so Go hooks survive the reloads. onListen,
// if set, gets the listen address once serving.
func (a *App) Develop(onListen func(addr string)) error {
	a.opts.AutoReload = true
	stderr := a.opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	srv, err := a.Setup()
	if err != nil {
		return err
	}
	var current atomic.Pointer[Server]
	current.Store(srv)
	gate := &reloadGate{wait: 10 * time.Second}
	httpSrv := srv.httpServer(gate.handler(func() http.Handler { return current.Load().Handler }))
	ln, addr, err := srv.listener()
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()
	dir := a.opts.Dir
	if dir == "" {
		dir = "."
	}
	fmt.Fprintf(stderr, "grapi develop: listening on %s, watching %s\n", addr, dir)
	if onListen != nil {
		onListen(addr)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	last := Snapshot(dir)
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-errc:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case <-stop:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = httpSrv.Shutdown(ctx)
			return current.Load().Close()
		case <-tick.C:
			now := Snapshot(dir)
			if now == last {
				continue
			}
			last = now
			gate.begin()
			next, err := a.Setup()
			if err != nil {
				gate.end()
				fmt.Fprintln(stderr, "grapi develop: reload failed, keeping the previous version:", err)
				continue
			}
			old := current.Swap(next)
			gate.end()
			_ = old.Close()
			fmt.Fprintln(stderr, "grapi develop: reloaded")
		}
	}
}
