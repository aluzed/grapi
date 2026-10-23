package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aluzed/grapi"
)

// snapshot is the modification state of src/ and config/.
func snapshot(dir string) string {
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

// develop is `grapi develop`: start, then reload in process when src/ or
// config/ changes (the schema sync runs again).
func develop(args []string, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("develop")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := grapi.Options{Dir: *dir, ConfigDir: *configDir, Version: version, Stderr: stderr, AutoReload: true}
	srv, err := grapi.New(opts).Setup()
	if err != nil {
		return err
	}
	var current atomic.Pointer[grapi.Server]
	current.Store(srv)
	httpSrv := &http.Server{
		Addr:              srv.Addr,
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { current.Load().Handler.ServeHTTP(w, r) }),
	}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	fmt.Fprintf(stderr, "grapi develop: listening on %s, watching %s\n", srv.Addr, *dir)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	last := snapshot(*dir)
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
			now := snapshot(*dir)
			if now == last {
				continue
			}
			last = now
			next, err := grapi.New(opts).Setup()
			if err != nil {
				fmt.Fprintln(stderr, "grapi develop: reload failed, keeping the previous version:", err)
				continue
			}
			old := current.Swap(next)
			_ = old.Close()
			fmt.Fprintln(stderr, "grapi develop: reloaded")
		}
	}
}
