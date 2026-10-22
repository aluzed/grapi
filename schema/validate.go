package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/slug"
)

// Severity of an Issue. Errors are rules Strapi enforces when it boots: the
// application does not start. Warnings are checks grapi adds on top; Strapi
// accepts those schemas, so refusing them would break existing projects.
type Severity string

// Severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue codes. docs/divergences.md lists the warning codes.
const (
	CodeDuplicateName       = "duplicate-name"
	CodeDuplicateTable      = "duplicate-table"
	CodeUnknownType         = "unknown-type"
	CodeUnknownTarget       = "unknown-target"
	CodeInverseMissing      = "inverse-missing"
	CodeInverseNotRelation  = "inverse-not-relation"
	CodeOneToManyOwner      = "one-to-many-owner"
	CodeManyToOneNotOwner   = "many-to-one-not-owner"
	CodeMorphTarget         = "morph-target"
	CodeEnumInvalid         = "enum-invalid"
	CodeEnumEmpty           = "enum-empty"
	CodeEnumCollision       = "enum-collision"
	CodeUnknownComponent    = "unknown-component"
	CodeComponentCycle      = "component-cycle"
	CodeInverseMismatch     = "inverse-mismatch"
	CodeUnidirInverse       = "unidirectional-inverse"
	CodeUIDTarget           = "uid-target"
	CodeBounds              = "bounds"
	CodeEnumDefault         = "enum-default"
	CodeMediaAllowedTypes   = "media-allowed-types"
	CodeRepeatableBounds    = "repeatable-bounds"
	CodeLocalizedAttribute  = "localized-attribute"
	CodeIdentifierTooLong   = "identifier-too-long"
	CodeRelationKind        = "relation-kind"
	CodeComponentRelationBi = "component-bidirectional-relation"
)

// Issue is one finding of Validate.
type Issue struct {
	UID       UID
	Attribute string
	Code      string
	Severity  Severity
	Message   string
}

func (i Issue) String() string {
	where := string(i.UID)
	if i.Attribute != "" {
		where += "." + i.Attribute
	}
	return fmt.Sprintf("%s %s [%s]: %s", i.Severity, where, i.Code, i.Message)
}

// Report lists every issue found, in a stable order.
type Report struct{ Issues []Issue }

// Errors returns the issues that prevent the application from starting.
func (r *Report) Errors() []Issue { return r.filter(SeverityError) }

// Warnings returns the issues that are only logged.
func (r *Report) Warnings() []Issue { return r.filter(SeverityWarning) }

func (r *Report) filter(s Severity) []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == s {
			out = append(out, i)
		}
	}
	return out
}

// Err returns nil without errors, otherwise one error listing all of them.
func (r *Report) Err() error {
	errs := r.Errors()
	if len(errs) == 0 {
		return nil
	}
	lines := make([]string, len(errs))
	for i, e := range errs {
		lines[i] = "  " + e.String()
	}
	return fmt.Errorf("invalid schemas (%d errors):\n%s", len(errs), strings.Join(lines, "\n"))
}

// maxIdentifierLength is where database identifiers get shortened (DB-002).
const maxIdentifierLength = 55

var graphQLEnum = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)

var mediaTypes = map[string]bool{"images": true, "videos": true, "files": true, "audios": true}

type validator struct {
	set    *Set
	report Report
}

// Validate checks a whole application: names, relations, components, enums,
// bounds. It never stops at the first problem.
func Validate(set *Set) *Report {
	v := &validator{set: set}
	v.names()
	for _, ct := range set.ContentTypes.All() {
		for name, a := range ct.Attributes.All() {
			if a.Auto {
				continue
			}
			v.attribute(ct.UID, name, a, ct.localized(), false)
		}
		v.identifier(ct.UID, "", ct.CollectionName)
	}
	for _, c := range set.Components.All() {
		for name, a := range c.Attributes.All() {
			v.attribute(c.UID, name, a, false, true)
		}
		v.identifier(c.UID, "", c.CollectionName)
	}
	v.componentCycles()
	sort.SliceStable(v.report.Issues, func(i, j int) bool {
		a, b := v.report.Issues[i], v.report.Issues[j]
		if a.Severity != b.Severity {
			return a.Severity == SeverityError
		}
		return false
	})
	return &v.report
}

func (v *validator) add(sev Severity, uid UID, attr, code, format string, args ...any) {
	v.report.Issues = append(v.report.Issues, Issue{
		UID: uid, Attribute: attr, Code: code, Severity: sev, Message: fmt.Sprintf(format, args...),
	})
}

func (v *validator) names() {
	// Strapi checks singular and plural names of api types against one shared
	// pool: a plural may not equal another type's singular either.
	seen := map[string]UID{}
	tables := map[string]UID{}
	for _, ct := range v.set.ContentTypes.All() {
		if ct.UID.Namespace() == NamespaceAPI {
			for _, n := range []string{ct.Info.SingularName, ct.Info.PluralName} {
				k := strings.ReplaceAll(snakeCase(n), "_", "-")
				if other, ok := seen[k]; ok {
					v.add(SeverityError, ct.UID, "", CodeDuplicateName, "the name %q should be unique (already used by %s)", n, other)
					continue
				}
				seen[k] = ct.UID
			}
		}
		if other, ok := tables[ct.CollectionName]; ok {
			v.add(SeverityError, ct.UID, "", CodeDuplicateTable, "DB table %q already exists (%s). Change the collectionName", ct.CollectionName, other)
		}
		tables[ct.CollectionName] = ct.UID
	}
	for _, c := range v.set.Components.All() {
		if other, ok := tables[c.CollectionName]; ok {
			v.add(SeverityError, c.UID, "", CodeDuplicateTable, "DB table %q already exists (%s). Change the collectionName", c.CollectionName, other)
		}
		tables[c.CollectionName] = c.UID
	}
}

func (v *validator) identifier(uid UID, attr, name string) {
	if len(name) > maxIdentifierLength {
		v.add(SeverityWarning, uid, attr, CodeIdentifierTooLong, "%q is longer than %d characters and will be shortened in the database", name, maxIdentifierLength)
	}
}

func (v *validator) attribute(uid UID, name string, a *Attribute, typeLocalized, inComponent bool) {
	v.identifier(uid, name, name)
	switch a.Type {
	case TypeRelation:
		v.relation(uid, name, a, inComponent)
	case TypeComponent:
		if _, ok := v.set.Component(UID(a.Component)); !ok {
			v.add(SeverityError, uid, name, CodeUnknownComponent, "component %q not found", a.Component)
		}
		if !a.IsRepeatable() && (a.Min != nil || a.Max != nil) {
			v.add(SeverityWarning, uid, name, CodeRepeatableBounds, "min and max only apply to repeatable components")
		}
		v.bounds(uid, name, a.Min, a.Max, "min", "max")
	case TypeDynamicZone:
		for _, c := range a.Components {
			if _, ok := v.set.Component(UID(c)); !ok {
				v.add(SeverityError, uid, name, CodeUnknownComponent, "component %q not found", c)
			}
		}
		v.bounds(uid, name, a.Min, a.Max, "min", "max")
	case TypeEnumeration:
		v.enumeration(uid, name, a)
	case TypeUID:
		v.uidTarget(uid, name, a)
	case TypeMedia:
		for _, t := range a.AllowedTypes {
			if !mediaTypes[t] {
				v.add(SeverityWarning, uid, name, CodeMediaAllowedTypes, "allowed type %q is not one of images, videos, files, audios", t)
			}
		}
	case TypeCustomField:
		// Resolved to the underlying type by the custom field registry before validation.
		v.add(SeverityError, uid, name, CodeUnknownType, "custom field %q is not registered", a.CustomField)
	default:
		if !a.IsScalar() {
			v.add(SeverityError, uid, name, CodeUnknownType, "unknown attribute type %q", a.Type)
		}
	}
	if a.IsScalar() {
		v.bounds(uid, name, a.Min, a.Max, "min", "max")
		if a.MinLength != nil && a.MaxLength != nil && *a.MinLength > *a.MaxLength {
			v.add(SeverityWarning, uid, name, CodeBounds, "minLength (%d) is greater than maxLength (%d)", *a.MinLength, *a.MaxLength)
		}
	}
	if !typeLocalized && localizedOption(a.PluginOptions) {
		v.add(SeverityWarning, uid, name, CodeLocalizedAttribute, "attribute is localized but its content type is not")
	}
}

func (v *validator) bounds(uid UID, name string, minRaw, maxRaw json.RawMessage, minName, maxName string) {
	if minRaw == nil || maxRaw == nil {
		return
	}
	var lo, hi float64
	if json.Unmarshal(minRaw, &lo) != nil || json.Unmarshal(maxRaw, &hi) != nil {
		return
	}
	if lo > hi {
		v.add(SeverityWarning, uid, name, CodeBounds, "%s (%v) is greater than %s (%v)", minName, lo, maxName, hi)
	}
}

func (v *validator) enumeration(uid UID, name string, a *Attribute) {
	regressed := map[string]string{}
	reportedCollision := false
	for _, value := range a.Enum {
		r := slug.RegressedEnumValue(value)
		switch {
		case r == "":
			v.add(SeverityError, uid, name, CodeEnumEmpty, "at least one value of the enumeration %q appears to be empty. Only alphanumerical characters are taken into account", name)
		case !graphQLEnum.MatchString(r):
			v.add(SeverityError, uid, name, CodeEnumInvalid, "invalid enumeration value %q. Values should have at least one alphabetical character preceding the first occurence of a number", value)
		}
		if prev, ok := regressed[r]; ok && r != "" && !reportedCollision {
			v.add(SeverityError, uid, name, CodeEnumCollision, "enumeration values %q and %q collide when normalized", prev, value)
			reportedCollision = true
		}
		regressed[r] = value
	}
	if a.Default != nil {
		var def string
		if json.Unmarshal(a.Default, &def) == nil && def != "" && !contains(a.Enum, def) {
			v.add(SeverityWarning, uid, name, CodeEnumDefault, "default %q is not one of the enumeration values", def)
		}
	}
}

func (v *validator) uidTarget(uid UID, name string, a *Attribute) {
	if a.TargetField == "" {
		return
	}
	attrs, _ := v.set.attributesOf(string(uid))
	target, ok := attrs.Get(a.TargetField)
	switch {
	case !ok:
		v.add(SeverityWarning, uid, name, CodeUIDTarget, "targetField %q does not exist", a.TargetField)
	case target.Type != TypeString && target.Type != TypeText:
		v.add(SeverityWarning, uid, name, CodeUIDTarget, "targetField %q must be a string or text attribute, not %s", a.TargetField, target.Type)
	}
}

func (v *validator) relation(uid UID, name string, a *Attribute, inComponent bool) {
	if !IsRelationKind(a.Relation) {
		v.add(SeverityError, uid, name, CodeRelationKind, "unknown relation kind %q", a.Relation)
		return
	}
	switch a.Relation {
	case MorphToOne, MorphToMany:
		return
	case MorphOne, MorphMany:
		attrs, ok := v.set.attributesOf(a.Target)
		if !ok {
			v.add(SeverityError, uid, name, CodeMorphTarget, "morph target not found. Looking for %s", a.Target)
			return
		}
		if _, ok := attrs.Get(a.MorphBy); !ok {
			v.add(SeverityError, uid, name, CodeMorphTarget, "morph target attribute not found. Looking for %s", a.MorphBy)
		}
		return
	}
	targetAttrs, ok := v.set.attributesOf(a.Target)
	if !ok {
		v.add(SeverityError, uid, name, CodeUnknownTarget, "unknown target %s", a.Target)
		return
	}
	bidirectional := a.InversedBy != "" || a.MappedBy != ""
	if inComponent && bidirectional {
		v.add(SeverityWarning, uid, name, CodeComponentRelationBi, "relations in components must be unidirectional")
	}
	if (a.Relation == OneWay || a.Relation == ManyWay) && bidirectional {
		v.add(SeverityWarning, uid, name, CodeUnidirInverse, "%s relations cannot have inversedBy or mappedBy", a.Relation)
		return
	}
	// Strapi treats a unidirectional oneToMany without join table as an owner too.
	noJoinTable := a.UseJoinTable != nil && !*a.UseJoinTable
	if a.Relation == OneToMany && (a.InversedBy != "" || (!bidirectional && noJoinTable)) {
		v.add(SeverityError, uid, name, CodeOneToManyOwner, "one side of a oneToMany cannot be the owner side in a bidirectional relation")
	}
	if a.Relation == ManyToOne && a.MappedBy != "" && a.InversedBy == "" {
		v.add(SeverityError, uid, name, CodeManyToOneNotOwner, "the many side of a manyToOne must be the owning side")
	}
	inverseName, sev := a.InversedBy, SeverityError
	if inverseName == "" {
		inverseName, sev = a.MappedBy, SeverityWarning
	}
	if inverseName == "" {
		return
	}
	inverse, ok := targetAttrs.Get(inverseName)
	if !ok {
		v.add(sev, uid, name, CodeInverseMissing, "inverse attribute %s not found in target %s", inverseName, a.Target)
		return
	}
	if inverse.Type != TypeRelation {
		v.add(sev, uid, name, CodeInverseNotRelation, "inverse attribute %s targets a non relational attribute in %s", inverseName, a.Target)
		return
	}
	backRef := inverse.MappedBy
	if a.MappedBy != "" {
		backRef = inverse.InversedBy
	}
	if inverse.Target != string(uid) || backRef != name || inverse.Relation != expectedInverse(a.Relation) {
		v.add(SeverityWarning, uid, name, CodeInverseMismatch, "inverse %s.%s does not point back as %s(%s)", a.Target, inverseName, expectedInverse(a.Relation), name)
	}
}

func expectedInverse(kind string) string {
	switch kind {
	case OneToMany:
		return ManyToOne
	case ManyToOne:
		return OneToMany
	}
	return kind
}

func (v *validator) componentCycles() {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	reported := map[string]bool{}
	var visit func(uid string, path []string)
	visit = func(uid string, path []string) {
		switch state[uid] {
		case done:
			return
		case visiting:
			start := 0
			for i, p := range path {
				if p == uid {
					start = i
				}
			}
			cycle := append(append([]string(nil), path[start:]...), uid)
			if !reported[uid] {
				reported[uid] = true
				v.add(SeverityError, UID(uid), "", CodeComponentCycle, "components contain each other: %s", strings.Join(cycle, " -> "))
			}
			return
		}
		state[uid] = visiting
		if c, ok := v.set.Components.Get(uid); ok {
			for _, a := range c.Attributes.All() {
				if a.Type == TypeComponent {
					visit(a.Component, append(path, uid))
				}
			}
		}
		state[uid] = done
	}
	for _, uid := range v.set.Components.Keys() {
		visit(uid, nil)
	}
}

func (ct *ContentType) localized() bool { return localizedOption(ct.PluginOptions) }

func localizedOption(raw json.RawMessage) bool {
	if raw == nil {
		return false
	}
	var opts struct {
		I18n struct {
			Localized bool `json:"localized"`
		} `json:"i18n"`
	}
	return json.Unmarshal(raw, &opts) == nil && opts.I18n.Localized
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
