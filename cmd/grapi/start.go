package main

import (
	"flag"
	"io"
	"os"

	"github.com/aluzed/grapi"
)

// envFlag sets an environment variable when the flag is given.
type envFlag string

func (e envFlag) String() string     { return "" }
func (e envFlag) Set(v string) error { return os.Setenv(string(e), v) }

func projectFlags(name string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory (the one holding src/)")
	configDir := fs.String("config", "", "configuration directory (default <dir>/config)")
	fs.Var(envFlag("GRAPI_ENV"), "env", "environment (GRAPI_ENV, NODE_ENV)")
	fs.Var(envFlag("GRAPI_LOG_LEVEL"), "log-level", "debug, info, warn or error")
	return fs, dir, configDir
}

func start(args []string, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("start")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return grapi.New(grapi.Options{Dir: *dir, ConfigDir: *configDir, Version: version, Stderr: stderr}).Start()
}
