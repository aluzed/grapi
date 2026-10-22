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
	w := [2]int{}
	for _, r := range rows {
		for i, c := range r {
			w[i] = max(w[i], utf8.RuneCountInString(c)+2)
		}
	}
	line := func(l, m, r string) string {
		return l + strings.Repeat("─", w[0]) + m + strings.Repeat("─", w[1]) + r + "\n"
	}
	cell := func(s string, n int) string { return " " + s + strings.Repeat(" ", n-1-utf8.RuneCountInString(s)) }
	var b strings.Builder
	b.WriteString(line("┌", "┬", "┐"))
	for i, r := range rows {
		if i > 0 {
			b.WriteString(line("├", "┼", "┤"))
		}
		b.WriteString("│" + cell(r[0], w[0]) + "│" + cell(r[1], w[1]) + "│\n")
	}
	b.WriteString(line("└", "┴", "┘"))
	_, err = fmt.Fprint(stdout, b.String())
	return err
}
