// Package clitable draws the boxed tables of Strapi's CLI (cli-table3).
package clitable

import (
	"strings"
	"unicode/utf8"
)

// Render draws rows, the first being the header, with a separator between
// every row.
func Render(rows [][]string) string {
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

// Route is a row of routes:list.
type Route struct {
	Method, Path, Handler string
	Policies              []string
}

// Routes is Strapi's routes:list table: a GET route shows as HEAD|GET, as
// koa answers both. details adds the handler and the policies.
func Routes(routes []Route, details bool) string {
	head := []string{"Method", "Path"}
	if details {
		head = append(head, "Handler", "Policies")
	}
	rows := [][]string{head}
	for _, r := range routes {
		m := r.Method
		if m == "GET" {
			m = "HEAD|GET"
		}
		row := []string{m, r.Path}
		if details {
			ps := strings.Join(r.Policies, ", ")
			if ps == "" {
				ps = "-"
			}
			h := r.Handler
			if h == "" {
				h = "-"
			}
			row = append(row, h, ps)
		}
		rows = append(rows, row)
	}
	return Render(rows)
}
