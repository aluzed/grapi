package schema

import (
	"fmt"
	"strings"
)

// UID identifies a content type ("api::article.article", "plugin::upload.file",
// "admin::user") or a component ("shared.seo").
type UID string

// Namespaces of content type UIDs.
const (
	NamespaceAPI    = "api"
	NamespacePlugin = "plugin"
	NamespaceAdmin  = "admin"
	NamespaceStrapi = "strapi"
)

// APIUID builds the UID of a content type declared in src/api/<api>.
func APIUID(api, singular string) UID { return UID(fmt.Sprintf("api::%s.%s", api, singular)) }

// PluginUID builds the UID of a content type declared by a plugin.
func PluginUID(plugin, singular string) UID {
	return UID(fmt.Sprintf("plugin::%s.%s", plugin, singular))
}

// AdminUID builds the UID of an admin content type.
func AdminUID(singular string) UID { return UID("admin::" + singular) }

// ComponentUID builds the UID of a component file src/components/<category>/<name>.json.
func ComponentUID(category, name string) UID { return UID(category + "." + name) }

// Namespace returns "api", "plugin", "admin", "strapi", or "" for a component.
func (u UID) Namespace() string {
	ns, _, ok := strings.Cut(string(u), "::")
	if !ok {
		return ""
	}
	return ns
}

// IsComponent reports whether u names a component.
func (u UID) IsComponent() bool { return !strings.Contains(string(u), "::") }

// Origin returns the api or plugin name ("article" in api::article.article).
func (u UID) Origin() string {
	_, rest, ok := strings.Cut(string(u), "::")
	if !ok {
		return ""
	}
	origin, _, _ := strings.Cut(rest, ".")
	return origin
}

func (u UID) String() string { return string(u) }
