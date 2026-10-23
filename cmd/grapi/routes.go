package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/aluzed/grapi/hook"
	"github.com/aluzed/grapi/internal/clitable"
	"github.com/aluzed/grapi/internal/project"
)

// routesList is `grapi routes:list`: Strapi's table, a GET route listed as
// HEAD|GET as koa does.
func routesList(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("routes:list")
	details := fs.Bool("details", false, "add the handler and the policies of each route")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := project.Load(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	var routes []clitable.Route
	for _, r := range p.Routes() {
		routes = append(routes, clitable.Route{Method: r.Method, Path: r.Path, Handler: r.Action})
	}
	_, err = fmt.Fprint(stdout, clitable.Routes(routes, *details))
	return err
}

// listCommand is `grapi content-types:list` and `components:list`.
func listCommand(kind string, args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags(kind + ":list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := project.Load(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	var names []string
	if kind == "content-types" {
		for uid := range p.Res.Set.ContentTypes.All() {
			names = append(names, string(uid))
		}
	} else {
		for uid := range p.Res.Set.Components.All() {
			names = append(names, string(uid))
		}
	}
	sort.Strings(names)
	rows := [][]string{{"Name"}}
	for _, n := range names {
		rows = append(rows, []string{n})
	}
	_, err = fmt.Fprint(stdout, clitable.Render(rows))
	return err
}

// builtinMiddlewares are the middlewares grapi implements, by Strapi's names.
var builtinMiddlewares = []string{
	"strapi::cors", "strapi::errors", "strapi::logger", "strapi::body", "strapi::query",
	"strapi::security", "strapi::public", "plugin::users-permissions.rateLimit",
}

// staticList is `grapi middlewares:list` and `hooks:list`.
func staticList(kind string, stdout io.Writer) error {
	names := builtinMiddlewares
	if kind == "hooks" {
		names = []string{hook.BeforeSync, hook.AfterSync}
	}
	rows := [][]string{{"Name"}}
	for _, n := range names {
		rows = append(rows, []string{n})
	}
	_, err := fmt.Fprint(stdout, clitable.Render(rows))
	return err
}
