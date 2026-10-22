package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/query"
)

const coreStoreUID = "strapi::core-store"

type storeEntry struct {
	Key         any `json:"key"`
	Value       any `json:"value"`
	Type        any `json:"type"`
	Environment any `json:"environment"`
	Tag         any `json:"tag"`
}

func engineOf(p *grapi.Project) *query.Engine { return &query.Engine{DB: p.DB, MD: p.MD} }

// configurationDump is `grapi configuration:dump [-f file] [-p]`: the
// plugin_ entries of the core store, as Strapi writes them.
func configurationDump(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("configuration:dump")
	file := fs.String("f", "", "output file (stdout by default)")
	pretty := fs.Bool("p", false, "indent the JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := grapi.LoadProject(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	rows, err := engineOf(p).FindMany(context.Background(), coreStoreUID, query.Params{OrderBy: "key"})
	if err != nil {
		return err
	}
	out := []storeEntry{}
	for _, r := range rows {
		key, _ := r.Get("key")
		if k, _ := key.(string); !strings.HasPrefix(k, "plugin_") {
			continue
		}
		value, _ := r.Get("value")
		typ, _ := r.Get("type")
		env, _ := r.Get("environment")
		tag, _ := r.Get("tag")
		out = append(out, storeEntry{key, value, typ, env, tag})
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if *pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(out); err != nil {
		return err
	}
	if *file == "" {
		_, err = stdout.Write(b.Bytes())
		return err
	}
	if err := os.WriteFile(*file, b.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Successfully exported %d configuration entries\n", len(out))
	return nil
}

// configurationRestore is `grapi configuration:restore [-f file] [-s
// replace|merge|keep]`, Strapi's importers.
func configurationRestore(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("configuration:restore")
	file := fs.String("f", "", "input file (stdin by default)")
	strategy := fs.String("s", "replace", "replace, merge or keep")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *strategy {
	case "replace", "merge", "keep":
	default:
		return fmt.Errorf("no importer available for strategy %q", *strategy)
	}
	var raw []byte
	var err error
	if *file != "" {
		raw, err = os.ReadFile(*file)
	} else {
		raw, err = io.ReadAll(stdin)
	}
	if err != nil {
		return err
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fmt.Errorf("invalid input data: %w. Expected a valid JSON array", err)
	}
	p, err := grapi.LoadProject(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	ctx, e := context.Background(), engineOf(p)
	created, existing := 0, 0
	for _, conf := range entries {
		key, ok := conf["key"].(string)
		if !ok {
			return errors.New("invalid input data: every entry needs a key")
		}
		n, err := e.Count(ctx, coreStoreUID, query.Params{Where: map[string]any{"key": key}})
		if err != nil {
			return err
		}
		data := query.Data{}
		for k, v := range conf {
			data[k] = v
		}
		switch {
		case n == 0:
			created++
			_, err = e.Create(ctx, coreStoreUID, data, query.Params{})
		case *strategy == "keep":
			existing++
		default:
			existing++
			_, err = e.Update(ctx, coreStoreUID, map[string]any{"key": key}, data, query.Params{})
		}
		if err != nil {
			return err
		}
	}
	word := map[string]string{"replace": "replaced", "merge": "merged", "keep": "untouched"}[*strategy]
	fmt.Fprintf(stdout, "Successfully imported configuration with %s strategy. Statistics: %d created, %d %s.\n", *strategy, created, existing, word)
	return nil
}
