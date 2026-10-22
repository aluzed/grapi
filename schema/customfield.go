package schema

import (
	"fmt"
	"regexp"
	"sort"
)

// Underlying types a custom field may be stored as (Strapi's ALLOWED_TYPES).
var customFieldTypes = map[string]bool{
	TypeBigInteger: true, TypeBoolean: true, TypeDate: true, TypeDateTime: true, TypeDecimal: true,
	TypeEmail: true, TypeEnumeration: true, TypeFloat: true, TypeInteger: true, TypeJSON: true,
	TypePassword: true, TypeRichText: true, TypeString: true, TypeText: true, TypeTime: true, TypeUID: true,
}

var customFieldName = regexp.MustCompile(`^[a-zA-Z$_-][a-zA-Z0-9$_-]*$`)

// CustomField is a field type declared by a plugin or the application: the
// admin renders it its own way, the database stores it as Type.
type CustomField struct {
	Name      string
	Plugin    string
	Type      string
	InputSize *InputSize
}

// InputSize is the admin layout hint of a custom field.
type InputSize struct {
	Default     int
	IsResizable bool
}

// CustomFields is the custom field registry.
type CustomFields struct{ byUID map[string]CustomField }

// Register adds cf under plugin::<plugin>.<name>, or global::<name> when it has
// no plugin or the plugin is not installed (pluginExists may be nil).
func (r *CustomFields) Register(cf CustomField, pluginExists func(string) bool) (string, error) {
	if cf.Name == "" || cf.Type == "" {
		return "", fmt.Errorf("custom fields require a 'name' and 'type' key")
	}
	if !customFieldTypes[cf.Type] {
		return "", fmt.Errorf("custom field type: '%s' is not a valid Strapi type or it can't be used with a Custom Field", cf.Type)
	}
	if !customFieldName.MatchString(cf.Name) {
		return "", fmt.Errorf("custom field name: '%s' is not a valid object key", cf.Name)
	}
	if cf.InputSize != nil && !validInputSize(cf.InputSize.Default) {
		return "", fmt.Errorf("custom field %s: inputSize.default must be one of 4, 6, 8 or 12", cf.Name)
	}
	uid := "global::" + cf.Name
	if cf.Plugin != "" && pluginExists != nil && pluginExists(cf.Plugin) {
		uid = "plugin::" + cf.Plugin + "." + cf.Name
	}
	if r.byUID == nil {
		r.byUID = map[string]CustomField{}
	}
	if _, ok := r.byUID[uid]; ok {
		return "", fmt.Errorf("custom field: '%s' has already been registered", uid)
	}
	r.byUID[uid] = cf
	return uid, nil
}

func validInputSize(n int) bool { return n == 4 || n == 6 || n == 8 || n == 12 }

// Get returns the custom field registered under uid.
func (r *CustomFields) Get(uid string) (CustomField, bool) {
	cf, ok := r.byUID[uid]
	return cf, ok
}

// UIDs returns the registered uids, sorted.
func (r *CustomFields) UIDs() []string {
	out := make([]string, 0, len(r.byUID))
	for uid := range r.byUID {
		out = append(out, uid)
	}
	sort.Strings(out)
	return out
}

// ResolveCustomFields gives every customField attribute its underlying type.
// The schema still serializes as "type": "customField". Unknown custom fields
// are left as is: Validate reports them.
func (s *Set) ResolveCustomFields(r *CustomFields) {
	resolve := func(attrs *OrderedMap[*Attribute]) {
		for _, a := range attrs.All() {
			if a.Type != TypeCustomField {
				continue
			}
			if cf, ok := r.Get(a.CustomField); ok {
				a.Type = cf.Type
				a.resolvedCustomField = true
			}
		}
	}
	for _, ct := range s.ContentTypes.All() {
		resolve(&ct.Attributes)
	}
	for _, c := range s.Components.All() {
		resolve(&c.Attributes)
	}
}
