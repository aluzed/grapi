package main

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"

	"github.com/aluzed/grapi"
)

// report is `grapi report`: versions and project facts, never a secret.
func report(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "grapi: %s\ngo: %s\nos: %s/%s\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	p, err := grapi.LoadProject(*dir, *configDir, io.Discard)
	if err != nil {
		fmt.Fprintf(stdout, "project: %v\n", err)
		return nil
	}
	defer func() { _ = p.DB.Close() }()
	fmt.Fprintf(stdout, "environment: %s\ndatabase: %s\n", p.Conf.Environment, p.DB.Dialect.Name())
	var cts, plugins []string
	seen := map[string]bool{}
	for uid := range p.Res.Set.ContentTypes.All() {
		if strings.HasPrefix(uid, "api::") {
			cts = append(cts, uid)
		}
		if name, ok := strings.CutPrefix(uid, "plugin::"); ok {
			name, _, _ = strings.Cut(name, ".")
			if !seen[name] {
				seen[name] = true
				plugins = append(plugins, name)
			}
		}
	}
	sort.Strings(cts)
	sort.Strings(plugins)
	fmt.Fprintf(stdout, "content types: %d (%s)\nplugins: %s\n", len(cts), strings.Join(cts, ", "), strings.Join(plugins, ", "))
	_ = stderr
	return nil
}
