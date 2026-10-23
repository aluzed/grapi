package httpapi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/aluzed/grapi/internal/jsnum"
)

// The users-permissions bodies are checked with yup in strict mode
// (validateYupSchema): no cast, all errors collected in yup's order.

type bodyField struct {
	name     string
	required bool
	// mixed fields skip the string type check.
	mixed bool
	email bool
	min   int
	tests []func(value any, body map[string]any) string
}

type bodySchema struct {
	fields    []bodyField
	noUnknown bool
}

type bodyError struct {
	path    []string
	message string
	value   any
	hasVal  bool
}

func (e bodyError) MarshalJSON() ([]byte, error) {
	m := map[string]any{"path": e.path, "message": e.message, "name": "ValidationError"}
	if e.hasVal {
		m["value"] = e.value
	}
	return json.Marshal(m)
}

// yup's email regex (0.32).
var yupEmail = regexp.MustCompile(`(?i)^((([a-z]|\d|[!#\$%&'\*\+\-\/=\?\^_` + "`" + `{\|}~]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])+(\.([a-z]|\d|[!#\$%&'\*\+\-\/=\?\^_` + "`" + `{\|}~]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])+)*)|((\x22)((((\x20|\x09)*(\x0d\x0a))?(\x20|\x09)+)?(([\x01-\x08\x0b\x0c\x0e-\x1f\x7f]|\x21|[\x23-\x5b]|[\x5d-\x7e]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])|(\\([\x01-\x09\x0b\x0c\x0d-\x7f]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}]))))*(((\x20|\x09)*(\x0d\x0a))?(\x20|\x09)+)?(\x22)))@((([a-z]|\d|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])|(([a-z]|\d|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])([a-z]|\d|-|\.|_|~|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])*([a-z]|\d|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])))\.)+(([a-z]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])|(([a-z]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])([a-z]|\d|-|\.|_|~|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])*([a-z]|[\x{00A0}-\x{D7FF}\x{F900}-\x{FDCF}\x{FDF0}-\x{FFEF}])))$`)

func yupPrint(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return `"` + x + `"`
	case bool:
		return fmt.Sprint(x)
	case float64:
		return jsnum.String(x)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (sc bodySchema) validate(body map[string]any) error {
	var errs []bodyError
	// yup validates the fields in reverse order (its toposort of fields
	// without dependencies), then sorts the errors by the first key whose
	// name is contained in the error path: password and passwordConfirmation
	// tie, so the latter comes first.
	for i := len(sc.fields) - 1; i >= 0; i-- {
		f := sc.fields[i]
		value, present := body[f.name]
		fail := func(msg string) {
			errs = append(errs, bodyError{path: []string{f.name}, message: msg, value: value, hasVal: present})
		}
		str, isStr := value.(string)
		if present && !f.mixed && !isStr {
			fail(fmt.Sprintf("%s must be a `string` type, but the final value was: `%s`.", f.name, yupPrint(value)))
			continue
		}
		if f.required && (!present || value == nil || (isStr && str == "")) {
			fail(f.name + " is a required field")
			continue
		}
		if !present || value == nil {
			continue
		}
		if f.email && isStr && str != "" && !yupEmail.MatchString(str) {
			fail(f.name + " must be a valid email")
		}
		if f.min > 0 && isStr && utf8.RuneCountInString(str) < f.min {
			fail(fmt.Sprintf("%s must be at least %d characters", f.name, f.min))
		}
		for _, t := range f.tests {
			if msg := t(value, body); msg != "" {
				fail(msg)
			}
		}
	}
	keyIndex := func(e bodyError) int {
		path := strings.Join(e.path, ".")
		for i, f := range sc.fields {
			if strings.Contains(path, f.name) {
				return i
			}
		}
		return len(sc.fields)
	}
	sort.SliceStable(errs, func(a, b int) bool { return keyIndex(errs[a]) < keyIndex(errs[b]) })
	if sc.noUnknown {
		var unknown []string
		for _, k := range sortedKeys(body) {
			known := false
			for _, f := range sc.fields {
				known = known || f.name == k
			}
			if !known {
				unknown = append(unknown, k)
			}
		}
		if len(unknown) > 0 {
			errs = append(errs, bodyError{path: []string{}, message: "this field has unspecified keys: " + strings.Join(unknown, ", "), value: body, hasVal: true})
		}
	}
	if len(errs) == 0 {
		return nil
	}
	msg := errs[0].message
	if len(errs) > 1 {
		msg = fmt.Sprintf("%d errors occurred", len(errs))
	}
	return validationError(msg, map[string]any{"errors": errs})
}

func maxPasswordBytes(value any, _ map[string]any) string {
	if s, ok := value.(string); ok && len(s) > 72 {
		return "Password must be less than 73 bytes"
	}
	return ""
}

func matchesPassword(value any, body map[string]any) string {
	if value != body["password"] {
		return "Passwords do not match"
	}
	return ""
}
