// Package registry loads an application's schemas and holds the namespaced registries.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/aluzed/grapi/schema"
)

// PluginSchemas are the content types a plugin ships: an fs.FS holding
// content-types/<name>/schema.json.
type PluginSchemas struct {
	Name string
	FS   fs.FS
}

// Options of Load.
type Options struct {
	Plugins []PluginSchemas
}

// Result of Load.
type Result struct {
	Set      *schema.Set
	Code     []CodeFile
	Warnings []string
}

// Load reads src/components, src/api and src/extensions of app, merges plugin
// schemas with their extensions, prepares everything and validates the result.
// Directories are walked in sorted order, so two loads give the same result.
func Load(app fs.FS, opts Options) (*Result, error) {
	l := &loader{app: app, res: &Result{Set: &schema.Set{}}}
	l.components()
	l.apis()
	l.plugins(opts.Plugins)
	l.userCode()
	if err := errors.Join(l.errs...); err != nil {
		return nil, err
	}
	report := schema.Validate(l.res.Set)
	for _, w := range report.Warnings() {
		l.res.Warnings = append(l.res.Warnings, w.String())
	}
	for _, ct := range l.res.Set.ContentTypes.All() {
		l.res.Warnings = append(l.res.Warnings, ct.Warnings...)
	}
	if err := report.Err(); err != nil {
		return nil, err
	}
	sort.Slice(l.res.Code, func(i, j int) bool { return l.res.Code[i].Path < l.res.Code[j].Path })
	return l.res, nil
}

type loader struct {
	app  fs.FS
	res  *Result
	errs []error
}

func (l *loader) fail(format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf(format, args...))
}

func (l *loader) components() {
	for _, cat := range subdirs(l.app, "src/components") {
		dir := path.Join("src/components", cat)
		for _, e := range entries(l.app, dir) {
			name := e.Name()
			p := path.Join(dir, name)
			if e.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			if base, ok := strings.CutSuffix(name, ".json"); ok {
				c, err := readComponent(l.app, p)
				if err != nil {
					l.fail("%s: %w", p, err)
					continue
				}
				if err := c.Prepare(cat, base); err != nil {
					l.fail("%s: %w", p, err)
					continue
				}
				l.res.Set.AddComponent(c)
			} else if isScript(name) {
				l.code(p, KindComponentScript)
			}
		}
	}
}

func (l *loader) apis() {
	for _, dirName := range subdirs(l.app, "src/api") {
		api := normalizeName(dirName)
		apiDir := path.Join("src/api", dirName)
		for _, ctDirName := range subdirs(l.app, path.Join(apiDir, "content-types")) {
			ctDir := path.Join(apiDir, "content-types", ctDirName)
			key := normalizeName(ctDirName)
			p := path.Join(ctDir, "schema.json")
			raw, err := fs.ReadFile(l.app, p)
			if err != nil {
				l.fail("could not load content type found at %s: no schema.json", ctDir)
				continue
			}
			ct, err := schema.ParseContentType(raw)
			if err != nil {
				l.fail("%s: %w", p, err)
				continue
			}
			if ct.Info.SingularName != key {
				l.fail("%s: the key of the content-type should be the same as its singularName. Found %s and %s", p, key, ct.Info.SingularName)
				continue
			}
			if ct.CollectionName == "" {
				ct.CollectionName = ct.Info.SingularName
			}
			if err := ct.Prepare(schema.APIUID(api, key), ""); err != nil {
				l.fail("%s: %w", p, err)
				continue
			}
			l.res.Set.AddContentType(ct)
			for _, f := range files(l.app, ctDir) {
				if isScript(f) && strings.HasPrefix(f, "lifecycles.") {
					l.code(path.Join(ctDir, f), KindLifecycles)
				}
			}
		}
		l.apiCode(apiDir)
	}
}

func (l *loader) plugins(plugins []PluginSchemas) {
	for _, p := range plugins {
		for _, ctName := range subdirs(p.FS, "content-types") {
			base, err := fs.ReadFile(p.FS, path.Join("content-types", ctName, "schema.json"))
			if err != nil {
				l.fail("plugin %s: content type %s: %w", p.Name, ctName, err)
				continue
			}
			extPath := path.Join("src/extensions", p.Name, "content-types", ctName, "schema.json")
			if ext, err := fs.ReadFile(l.app, extPath); err == nil {
				if base, err = shallowMerge(base, ext); err != nil {
					l.fail("%s: %w", extPath, err)
					continue
				}
			}
			ct, err := schema.ParseContentType(base)
			if err != nil {
				l.fail("plugin %s: content type %s: %w", p.Name, ctName, err)
				continue
			}
			if err := ct.Prepare(schema.PluginUID(p.Name, ctName), p.Name); err != nil {
				l.fail("plugin %s: %w", p.Name, err)
				continue
			}
			l.res.Set.AddContentType(ct)
		}
	}
}

// shallowMerge is Strapi's { ...pluginSchema, ...extensionSchema }: top-level
// keys of the extension replace the plugin's, attributes included as a block.
func shallowMerge(base, ext []byte) ([]byte, error) {
	var b, e schema.OrderedMap[json.RawMessage]
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(ext, &e); err != nil {
		return nil, err
	}
	for k, v := range e.All() {
		b.Set(k, v)
	}
	return json.Marshal(b)
}

func readComponent(fsys fs.FS, p string) (*schema.Component, error) {
	raw, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, err
	}
	return schema.ParseComponent(raw)
}

var kebab = regexp.MustCompile(`^([a-z][a-z0-9]*)(-[a-z0-9]+)*$`)

// normalizeName keeps kebab-case names and kebab-cases the others, like Strapi.
func normalizeName(name string) string {
	if kebab.MatchString(name) {
		return name
	}
	return schema.KebabCase(name)
}

func entries(fsys fs.FS, dir string) []fs.DirEntry {
	es, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	sort.Slice(es, func(i, j int) bool { return es[i].Name() < es[j].Name() })
	return es
}

func subdirs(fsys fs.FS, dir string) []string {
	var out []string
	for _, e := range entries(fsys, dir) {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

func files(fsys fs.FS, dir string) []string {
	var out []string
	for _, e := range entries(fsys, dir) {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

func isScript(name string) bool {
	for _, ext := range []string{".js", ".ts", ".mjs", ".cjs", ".mts", ".cts"} {
		if strings.HasSuffix(name, ext) && !strings.HasSuffix(name, ".d.ts") {
			return true
		}
	}
	return false
}
