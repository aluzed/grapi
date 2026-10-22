package registry

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// Kinds of user code found in a Strapi project. grapi does not run JS or TS
// (phase 1): each kind comes with the advice printed by `grapi doctor`.
const (
	KindDefaultFactory  = "default-factory"
	KindCustomRoutes    = "custom-routes"
	KindRoutesJSON      = "routes-json"
	KindController      = "controller"
	KindService         = "service"
	KindPolicy          = "policy"
	KindMiddleware      = "middleware"
	KindLifecycles      = "lifecycles"
	KindAppIndex        = "app-index"
	KindAPIIndex        = "api-index"
	KindPluginExtension = "plugin-extension"
	KindConfig          = "config"
	KindComponentScript = "component-script"
)

var advice = map[string]string{
	KindDefaultFactory:  "nothing to do: grapi provides the default router, controller and service",
	KindCustomRoutes:    "declare the routes in routes/*.json or register them from Go",
	KindRoutesJSON:      "read as is",
	KindController:      "rewrite as a Go controller override (framework mode) or wait for JS hooks",
	KindService:         "rewrite as a Go service (framework mode) or wait for JS hooks",
	KindPolicy:          "rewrite as a Go policy (framework mode) or wait for JS hooks",
	KindMiddleware:      "rewrite as a Go middleware (framework mode) or wait for JS hooks",
	KindLifecycles:      "rewrite as Go database lifecycles or document middlewares (framework mode)",
	KindAppIndex:        "move register/bootstrap logic to Go hooks (framework mode)",
	KindAPIIndex:        "move register/bootstrap logic to Go hooks (framework mode)",
	KindPluginExtension: "rewrite the plugin extension in Go (framework mode)",
	KindConfig:          "convert to YAML configuration",
	KindComponentScript: "components must be JSON files",
}

// CodeFile is a user code file grapi does not execute.
type CodeFile struct {
	Path   string
	Kind   string
	UID    string
	Advice string
}

func (l *loader) code(p, kind string) {
	l.res.Code = append(l.res.Code, CodeFile{Path: p, Kind: kind, Advice: advice[kind]})
}

var (
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment  = regexp.MustCompile(`(?m)^\s*//.*$`)
	spaces       = regexp.MustCompile(`\s+`)
	// The only shape treated as "default": import or require of factories, then
	// export of createCoreX('<uid>') with no second argument.
	defaultFactory = regexp.MustCompile(`^(?:import\{factories\}from(['"])@strapi/strapi(['"]);?|const\{factories\}=require\((['"])@strapi/strapi(['"])\);?)?` +
		`(?:exportdefault|module\.exports=)(?:factories|require\(['"]@strapi/strapi['"]\)\.factories)\.createCore(?:Router|Controller|Service)\((?:'([^']+)'|"([^"]+)")\);?$`)
)

// defaultFactoryUID returns the uid when src only calls the default factory.
func defaultFactoryUID(src string) (string, bool) {
	s := blockComment.ReplaceAllString(src, "")
	s = lineComment.ReplaceAllString(s, "")
	s = spaces.ReplaceAllString(s, "")
	m := defaultFactory.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[5] + m[6], true
}

func (l *loader) apiCode(apiDir string) {
	for _, f := range files(l.app, apiDir) {
		if isScript(f) && strings.HasPrefix(f, "index.") {
			l.code(path.Join(apiDir, f), KindAPIIndex)
		}
	}
	kinds := map[string]string{
		"routes": KindCustomRoutes, "controllers": KindController, "services": KindService,
		"policies": KindPolicy, "middlewares": KindMiddleware,
	}
	for _, dir := range []string{"routes", "controllers", "services", "policies", "middlewares"} {
		d := path.Join(apiDir, dir)
		for _, f := range files(l.app, d) {
			p := path.Join(d, f)
			if dir == "routes" && strings.HasSuffix(f, ".json") {
				l.code(p, KindRoutesJSON)
				continue
			}
			if !isScript(f) {
				continue
			}
			src, err := fs.ReadFile(l.app, p)
			if err == nil && (dir == "routes" || dir == "controllers" || dir == "services") {
				if uid, ok := defaultFactoryUID(string(src)); ok {
					l.res.Code = append(l.res.Code, CodeFile{Path: p, Kind: KindDefaultFactory, UID: uid, Advice: advice[KindDefaultFactory]})
					continue
				}
			}
			l.code(p, kinds[dir])
		}
	}
}

func (l *loader) userCode() {
	for _, f := range files(l.app, "src") {
		if isScript(f) && strings.HasPrefix(f, "index.") {
			l.code(path.Join("src", f), KindAppIndex)
		}
	}
	for _, dir := range []string{"src/middlewares", "src/policies"} {
		kind := KindMiddleware
		if dir == "src/policies" {
			kind = KindPolicy
		}
		for _, f := range files(l.app, dir) {
			if isScript(f) {
				l.code(path.Join(dir, f), kind)
			}
		}
	}
	for _, plugin := range subdirs(l.app, "src/extensions") {
		d := path.Join("src/extensions", plugin)
		for _, f := range files(l.app, d) {
			if isScript(f) && strings.HasPrefix(f, "strapi-server.") {
				l.code(path.Join(d, f), KindPluginExtension)
			}
		}
		_ = fs.WalkDir(l.app, path.Join(d, "content-types"), func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() && isScript(e.Name()) {
				kind := KindPluginExtension
				if strings.HasPrefix(e.Name(), "lifecycles.") {
					kind = KindLifecycles
				}
				l.code(p, kind)
			}
			return nil
		})
	}
	for _, f := range files(l.app, "config") {
		if isScript(f) {
			l.code(path.Join("config", f), KindConfig)
		}
	}
	_ = fs.WalkDir(l.app, "config/env", func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && isScript(e.Name()) {
			l.code(p, KindConfig)
		}
		return nil
	})
}
