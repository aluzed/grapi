package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// goKinds are the Go skeletons of a framework project. Each file adds a
// setup function in init; main applies them to the App.
var goKinds = map[string]bool{"policy": true, "route": true, "document-middleware": true, "lifecycle": true}

var goTemplates = template.Must(template.New("").Parse(`
{{define "policy"}}package main

import "github.com/aluzed/grapi"

func init() {
	setups = append(setups, func(app *grapi.App) {
		app.Policy({{printf "%q" .Name}}, func(c *grapi.PolicyContext, config any) (bool, error) {
			// false answers 403 PolicyError.
			return c.User != nil, nil
		})
	})
}
{{end}}
{{define "route"}}package main

import (
	"net/http"

	"github.com/aluzed/grapi"
)

func init() {
	setups = append(setups, func(app *grapi.App) {
		{{if .Public}}app.PublicRoute({{printf "%q" .Method}}, {{printf "%q" .Path}}, {{else}}app.Route({{printf "%q" .Method}}, {{printf "%q" .Path}}, {{printf "%q" .Action}}, {{end}}func(c *grapi.Request) error {
			return c.JSON(http.StatusOK, map[string]any{"data": nil})
		})
	})
}
{{end}}
{{define "document-middleware"}}package main

import (
	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

func init() {
	setups = append(setups, func(app *grapi.App) {
		app.UseDocuments(func(c *document.Context, next document.Next) (any, error) {
			{{if .UID}}if c.UID != {{printf "%q" .UID}} {
				return next()
			}
			{{end}}return next()
		})
	})
}
{{end}}
{{define "lifecycle"}}package main

import (
	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/hook"
)

func init() {
	setups = append(setups, func(app *grapi.App) {
		app.{{.Method}}({{printf "%q" .Model}}).Bind(func(e *hook.RowEvent) error {
			// An error aborts the write.
			return nil
		})
	})
}
{{end}}`))

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func snake(parts ...string) string {
	return strings.Trim(nonWord.ReplaceAllString(strings.ToLower(strings.Join(parts, "_")), "_"), "_")
}

func generateGo(kind string, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("generate "+kind, flag.ContinueOnError)
	dir := fs.String("dir", ".", "project directory")
	name := fs.String("name", "", "policy or middleware name")
	method := fs.String("method", "GET", "route method")
	path := fs.String("path", "", "route path, under the API prefix")
	action := fs.String("action", "", "route action for the permissions, api::<api>.<controller>.<action>")
	public := fs.Bool("public", false, "route without authentication (config.auth: false)")
	uid := fs.String("uid", "", "document middleware: only this model")
	model := fs.String("model", "", "lifecycle: the model uid, api::post.post")
	event := fs.String("event", "create", "lifecycle: create, update, write or delete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !isFramework(*dir) {
		return errors.New("the Go skeletons need a framework project (go.mod and main.go, grapi new --framework)")
	}
	data := map[string]any{}
	var file string
	switch kind {
	case "policy":
		if *name == "" {
			return errors.New("--name is required")
		}
		if !strings.Contains(*name, "::") {
			*name = "global::" + *name
		}
		data["Name"], file = *name, "policy_"+snake(*name)
	case "route":
		if !strings.HasPrefix(*path, "/") {
			return errors.New("--path is required and starts with /")
		}
		if !*public && *action == "" {
			return errors.New("--action is required unless --public")
		}
		*method = strings.ToUpper(*method)
		data["Method"], data["Path"], data["Action"], data["Public"] = *method, *path, *action, *public
		file = "route_" + snake(*method, *path)
	case "document-middleware":
		if *name == "" {
			return errors.New("--name is required")
		}
		data["UID"], file = *uid, "middleware_"+snake(*name)
	case "lifecycle":
		methods := map[string]string{"create": "OnRowCreate", "update": "OnRowUpdate", "write": "OnRowWrite", "delete": "OnRowDelete"}
		if *model == "" || methods[*event] == "" {
			return errors.New("--model is required, and --event is create, update, write or delete")
		}
		data["Model"], data["Method"], file = *model, methods[*event], "lifecycle_"+snake(*model, *event)
	}
	var buf bytes.Buffer
	if err := goTemplates.ExecuteTemplate(&buf, kind, data); err != nil {
		return err
	}
	src, err := format.Source(bytes.TrimLeft(buf.Bytes(), "\n"))
	if err != nil {
		return err
	}
	target := filepath.Join(*dir, file+".go")
	if _, err := os.Stat(target); err == nil { //nolint:gosec // the project directory the user named
		return fmt.Errorf("%s already exists", target)
	}
	if err := os.WriteFile(target, src, 0o600); err != nil { //nolint:gosec // the project directory the user named
		return err
	}
	fmt.Fprintf(stdout, "Created %s\n", target)
	if main, err := os.ReadFile(filepath.Join(*dir, "main.go")); err == nil && !bytes.Contains(main, []byte("setups")) { //nolint:gosec // the project's main.go
		fmt.Fprintln(stdout, "main.go does not apply the generated setups yet: declare `var setups []func(app *grapi.App)` and run `for _, setup := range setups { setup(app) }` before app.Start().")
	}
	return nil
}
