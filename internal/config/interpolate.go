package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A directive is ${env:KEY}, ${env:KEY:default}, ${env.<helper>:KEY:default},
// ${env.oneOf:KEY:a|b|c:default} or ${file:path}.
var directive = regexp.MustCompile(`\$\{(env(?:\.[a-zA-Z]+)?|file):([^{}]*)\}`)

type resolver struct {
	env  Env
	root string
}

// resolve interpolates every string of a decoded config value. A string
// that is a single directive takes the directive's type.
func (r *resolver) resolve(v any, path string) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			out, err := r.resolve(e, joinKey(path, k))
			if err != nil {
				return nil, err
			}
			x[k] = out
		}
		return x, nil
	case []any:
		for i, e := range x {
			out, err := r.resolve(e, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			x[i] = out
		}
		return x, nil
	case string:
		return r.resolveString(x, path)
	}
	return v, nil
}

func (r *resolver) resolveString(s, path string) (any, error) {
	// A directive inside another one (${file:dkim/${env:KEY}}) is replaced
	// by its text first, innermost first.
	for {
		nested := false
		locs := directive.FindAllStringSubmatchIndex(s, -1)
		for i := len(locs) - 1; i >= 0; i-- {
			l := locs[i]
			if strings.Count(s[:l[0]], "${") <= strings.Count(s[:l[0]], "}") {
				continue
			}
			v, err := r.run(s[l[2]:l[3]], s[l[4]:l[5]], path)
			if err != nil {
				return nil, err
			}
			text := ""
			if v != nil {
				text = fmt.Sprint(v)
			}
			s = s[:l[0]] + text + s[l[1]:]
			nested = true
		}
		if !nested {
			break
		}
	}
	locs := directive.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return s, nil
	}
	if len(locs) == 1 && locs[0][0] == 0 && locs[0][1] == len(s) {
		return r.run(s[locs[0][2]:locs[0][3]], s[locs[0][4]:locs[0][5]], path)
	}
	var b strings.Builder
	last := 0
	for _, l := range locs {
		b.WriteString(s[last:l[0]])
		v, err := r.run(s[l[2]:l[3]], s[l[4]:l[5]], path)
		if err != nil {
			return nil, err
		}
		if v != nil {
			fmt.Fprint(&b, v)
		}
		last = l[1]
	}
	b.WriteString(s[last:])
	return b.String(), nil
}

func (r *resolver) run(kind, args, path string) (any, error) {
	if kind == "file" {
		name := args
		if !filepath.IsAbs(name) {
			name = filepath.Join(r.root, name)
		}
		b, err := os.ReadFile(name) //nolint:gosec // a path from the project's own config
		if err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
		return string(b), nil
	}
	fn := strings.TrimPrefix(strings.TrimPrefix(kind, "env"), ".")
	var choices []string
	if fn == "oneOf" {
		parts := strings.SplitN(args, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("config %s: env.oneOf needs KEY:a|b|c", path)
		}
		choices = strings.Split(parts[1], "|")
		def, hasDef := "", len(parts) == 3
		if hasDef {
			def = parts[2]
		}
		v, err := r.env.envCall(fn, parts[0], def, hasDef, choices)
		if err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
		return v, nil
	}
	key, def, hasDef := strings.Cut(args, ":")
	v, err := r.env.envCall(fn, key, def, hasDef, nil)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return v, nil
}

func joinKey(parent, k string) string {
	if parent == "" {
		return k
	}
	return parent + "." + k
}
