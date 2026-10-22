package main

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/aluzed/grapi"
)

// routesList is `grapi routes:list`: Strapi's table, a GET route listed as
// HEAD|GET as koa does.
func routesList(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("routes:list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := grapi.LoadProject(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	rows := [][2]string{{"Method", "Path"}}
	for _, r := range p.Routes() {
		m := r.Method
		if m == "GET" {
			m = "HEAD|GET"
		}
		rows = append(rows, [2]string{m, r.Path})
	}
	_, err = fmt.Fprint(stdout, table(rows))
	return err
}

// table draws Strapi's cli-table3 boxes, a separator between every row.
func table[R ~[2]string | ~[1]string](rows []R) string {
	cols := len(rows[0])
	w := make([]int, cols)
	for _, r := range rows {
		for i := 0; i < cols; i++ {
			w[i] = max(w[i], utf8.RuneCountInString(r[i])+2)
		}
	}
	line := func(l, m, r string) string {
		parts := make([]string, cols)
		for i := range parts {
			parts[i] = strings.Repeat("─", w[i])
		}
		return l + strings.Join(parts, m) + r + "\n"
	}
	cell := func(s string, n int) string { return " " + s + strings.Repeat(" ", n-1-utf8.RuneCountInString(s)) }
	var b strings.Builder
	b.WriteString(line("┌", "┬", "┐"))
	for i, r := range rows {
		if i > 0 {
			b.WriteString(line("├", "┼", "┤"))
		}
		b.WriteString("│")
		for c := 0; c < cols; c++ {
			b.WriteString(cell(r[c], w[c]) + "│")
		}
		b.WriteString("\n")
	}
	b.WriteString(line("└", "┴", "┘"))
	return b.String()
}

// listCommand is `grapi content-types:list` and `components:list`.
func listCommand(kind string, args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags(kind + ":list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := grapi.LoadProject(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	rows := [][1]string{{"Name"}}
	if kind == "content-types" {
		for uid := range p.Res.Set.ContentTypes.All() {
			rows = append(rows, [1]string{uid})
		}
	} else {
		for uid := range p.Res.Set.Components.All() {
			rows = append(rows, [1]string{uid})
		}
	}
	_, err = fmt.Fprint(stdout, table(rows))
	return err
}
