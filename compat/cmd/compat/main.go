// Command compat runs the compatibility corpus against a target and compares runs.
//
//	compat run  -corpus compat/corpus -fixture basic -url http://127.0.0.1:41337 -tokens tokens.json -out out/ref
//	compat diff -a out/ref -b out/grapi -report report.md
//	compat schema-sync -fixture compat/fixtures/basic -client sqlite -db data.db
//	compat query -fixture compat/fixtures/basic -client sqlite -db data.db -cases cases.json -out grapi.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/aluzed/grapi/compat/corpus"
	"github.com/aluzed/grapi/compat/diff"
	"github.com/aluzed/grapi/compat/runner"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: compat run|diff|snapshot|perf|schema-sync|query-cases|query|query-diff [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = run(os.Args[2:])
	case "diff":
		err = compare(os.Args[2:])
	case "snapshot":
		err = snapshot(os.Args[2:])
	case "perf":
		err = perf(os.Args[2:])
	case "schema-sync":
		err = schemaSync(os.Args[2:])
	case "query-cases":
		err = queryCasesCmd(os.Args[2:])
	case "query":
		err = queryCmd(os.Args[2:])
	case "query-diff":
		err = queryDiffCmd(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "compat:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dir := fs.String("corpus", "compat/corpus", "corpus directory")
	fixture := fs.String("fixture", "", "only run corpus files for this fixture")
	url := fs.String("url", "", "target base URL")
	tokens := fs.String("tokens", "", "JSON file mapping token names to keys")
	out := fs.String("out", "", "output directory")
	_ = fs.Parse(args)
	if *url == "" || *out == "" {
		return fmt.Errorf("-url and -out are required")
	}
	files, err := corpus.Load(*dir, *fixture)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no corpus file for fixture %q in %s", *fixture, *dir)
	}
	t := runner.Target{BaseURL: *url, Tokens: map[string]string{}}
	if *tokens != "" {
		raw, err := os.ReadFile(*tokens)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &t.Tokens); err != nil {
			return fmt.Errorf("%s: %w", *tokens, err)
		}
	}
	return runner.Run(t, files, *out)
}

func compare(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	a := fs.String("a", "", "first run output (reference)")
	b := fs.String("b", "", "second run output")
	report := fs.String("report", "", "Markdown report path (stdout when empty)")
	title := fs.String("title", "Compat report", "report title")
	_ = fs.Parse(args)
	cases, err := diff.Compare(*a, *b)
	if err != nil {
		return err
	}
	w := os.Stdout
	if *report != "" {
		f, err := os.Create(*report)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	diff.Report(w, *title, cases)
	if diff.Failed(cases) {
		return fmt.Errorf("%s: divergences found", *b)
	}
	return nil
}
