package permissions

import (
	"path"
	"strings"
)

// Permission is one admin_permissions row.
type Permission struct {
	Action  string
	Subject string
	// Fields and Locales are properties.fields and properties.locales; nil
	// means no restriction, an empty list allows nothing.
	Fields     []string
	Locales    []string
	Conditions []string
}

type rule struct {
	fields  []string
	locales []string
	// filter is nil when the rule is unconditional.
	filter any
}

// Ability answers what a user may do.
type Ability struct {
	superAdmin bool
	rules      map[string]map[string][]rule
}

// SuperAdmin may do everything, without condition.
func SuperAdmin() *Ability { return &Ability{superAdmin: true} }

// Build evaluates the conditions of perms for u. As in Strapi, the
// conditions of one permission are alternatives: one returning true lifts
// the restriction, all returning false drop the permission, unknown ones are
// ignored.
func Build(perms []Permission, u User, conds *Conditions) *Ability {
	a := &Ability{rules: map[string]map[string][]rule{}}
	for _, p := range perms {
		var filters []any
		unconditional := len(p.Conditions) == 0
		evaluated := 0
		for _, id := range p.Conditions {
			c, ok := conds.Get(id)
			if !ok || c.Handler == nil {
				continue
			}
			switch r := c.Handler(u).(type) {
			case bool:
				evaluated++
				if r {
					unconditional = true
				}
			case map[string]any:
				evaluated++
				filters = append(filters, r)
			}
		}
		if evaluated == 0 {
			unconditional = true
		} else if !unconditional && len(filters) == 0 {
			continue
		}
		r := rule{fields: p.Fields, locales: p.Locales}
		if !unconditional {
			if len(filters) == 1 {
				r.filter = filters[0]
			} else {
				r.filter = map[string]any{"$or": filters}
			}
		}
		if a.rules[p.Action] == nil {
			a.rules[p.Action] = map[string][]rule{}
		}
		a.rules[p.Action][p.Subject] = append(a.rules[p.Action][p.Subject], r)
	}
	return a
}

// IsSuperAdmin reports the super admin ability.
func (a *Ability) IsSuperAdmin() bool { return a.superAdmin }

func (a *Ability) matching(action, subject string) []rule {
	bySubject := a.rules[action]
	if bySubject == nil {
		return nil
	}
	out := append([]rule(nil), bySubject[subject]...)
	if subject != "" {
		out = append(out, bySubject[""]...)
	}
	return out
}

// fieldMatches: a pattern covers the field and its sub-paths, "*" matches
// one segment.
func fieldMatches(pattern, field string) bool {
	if pattern == field || strings.HasPrefix(field, pattern+".") {
		return true
	}
	ok, _ := path.Match(strings.ReplaceAll(pattern, ".", "/"), strings.ReplaceAll(field, ".", "/"))
	return ok
}

func allowsField(r rule, field string) bool {
	if field == "" || r.fields == nil {
		return true
	}
	for _, f := range r.fields {
		if fieldMatches(f, field) {
			return true
		}
	}
	return false
}

// Can reports whether action is allowed on subject (and field when not
// empty), ignoring the conditions: a conditional permission still lets the
// user try, the filter restricts the rows.
func (a *Ability) Can(action, subject, field string) bool {
	if a.superAdmin {
		return true
	}
	for _, r := range a.matching(action, subject) {
		if allowsField(r, field) {
			return true
		}
	}
	return false
}

// CanLocale reports whether action is allowed on subject in locale.
func (a *Ability) CanLocale(action, subject, locale string) bool {
	if a.superAdmin {
		return true
	}
	for _, r := range a.matching(action, subject) {
		if r.locales == nil || contains(r.locales, locale) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// Filter is the where tree restricting the rows action may touch on
// subject: nil when every row is allowed, and allowed is false when none is.
// Rules are alternatives ($or); a rule's locales add a locale filter.
func (a *Ability) Filter(action, subject string) (filter any, allowed bool) {
	if a.superAdmin {
		return nil, true
	}
	rules := a.matching(action, subject)
	if len(rules) == 0 {
		return nil, false
	}
	var alternatives []any
	for _, r := range rules {
		var parts []any
		if r.filter != nil {
			parts = append(parts, r.filter)
		}
		if r.locales != nil {
			parts = append(parts, map[string]any{"locale": map[string]any{"$in": r.locales}})
		}
		switch len(parts) {
		case 0:
			return nil, true
		case 1:
			alternatives = append(alternatives, parts[0])
		default:
			alternatives = append(alternatives, map[string]any{"$and": parts})
		}
	}
	if len(alternatives) == 1 {
		return alternatives[0], true
	}
	return map[string]any{"$or": alternatives}, true
}

// Fields are the fields action may touch on subject, nil for all of them.
func (a *Ability) Fields(action, subject string) []string {
	if a.superAdmin {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range a.matching(action, subject) {
		if r.fields == nil {
			return nil
		}
		for _, f := range r.fields {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}
