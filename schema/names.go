package schema

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// deburr drops combining marks like lodash's deburr ("élan" becomes "elan").
func deburr(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ordinalLen returns the length of an ordinal suffix ("st", "nd", "rd", "th")
// starting at rs[i] after the digit d, or 0. lodash keeps "2nd" as one word
// when the suffix ends the word.
func ordinalLen(rs []rune, i int, d rune) int {
	if i+2 > len(rs) {
		return 0
	}
	suffix := string(rs[i : i+2])
	valid := map[rune]string{'1': "st", '2': "nd", '3': "rd"}[d]
	if suffix != valid && (suffix != "th" || d == '1' || d == '2' || d == '3') {
		return 0
	}
	if i+2 < len(rs) {
		next := rs[i+2]
		if unicode.IsLower(next) || unicode.IsDigit(next) {
			return 0
		}
	}
	return 2
}

// words splits like lodash's words(): on separators, on lower to upper case
// changes, before the last capital of an acronym, and between letters and digits.
func words(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	rs := []rune(deburr(s))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			switch {
			case unicode.IsLower(prev) && unicode.IsUpper(r):
				flush()
			case unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(rs) && unicode.IsLower(rs[i+1]):
				flush()
			case unicode.IsDigit(prev) && !unicode.IsDigit(r):
				if n := ordinalLen(rs, i, prev); n > 0 {
					cur = append(cur, rs[i:i+n]...)
					i += n - 1
					continue
				}
				flush()
			case !unicode.IsDigit(prev) && unicode.IsDigit(r):
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

// pascalCase is lodash's upperFirst(camelCase(s)).
func pascalCase(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		rs := []rune(strings.ToLower(w))
		rs[0] = unicode.ToUpper(rs[0])
		b.WriteString(string(rs))
	}
	return b.String()
}

// snakeCase is lodash's snakeCase, used for reserved name checks and column names.
func snakeCase(s string) string {
	ws := words(s)
	for i, w := range ws {
		ws[i] = strings.ToLower(w)
	}
	return strings.Join(ws, "_")
}
