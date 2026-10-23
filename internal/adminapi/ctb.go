package adminapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db/naming"
	"github.com/aluzed/grapi/schema"
)

const ctbRead = "plugin::content-type-builder.read"

func (s *Server) ctbRoutes() {
	s.allowed("GET /content-type-builder/schema", ctbRead, s.ctbSchema)
	s.allowed("GET /content-type-builder/status", ctbRead, s.ctbStatus)
	s.allowed("POST /content-type-builder/update-schema", ctbRead, s.ctbUpdate)
}

// SchemaFile is a schema as the builder edits it: the file's JSON, its
// attributes as an ordered list.
type SchemaFile struct {
	UID        string           `json:"uid"`
	Kind       string           `json:"kind,omitempty"`
	Category   string           `json:"category,omitempty"`
	Info       map[string]any   `json:"info"`
	Options    map[string]any   `json:"options"`
	Plugin     map[string]any   `json:"pluginOptions"`
	Attributes []map[string]any `json:"attributes"`
	// Editable is false for the plugins' types, shown for the relations.
	Editable bool `json:"editable"`
}

func (s *Server) srcPath(parts ...string) string {
	return filepath.Join(append([]string{s.cfg.ProjectDir, "src"}, parts...)...)
}

// ctPath is the schema file of an api:: content type.
func (s *Server) ctPath(uid string) (string, bool) {
	rest, ok := strings.CutPrefix(uid, "api::")
	api, name, ok2 := strings.Cut(rest, ".")
	if !ok || !ok2 {
		return "", false
	}
	return s.srcPath("api", api, "content-types", name, "schema.json"), true
}

func (s *Server) componentPath(uid string) (string, bool) {
	cat, name, ok := strings.Cut(uid, ".")
	if !ok {
		return "", false
	}
	return s.srcPath("components", cat, name+".json"), true
}

func readOrdered(path string) (*schema.OrderedMap[json.RawMessage], error) {
	raw, err := os.ReadFile(path) //nolint:gosec // a path of the project's src
	if err != nil {
		return nil, err
	}
	var m schema.OrderedMap[json.RawMessage]
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

func decodeRaw(raw json.RawMessage) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func attributeList(top *schema.OrderedMap[json.RawMessage]) []map[string]any {
	out := []map[string]any{}
	raw, ok := top.Get("attributes")
	if !ok {
		return out
	}
	var attrs schema.OrderedMap[json.RawMessage]
	if json.Unmarshal(raw, &attrs) != nil {
		return out
	}
	for name, a := range attrs.All() {
		m := decodeRaw(a)
		m["name"] = name
		out = append(out, m)
	}
	return out
}

func toSchemaFile(uid string, top *schema.OrderedMap[json.RawMessage], editable bool) SchemaFile {
	f := SchemaFile{UID: uid, Editable: editable, Attributes: attributeList(top)}
	get := func(k string) map[string]any {
		raw, _ := top.Get(k)
		return decodeRaw(raw)
	}
	f.Info, f.Options, f.Plugin = get("info"), get("options"), get("pluginOptions")
	if raw, ok := top.Get("kind"); ok {
		_ = json.Unmarshal(raw, &f.Kind)
	}
	return f
}

// ctbSchema lists the project's content types and components from their
// files, and the plugins' content types (read only, relation targets).
func (s *Server) ctbSchema(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	var cts, comps []SchemaFile
	for uid, ct := range s.set.ContentTypes.All() {
		if strings.HasPrefix(uid, "api::") {
			path, _ := s.ctPath(uid)
			top, err := readOrdered(path)
			if err != nil {
				continue
			}
			cts = append(cts, toSchemaFile(uid, top, true))
			continue
		}
		if strings.HasPrefix(uid, "plugin::") && contains(s.ContentManagerTypes(), uid) {
			raw, _ := json.Marshal(map[string]any{"kind": ct.Kind, "info": ct.Info, "attributes": map[string]any{}})
			var top schema.OrderedMap[json.RawMessage]
			_ = json.Unmarshal(raw, &top)
			cts = append(cts, toSchemaFile(uid, &top, false))
		}
	}
	for uid := range s.set.Components.All() {
		path, _ := s.componentPath(uid)
		top, err := readOrdered(path)
		if err != nil {
			continue
		}
		f := toSchemaFile(uid, top, true)
		f.Category, _, _ = strings.Cut(uid, ".")
		comps = append(comps, f)
	}
	sort.Slice(cts, func(i, j int) bool { return cts[i].UID < cts[j].UID })
	sort.Slice(comps, func(i, j int) bool { return comps[i].UID < comps[j].UID })
	if cts == nil {
		cts = []SchemaFile{}
	}
	if comps == nil {
		comps = []SchemaFile{}
	}
	return data(w, map[string]any{"contentTypes": cts, "components": comps, "develop": s.cfg.Develop})
}

// schemaHash changes when the loaded schemas do: the builder polls it to
// know the reload happened.
func (s *Server) schemaHash() string {
	h := sha256.New()
	for uid, ct := range s.set.ContentTypes.All() {
		b, _ := json.Marshal(ct)
		h.Write([]byte(uid))
		h.Write(b)
	}
	for uid, c := range s.set.Components.All() {
		b, _ := json.Marshal(c)
		h.Write([]byte(uid))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (s *Server) ctbStatus(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	return data(w, map[string]any{"develop": s.cfg.Develop, "schemaHash": s.schemaHash()})
}

// Change is one entry of an update-schema batch.
type Change struct {
	Action string     `json:"action"`
	UID    string     `json:"uid"`
	Data   SchemaFile `json:"data"`
}

var kebab = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

var inverse = map[string]string{schema.OneToOne: schema.OneToOne, schema.OneToMany: schema.ManyToOne, schema.ManyToOne: schema.OneToMany, schema.ManyToMany: schema.ManyToMany}

// plan is the files a batch writes (path to content, nil to delete).
type plan struct {
	files   map[string]*schema.OrderedMap[json.RawMessage]
	removes []string
	renames []rename
	errs    []FieldError
}

// rename is a scalar attribute renamed in place: a migration renames its
// column, so the values stay.
type rename struct {
	uid, from, to string
}

func (p *plan) fail(path []string, format string, args ...any) {
	p.errs = append(p.errs, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
}

func setRaw(m *schema.OrderedMap[json.RawMessage], k string, v any) {
	b, _ := json.Marshal(v)
	m.Set(k, b)
}

// attributesOf turns the edited list into the file's attributes object,
// pulling the inverse side of the bidirectional relations out.
func (s *Server) attributesOf(uid string, list []map[string]any, p *plan, at []string, dp bool) (*schema.OrderedMap[json.RawMessage], map[string]map[string]any) {
	// Names already in the schema load: only the new ones face the
	// builder's stricter reserved names.
	existing := map[string]bool{}
	if ct, ok := s.set.ContentType(schema.UID(uid)); ok {
		for n := range ct.Attributes.All() {
			existing[n] = true
		}
	} else if c, ok := s.set.Component(schema.UID(uid)); ok {
		for n := range c.Attributes.All() {
			existing[n] = true
		}
	}
	var attrs schema.OrderedMap[json.RawMessage]
	inverses := map[string]map[string]any{}
	seen := map[string]bool{}
	for i, a := range list {
		name, _ := a["name"].(string)
		path := append(append([]string{}, at...), "attributes", fmt.Sprint(i))
		switch {
		case name == "":
			p.fail(path, "an attribute needs a name")
			continue
		case seen[name]:
			p.fail(path, "the attribute %s is defined twice", name)
			continue
		case !existing[name] && schema.IsReservedAttributeName(name, dp):
			p.fail(path, "the attribute name %s is reserved", name)
			continue
		}
		seen[name] = true
		out := map[string]any{}
		for k, v := range a {
			if k != "name" && k != "targetAttribute" && k != "renamedFrom" {
				out[k] = v
			}
		}
		if from, _ := a["renamedFrom"].(string); from != "" && from != name {
			p.renames = append(p.renames, rename{uid: uid, from: from, to: name})
		}
		if t, _ := a["type"].(string); t == schema.TypeRelation {
			rel, _ := a["relation"].(string)
			if back, ok := inverse[rel]; ok {
				if ta, _ := a["targetAttribute"].(string); ta != "" && out["mappedBy"] == nil {
					out["inversedBy"] = ta
					target, _ := a["target"].(string)
					inverses[target+"#"+ta] = map[string]any{"type": schema.TypeRelation, "relation": back, "target": uid, "mappedBy": name}
				}
			}
		}
		setRaw(&attrs, name, attributeJSON(out))
	}
	return &attrs, inverses
}

// attributeJSON writes type first, then the other keys sorted: stable
// files, shaped like Strapi's.
func attributeJSON(m map[string]any) *schema.OrderedMap[json.RawMessage] {
	var out schema.OrderedMap[json.RawMessage]
	if t, ok := m["type"]; ok {
		setRaw(&out, "type", t)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		setRaw(&out, k, m[k])
	}
	return &out
}

func collectionName(plural string) string { return strings.ReplaceAll(plural, "-", "_") }

// build turns a batch into a plan, against the files on disk.
func (s *Server) build(changes []Change, components []Change) *plan {
	p := &plan{files: map[string]*schema.OrderedMap[json.RawMessage]{}}
	load := func(path string) *schema.OrderedMap[json.RawMessage] {
		if m, ok := p.files[path]; ok {
			return m
		}
		m, err := readOrdered(path)
		if err != nil {
			return nil
		}
		p.files[path] = m
		return m
	}
	allInverses := map[string]map[string]any{}
	for i, c := range components {
		at := []string{"components", fmt.Sprint(i)}
		uid := c.UID
		if c.Action == "create" {
			cat, name := c.Data.Category, ""
			if dn, _ := c.Data.Info["displayName"].(string); dn != "" {
				name = schema.KebabCase(dn)
			}
			if !kebab.MatchString(cat) || !kebab.MatchString(name) {
				p.fail(at, "the category and the name must be kebab case")
				continue
			}
			uid = cat + "." + name
			if _, ok := s.set.Component(schema.UID(uid)); ok {
				p.fail(at, "the component %s already exists", uid)
				continue
			}
		}
		path, ok := s.componentPath(uid)
		if !ok {
			p.fail(at, "invalid component uid %s", uid)
			continue
		}
		if c.Action == "delete" {
			p.removes = append(p.removes, path)
			continue
		}
		top := load(path)
		if top == nil {
			if c.Action != "create" {
				p.fail(at, "unknown component %s", uid)
				continue
			}
			top = &schema.OrderedMap[json.RawMessage]{}
			cat, name, _ := strings.Cut(uid, ".")
			setRaw(top, "collectionName", "components_"+collectionName(cat+"_"+name+"s"))
		}
		setRaw(top, "info", c.Data.Info)
		if c.Data.Options == nil {
			c.Data.Options = map[string]any{}
		}
		setRaw(top, "options", c.Data.Options)
		attrs, inv := s.attributesOf(uid, c.Data.Attributes, p, at, false)
		for k, v := range inv {
			allInverses[k] = v
		}
		setRaw(top, "attributes", attrs)
		p.files[path] = top
	}
	for i, c := range changes {
		at := []string{"contentTypes", fmt.Sprint(i)}
		uid := c.UID
		singular, _ := c.Data.Info["singularName"].(string)
		plural, _ := c.Data.Info["pluralName"].(string)
		if c.Action == "create" {
			if !kebab.MatchString(singular) || !kebab.MatchString(plural) || singular == plural {
				p.fail(at, "singularName and pluralName must be distinct kebab case names")
				continue
			}
			if schema.IsReservedModelName(singular) || schema.IsReservedModelName(plural) {
				p.fail(at, "the name %s is reserved", singular)
				continue
			}
			for existing, ct := range s.set.ContentTypes.All() {
				if ct.Info.SingularName == singular || ct.Info.PluralName == plural || ct.Info.SingularName == plural || ct.Info.PluralName == singular {
					p.fail(at, "the name is already used by %s", existing)
				}
			}
			uid = "api::" + singular + "." + singular
		}
		if !strings.HasPrefix(uid, "api::") {
			p.fail(at, "only the api:: content types can be edited")
			continue
		}
		path, _ := s.ctPath(uid)
		if c.Action == "delete" {
			p.removes = append(p.removes, filepath.Dir(path))
			continue
		}
		top := load(path)
		if top == nil {
			if c.Action != "create" {
				p.fail(at, "unknown content type %s", uid)
				continue
			}
			top = &schema.OrderedMap[json.RawMessage]{}
			kind := c.Data.Kind
			if kind != schema.SingleType {
				kind = schema.CollectionType
			}
			setRaw(top, "kind", kind)
			setRaw(top, "collectionName", collectionName(plural))
		} else if raw, ok := top.Get("info"); ok {
			old := decodeRaw(raw)
			// The names make the uid and the table: they do not change here.
			c.Data.Info["singularName"], c.Data.Info["pluralName"] = old["singularName"], old["pluralName"]
		}
		setRaw(top, "info", c.Data.Info)
		if c.Data.Options == nil {
			c.Data.Options = map[string]any{}
		}
		setRaw(top, "options", c.Data.Options)
		if len(c.Data.Plugin) > 0 {
			setRaw(top, "pluginOptions", c.Data.Plugin)
		} else {
			top.Delete("pluginOptions")
		}
		dp, _ := c.Data.Options["draftAndPublish"].(bool)
		attrs, inv := s.attributesOf(uid, c.Data.Attributes, p, at, dp)
		for k, v := range inv {
			allInverses[k] = v
		}
		setRaw(top, "attributes", attrs)
		p.files[path] = top
	}
	s.dropReferences(p, changes, components, load)
	for key, attr := range allInverses {
		target, name, _ := strings.Cut(key, "#")
		path, ok := s.ctPath(target)
		if !ok {
			p.fail([]string{"attributes"}, "the inverse side of a relation must be an api:: content type, not %s", target)
			continue
		}
		top := load(path)
		if top == nil {
			p.fail([]string{"attributes"}, "unknown relation target %s", target)
			continue
		}
		raw, _ := top.Get("attributes")
		var attrs schema.OrderedMap[json.RawMessage]
		_ = json.Unmarshal(raw, &attrs)
		if existing, ok := attrs.Get(name); ok {
			if m := decodeRaw(existing); m["mappedBy"] != attr["mappedBy"] || m["target"] != attr["target"] {
				p.fail([]string{"attributes"}, "%s already has an attribute %s", target, name)
				continue
			}
		}
		setRaw(&attrs, name, attributeJSON(attr))
		setRaw(top, "attributes", attrs)
	}
	return p
}

// dropReferences removes, from the other schemas, the relations to a
// deleted content type and the uses of a deleted component.
func (s *Server) dropReferences(p *plan, changes, components []Change, load func(string) *schema.OrderedMap[json.RawMessage]) {
	gone := map[string]bool{}
	for _, c := range changes {
		if c.Action == "delete" {
			gone[c.UID] = true
		}
	}
	for _, c := range components {
		if c.Action == "delete" {
			gone[c.UID] = true
		}
	}
	if len(gone) == 0 {
		return
	}
	var paths []string
	for uid := range s.set.ContentTypes.All() {
		if path, ok := s.ctPath(uid); ok && !gone[uid] {
			paths = append(paths, path)
		}
	}
	for uid := range s.set.Components.All() {
		if path, ok := s.componentPath(uid); ok && !gone[uid] {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		top := load(path)
		if top == nil {
			continue
		}
		raw, _ := top.Get("attributes")
		var attrs schema.OrderedMap[json.RawMessage]
		if json.Unmarshal(raw, &attrs) != nil {
			continue
		}
		changed := false
		for name, a := range attrs.All() {
			m := decodeRaw(a)
			target, _ := m["target"].(string)
			comp, _ := m["component"].(string)
			switch {
			case gone[target] || gone[comp]:
				attrs.Delete(name)
				changed = true
			case m["type"] == schema.TypeDynamicZone:
				list, _ := m["components"].([]any)
				kept := []any{}
				for _, c := range list {
					if cs, _ := c.(string); !gone[cs] {
						kept = append(kept, c)
					}
				}
				if len(kept) != len(list) {
					m["components"] = kept
					setRaw(&attrs, name, m)
					changed = true
				}
			}
		}
		if changed {
			setRaw(top, "attributes", attrs)
			p.files[path] = top
		}
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		b, err := os.ReadFile(path) //nolint:gosec // the project's own src
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600) //nolint:gosec // a temporary copy
	})
}

func writeAtomic(path string, m *schema.OrderedMap[json.RawMessage]) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// apply writes the plan under root (the project, or its copy).
func (s *Server) apply(p *plan, root string) error {
	rebase := func(path string) string {
		rel, _ := filepath.Rel(s.cfg.ProjectDir, path)
		return filepath.Join(root, rel)
	}
	for path, m := range p.files {
		if err := writeAtomic(rebase(path), m); err != nil {
			return err
		}
	}
	for _, path := range p.removes {
		target := rebase(path)
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		// An api folder left with no content type goes too.
		if dir := filepath.Dir(target); filepath.Base(dir) == "content-types" {
			if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
				_ = os.RemoveAll(filepath.Dir(dir))
			}
		}
	}
	return nil
}

func (s *Server) ctbUpdate(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	if !s.cfg.Develop {
		return &Error{Status: http.StatusForbidden, Name: "ForbiddenError", Message: "The Content-Type Builder writes schemas under grapi develop only"}
	}
	var b struct {
		ContentTypes []Change `json:"contentTypes"`
		Components   []Change `json:"components"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	p := s.build(b.ContentTypes, b.Components)
	migrations := s.renameMigrations(p)
	if len(p.errs) > 0 {
		return validation(p.errs)
	}
	tmp, err := os.MkdirTemp("", "grapi-ctb-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := copyTree(s.srcPath(), filepath.Join(tmp, "src")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := s.apply(p, tmp); err != nil {
		return err
	}
	if _, _, err := builtin.Load(os.DirFS(tmp)); err != nil {
		return validation([]FieldError{{Path: []string{}, Message: err.Error()}})
	}
	// The migrations go first: the reload runs them before the schema sync,
	// which then finds the columns under their new names.
	for path, sql := range migrations {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(sql), 0o600); err != nil { //nolint:gosec // a name built from checked identifiers
			return err
		}
	}
	if err := s.apply(p, s.cfg.ProjectDir); err != nil {
		return err
	}
	for _, rn := range p.renames {
		if err := s.renameInViewConfig(r.Context(), rn); err != nil {
			return err
		}
	}
	for _, c := range b.ContentTypes {
		s.audit(r, u, "content-type."+c.Action, map[string]any{"uid": c.UID, "singularName": c.Data.Info["singularName"]})
	}
	for _, c := range b.Components {
		s.audit(r, u, "component."+c.Action, map[string]any{"uid": c.UID, "category": c.Data.Category})
	}
	return data(w, map[string]any{"schemaHash": s.schemaHash()})
}

var renameable = map[string]bool{
	schema.TypeString: true, schema.TypeText: true, schema.TypeRichText: true, schema.TypeBlocks: true, schema.TypeEmail: true,
	schema.TypePassword: true, schema.TypeUID: true, schema.TypeEnumeration: true, schema.TypeInteger: true, schema.TypeBigInteger: true,
	schema.TypeFloat: true, schema.TypeDecimal: true, schema.TypeDate: true, schema.TypeTime: true, schema.TypeDateTime: true,
	schema.TypeTimestamp: true, schema.TypeBoolean: true, schema.TypeJSON: true,
}

// renameMigrations checks the renames of a plan and returns their SQL
// migrations, by file path.
func (s *Server) renameMigrations(p *plan) map[string]string {
	out := map[string]string{}
	stamp := s.now().UTC()
	for i, rn := range p.renames {
		var attrs *schema.OrderedMap[*schema.Attribute]
		if ct, ok := s.set.ContentType(schema.UID(rn.uid)); ok {
			attrs = &ct.Attributes
		} else if c, ok := s.set.Component(schema.UID(rn.uid)); ok {
			attrs = &c.Attributes
		} else {
			p.fail([]string{"attributes"}, "%s is new: nothing to rename", rn.uid)
			continue
		}
		old, ok := attrs.Get(rn.from)
		if !ok {
			p.fail([]string{"attributes"}, "%s has no attribute %s to rename", rn.uid, rn.from)
			continue
		}
		if _, taken := attrs.Get(rn.to); taken {
			p.fail([]string{"attributes"}, "%s already has an attribute %s", rn.uid, rn.to)
			continue
		}
		if !renameable[old.Type] || old.IsCustomField() {
			p.fail([]string{"attributes"}, "renaming a %s field is not supported: create a new field instead", old.Type)
			continue
		}
		model, ok := s.e.MD.Model(rn.uid)
		if !ok {
			p.fail([]string{"attributes"}, "no table for %s", rn.uid)
			continue
		}
		from, err1 := naming.Default.Column(naming.SnakeCase(rn.from))
		to, err2 := naming.Default.Column(naming.SnakeCase(rn.to))
		if ma, ok := model.Attributes.Get(rn.from); ok && ma.ColumnName != "" {
			from = ma.ColumnName
		}
		if err1 != nil || err2 != nil {
			p.fail([]string{"attributes"}, "invalid column name for %s", rn.to)
			continue
		}
		q := s.e.DB.Dialect.Quote
		name := fmt.Sprintf("%s.%02d.rename-%s-%s", stamp.Format("2006.01.02T15.04.05"), i, schema.KebabCase(model.TableName), schema.KebabCase(rn.to))
		path := filepath.Join(s.cfg.ProjectDir, "database", "migrations", name+".sql")
		out[path] = fmt.Sprintf("-- Content-Type Builder: %s.%s renamed to %s, values kept.\nALTER TABLE %s RENAME COLUMN %s TO %s;\n",
			rn.uid, rn.from, rn.to, q(model.TableName), q(from), q(to))
	}
	return out
}

// renameInViewConfig moves a renamed field's view configuration.
func (s *Server) renameInViewConfig(ctx context.Context, rn rename) error {
	key := "configuration_content_types::" + rn.uid
	var c ViewConfig
	found, err := s.store.Get(ctx, cmScope, key, &c)
	if err != nil || !found {
		return err
	}
	if m, ok := c.Metadatas[rn.from]; ok {
		delete(c.Metadatas, rn.from)
		c.Metadatas[rn.to] = m
	}
	for i, n := range c.Layouts.List {
		if n == rn.from {
			c.Layouts.List[i] = rn.to
		}
	}
	for _, row := range c.Layouts.Edit {
		for i := range row {
			if row[i].Name == rn.from {
				row[i].Name = rn.to
			}
		}
	}
	if c.Settings.MainField == rn.from {
		c.Settings.MainField = rn.to
	}
	if c.Settings.DefaultSortBy == rn.from {
		c.Settings.DefaultSortBy = rn.to
	}
	return s.store.Set(ctx, cmScope, key, c)
}
