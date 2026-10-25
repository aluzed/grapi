package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/jsnum"
)

// Env is the process environment as Strapi's env() helper sees it: a key
// is set as soon as it exists, even empty.
type Env map[string]string

// OSEnv snapshots os.Environ.
func OSEnv() Env {
	e := Env{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		e[k] = v
	}
	return e
}

// LoadDotenv adds the variables of a dotenv file without overriding the
// ones already set. A missing file is not an error.
func (e Env) LoadDotenv(path string) error {
	f, err := os.Open(path) //nolint:gosec // the project's own .env
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := parseDotenvLine(sc.Text())
		if !ok {
			continue
		}
		if _, set := e[k]; !set {
			e[k] = v
		}
	}
	return sc.Err()
}

var dotenvKey = regexp.MustCompile(`^[\w.-]+$`)

func parseDotenvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if !dotenvKey.MatchString(k) {
		return "", "", false
	}
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'' || v[0] == '`') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			inner := v[1 : end+1]
			if v[0] == '"' {
				inner = strings.ReplaceAll(inner, `\n`, "\n")
			}
			return k, inner, true
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return k, v, true
}

// number turns a JS number into the int a typed config expects when it
// is one.
func number(f float64) any {
	if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1<<53 {
		return int(f)
	}
	return f
}

func envArray(v string) []any {
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = v[1 : len(v)-1]
	}
	parts := strings.Split(v, ",")
	out := make([]any, len(parts))
	for i, p := range parts {
		out[i] = strings.Trim(strings.Trim(p, " "), `"`)
	}
	return out
}

// envCall runs one env helper. def is the raw default text, absent when
// hasDef is false.
func (e Env) envCall(fn, key, def string, hasDef bool, choices []string) (any, error) {
	raw, set := e[key]
	if fn == "oneOf" {
		if hasDef && def != "" && !contains(choices, def) {
			return nil, fmt.Errorf("env.oneOf requires defaultValue to be included in expectedValues")
		}
		v, ok := raw, set
		if !set && hasDef {
			v, ok = def, true
		}
		if ok && contains(choices, v) {
			return v, nil
		}
		if hasDef {
			return def, nil
		}
		return nil, nil
	}
	if !set {
		if !hasDef {
			return nil, nil
		}
		return typedDefault(fn, key, def)
	}
	switch fn {
	case "":
		return raw, nil
	case "int":
		return number(jsnum.ParseInt(raw)), nil
	case "float":
		return number(jsnum.ParseFloat(raw)), nil
	case "bool":
		return raw == "true", nil
	case "json":
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("invalid json environment variable %s: %w", key, err)
		}
		return v, nil
	case "array":
		return envArray(raw), nil
	case "date":
		return parseDate(raw), nil
	}
	return nil, fmt.Errorf("unknown env helper env.%s", fn)
}

func typedDefault(fn, key, def string) (any, error) {
	switch fn {
	case "", "oneOf":
		return def, nil
	case "int", "float":
		f, err := strconv.ParseFloat(def, 64)
		if err != nil {
			return nil, fmt.Errorf("default of env.%s %s is not a number: %q", fn, key, def)
		}
		return number(f), nil
	case "bool":
		return def == "true", nil
	case "json":
		var v any
		if err := json.Unmarshal([]byte(def), &v); err != nil {
			return nil, fmt.Errorf("default of env.json %s is not JSON: %w", key, err)
		}
		return v, nil
	case "array":
		return envArray(def), nil
	case "date":
		return parseDate(def), nil
	}
	return nil, fmt.Errorf("unknown env helper env.%s", fn)
}

// parseDate accepts the formats a JS Date reliably parses; anything else
// is an invalid date, null.
func parseDate(s string) any {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
