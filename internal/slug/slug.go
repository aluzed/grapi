// Package slug reproduces Strapi's slugify (@sindresorhus/slugify 1.x): uid
// fields, enum value checks and collection names all go through it, so the
// output must match byte for byte.
package slug

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Options mirrors the slugify options Strapi exposes on uid attributes.
type Options struct {
	Separator                 string
	Lowercase                 bool
	Decamelize                bool
	CustomReplacements        [][2]string
	PreserveLeadingUnderscore bool
}

// Defaults returns slugify's defaults: "-", lowercase, decamelize.
func Defaults() Options {
	return Options{Separator: "-", Lowercase: true, Decamelize: true}
}

var (
	decamel1   = regexp.MustCompile(`([A-Z]{2,})(\d+)`)
	decamel2   = regexp.MustCompile(`([a-z\d]+)([A-Z]{2,})`)
	decamel3   = regexp.MustCompile(`([a-z\d])([A-Z])`)
	decamel4   = regexp.MustCompile(`([A-Z]+)([A-Z][a-z\d]+)`)
	notAlnum   = regexp.MustCompile(`[^a-zA-Z\d]+`)
	notLowAlnm = regexp.MustCompile(`[^a-z\d]+`)
)

// Make slugifies s.
func Make(s string, o Options) string {
	prependUnderscore := o.PreserveLeadingUnderscore && strings.HasPrefix(s, "_")
	s = Transliterate(s, o.CustomReplacements)
	if o.Decamelize {
		s = decamel1.ReplaceAllString(s, "$1 $2")
		s = decamel2.ReplaceAllString(s, "$1 $2")
		s = decamel3.ReplaceAllString(s, "$1 $2")
		s = decamel4.ReplaceAllString(s, "$1 $2")
	}
	pattern := notAlnum
	if o.Lowercase {
		s = strings.ToLower(s)
		pattern = notLowAlnm
	}
	s = pattern.ReplaceAllLiteralString(s, o.Separator)
	s = strings.ReplaceAll(s, `\`, "")
	if o.Separator != "" {
		s = removeMootSeparators(s, o.Separator)
	}
	if prependUnderscore {
		s = "_" + s
	}
	return s
}

func removeMootSeparators(s, sep string) string {
	q := regexp.QuoteMeta(sep)
	// Same regexes as the JS: for a multi-character separator {2,} binds to its
	// last character only, and that quirk is kept on purpose.
	s = regexp.MustCompile(q+`{2,}`).ReplaceAllLiteralString(s, sep)
	return regexp.MustCompile(`^`+q+`|`+q+`$`).ReplaceAllLiteralString(s, "")
}

// Transliterate is @sindresorhus/transliterate: NFC, replacement table (custom
// entries override built-in ones in place), then lodash's deburr.
func Transliterate(s string, custom [][2]string) string {
	s = norm.NFC.String(s)
	table := transliterations
	if len(custom) > 0 {
		table = mergeReplacements(transliterations, custom)
	}
	for _, r := range table {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return Deburr(s)
}

func mergeReplacements(base, custom [][2]string) [][2]string {
	out := append([][2]string(nil), base...)
	index := make(map[string]int, len(out))
	for i, r := range out {
		index[r[0]] = i
	}
	for _, r := range custom {
		if i, ok := index[r[0]]; ok {
			out[i][1] = r[1]
		} else {
			index[r[0]] = len(out)
			out = append(out, r)
		}
	}
	return out
}

// Deburr is lodash's deburr: Latin-1 and Latin Extended-A letters to ASCII,
// combining marks removed.
func Deburr(s string) string {
	var b strings.Builder
	for _, r := range s {
		if v, ok := deburred[r]; ok {
			b.WriteString(v)
			continue
		}
		if isComboMark(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isComboMark(r rune) bool {
	return unicode.In(r, comboMarks)
}

var comboMarks = &unicode.RangeTable{R16: []unicode.Range16{
	{Lo: 0x0300, Hi: 0x036f, Stride: 1},
	{Lo: 0x20d0, Hi: 0x20ff, Stride: 1},
	{Lo: 0xfe20, Hi: 0xfe2f, Stride: 1},
}}

// RegressedEnumValue is how Strapi normalizes an enum value before checking it
// against the GraphQL name pattern and for collisions.
func RegressedEnumValue(v string) string {
	return Make(v, Options{Separator: "_"})
}
