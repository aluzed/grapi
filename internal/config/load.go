package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/aluzed/grapi/internal/jsnum"
)

var extensions = map[string]bool{".yaml": true, ".yml": true, ".json": true}

// restricted are the file names Strapi refuses to load.
var restricted = []string{
	"uuid", "hosting", "license", "enforce", "disable", "enable", "telemetry",
	"strapi", "internal",
	"launchedat", "serveadminpanel", "autoreload", "environment", "packagejsonstrapi", "info", "dirs",
	"middleware", "plugin",
}

var builtinFiles = []string{"admin", "server", "api", "database", "middlewares", "plugins", "features"}

// Config is the merged configuration.
type Config struct {
	root map[string]any
	// Env is the environment the config was resolved with.
	Env Env
	// Environment is GRAPI_ENV (NODE_ENV), "development" by default.
	Environment string
	sources     map[string]string
}

// Options of Load.
type Options struct {
	// Dir is the project root; config files are read from Dir/config.
	Dir string
	// ConfigDir overrides Dir/config.
	ConfigDir string
	// Env defaults to the process environment.
	Env Env
	Log *slog.Logger
}

// Load reads .env, the defaults, config/* then config/env/<environment>/*.
func Load(opts Options) (*Config, error) {
	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	env := opts.Env
	if env == nil {
		env = OSEnv()
	}
	dotenv := filepath.Join(opts.Dir, ".env")
	if p, ok := env.Lookup("GRAPI_ENV_PATH", log); ok && p != "" {
		dotenv = p
	}
	if err := env.LoadDotenv(dotenv); err != nil {
		return nil, err
	}
	environment, _ := env.Lookup("GRAPI_ENV", log)
	if environment == "" {
		environment = "development"
	}
	dir := opts.ConfigDir
	if dir == "" {
		dir = filepath.Join(opts.Dir, "config")
	}
	c := &Config{root: defaults(env), Env: env, Environment: environment, sources: map[string]string{}}
	r := &resolver{env: env, root: opts.Dir}
	for _, d := range []string{dir, filepath.Join(dir, "env", environment)} {
		files, err := loadDir(d, r, log)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			c.root[f.key] = merge(c.root[f.key], f.value)
			c.sources[f.key] = f.path
		}
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

type file struct {
	key, path string
	value     any
}

func loadDir(dir string, r *resolver, log *slog.Logger) ([]file, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	seen := map[string]bool{}
	var out []file
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		base := strings.TrimSuffix(name, filepath.Ext(name))
		lower := strings.ToLower(base)
		if !extensions[ext] {
			if ext != ".js" && ext != ".ts" {
				log.Warn("config file not loaded, extension must be one of .yaml,.yml,.json: " + name)
			}
			continue
		}
		if contains(restricted, lower) {
			log.Warn("config file not loaded, restricted filename: " + name)
			continue
		}
		for _, n := range append(append([]string{}, restricted...), builtinFiles...) {
			if strings.HasPrefix(n, lower) && n != lower {
				log.Warn("config file not loaded, filename cannot start with " + n + ": " + name)
			}
		}
		if seen[lower] {
			log.Warn("config file not loaded, case-insensitive name matches other config file: " + name)
			continue
		}
		seen[lower] = true
		path := filepath.Join(dir, name)
		v, err := decode(path, ext)
		if err != nil {
			return nil, err
		}
		v, err = r.resolve(v, base)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, file{key: base, path: path, value: v})
	}
	return out, nil
}

func decode(path, ext string) (any, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the project's own config
	if err != nil {
		return nil, err
	}
	var v any
	if ext == ".json" {
		err = json.Unmarshal(b, &v)
	} else {
		err = yaml.Unmarshal(b, &v)
	}
	if err != nil {
		return nil, fmt.Errorf("could not load config file %s: %w", path, err)
	}
	return normalize(v), nil
}

// normalize turns yaml's typed values into the JSON shapes (float64
// numbers stay ints when yaml gives ints).
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
		return x
	case map[any]any:
		m := map[string]any{}
		for k, e := range x {
			m[fmt.Sprint(k)] = normalize(e)
		}
		return m
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
		return x
	case float64:
		return number(x)
	case nil:
		return nil
	}
	return v
}

// merge is lodash's merge: objects merge deeply, arrays index by index,
// and an undefined source never overwrites.
func merge(dst, src any) any {
	switch s := src.(type) {
	case map[string]any:
		d, ok := dst.(map[string]any)
		if !ok {
			d = map[string]any{}
			if l, isList := dst.([]any); isList {
				for i, e := range l {
					d[fmt.Sprint(i)] = e
				}
			}
		}
		for k, v := range s {
			d[k] = merge(d[k], v)
		}
		return d
	case []any:
		d, ok := dst.([]any)
		if !ok {
			d = nil
		}
		out := append([]any(nil), d...)
		for i, v := range s {
			if i < len(out) {
				out[i] = merge(out[i], v)
			} else {
				out = append(out, merge(nil, v))
			}
		}
		return out
	}
	return src
}

// defaults are Strapi's defaultServerConfig and friends. Like Strapi, the
// server takes HOST and PORT when the project config does not set them.
func defaults(env Env) map[string]any {
	host := any("0.0.0.0")
	if h := env["HOST"]; h != "" {
		host = h
	}
	port := any(1337)
	if p := jsnum.Number(env["PORT"]); jsnum.IsInteger(p) && p != 0 {
		port = int(p)
	}
	return map[string]any{
		"server": map[string]any{
			"host":   host,
			"port":   port,
			"proxy":  false,
			"cron":   map[string]any{"enabled": false},
			"dirs":   map[string]any{"public": "./public"},
			"logger": map[string]any{"updates": map[string]any{"enabled": true}, "startup": map[string]any{"enabled": true}},
		},
		"admin": map[string]any{},
		"api":   map[string]any{"rest": map[string]any{"prefix": "/api", "defaultLimit": 25}},
	}
}

// Get returns the value at a dotted path, nil when absent.
func (c *Config) Get(path string) any {
	var cur any = c.root
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

// String reads a string; ok is false when absent.
func (c *Config) String(path string) (string, bool) {
	s, ok := c.Get(path).(string)
	return s, ok
}

// Int reads an integer.
func (c *Config) Int(path string) (int, bool) {
	switch x := c.Get(path).(type) {
	case int:
		return x, true
	case float64:
		return int(x), true
	}
	return 0, false
}

// Bool reads a boolean.
func (c *Config) Bool(path string) (bool, bool) {
	b, ok := c.Get(path).(bool)
	return b, ok
}

// Raw is the whole merged tree.
func (c *Config) Raw() map[string]any { return c.root }
