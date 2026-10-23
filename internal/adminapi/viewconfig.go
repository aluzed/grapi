package adminapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/schema"
)

var cmScope = store.Scope{Type: "plugin", Name: "content_manager"}

// LayoutField is one field of an edit layout row.
type LayoutField struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

// FieldMeta is the per-field configuration of the edit and list views.
type FieldMeta struct {
	Edit struct {
		Label       string `json:"label"`
		Description string `json:"description"`
		Placeholder string `json:"placeholder"`
		Visible     bool   `json:"visible"`
		Editable    bool   `json:"editable"`
		MainField   string `json:"mainField,omitempty"`
	} `json:"edit"`
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

func listable(name string, a *schema.Attribute) bool {
	if name == schema.AttrDocumentID || !a.IsVisible() {
		return false
	}
	switch a.Type {
	case schema.TypeJSON, schema.TypePassword, schema.TypeRichText, schema.TypeBlocks, schema.TypeDynamicZone, schema.TypeComponent:
		return false
	case schema.TypeRelation:
		return !a.IsMorph()
	}
	return true
}

// appendEdit is Strapi's appendToEditLayout: fields fill rows of 12.
func appendEdit(layout [][]LayoutField, names []string, attrs *schema.OrderedMap[*schema.Attribute]) [][]LayoutField {
	if len(names) == 0 {
		return layout
	}
	if len(layout) == 0 {
		layout = [][]LayoutField{{}}
	}
	row := len(layout) - 1
	for _, n := range names {
		a, _ := attrs.Get(n)
		size := fieldSize(a.Type)
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

// defaultConfig is the configuration Strapi creates for a new type.
func defaultConfig(ct *schema.ContentType) *ViewConfig {
	c := &ViewConfig{UID: string(ct.UID), Metadatas: map[string]*FieldMeta{}}
	c.Settings.Bulkable, c.Settings.Filterable, c.Settings.Searchable = true, true, true
	c.Settings.PageSize, c.Settings.MainField, c.Settings.DefaultSortOrder = 10, defaultMainField(ct), "ASC"
	c.Settings.DefaultSortBy = c.Settings.MainField
	c.Layouts.List = []string{"id"}
	var edit []string
	for name, a := range ct.Attributes.All() {
		m := &FieldMeta{}
		m.Edit.Label, m.Edit.Visible, m.Edit.Editable = name, a.IsVisible(), true
		m.List.Label, m.List.Searchable, m.List.Sortable = name, true, listable(name, a)
		c.Metadatas[name] = m
		if listable(name, a) && len(c.Layouts.List) < 4 {
			c.Layouts.List = append(c.Layouts.List, name)
		}
		if editable(name, a) {
			edit = append(edit, name)
		}
	}
	c.Layouts.Edit = appendEdit(nil, edit, &ct.Attributes)
	return c
}

// effectiveConfig is the stored configuration synced with the schema (new
// fields appended, gone fields dropped), or the default one.
func (s *Server) effectiveConfig(ctx context.Context, ct *schema.ContentType) (*ViewConfig, error) {
	def := defaultConfig(ct)
	var stored ViewConfig
	found, err := s.store.Get(ctx, cmScope, "configuration_content_types::"+string(ct.UID), &stored)
	if err != nil || !found {
		return def, err
	}
	out := &stored
	out.UID = string(ct.UID)
	if out.Metadatas == nil {
		out.Metadatas = map[string]*FieldMeta{}
	}
	var list []string
	for _, n := range out.Layouts.List {
		if a, ok := ct.Attributes.Get(n); n == "id" || ok && listable(n, a) {
			list = append(list, n)
		}
	}
	if len(list) == 0 {
		list = def.Layouts.List
	}
	out.Layouts.List = list
	placed := map[string]bool{}
	var edit [][]LayoutField
	for _, row := range out.Layouts.Edit {
		var kept []LayoutField
		for _, f := range row {
			if a, ok := ct.Attributes.Get(f.Name); ok && editable(f.Name, a) && !placed[f.Name] {
				placed[f.Name] = true
				kept = append(kept, f)
			}
		}
		if len(kept) > 0 {
			edit = append(edit, kept)
		}
	}
	var missing []string
	for name, a := range ct.Attributes.All() {
		if _, ok := out.Metadatas[name]; !ok {
			out.Metadatas[name] = def.Metadatas[name]
		}
		if editable(name, a) && !placed[name] {
			missing = append(missing, name)
		}
	}
	out.Layouts.Edit = appendEdit(edit, missing, &ct.Attributes)
	for name := range out.Metadatas {
		if _, ok := ct.Attributes.Get(name); !ok {
			delete(out.Metadatas, name)
		}
	}
	if out.Settings.PageSize <= 0 {
		out.Settings.PageSize = def.Settings.PageSize
	}
	if out.Settings.MainField == "" {
		out.Settings.MainField = def.Settings.MainField
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
	if set.MainField != "id" {
		if attr, ok := ct.Attributes.Get(set.MainField); !ok || !listable(set.MainField, attr) || attr.Type == schema.TypeRelation || attr.Type == schema.TypeMedia {
			errs = append(errs, FieldError{Path: []string{"settings", "mainField"}, Message: "mainField must be a sortable field"})
		}
	}
	if set.DefaultSortOrder != "ASC" && set.DefaultSortOrder != "DESC" {
		errs = append(errs, FieldError{Path: []string{"settings", "defaultSortOrder"}, Message: "defaultSortOrder must be ASC or DESC"})
	}
	if _, ok := ct.Attributes.Get(set.DefaultSortBy); !ok && set.DefaultSortBy != "id" {
		errs = append(errs, FieldError{Path: []string{"settings", "defaultSortBy"}, Message: "defaultSortBy must be a field"})
	}
	for i, n := range c.Layouts.List {
		if a, ok := ct.Attributes.Get(n); n != "id" && (!ok || !listable(n, a)) {
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
		if _, ok := ct.Attributes.Get(name); !ok {
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
