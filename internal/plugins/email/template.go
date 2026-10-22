package email

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/jsnum"
)

// keysDeep is @strapi/utils' objects.keysDeep: the paths of the leaves
// (an empty object has none, arrays go by index).
func keysDeep(v any, prefix string) []string {
	join := func(k string) string {
		if prefix == "" {
			return k
		}
		return prefix + "." + k
	}
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []string
		for _, k := range keys {
			out = append(out, keysDeep(x[k], join(k))...)
		}
		return out
	case []any:
		var out []string
		for i, e := range x {
			out = append(out, keysDeep(e, join(strconv.Itoa(i)))...)
		}
		return out
	}
	return []string{prefix}
}

func lookup(data map[string]any, path string) any {
	var cur any = data
	for _, p := range strings.Split(path, ".") {
		switch x := cur.(type) {
		case map[string]any:
			cur = x[p]
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			cur = x[i]
		default:
			return nil
		}
	}
	return cur
}

// jsString is String(v) for template output.
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return jsnum.String(x)
	case int, int64:
		return fmt.Sprint(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = jsString(e)
		}
		return strings.Join(parts, ",")
	case map[string]any:
		return "[object Object]"
	}
	return fmt.Sprint(v)
}

// Template is users-permissions' template: lodash _.template with a strict
// interpolation on the data's paths, evaluate and escape off. Anything
// else, <% code %> included, stays as written.
func Template(layout string, data map[string]any) string {
	paths := keysDeep(data, "")
	if len(paths) == 0 {
		return layout
	}
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = regexp.QuoteMeta(p)
	}
	re := regexp.MustCompile(`<%=\s*(` + strings.Join(quoted, "|") + `)\s*%>`)
	return re.ReplaceAllStringFunc(layout, func(m string) string {
		return jsString(lookup(data, re.FindStringSubmatch(m)[1]))
	})
}
