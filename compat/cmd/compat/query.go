package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/query"
)

// queryCases is a compat/querycases file: rows inserted through Strapi's
// db.query(uid).create, then queries run by both engines on that database.
type queryCases struct {
	Fixture string      `json:"fixture" yaml:"fixture"`
	Seed    []seedRow   `json:"seed" yaml:"seed"`
	Cases   []queryCase `json:"cases" yaml:"cases"`
}

type seedRow struct {
	UID  string         `json:"uid" yaml:"uid"`
	Data map[string]any `json:"data" yaml:"data"`
}

type queryCase struct {
	Name   string         `json:"name" yaml:"name"`
	UID    string         `json:"uid" yaml:"uid"`
	Op     string         `json:"op,omitempty" yaml:"op"`
	Params map[string]any `json:"params,omitempty" yaml:"params"`
}

type queryResult struct {
	Name   string `json:"name"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// queryCasesCmd converts a YAML cases file to the JSON the Node side reads.
func queryCasesCmd(args []string) error {
	fs := flag.NewFlagSet("query-cases", flag.ExitOnError)
	in := fs.String("in", "", "YAML cases file")
	out := fs.String("out", "", "JSON output")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var c queryCases
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("%s: %w", *in, err)
	}
	seen := map[string]bool{}
	for _, qc := range c.Cases {
		if qc.Name == "" || seen[qc.Name] {
			return fmt.Errorf("%s: case names must be set and unique (%q)", *in, qc.Name)
		}
		seen[qc.Name] = true
	}
	b, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, b, 0o600)
}

// queryCmd runs the cases with grapi's query engine.
func queryCmd(args []string) error {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	fixture := fs.String("fixture", "", "fixture directory")
	client := fs.String("client", "sqlite", "sqlite or postgres")
	dsn := fs.String("db", "", "SQLite file or Postgres connection string")
	casesFile := fs.String("cases", "", "JSON cases (query-cases output)")
	out := fs.String("out", "", "results file")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(*casesFile)
	if err != nil {
		return err
	}
	var c queryCases
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	_, md, err := builtin.Load(os.DirFS(*fixture))
	if err != nil {
		return err
	}
	cfg := db.Config{Client: *client, Filename: *dsn}
	if *client == "postgres" {
		cfg = db.Config{Client: *client, ConnectionString: *dsn}
	}
	ctx := context.Background()
	d, err := db.Open(ctx, cfg, nil)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	e := &query.Engine{DB: d, MD: md}
	results := make([]queryResult, 0, len(c.Cases))
	for _, qc := range c.Cases {
		p, err := toParams(qc.Params)
		r := queryResult{Name: qc.Name}
		if err == nil {
			r.Result, err = runCase(ctx, e, qc, p)
		}
		if err != nil {
			r.Error = err.Error()
			r.Result = nil
		}
		results = append(results, r)
	}
	b, err := json.MarshalIndent(results, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, b, 0o600)
}

func runCase(ctx context.Context, e *query.Engine, qc queryCase, p query.Params) (any, error) {
	switch qc.Op {
	case "count":
		return e.Count(ctx, qc.UID, p)
	case "findOne":
		row, err := e.FindOne(ctx, qc.UID, p)
		if row == nil {
			return nil, err
		}
		return row, err
	case "", "findMany":
		rows, err := e.FindMany(ctx, qc.UID, p)
		if rows == nil {
			rows = []query.Row{}
		}
		return rows, err
	}
	return nil, fmt.Errorf("unknown op %q", qc.Op)
}

func toParams(m map[string]any) (query.Params, error) {
	var p query.Params
	for k, v := range m {
		switch k {
		case "where":
			p.Where = v
		case "filters":
			p.Filters = v
		case "orderBy":
			p.OrderBy = v
		case "_q":
			p.Search = fmt.Sprint(v)
		case "limit", "offset":
			f, ok := v.(float64)
			if !ok {
				return p, fmt.Errorf("%s must be a number", k)
			}
			n := int(f)
			if k == "limit" {
				p.Limit = &n
			} else {
				p.Offset = &n
			}
		case "select":
			list, _ := v.([]any)
			for _, s := range list {
				p.Select = append(p.Select, fmt.Sprint(s))
			}
		default:
			return p, fmt.Errorf("unsupported param %s", k)
		}
	}
	return p, nil
}

// queryDiffCmd compares Strapi's results with grapi's: same rows in the same
// order, or an error on both sides.
func queryDiffCmd(args []string) error {
	fs := flag.NewFlagSet("query-diff", flag.ExitOnError)
	a := fs.String("a", "", "Strapi results")
	b := fs.String("b", "", "grapi results")
	report := fs.String("report", "", "Markdown report")
	title := fs.String("title", "Query compat", "report title")
	_ = fs.Parse(args)
	load := func(p string) (map[string]queryResult, []string, error) {
		raw, err := os.ReadFile(p) //nolint:gosec // paths come from the bench command line
		if err != nil {
			return nil, nil, err
		}
		var list []queryResult
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, nil, err
		}
		m := map[string]queryResult{}
		var order []string
		for _, r := range list {
			m[r.Name] = r
			order = append(order, r.Name)
		}
		return m, order, nil
	}
	ra, order, err := load(*a)
	if err != nil {
		return err
	}
	rb, _, err := load(*b)
	if err != nil {
		return err
	}
	var body strings.Builder
	var empty []string
	ok, diff := 0, 0
	for _, name := range order {
		x, y := ra[name], rb[name]
		if x.Error == "" && isEmptyResult(x.Result) {
			empty = append(empty, name)
		}
		same := false
		switch {
		case x.Error != "" || y.Error != "":
			same = x.Error != "" && y.Error != ""
		default:
			same = reflect.DeepEqual(normalizeJSON(x.Result), normalizeJSON(y.Result))
		}
		if same {
			ok++
			continue
		}
		diff++
		ja, _ := json.Marshal(x)
		jb, _ := json.Marshal(y)
		fmt.Fprintf(&body, "\n## %s : DIFF\n\n- Strapi: `%s`\n- grapi: `%s`\n", name, truncate(string(ja)), truncate(string(jb)))
	}
	out := fmt.Sprintf("# %s\n\n| OK | DIFF |\n|---|---|\n| %d | %d |\n%s", *title, ok, diff, body.String())
	if len(empty) > 0 {
		// An empty result matches trivially: these cases prove less.
		out += fmt.Sprintf("\nEmpty results (%d): %s\n", len(empty), strings.Join(empty, ", "))
	}
	if err := os.WriteFile(*report, []byte(out), 0o600); err != nil {
		return err
	}
	fmt.Print(out)
	if diff > 0 {
		return fmt.Errorf("%d cases differ", diff)
	}
	return nil
}

func isEmptyResult(v any) bool {
	switch x := normalizeJSON(v).(type) {
	case nil:
		return true
	case []any:
		return len(x) == 0
	case float64:
		return x == 0
	}
	return false
}

func normalizeJSON(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func truncate(s string) string {
	if len(s) > 1500 {
		return s[:1500] + "..."
	}
	return s
}
