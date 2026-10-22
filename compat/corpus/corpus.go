// Package corpus loads the compat bench request corpus.
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// File is one corpus file: an ordered list of cases run against a fixture.
type File struct {
	Name    string `yaml:"-"`
	Fixture string `yaml:"fixture"`
	Cases   []Case `yaml:"cases"`
}

// Case is one HTTP request. Path, Query and string values in Body may use
// {{var}} placeholders filled from earlier Save entries.
type Case struct {
	Name   string            `yaml:"name"`
	Method string            `yaml:"method"`
	Path   string            `yaml:"path"`
	Query  string            `yaml:"query"`
	Token  string            `yaml:"token"`
	Body   any               `yaml:"body"`
	Save   map[string]string `yaml:"save"`
	Skip   string            `yaml:"skip"`
}

// Load reads every *.yaml file of dir whose fixture matches (all when fixture is empty).
func Load(dir, fixture string) ([]File, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var files []File
	for _, p := range paths {
		f, err := loadFile(p)
		if err != nil {
			return nil, err
		}
		if fixture == "" || f.Fixture == fixture {
			files = append(files, f)
		}
	}
	return files, nil
}

func loadFile(path string) (File, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // corpus paths come from the repo
	if err != nil {
		return File{}, err
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	f.Name = strings.TrimSuffix(filepath.Base(path), ".yaml")
	if f.Fixture == "" {
		return File{}, fmt.Errorf("%s: missing fixture", path)
	}
	seen := map[string]bool{}
	for i, c := range f.Cases {
		if c.Name == "" {
			return File{}, fmt.Errorf("%s: case %d has no name", path, i)
		}
		if seen[c.Name] {
			return File{}, fmt.Errorf("%s: duplicate case name %q", path, c.Name)
		}
		seen[c.Name] = true
		if c.Method == "" {
			f.Cases[i].Method = "GET"
		}
	}
	return f, nil
}
