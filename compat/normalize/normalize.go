// Package normalize rewrites API responses so that two engines can be compared:
// generated ids, timestamps and secrets differ by nature, their structure must not.
package normalize

import (
	"fmt"
	"regexp"
	"strings"
)

// documentIDPattern matches Strapi documentIds (cuid2) embedded in free text,
// such as error messages naming the document they could not find.
var documentIDPattern = regexp.MustCompile(`\b[a-z0-9]{24}\b`)

var timestampKeys = map[string]bool{
	"createdAt": true, "updatedAt": true, "publishedAt": true, "firstPublishedAt": true,
}

var secretKeys = map[string]bool{
	"accessKey": true, "jwt": true, "token": true, "refreshToken": true, "hash": true,
}

// Normalizer keeps alias tables across a whole run, so the same id gets the
// same alias in every response of that run.
type Normalizer struct {
	ids  map[string]string
	docs map[string]string
}

// New returns a Normalizer with empty alias tables.
func New() *Normalizer {
	return &Normalizer{ids: map[string]string{}, docs: map[string]string{}}
}

// Value returns a normalized copy of a decoded JSON value.
func (n *Normalizer) Value(v any) any {
	return n.walk("", v)
}

func (n *Normalizer) walk(key string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = n.walk(k, child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = n.walk(key, child)
		}
		return out
	case nil:
		return nil
	}
	switch {
	case timestampKeys[key]:
		return "<ts>"
	case secretKeys[key]:
		return "<secret>"
	case key == "documentId":
		return alias(n.docs, "#doc", fmt.Sprint(v))
	case key == "id":
		return alias(n.ids, "#id", fmt.Sprint(v))
	case key == "url":
		if s, ok := v.(string); ok && strings.Contains(s, "/uploads/") {
			return "<upload-url>"
		}
	}
	if s, ok := v.(string); ok {
		return n.knownDocumentIDs(sortComponentUIDLists(s))
	}
	return v
}

// knownDocumentIDs only rewrites ids already seen in a documentId field, so
// unrelated 24-character words stay untouched.
func (n *Normalizer) knownDocumentIDs(s string) string {
	return documentIDPattern.ReplaceAllStringFunc(s, func(m string) string {
		if a, ok := n.docs[m]; ok {
			return a
		}
		return m
	})
}

func alias(table map[string]string, prefix, raw string) string {
	if a, ok := table[raw]; ok {
		return a
	}
	a := fmt.Sprintf("%s%d", prefix, len(table)+1)
	table[raw] = a
	return a
}
