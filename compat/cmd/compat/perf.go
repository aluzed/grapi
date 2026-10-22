package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aluzed/grapi/compat/corpus"
	"github.com/aluzed/grapi/compat/runner"
)

// perf load tests the variable free GET cases of a fixture's corpus.
func perf(args []string) error {
	fs := flag.NewFlagSet("perf", flag.ExitOnError)
	dir := fs.String("corpus", "compat/corpus", "corpus directory")
	fixture := fs.String("fixture", "", "fixture")
	url := fs.String("url", "", "target base URL")
	concurrency := fs.Int("c", 32, "concurrent workers")
	duration := fs.Duration("d", 30*time.Second, "duration")
	_ = fs.Parse(args)
	files, err := corpus.Load(*dir, *fixture)
	if err != nil {
		return err
	}
	urls := runner.PerfURLs(*url, files)
	if len(urls) == 0 {
		return fmt.Errorf("no variable free GET case for %s", *fixture)
	}
	res := runner.Perf(urls, *concurrency, *duration)
	return json.NewEncoder(os.Stdout).Encode(res)
}
