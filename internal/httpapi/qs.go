// Package httpapi serves Strapi's REST content API: query parsing, routes,
// controllers, permissions, sanitization and error format.
package httpapi

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The query string parser of the qs package with Strapi's settings
// (strictNullHandling, arrayLimit 100, depth 20): "a[b][0]=x" is
// {"a": {"b": ["x"]}}, a key without "=" is null.

const (
	qsArrayLimit     = 100
	qsDepth          = 20
	qsParameterLimit = 1000
)

// undefined is a hole of a sparse array, removed by compact.
type undefined struct{}

// qsDecode is decodeURIComponent after "+" to space; an invalid sequence
// leaves the string as it was.
func qsDecode(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	if d, err := url.PathUnescape(s); err == nil && utf8.ValidString(d) {
		return d
	}
	return s
}

// Keys of JavaScript's Object.prototype: qs drops a key using one.
var protoKeys = map[string]bool{
	"__proto__": true, "constructor": true, "hasOwnProperty": true, "isPrototypeOf": true,
	"propertyIsEnumerable": true, "toLocaleString": true, "toString": true, "valueOf": true,
	"__defineGetter__": true, "__defineSetter__": true, "__lookupGetter__": true, "__lookupSetter__": true,
}

// ParseQuery parses a raw query string like qs.parse.
func ParseQuery(raw string) map[string]any {
	raw = strings.TrimPrefix(raw, "?")
	temp := map[string]any{}
	var order []string
	if raw == "" {
		return map[string]any{}
	}
	parts := strings.Split(raw, "&")
	if len(parts) > qsParameterLimit {
		parts = parts[:qsParameterLimit]
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		pos := strings.Index(part, "]=")
		if pos == -1 {
			pos = strings.Index(part, "=")
		} else {
			pos++
		}
		var key string
		var val any
		if pos == -1 {
			key = qsDecode(part)
			val = nil
		} else {
			key = qsDecode(part[:pos])
			val = qsDecode(part[pos+1:])
		}
		if existing, ok := temp[key]; ok {
			temp[key] = combine(existing, val)
			continue
		}
		temp[key] = val
		order = append(order, key)
	}
	var obj any = map[string]any{}
	for _, k := range order {
		obj = qsMerge(obj, parseKeys(k, temp[k]))
	}
	out, _ := compact(obj).(map[string]any)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func combine(a, b any) any {
	var out []any
	if l, ok := a.([]any); ok {
		out = append(out, l...)
	} else {
		out = append(out, a)
	}
	if l, ok := b.([]any); ok {
		out = append(out, l...)
	} else {
		out = append(out, b)
	}
	return out
}

// splitKeyIntoSegments is qs 6.16's: bracket groups are balanced, and an
// unterminated group or what lies past the depth stays one literal segment.
func splitKeyIntoSegments(key string) ([]string, bool) {
	var segments []string
	first := strings.IndexByte(key, '[')
	parent := key
	if first >= 0 {
		parent = key[:first]
	}
	if parent != "" {
		if protoKeys[parent] {
			return nil, false
		}
		segments = append(segments, parent)
	}
	open, collected := first, 0
	for open >= 0 && collected < qsDepth {
		level, closeAt := 1, -1
		for i := open + 1; i < len(key) && closeAt < 0; i++ {
			switch key[i] {
			case '[':
				level++
			case ']':
				level--
				if level == 0 {
					closeAt = i
				}
			}
		}
		if closeAt < 0 {
			return append(segments, "["+key[open:]+"]"), true
		}
		seg := key[open : closeAt+1]
		if protoKeys[seg[1:len(seg)-1]] {
			return nil, false
		}
		segments = append(segments, seg)
		collected++
		next := strings.IndexByte(key[closeAt+1:], '[')
		if next < 0 {
			open = -1
		} else {
			open = closeAt + 1 + next
		}
	}
	if open >= 0 {
		segments = append(segments, "["+key[open:]+"]")
	}
	return segments, true
}

func parseKeys(key string, val any) any {
	if key == "" {
		return nil
	}
	keys, ok := splitKeyIntoSegments(key)
	if !ok {
		return nil
	}
	return parseObject(keys, val)
}

func parseObject(chain []string, val any) any {
	leaf := val
	for i := len(chain) - 1; i >= 0; i-- {
		root := chain[i]
		if root == "[]" {
			leaf = combine([]any{}, leaf)
			continue
		}
		clean := root
		if strings.HasPrefix(root, "[") && strings.HasSuffix(root, "]") {
			clean = root[1 : len(root)-1]
		}
		if idx, err := strconv.Atoi(clean); err == nil && root != clean && strconv.Itoa(idx) == clean && idx >= 0 && idx < qsArrayLimit {
			arr := make([]any, idx+1)
			for j := range arr {
				arr[j] = undefined{}
			}
			arr[idx] = leaf
			leaf = arr
			continue
		}
		if clean == "__proto__" {
			leaf = map[string]any{}
			continue
		}
		leaf = map[string]any{clean: leaf}
	}
	return leaf
}

func isObj(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func arrayToObject(a []any) map[string]any {
	out := map[string]any{}
	for i, v := range a {
		if _, hole := v.(undefined); !hole {
			out[strconv.Itoa(i)] = v
		}
	}
	return out
}

// qsMerge is qs' utils.merge.
func qsMerge(target, source any) any {
	if source == nil {
		return target
	}
	if !isObj(source) {
		switch t := target.(type) {
		case []any:
			return append(t, source)
		}
		return []any{target, source}
	}
	if !isObj(target) {
		return combine([]any{target}, source)
	}
	ta, tIsArr := target.([]any)
	sa, sIsArr := source.([]any)
	if tIsArr && sIsArr {
		for i, item := range sa {
			if _, hole := item.(undefined); hole {
				continue
			}
			if i < len(ta) {
				if _, hole := ta[i].(undefined); !hole {
					if isObj(ta[i]) && isObj(item) {
						ta[i] = qsMerge(ta[i], item)
					} else {
						ta = append(ta, item)
					}
					continue
				}
				ta[i] = item
				continue
			}
			for len(ta) < i {
				ta = append(ta, undefined{})
			}
			ta = append(ta, item)
		}
		return ta
	}
	var mt map[string]any
	if tIsArr {
		mt = arrayToObject(ta)
	} else {
		mt = target.(map[string]any)
	}
	var sm map[string]any
	if sIsArr {
		sm = arrayToObject(sa)
	} else {
		sm = source.(map[string]any)
	}
	keys := make([]string, 0, len(sm))
	for k := range sm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if existing, ok := mt[k]; ok {
			mt[k] = qsMerge(existing, sm[k])
		} else {
			mt[k] = sm[k]
		}
	}
	return mt
}

func compact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = compact(e)
		}
		return x
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			if _, hole := e.(undefined); hole {
				continue
			}
			out = append(out, compact(e))
		}
		return out
	}
	return v
}
