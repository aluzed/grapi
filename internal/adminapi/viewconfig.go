package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/schema"
)

var cmScope = store.Scope{Type: "plugin", Name: "content_manager"}

// LayoutField is one field of an edit layout row.
type LayoutField struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

// FieldEdit is the edit part of a field's metadata. id and documentId have
// an empty one, kept as {} so that a configuration reads back as stored.
type FieldEdit struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Placeholder string `json:"placeholder"`
	Visible     bool   `json:"visible"`
	Editable    bool   `json:"editable"`
	MainField   string `json:"mainField,omitempty"`
	empty       bool
}

type fieldEdit FieldEdit

// MarshalJSON writes an empty edit part as {}.
func (e FieldEdit) MarshalJSON() ([]byte, error) {
	if e.empty {
		return []byte("{}"), nil
	}
	return json.Marshal(fieldEdit(e))
}

// UnmarshalJSON remembers an empty edit part.
func (e *FieldEdit) UnmarshalJSON(b []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	if len(keys) == 0 {
		*e = FieldEdit{empty: true}
		return nil
	}
	return json.Unmarshal(b, (*fieldEdit)(e))
}

// FieldMeta is the per-field configuration of the edit and list views.
type FieldMeta struct {
	Edit FieldEdit `json:"edit"`
	List struct {
		Label      string `json:"label"`
		Searchable bool   `json:"searchable"`
		Sortable   bool   `json:"sortable"`
	} `json:"list"`
}

// ViewConfig is Strapi's content manager configuration of a content type
// (core store, plugin_content_manager_configuration_content_types::<uid>).
type ViewConfig struct {
	UID      string `json:"uid"`
	Settings struct {
		Bulkable         bool   `json:"bulkable"`
		Filterable       bool   `json:"filterable"`
		Searchable       bool   `json:"searchable"`
		PageSize         int    `json:"pageSize"`
		MainField        string `json:"mainField"`
		DefaultSortBy    string `json:"defaultSortBy"`
		DefaultSortOrder string `json:"defaultSortOrder"`
		RelationOpenMode string `json:"relationOpenMode,omitempty"`
	} `json:"settings"`
	Metadatas map[string]*FieldMeta `json:"metadatas"`
	Layouts   struct {
		List []string        `json:"list"`
		Edit [][]LayoutField `json:"edit"`
	} `json:"layouts"`
}

var fieldSizes = map[string]int{
	schema.TypeDynamicZone: 12, schema.TypeComponent: 12, schema.TypeJSON: 12, schema.TypeRichText: 12, schema.TypeBlocks: 12,
	schema.TypeBoolean: 4, schema.TypeDate: 4, schema.TypeTime: 4, schema.TypeBigInteger: 4, schema.TypeDecimal: 4, schema.TypeFloat: 4, schema.TypeInteger: 4,
}

var fixedSize = map[string]bool{schema.TypeDynamicZone: true, schema.TypeComponent: true, schema.TypeJSON: true, schema.TypeRichText: true, schema.TypeBlocks: true}

func fieldSize(t string) int {
	if n, ok := fieldSizes[t]; ok {
		return n
	}
	return 6
}

var notEditable = map[string]bool{
	"id": true, schema.AttrDocumentID: true, "createdAt": true, "updatedAt": true, "publishedAt": true,
	schema.AttrCreatedBy: true, schema.AttrUpdatedBy: true, "locale": true, "localizations": true,
}

func editable(name string, a *schema.Attribute) bool {
	return !notEditable[name] && a.IsVisible() && a.IsWritable()
}

// cmField is an attribute of the content manager's model of a type.
type cmField struct {
	name   string
	attr   *schema.Attribute
	hidden bool
}

type cmModel []cmField

func (m cmModel) get(name string) (cmField, bool) {
	for _, f := range m {
		if f.name == name {
			return f, true
		}
	}
	return cmField{}, false
}

// hiddenAttributes are the schema's config.attributes.<name>.hidden: kept
// out of the list and edit views.
func hiddenAttributes(ct *schema.ContentType) map[string]bool {
	var config struct {
		Attributes map[string]struct {
			Hidden bool `json:"hidden"`
		} `json:"attributes"`
	}
	out := map[string]bool{}
	if raw, ok := ct.Extra["config"]; ok && json.Unmarshal(raw, &config) == nil {
		for name, a := range config.Attributes {
			out[name] = a.Hidden
		}
	}
	return out
}

// cmFields is Strapi's toContentManagerModel: id, the visible attributes,
// the timestamps, the creator fields, then documentId. Morph relations are
// left out, and so is publishedAt, which Strapi does not show.
func cmFields(ct *schema.ContentType) cmModel {
	hidden := hiddenAttributes(ct)
	out := cmModel{{"id", &schema.Attribute{Type: schema.TypeInteger}, false}}
	for name, a := range ct.Attributes.All() {
		if notEditable[name] || !a.IsVisible() || a.Type == schema.TypeRelation && a.IsMorph() {
			continue
		}
		out = append(out, cmField{name, a, hidden[name]})
	}
	for _, name := range []string{"createdAt", "updatedAt", schema.AttrCreatedBy, schema.AttrUpdatedBy} {
		if a, ok := ct.Attributes.Get(name); ok {
			out = append(out, cmField{name, a, hidden[name]})
		}
	}
	return append(out, cmField{schema.AttrDocumentID, &schema.Attribute{Type: schema.TypeString}, false})
}

func cmVisible(f cmField) bool { return !f.hidden && !notEditable[f.name] && f.attr.IsVisible() }

func cmSortable(a *schema.Attribute) bool {
	switch a.Type {
	case schema.TypeComponent, schema.TypeJSON, schema.TypeMedia, schema.TypeRichText, schema.TypeDynamicZone, schema.TypeBlocks:
		return false
	case schema.TypeRelation:
		return a.Relation == schema.OneToOne || a.Relation == schema.ManyToOne
	}
	return true
}

func cmListable(f cmField) bool {
	if f.hidden {
		return false
	}
	switch a := f.attr; a.Type {
	case schema.TypeJSON, schema.TypePassword, schema.TypeRichText, schema.TypeDynamicZone, schema.TypeBlocks:
		return false
	case schema.TypeRelation:
		switch a.Relation {
		case schema.OneToOne, schema.OneToMany, schema.ManyToOne, schema.ManyToMany:
			return true
		}
		return false
	}
	return true
}

// cmMainField is getDefaultMainField: the first string attribute, which is
// documentId when the type has none of its own.
func cmMainField(m cmModel) string {
	for _, f := range m {
		if f.attr.Type == schema.TypeString && f.name != "id" {
			return f.name
		}
	}
	return "id"
}

// appendEdit is Strapi's appendToEditLayout: fields fill rows of 12.
func appendEdit(layout [][]LayoutField, names []string, m cmModel) [][]LayoutField {
	if len(names) == 0 {
		return layout
	}
	if len(layout) == 0 {
		layout = [][]LayoutField{{}}
	}
	row := len(layout) - 1
	for _, n := range names {
		f, _ := m.get(n)
		size := fieldSize(f.attr.Type)
		used := 0
		for _, f := range layout[row] {
			used += f.Size
		}
		if used+size > 12 {
			layout = append(layout, []LayoutField{})
			row++
		}
		layout[row] = append(layout[row], LayoutField{Name: n, Size: size})
	}
	return layout
}

func allowedSize(a *schema.Attribute, size int) bool {
	if fixedSize[a.Type] && size != fieldSize(a.Type) {
		return false
	}
	return size <= 12
}

func (s *Server) targetMainField(a *schema.Attribute) string {
	if t, ok := s.set.ContentType(schema.UID(a.Target)); ok {
		return cmMainField(cmFields(t))
	}
	return ""
}

func (s *Server) defaultMeta(f cmField) *FieldMeta {
	name, a := f.name, f.attr
	m := &FieldMeta{}
	if name == "id" || name == schema.AttrDocumentID {
		m.Edit.empty = true
		m.List.Label, m.List.Searchable, m.List.Sortable = name, true, true
		return m
	}
	m.Edit.Label, m.Edit.Visible, m.Edit.Editable = name, cmVisible(f), true
	if a.Type == schema.TypeRelation {
		m.Edit.MainField = s.targetMainField(a)
	}
	m.List.Label, m.List.Searchable, m.List.Sortable = name, cmSortable(a), cmSortable(a)
	return m
}

func defaultList(m cmModel) []string {
	var list []string
	for _, f := range m {
		if cmListable(f) && f.name != schema.AttrDocumentID && len(list) < 4 {
			list = append(list, f.name)
		}
	}
	return list
}

func defaultEdit(m cmModel) [][]LayoutField {
	var names []string
	for _, f := range m {
		if cmVisible(f) {
			names = append(names, f.name)
		}
	}
	return appendEdit(nil, names, m)
}

// defaultConfig is the configuration Strapi creates for a new type.
func (s *Server) defaultConfig(ct *schema.ContentType) *ViewConfig {
	m := cmFields(ct)
	c := &ViewConfig{UID: string(ct.UID), Metadatas: map[string]*FieldMeta{}}
	main := cmMainField(m)
	c.Settings.Bulkable, c.Settings.Filterable, c.Settings.Searchable = true, true, true
	c.Settings.PageSize, c.Settings.RelationOpenMode = 10, "modal"
	c.Settings.MainField, c.Settings.DefaultSortBy, c.Settings.DefaultSortOrder = main, main, "ASC"
	for _, f := range m {
		c.Metadatas[f.name] = s.defaultMeta(f)
	}
	c.Layouts.List, c.Layouts.Edit = defaultList(m), defaultEdit(m)
	return c
}

// sortableBy is getSortableAttributes: what defaultSortBy may name.
func sortableBy(m cmModel, ct *schema.ContentType, name string) bool {
	if i := strings.IndexAny(name, ":."); i >= 0 {
		name = name[:i]
	}
	switch name {
	case "id", schema.AttrDocumentID, schema.AttrCreatedBy, schema.AttrUpdatedBy:
		return true
	}
	if a, ok := m.get(name); ok && cmListable(a) {
		return true
	}
	a, ok := ct.Attributes.Get(name)
	return ok && !a.IsVisible() && a.IsWritable()
}

// effectiveConfig is Strapi's syncConfiguration of the stored configuration
// with the schema, or the default configuration when none is stored.
func (s *Server) effectiveConfig(ctx context.Context, ct *schema.ContentType) (*ViewConfig, error) {
	def := s.defaultConfig(ct)
	var stored ViewConfig
	found, err := s.store.Get(ctx, cmScope, "configuration_content_types::"+string(ct.UID), &stored)
	if err != nil || !found {
		return def, err
	}
	m := cmFields(ct)
	out := &stored
	out.UID = string(ct.UID)

	set := &out.Settings
	if *set == (ViewConfig{}).Settings {
		out.Settings = def.Settings
	} else {
		if f, ok := m.get(set.MainField); !ok || !cmSortable(f.attr) {
			set.MainField = def.Settings.MainField
		}
		if !sortableBy(m, ct, set.DefaultSortBy) {
			set.DefaultSortBy = def.Settings.MainField
		}
		if set.PageSize <= 0 {
			set.PageSize = def.Settings.PageSize
		}
	}

	if len(out.Layouts.List) == 0 && len(out.Layouts.Edit) == 0 {
		out.Layouts = def.Layouts
	} else {
		var list []string
		for _, n := range out.Layouts.List {
			if a, ok := m.get(n); ok && cmListable(a) {
				list = append(list, n)
			}
		}
		var edit [][]LayoutField
		var reappend []string
		for _, row := range out.Layouts.Edit {
			var kept []LayoutField
			for _, f := range row {
				a, ok := m.get(f.Name)
				if !ok || !cmVisible(a) {
					continue
				}
				if !allowedSize(a.attr, f.Size) {
					reappend = append(reappend, f.Name)
					continue
				}
				kept = append(kept, f)
			}
			if len(kept) > 0 {
				edit = append(edit, kept)
			}
		}
		edit = appendEdit(edit, reappend, m)
		var added []string
		for _, f := range m {
			if _, ok := out.Metadatas[f.name]; !ok {
				added = append(added, f.name)
			}
		}
		if len(list) < 4 {
			for _, n := range added {
				if f, _ := m.get(n); cmListable(f) && !slices.Contains(list, n) && len(list) < 4 {
					list = append(list, n)
				}
			}
		}
		var addedEdit []string
		for _, n := range added {
			if f, _ := m.get(n); cmVisible(f) {
				addedEdit = append(addedEdit, n)
			}
		}
		edit = appendEdit(edit, addedEdit, m)
		if len(list) == 0 {
			list = def.Layouts.List
		}
		if len(edit) == 0 {
			edit = def.Layouts.Edit
		}
		out.Layouts.List, out.Layouts.Edit = list, edit
	}

	if len(out.Metadatas) == 0 {
		out.Metadatas = def.Metadatas
	} else {
		metas := map[string]*FieldMeta{}
		for _, f := range m {
			meta, ok := out.Metadatas[f.name]
			if !ok || meta == nil {
				metas[f.name] = def.Metadatas[f.name]
				continue
			}
			if meta.List.Sortable && !cmSortable(f.attr) {
				meta.List.Sortable = false
			}
			if meta.List.Searchable && !cmSortable(f.attr) {
				meta.List.Searchable = false
			}
			if meta.Edit.MainField != "" {
				if f.attr.Type != schema.TypeRelation {
					meta.Edit.MainField = ""
				} else if meta.Edit.MainField != "id" {
					if t, ok := s.set.ContentType(schema.UID(f.attr.Target)); ok {
						if ta, ok := cmFields(t).get(meta.Edit.MainField); !ok || !cmSortable(ta.attr) && !cmListable(ta) {
							meta.Edit.MainField = s.targetMainField(f.attr)
						}
					}
				}
			}
			metas[f.name] = meta
		}
		out.Metadatas = metas
	}
	return out, nil
}

func (s *Server) viewConfigRoutes() {
	s.authed("GET /content-manager/configuration/{uid}", s.getViewConfig)
	s.authed("PUT /content-manager/configuration/{uid}", s.putViewConfig)
}

func configureAction(ct *schema.ContentType) string {
	if ct.IsSingleType() {
		return "plugin::content-manager.single-types.configure-view"
	}
	return "plugin::content-manager.collection-types.configure-view"
}

func (s *Server) getViewConfig(w http.ResponseWriter, r *http.Request, u *User) error {
	ct, _, _, err := s.cmAccess(r, u, actionRead)
	if err != nil {
		return err
	}
	c, err := s.effectiveConfig(r.Context(), ct)
	if err != nil {
		return err
	}
	return data(w, c)
}

func (s *Server) putViewConfig(w http.ResponseWriter, r *http.Request, u *User) error {
	uid := r.PathValue("uid")
	ct, ok := s.set.ContentType(schema.UID(uid))
	if !ok || !contains(s.ContentManagerTypes(), uid) {
		return errNotFound
	}
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return err
	}
	if !a.Can(configureAction(ct), "", "") {
		return errForbidden
	}
	var c ViewConfig
	if err := decode(r, &c); err != nil {
		return err
	}
	var errs []FieldError
	set := c.Settings
	if set.PageSize < 1 || set.PageSize > 100 {
		errs = append(errs, FieldError{Path: []string{"settings", "pageSize"}, Message: "pageSize must be between 1 and 100"})
	}
	m := cmFields(ct)
	if set.MainField != "id" {
		if f, ok := m.get(set.MainField); !ok || !cmSortable(f.attr) || f.attr.Type == schema.TypeRelation {
			errs = append(errs, FieldError{Path: []string{"settings", "mainField"}, Message: "mainField must be a sortable field"})
		}
	}
	switch set.RelationOpenMode {
	case "", "modal", "newTab", "page":
	default:
		errs = append(errs, FieldError{Path: []string{"settings", "relationOpenMode"}, Message: "relationOpenMode must be modal, newTab or page"})
	}
	if set.DefaultSortOrder != "ASC" && set.DefaultSortOrder != "DESC" {
		errs = append(errs, FieldError{Path: []string{"settings", "defaultSortOrder"}, Message: "defaultSortOrder must be ASC or DESC"})
	}
	if !sortableBy(m, ct, set.DefaultSortBy) {
		errs = append(errs, FieldError{Path: []string{"settings", "defaultSortBy"}, Message: "defaultSortBy must be a field"})
	}
	for i, n := range c.Layouts.List {
		if a, ok := m.get(n); !ok || !cmListable(a) {
			errs = append(errs, FieldError{Path: []string{"layouts", "list", strconv.Itoa(i)}, Message: n + " cannot be listed"})
		}
	}
	seen := map[string]bool{}
	for i, row := range c.Layouts.Edit {
		used := 0
		for j, f := range row {
			a, ok := ct.Attributes.Get(f.Name)
			path := []string{"layouts", "edit", strconv.Itoa(i), strconv.Itoa(j)}
			switch {
			case !ok || !editable(f.Name, a) || seen[f.Name]:
				errs = append(errs, FieldError{Path: path, Message: f.Name + " cannot be placed here"})
			case fixedSize[a.Type] && f.Size != 12, f.Size != 4 && f.Size != 6 && f.Size != 8 && f.Size != 12:
				errs = append(errs, FieldError{Path: path, Message: "invalid size for " + f.Name})
			}
			seen[f.Name] = true
			used += f.Size
		}
		if used > 12 {
			errs = append(errs, FieldError{Path: []string{"layouts", "edit", strconv.Itoa(i)}, Message: "a row holds 12 columns at most"})
		}
	}
	if len(errs) > 0 {
		return validation(errs)
	}
	c.UID = uid
	for name := range c.Metadatas {
		if _, ok := m.get(name); !ok {
			delete(c.Metadatas, name)
		}
	}
	if err := s.store.Set(r.Context(), cmScope, "configuration_content_types::"+uid, c); err != nil {
		return err
	}
	out, err := s.effectiveConfig(r.Context(), ct)
	if err != nil {
		return err
	}
	return data(w, out)
}
