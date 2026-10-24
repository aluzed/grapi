package main

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"

	"github.com/aluzed/grapi"
)

// adminURL is the admin panel on localhost for a listen address.
func adminURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "1337"
	}
	return "http://localhost:" + port + "/admin"
}

// openBrowser is replaced in tests.
var openBrowser = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // url is adminURL: localhost and a port
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // url is adminURL: localhost and a port
	default:
		cmd = exec.Command("xdg-open", url) //nolint:gosec // url is adminURL: localhost and a port
	}
	return cmd.Start()
}

// develop is `grapi develop`: start, then reload in process when src/ or
// config/ changes. A framework project (a main package next to src/) is
// built and restarted when its Go code changes instead.
func develop(args []string, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("develop")
	openAdmin := fs.Bool("open", false, "open the admin panel in the browser once started")
	typesOut := fs.String("types-out", "", "write the TypeScript types to this directory, again on each change")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *typesOut != "" {
		go watchTypes(*dir, *typesOut, stderr)
	}
	onListen := func(addr string) {
		if *openAdmin {
			if err := openBrowser(adminURL(addr)); err != nil {
				fmt.Fprintf(stderr, "grapi develop: could not open the browser: %v\n", err)
			}
		}
	}
	if isFramework(*dir) {
		return superviseFramework(*dir, *configDir, stderr, onListen)
	}
	opts := grapi.Options{Dir: *dir, ConfigDir: *configDir, Version: version, Stderr: stderr, AutoReload: true}
	return grapi.New(opts).Develop(onListen)
}
