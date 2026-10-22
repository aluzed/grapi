package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/schema"
)

var kebab = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

type attrFlags []string

func (a *attrFlags) String() string     { return strings.Join(*a, ",") }
func (a *attrFlags) Set(v string) error { *a = append(*a, v); return nil }

// attributes reads name:type[:required] flags, in order.
func attributes(list attrFlags) (schema.OrderedMap[json.RawMessage], error) {
	var out schema.OrderedMap[json.RawMessage]
	for _, a := range list {
		parts := strings.Split(a, ":")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return out, fmt.Errorf("--attr %q: expected name:type[:required]", a)
		}
		def := map[string]any{"type": parts[1]}
		if len(parts) > 2 && parts[2] == "required" {
			def["required"] = true
		}
		b, _ := json.Marshal(def)
		out.Set(parts[0], b)
	}
	return out, nil
}

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // names are checked in kebab case
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600) //nolint:gosec // names are checked in kebab case
}

// generate is `grapi generate content-type|component`, non interactive:
// the schema is written, then the project is loaded to validate it, and
// the file is removed when it does not load.
func generate(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: grapi generate content-type|component [flags]")
	}
	kind, args := args[0], args[1:]
	if kind == "migration" {
		return generateMigration(args, stdout, time.Now())
	}
	fs := flag.NewFlagSet("generate "+kind, flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory")
	name := fs.String("name", "", "singular name (kebab case), or category.name for a component")
	plural := fs.String("plural", "", "plural name (default: name + s)")
	api := fs.String("api", "", "api folder (default: the name)")
	ctKind := fs.String("kind", schema.CollectionType, "collectionType or singleType")
	dp := fs.Bool("draft-and-publish", false, "draft and publish")
	i18n := fs.Bool("i18n", false, "localized")
	var attrs attrFlags
	fs.Var(&attrs, "attr", "attribute name:type[:required] (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	attributesMap, err := attributes(attrs)
	if err != nil {
		return err
	}
	// The content-type builder's rules: stricter than loading.
	for n := range attributesMap.All() {
		if schema.IsReservedAttributeName(n, *dp) {
			return fmt.Errorf("the attribute name %q is reserved", n)
		}
	}
	for _, part := range strings.FieldsFunc(*name+"."+*plural+"."+*api, func(r rune) bool { return r == '.' }) {
		if !kebab.MatchString(part) {
			return fmt.Errorf("%q is not a kebab case name", part)
		}
	}
	if kind == "content-type" && schema.IsReservedModelName(*name) {
		return fmt.Errorf("the name %q is reserved", *name)
	}
	var path string
	var content any
	switch kind {
	case "content-type":
		if *name == "" {
			return errors.New("--name is required")
		}
		if *plural == "" {
			*plural = *name + "s"
		}
		if *api == "" {
			*api = *name
		}
		ct := map[string]any{
			"kind": *ctKind, "collectionName": strings.ReplaceAll(*plural, "-", "_"),
			"info":    map[string]any{"singularName": *name, "pluralName": *plural, "displayName": schema.PascalCase(*name)},
			"options": map[string]any{"draftAndPublish": *dp}, "attributes": attributesMap,
		}
		if *i18n {
			ct["pluginOptions"] = map[string]any{"i18n": map[string]any{"localized": true}}
		}
		path, content = filepath.Join(*dir, "src", "api", *api, "content-types", *name, "schema.json"), ct
	case "component":
		category, comp, ok := strings.Cut(*name, ".")
		if !ok || category == "" || comp == "" {
			return errors.New("--name must be category.name")
		}
		path = filepath.Join(*dir, "src", "components", category, comp+".json")
		if *plural == "" {
			*plural = comp + "s"
		}
		content = map[string]any{
			"collectionName": "components_" + strings.ReplaceAll(category+"_"+*plural, "-", "_"),
			"info":           map[string]any{"displayName": schema.PascalCase(comp)}, "options": map[string]any{}, "attributes": attributesMap,
		}
	default:
		return fmt.Errorf("unknown generator %q: content-type or component", kind)
	}
	if _, err := os.Stat(path); err == nil { //nolint:gosec // names are checked in kebab case
		return fmt.Errorf("%s already exists", path)
	}
	if err := writeJSONFile(path, content); err != nil {
		return err
	}
	if _, _, err := builtin.Load(os.DirFS(*dir)); err != nil {
		_ = os.Remove(path) //nolint:gosec // names are checked in kebab case
		return fmt.Errorf("the generated schema does not load, nothing written: %w", err)
	}
	fmt.Fprintf(stdout, "Created %s\n", path)
	return nil
}

// generateMigration writes database/migrations/<YYYY.MM.DDTHH.mm.ss>.<name>.sql,
// Strapi's naming, which orders the migrations.
func generateMigration(args []string, stdout io.Writer, now time.Time) error {
	fs := flag.NewFlagSet("generate migration", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory")
	name := fs.String("name", "", "migration name (kebab case)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !kebab.MatchString(*name) {
		return fmt.Errorf("--name %q is not a kebab case name", *name)
	}
	path := filepath.Join(*dir, "database", "migrations", now.Format("2006.01.02T15.04.05")+"."+*name+".sql")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { //nolint:gosec // the name is checked in kebab case
		return err
	}
	body := "-- " + *name + ": runs once, in a transaction, before the schema sync.\nSELECT 1;\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil { //nolint:gosec // the name is checked in kebab case
		return err
	}
	fmt.Fprintf(stdout, "Created %s\n", path)
	return nil
}
