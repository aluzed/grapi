package main

import (
	"flag"
	"io"

	"github.com/aluzed/grapi"
)

func projectFlags(name string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory (the one holding src/)")
	configDir := fs.String("config", "", "configuration directory (default <dir>/config)")
	return fs, dir, configDir
}

func start(args []string, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("start")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return grapi.New(grapi.Options{Dir: *dir, ConfigDir: *configDir, Version: version, Stderr: stderr}).Start()
}
