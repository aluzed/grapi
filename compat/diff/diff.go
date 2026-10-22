// Package diff compares the outputs of two runner runs and renders a report.
package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Outcome of one case.
const (
	OK      = "OK"
	Diff    = "DIFF"
	Skip    = "SKIP"
	Missing = "MISSING"
)

// Case is the comparison of one case between run A and run B.
type Case struct {
	File    string
	Name    string
	Outcome string
	Changes []string
}

// Compare walks every normalized result of run a and compares it with run b.
func Compare(a, b string) ([]Case, error) {
	var cases []Case
	err := filepath.WalkDir(a, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(a, path)
		if d.IsDir() {
			if rel == "raw" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		c := Case{File: filepath.Dir(rel), Name: strings.TrimSuffix(filepath.Base(rel), ".json")}
		left, err := read(path)
		if err != nil {
			return err
		}
		right, err := read(filepath.Join(b, rel))
		switch {
		case err != nil:
			c.Outcome = Missing
		case skipped(left) || skipped(right):
			c.Outcome = Skip
		default:
			c.Changes = Values("", left, right)
			c.Outcome = OK
			if len(c.Changes) > 0 {
				c.Outcome = Diff
			}
		}
		cases = append(cases, c)
		return nil
	})
	return cases, err
}

func read(path string) (any, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // paths come from our own run output
	if err != nil {
		return nil, err
	}
	var v any
	return v, json.Unmarshal(raw, &v)
}

func skipped(v any) bool {
	m, ok := v.(map[string]any)
	return ok && m["skipped"] != nil
}

// Values lists the JSON paths where a and b differ. Key order is ignored,
// array order is not (it is part of the API contract).
func Values(path string, a, b any) []string {
	am, aIsMap := a.(map[string]any)
	bm, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		var out []string
		for _, k := range sorted {
			av, aok := am[k]
			bv, bok := bm[k]
			switch {
			case !aok:
				out = append(out, fmt.Sprintf("%s: only in B = %s", join(path, k), short(bv)))
			case !bok:
				out = append(out, fmt.Sprintf("%s: only in A = %s", join(path, k), short(av)))
			default:
				out = append(out, Values(join(path, k), av, bv)...)
			}
		}
		return out
	}
	as, aIsArr := a.([]any)
	bs, bIsArr := b.([]any)
	if aIsArr && bIsArr {
		var out []string
		if len(as) != len(bs) {
			out = append(out, fmt.Sprintf("%s: length A=%d B=%d", path, len(as), len(bs)))
		}
		for i := 0; i < len(as) && i < len(bs); i++ {
			out = append(out, Values(fmt.Sprintf("%s[%d]", path, i), as[i], bs[i])...)
		}
		return out
	}
	if !reflect.DeepEqual(a, b) {
		return []string{fmt.Sprintf("%s: A=%s B=%s", path, short(a), short(b))}
	}
	return nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func short(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 120 {
		return string(b[:117]) + "..."
	}
	return string(b)
}

// Report writes a Markdown report: summary per corpus file, then every DIFF.
func Report(w io.Writer, title string, cases []Case) {
	fmt.Fprintf(w, "# %s\n\n| Corpus | OK | DIFF | SKIP | MISSING |\n|---|---|---|---|---|\n", title)
	type counts map[string]int
	byFile := map[string]counts{}
	var files []string
	for _, c := range cases {
		if byFile[c.File] == nil {
			byFile[c.File] = counts{}
			files = append(files, c.File)
		}
		byFile[c.File][c.Outcome]++
	}
	sort.Strings(files)
	for _, f := range files {
		n := byFile[f]
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d |\n", f, n[OK], n[Diff], n[Skip], n[Missing])
	}
	for _, c := range cases {
		if c.Outcome != Diff && c.Outcome != Missing {
			continue
		}
		fmt.Fprintf(w, "\n## %s / %s : %s\n\n", c.File, c.Name, c.Outcome)
		for _, ch := range c.Changes {
			fmt.Fprintf(w, "- `%s`\n", ch)
		}
	}
}

// Failed reports whether any case diverged.
func Failed(cases []Case) bool {
	for _, c := range cases {
		if c.Outcome == Diff || c.Outcome == Missing {
			return true
		}
	}
	return false
}
