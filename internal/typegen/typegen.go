// Package typegen writes the TypeScript declarations of
// `strapi ts:generate-types` (@strapi/typescript-utils): one interface per
// content type and component, for projects typed with @strapi/strapi.
package typegen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aluzed/grapi/schema"
)

// Files are the generated declarations.
type Files struct{ ContentTypes, Components string }

const (
	noContentTypes = "/*\n * The app doesn't have any content-types yet.\n */\n"
	noComponents   = "/*\n * The app doesn't have any components yet.\n */\n"
)

// Generate writes the declarations of set.
func Generate(set *schema.Set) Files {
	var f Files
	var cts []*schema.ContentType
	for _, ct := range set.ContentTypes.All() {
		cts = append(cts, ct)
	}
	sort.Slice(cts, func(i, j int) bool { return cts[i].UID < cts[j].UID })
	var comps []*schema.Component
	for _, c := range set.Components.All() {
		comps = append(comps, c)
	}
	sort.Slice(comps, func(i, j int) bool { return comps[i].UID < comps[j].UID })

	if len(cts) == 0 {
		f.ContentTypes = noContentTypes
	} else {
		var b strings.Builder
		b.WriteString("import type { Schema, Struct } from '@strapi/strapi';\n\n")
		var entries []string
		for _, ct := range cts {
			name := interfaceName(string(ct.UID))
			parent := "Struct.CollectionTypeSchema"
			if ct.IsSingleType() {
				parent = "Struct.SingleTypeSchema"
			}
			var info map[string]any
			raw, _ := json.Marshal(ct.Info)
			_ = json.Unmarshal(raw, &info)
			// Strapi sets draftAndPublish on every content type it loads.
			options := map[string]any{"draftAndPublish": false}
			if ct.Options != nil {
				for k, v := range ct.Options.All() {
					options[k] = decode(v)
				}
			}
			writeInterface(&b, name, parent, []literal{
				{"collectionName", ct.CollectionName}, {"info", info}, {"options", options}, {"pluginOptions", decode(ct.PluginOptions)},
			}, &ct.Attributes)
			entries = append(entries, fmt.Sprintf("      %s: %s;\n", quote(string(ct.UID)), name))
		}
		writeModule(&b, "ContentTypeSchemas", entries)
		f.ContentTypes = b.String()
	}
	if len(comps) == 0 {
		f.Components = noComponents
	} else {
		var b strings.Builder
		b.WriteString("import type { Schema, Struct } from '@strapi/strapi';\n\n")
		var entries []string
		for _, c := range comps {
			name := interfaceName(string(c.UID))
			var info map[string]any
			raw, _ := json.Marshal(c.Info)
			_ = json.Unmarshal(raw, &info)
			var options any
			if c.Options != nil {
				m := map[string]any{}
				for k, v := range c.Options.All() {
					m[k] = decode(v)
				}
				options = m
			}
			writeInterface(&b, name, "Struct.ComponentSchema", []literal{
				{"collectionName", c.CollectionName}, {"info", info}, {"options", options},
			}, &c.Attributes)
			entries = append(entries, fmt.Sprintf("      %s: %s;\n", quote(string(c.UID)), name))
		}
		writeModule(&b, "ComponentSchemas", entries)
		f.Components = b.String()
	}
	return f
}

type literal struct {
	key   string
	value any
}

func decode(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

// interfaceName is upperFirst(camelCase(uid)).
func interfaceName(uid string) string { return schema.PascalCase(uid) }

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

func writeInterface(b *strings.Builder, name, parent string, literals []literal, attrs *schema.OrderedMap[*schema.Attribute]) {
	fmt.Fprintf(b, "export interface %s extends %s {\n", name, parent)
	for _, l := range literals {
		if !empty(l.value) {
			fmt.Fprintf(b, "  %s: %s;\n", l.key, typeLiteral(l.value))
		}
	}
	b.WriteString("  attributes: {\n")
	var names []string
	for n := range attrs.All() {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return localeLess(names[i], names[j]) })
	for _, n := range names {
		a, _ := attrs.Get(n)
		if t := attributeType(a); t != "" {
			fmt.Fprintf(b, "    %s: %s;\n", n, t)
		}
	}
	b.WriteString("  };\n}\n\n")
}

func writeModule(b *strings.Builder, iface string, entries []string) {
	fmt.Fprintf(b, "declare module '@strapi/strapi' {\n  export namespace Public {\n    export interface %s {\n", iface)
	for _, e := range entries {
		b.WriteString(e)
	}
	b.WriteString("    }\n  }\n}\n")
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
}

func identifier(k string) string {
	if strings.Contains(k, "-") {
		return quote(k)
	}
	return k
}

// typeLiteral is toTypeLiteral.
func typeLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return quote(x)
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = typeLiteral(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = quote(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return localeLess(keys[i], keys[j]) })
		var b strings.Builder
		b.WriteString("{ ")
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %s; ", identifier(k), typeLiteral(x[k]))
		}
		b.WriteString("}")
		return b.String()
	}
	return fmt.Sprint(v)
}

var baseTypes = map[string]string{
	schema.TypeString: "String", schema.TypeText: "Text", schema.TypeRichText: "RichText", schema.TypePassword: "Password",
	schema.TypeEmail: "Email", schema.TypeDate: "Date", schema.TypeTime: "Time", schema.TypeDateTime: "DateTime",
	schema.TypeTimestamp: "Timestamp", schema.TypeInteger: "Integer", schema.TypeBigInteger: "BigInteger",
	schema.TypeFloat: "Float", schema.TypeDecimal: "Decimal", schema.TypeBoolean: "Boolean", schema.TypeJSON: "JSON",
	schema.TypeBlocks: "Blocks",
}

func ns(s string) string { return "Schema.Attribute." + s }

// attributeType is attributeToPropertySignature's type: the base type,
// then the modifiers.
func attributeType(a *schema.Attribute) string {
	var base string
	switch a.Type {
	case schema.TypeUID:
		base = ns("UID")
		opts := decode(a.Options)
		if a.TargetField != "" || opts != nil {
			params := []string{"undefined"}
			if a.TargetField != "" {
				params[0] = quote(a.TargetField)
			}
			if m, ok := opts.(map[string]any); ok {
				params = append(params, typeLiteral(m))
			}
			base += "<" + strings.Join(params, ", ") + ">"
		}
	case schema.TypeEnumeration:
		base = ns("Enumeration") + "<" + typeLiteral(a.Enum) + ">"
	case schema.TypeMedia:
		base = ns("Media")
		var params []string
		types := "undefined"
		if a.AllowedTypes != nil {
			q := make([]string, len(a.AllowedTypes))
			for i, t := range a.AllowedTypes {
				q[i] = quote(t)
			}
			types = strings.Join(q, " | ")
		}
		if a.AllowedTypes != nil || a.IsMultiple() {
			params = append(params, types)
		}
		if a.IsMultiple() {
			params = append(params, "true")
		}
		if len(params) > 0 {
			base += "<" + strings.Join(params, ", ") + ">"
		}
	case schema.TypeRelation:
		if strings.Contains(strings.ToLower(a.Relation), "morph") {
			base = ns("Relation") + "<" + quote(a.Relation) + ">"
		} else {
			base = ns("Relation") + "<" + quote(a.Relation) + ", " + quote(a.Target) + ">"
		}
	case schema.TypeComponent:
		base = ns("Component") + "<" + quote(a.Component) + ", " + strconv.FormatBool(a.IsRepeatable()) + ">"
	case schema.TypeDynamicZone:
		base = ns("DynamicZone") + "<" + typeLiteral(a.Components) + ">"
	default:
		t, ok := baseTypes[a.Type]
		if !ok {
			return ""
		}
		base = ns(t)
	}
	parts := []string{base}
	if a.IsRequired() {
		parts = append(parts, ns("Required"))
	}
	if a.IsPrivate() {
		parts = append(parts, ns("Private"))
	}
	if a.IsUnique() {
		parts = append(parts, ns("Unique"))
	}
	if a.Configurable != nil && *a.Configurable {
		parts = append(parts, ns("Configurable"))
	}
	if a.CustomField != "" {
		cf := ns("CustomField") + "<" + quote(a.CustomField)
		if o := decode(a.Options); o != nil {
			cf += ", " + typeLiteral(o)
		}
		parts = append(parts, cf+">")
	}
	if po, ok := decode(a.PluginOptions).(map[string]any); ok && len(po) > 0 {
		parts = append(parts, ns("SetPluginOptions")+"<"+typeLiteral(po)+">")
	}
	lo, hi := decode(a.Min), decode(a.Max)
	if lo != nil || hi != nil {
		mm := map[string]any{}
		kind := "number"
		if lo != nil {
			mm["min"] = lo
			if _, s := lo.(string); s {
				kind = "string"
			}
		}
		if hi != nil {
			mm["max"] = hi
			if _, s := hi.(string); s {
				kind = "string"
			}
		}
		parts = append(parts, ns("SetMinMax")+"<"+typeLiteral(mm)+", "+kind+">")
	}
	if a.MinLength != nil || a.MaxLength != nil {
		mm := map[string]any{}
		if a.MinLength != nil {
			mm["minLength"] = float64(*a.MinLength)
		}
		if a.MaxLength != nil {
			mm["maxLength"] = float64(*a.MaxLength)
		}
		parts = append(parts, ns("SetMinMaxLength")+"<"+typeLiteral(mm)+">")
	}
	if d := decode(a.Default); d != nil {
		parts = append(parts, ns("DefaultTo")+"<"+typeLiteral(d)+">")
	}
	return strings.Join(parts, " & ")
}

// localeLess approximates String.prototype.localeCompare on identifiers:
// letters compare case-insensitively first, lowercase first on a tie,
// punctuation and digits before letters.
func localeLess(a, b string) bool {
	weight := func(r rune) (int, rune) {
		switch {
		case unicode.IsLetter(r):
			return 2, unicode.ToLower(r)
		case unicode.IsDigit(r):
			return 1, r
		}
		return 0, r
	}
	ra, rb := []rune(a), []rune(b)
	for i := 0; i < len(ra) && i < len(rb); i++ {
		ca, la := weight(ra[i])
		cb, lb := weight(rb[i])
		if ca != cb {
			return ca < cb
		}
		if la != lb {
			return la < lb
		}
	}
	if len(ra) != len(rb) {
		return len(ra) < len(rb)
	}
	for i := range ra {
		if ra[i] != rb[i] {
			return unicode.IsLower(ra[i])
		}
	}
	return false
}
