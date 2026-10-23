package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
)

// queryCases is a compat/querycases file: rows inserted through Strapi's
// db.query(uid).create, then queries run by both engines on that database.
type queryCases struct {
	Fixture string `json:"fixture" yaml:"fixture"`
	// Isolated cases write: grapi plays the seed and the cases on a
	// database of its own instead of reading Strapi's.
	Isolated bool `json:"isolated,omitempty" yaml:"isolated"`
	// Hooks runs the fixture's project code on both sides: rules.js in
	// Strapi, its Go port (fixtureHooks) in grapi, which then serves the
	// doc* cases from a full App.
	Hooks bool        `json:"hooks,omitempty" yaml:"hooks"`
	Seed  []seedRow   `json:"seed" yaml:"seed"`
	Cases []queryCase `json:"cases" yaml:"cases"`
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
	// ExactError requires the same error message as Strapi.
	ExactError bool `json:"exactError,omitempty" yaml:"exactError"`
	// IgnoreIDs masks ids: Strapi creates nested components in parallel,
	// so their ids are not in a stable order.
	IgnoreIDs bool `json:"ignoreIds,omitempty" yaml:"ignoreIds"`
	// Save keeps values of the result (dotted paths) for later {{name}}.
	Save map[string]string `json:"save,omitempty" yaml:"save"`
}

type queryResult struct {
	Name       string `json:"name"`
	ExactError bool   `json:"exactError,omitempty"`
	IgnoreIDs  bool   `json:"ignoreIds,omitempty"`
	Result     any    `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
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
	seed := fs.Bool("seed", false, "insert the seed rows first (isolated cases)")
	hooks := fs.String("hooks", "", "fixture whose Go hooks the doc* cases run with (cases with hooks: true)")
	configDir := fs.String("config", "", "grapi config directory, with -hooks")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(*casesFile)
	if err != nil {
		return err
	}
	var c queryCases
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	res, md, err := builtin.Load(os.DirFS(*fixture))
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
	var docs documentService = document.New(e, res.Set)
	if c.Hooks {
		register, ok := fixtureHooks[*hooks]
		if !ok {
			return fmt.Errorf("no Go hooks for the fixture %q", *hooks)
		}
		app, closeApp, err := hookedApp(*fixture, *configDir, *client, *dsn, register)
		if err != nil {
			return err
		}
		defer closeApp()
		docs = app.Documents()
	}
	if *seed {
		for _, s := range c.Seed {
			if _, err := e.Create(ctx, s.UID, s.Data, query.Params{}); err != nil {
				return fmt.Errorf("seed %s: %w", s.UID, err)
			}
		}
	}
	results := make([]queryResult, 0, len(c.Cases))
	vars := map[string]any{}
	for _, qc := range c.Cases {
		qc.Params, _ = fillVars(qc.Params, vars).(map[string]any)
		r := queryResult{Name: qc.Name, ExactError: qc.ExactError, IgnoreIDs: qc.IgnoreIDs}
		var err error
		if strings.HasPrefix(qc.Op, "doc") {
			r.Result, err = runDocCase(ctx, docs, qc)
		} else {
			var p query.Params
			p, err = toParams(qc.Params)
			if err == nil {
				r.Result, err = runCase(ctx, e, qc, p)
			}
		}
		if err != nil {
			r.Error = err.Error()
			r.Result = nil
		}
		if len(qc.Save) > 0 && r.Error == "" {
			norm := normalizeJSON(r.Result)
			for k, path := range qc.Save {
				vars[k] = pickPath(norm, path)
			}
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
	data, _ := qc.Params["data"].(map[string]any)
	switch qc.Op {
	case "table":
		return e.Table(ctx, qc.UID)
	case "create":
		return e.Create(ctx, qc.UID, data, p)
	case "update":
		row, err := e.Update(ctx, qc.UID, qc.Params["where"], data, p)
		if row == nil {
			return nil, err
		}
		return row, err
	case "delete":
		row, err := e.Delete(ctx, qc.UID, qc.Params["where"], p)
		if row == nil {
			return nil, err
		}
		return row, err
	case "createMany":
		list, _ := qc.Params["data"].([]any)
		var rows []query.Data
		for _, d := range list {
			m, _ := d.(map[string]any)
			rows = append(rows, m)
		}
		n, ids, err := e.CreateMany(ctx, qc.UID, rows)
		return map[string]any{"count": n, "ids": ids}, err
	case "updateMany":
		n, err := e.UpdateMany(ctx, qc.UID, p, data)
		return map[string]any{"count": n}, err
	case "deleteMany":
		n, err := e.DeleteMany(ctx, qc.UID, p)
		return map[string]any{"count": n}, err
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

// documentService is the Document Service a case runs on: the bare one, or
// the App's when the fixture has Go hooks.
type documentService interface {
	FindMany(ctx context.Context, uid string, p document.Params) ([]query.Row, error)
	FindFirst(ctx context.Context, uid string, p document.Params) (query.Row, error)
	FindOne(ctx context.Context, uid, documentID string, p document.Params) (query.Row, error)
	Count(ctx context.Context, uid string, p document.Params) (int64, error)
	Create(ctx context.Context, uid string, p document.Params) (query.Row, error)
	Update(ctx context.Context, uid, documentID string, p document.Params) (query.Row, error)
	Delete(ctx context.Context, uid, documentID string, p document.Params) (*document.DeleteResult, error)
	Publish(ctx context.Context, uid, documentID string, p document.Params) ([]query.Row, error)
	Unpublish(ctx context.Context, uid, documentID string, p document.Params) ([]query.Row, error)
	DiscardDraft(ctx context.Context, uid, documentID string, p document.Params) ([]query.Row, error)
}

func runDocCase(ctx context.Context, docs documentService, qc queryCase) (any, error) {
	p := document.Params{}
	for k, v := range qc.Params {
		p[k] = v
	}
	docID, _ := p["documentId"].(string)
	delete(p, "documentId")
	result := func(doc string, rows []query.Row) map[string]any {
		entries := make([]any, len(rows))
		for i, r := range rows {
			entries[i] = r
		}
		return map[string]any{"documentId": doc, "entries": entries}
	}
	one := func(r query.Row, err error) (any, error) {
		if r == nil {
			return nil, err
		}
		return r, err
	}
	switch qc.Op {
	case "docFindMany":
		rows, err := docs.FindMany(ctx, qc.UID, p)
		if rows == nil {
			rows = []query.Row{}
		}
		return rows, err
	case "docFindFirst":
		return one(docs.FindFirst(ctx, qc.UID, p))
	case "docFindOne":
		return one(docs.FindOne(ctx, qc.UID, docID, p))
	case "docCount":
		return docs.Count(ctx, qc.UID, p)
	case "docCreate":
		return one(docs.Create(ctx, qc.UID, p))
	case "docUpdate":
		return one(docs.Update(ctx, qc.UID, docID, p))
	case "docDelete":
		res, err := docs.Delete(ctx, qc.UID, docID, p)
		if err != nil {
			return nil, err
		}
		return result(res.DocumentID, res.Entries), nil
	case "docPublish":
		rows, err := docs.Publish(ctx, qc.UID, docID, p)
		return result(docID, rows), err
	case "docUnpublish":
		rows, err := docs.Unpublish(ctx, qc.UID, docID, p)
		return result(docID, rows), err
	case "docDiscardDraft":
		rows, err := docs.DiscardDraft(ctx, qc.UID, docID, p)
		return result(docID, rows), err
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
		case "populate":
			p.Populate = v
		case "data":
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
	volatile := fs.Bool("volatile", false, "ignore document ids and timestamps (runs on two databases)")
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
			same = x.Error != "" && y.Error != "" && (!y.ExactError || x.Error == y.Error)
		default:
			xa, ya := normalizeJSON(x.Result), normalizeJSON(y.Result)
			if *volatile {
				xa, ya = maskVolatile(xa), maskVolatile(ya)
			}
			if y.IgnoreIDs {
				xa, ya = maskKey(xa, "id"), maskKey(ya, "id")
			}
			same = reflect.DeepEqual(xa, ya)
		}
		if same {
			ok++
			continue
		}
		diff++
		ja, _ := json.Marshal(x)
		jb, _ := json.Marshal(y)
		where := firstDiff(maskIf(*volatile, normalizeJSON(x.Result)), maskIf(*volatile, normalizeJSON(y.Result)), "$")
		fmt.Fprintf(&body, "\n## %s : DIFF\n\n- first difference: `%s`\n- Strapi: `%s`\n- grapi: `%s`\n", name, where, truncate(string(ja)), truncate(string(jb)))
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

var volatileKeys = map[string]bool{
	"documentId": true, "document_id": true, "createdAt": true, "updatedAt": true, "publishedAt": true,
	"created_at": true, "updated_at": true, "published_at": true,
}

// maskVolatile hides the values two runs cannot share: generated document
// ids and timestamps (only whether they are set is compared).
func maskVolatile(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if volatileKeys[k] {
				if val != nil {
					x[k] = "<set>"
				}
				continue
			}
			x[k] = maskVolatile(val)
		}
	case []any:
		for i := range x {
			x[i] = maskVolatile(x[i])
		}
	}
	return v
}

var varPattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

func fillVars(v any, vars map[string]any) any {
	switch x := v.(type) {
	case string:
		if m := varPattern.FindStringSubmatch(x); m != nil && m[0] == x {
			return vars[m[1]]
		}
		return varPattern.ReplaceAllStringFunc(x, func(s string) string {
			return fmt.Sprint(vars[varPattern.FindStringSubmatch(s)[1]])
		})
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = fillVars(e, vars)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = fillVars(e, vars)
		}
		return out
	}
	return v
}

func pickPath(v any, path string) any {
	for _, k := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			i, err := strconv.Atoi(k)
			if err != nil || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

func maskKey(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if k == key {
				x[k] = "<id>"
				continue
			}
			x[k] = maskKey(val, key)
		}
	case []any:
		for i := range x {
			x[i] = maskKey(x[i], key)
		}
	}
	return v
}

func maskIf(on bool, v any) any {
	if on {
		return maskVolatile(v)
	}
	return v
}

// firstDiff names the first path where two JSON values differ.
func firstDiff(a, b any, path string) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: %v vs %v", path, short(a), short(b))
		}
		keys := map[string]bool{}
		for k := range x {
			keys[k] = true
		}
		for k := range y {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			if !reflect.DeepEqual(x[k], y[k]) {
				return firstDiff(x[k], y[k], path+"."+k)
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return fmt.Sprintf("%s: length %d vs %v", path, len(x), short(b))
		}
		for i := range x {
			if !reflect.DeepEqual(x[i], y[i]) {
				return firstDiff(x[i], y[i], fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	return fmt.Sprintf("%s: %v vs %v", path, short(a), short(b))
}

func short(v any) string {
	b, _ := json.Marshal(v)
	return truncate(string(b))
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
