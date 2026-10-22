package schema

import (
	"fmt"
	"strings"
	"testing"
)

// ct builds a prepared api content type from its attributes JSON.
func ct(t *testing.T, name, attrs string, extra ...string) *ContentType {
	t.Helper()
	opts := ""
	if len(extra) > 0 {
		opts = "," + extra[0]
	}
	src := fmt.Sprintf(`{"collectionName":%q,"info":{"singularName":%q,"pluralName":%q,"displayName":%q},"attributes":%s%s}`,
		name+"s", name, name+"s", name, attrs, opts)
	c, err := ParseContentType([]byte(src))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := c.Prepare(APIUID(name, name), ""); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return c
}

func comp(t *testing.T, uid, attrs string) *Component {
	t.Helper()
	cat, name, _ := strings.Cut(uid, ".")
	c, err := ParseComponent([]byte(fmt.Sprintf(`{"collectionName":"components_%s_%ss","info":{"displayName":%q},"attributes":%s}`, cat, name, name, attrs)))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Prepare(cat, name); err != nil {
		t.Fatal(err)
	}
	return c
}

func set(cts []*ContentType, comps ...*Component) *Set {
	s := &Set{}
	for _, c := range cts {
		s.AddContentType(c)
	}
	for _, c := range comps {
		s.AddComponent(c)
	}
	return s
}

func hasIssue(r *Report, code string, sev Severity) bool {
	for _, i := range r.Issues {
		if i.Code == code && i.Severity == sev {
			return true
		}
	}
	return false
}

func TestValidateRules(t *testing.T) {
	type tc struct {
		name string
		set  func(t *testing.T) *Set
		code string
		sev  Severity
	}
	rel := func(attrs string) func(t *testing.T) *Set {
		return func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "post", attrs), ct(t, "tag", `{"posts":{"type":"relation","relation":"manyToMany","target":"api::post.post","mappedBy":"tags"},"owner":{"type":"relation","relation":"oneToMany","target":"api::post.post","mappedBy":"author"},"label":{"type":"string"}}`)})
		}
	}
	tests := []tc{
		{"unknown type", func(t *testing.T) *Set { return set([]*ContentType{ct(t, "a", `{"x":{"type":"nope"}}`)}) }, CodeUnknownType, SeverityError},
		{"unregistered custom field", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"x":{"type":"customField","customField":"plugin::color-picker.color"}}`)})
		}, CodeUnknownType, SeverityError},
		{"duplicate singular", func(t *testing.T) *Set {
			b := ct(t, "b", `{}`)
			b.Info.SingularName = "a"
			b.CollectionName = "bs"
			return set([]*ContentType{ct(t, "a", `{}`), b})
		}, CodeDuplicateName, SeverityError},
		{"plural equals another singular", func(t *testing.T) *Set {
			b := ct(t, "b", `{}`)
			b.Info.PluralName = "a"
			return set([]*ContentType{ct(t, "a", `{}`), b})
		}, CodeDuplicateName, SeverityError},
		{"duplicate table", func(t *testing.T) *Set {
			b := ct(t, "b", `{}`)
			b.CollectionName = "as"
			return set([]*ContentType{ct(t, "a", `{}`), b})
		}, CodeDuplicateTable, SeverityError},
		{"component table collides", func(t *testing.T) *Set {
			c := comp(t, "shared.seo", `{}`)
			c.CollectionName = "as"
			return set([]*ContentType{ct(t, "a", `{}`)}, c)
		}, CodeDuplicateTable, SeverityError},
		{"unknown relation target", rel(`{"x":{"type":"relation","relation":"oneToOne","target":"api::nope.nope"}}`), CodeUnknownTarget, SeverityError},
		{"unknown relation kind", rel(`{"x":{"type":"relation","relation":"oneToFew","target":"api::tag.tag"}}`), CodeRelationKind, SeverityError},
		{"inversedBy missing", rel(`{"tags":{"type":"relation","relation":"manyToMany","target":"api::tag.tag","inversedBy":"nope"}}`), CodeInverseMissing, SeverityError},
		{"inversedBy not relation", rel(`{"tags":{"type":"relation","relation":"manyToMany","target":"api::tag.tag","inversedBy":"label"}}`), CodeInverseNotRelation, SeverityError},
		{"mappedBy missing", rel(`{"tags":{"type":"relation","relation":"manyToMany","target":"api::tag.tag","mappedBy":"nope"}}`), CodeInverseMissing, SeverityWarning},
		{"oneToMany owner", rel(`{"x":{"type":"relation","relation":"oneToMany","target":"api::tag.tag","inversedBy":"owner"}}`), CodeOneToManyOwner, SeverityError},
		{"oneToMany without join table", rel(`{"x":{"type":"relation","relation":"oneToMany","target":"api::tag.tag","useJoinTable":false}}`), CodeOneToManyOwner, SeverityError},
		{"manyToOne not owner", rel(`{"author":{"type":"relation","relation":"manyToOne","target":"api::tag.tag","mappedBy":"owner"}}`), CodeManyToOneNotOwner, SeverityError},
		{"inverse kind mismatch", rel(`{"tags":{"type":"relation","relation":"oneToOne","target":"api::tag.tag","inversedBy":"posts"}}`), CodeInverseMismatch, SeverityWarning},
		{"inverse points elsewhere", rel(`{"other":{"type":"relation","relation":"manyToMany","target":"api::tag.tag","inversedBy":"posts"}}`), CodeInverseMismatch, SeverityWarning},
		{"oneWay with inverse", rel(`{"x":{"type":"relation","relation":"oneWay","target":"api::tag.tag","inversedBy":"posts"}}`), CodeUnidirInverse, SeverityWarning},
		{"morph target missing", rel(`{"x":{"type":"relation","relation":"morphMany","target":"api::nope.nope","morphBy":"related"}}`), CodeMorphTarget, SeverityError},
		{"morphBy missing", rel(`{"x":{"type":"relation","relation":"morphMany","target":"api::tag.tag","morphBy":"related"}}`), CodeMorphTarget, SeverityError},
		{"enum starts with digit", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"e":{"type":"enumeration","enum":["1st","second"]}}`)})
		}, CodeEnumInvalid, SeverityError},
		{"enum empty after normalize", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"e":{"type":"enumeration","enum":["ok","!!!"]}}`)})
		}, CodeEnumEmpty, SeverityError},
		{"enum collision", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"e":{"type":"enumeration","enum":["new york","new-york"]}}`)})
		}, CodeEnumCollision, SeverityError},
		{"enum default outside", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"e":{"type":"enumeration","enum":["a","b"],"default":"c"}}`)})
		}, CodeEnumDefault, SeverityWarning},
		{"unknown component", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"c":{"type":"component","component":"shared.nope"}}`)})
		}, CodeUnknownComponent, SeverityError},
		{"unknown dz component", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"d":{"type":"dynamiczone","components":["shared.nope"]}}`)})
		}, CodeUnknownComponent, SeverityError},
		{"component cycle", func(t *testing.T) *Set {
			return set(nil, comp(t, "x.a", `{"b":{"type":"component","component":"x.b"}}`), comp(t, "x.b", `{"a":{"type":"component","component":"x.a"}}`))
		}, CodeComponentCycle, SeverityError},
		{"self component", func(t *testing.T) *Set {
			return set(nil, comp(t, "x.a", `{"a":{"type":"component","component":"x.a","repeatable":true}}`))
		}, CodeComponentCycle, SeverityError},
		{"min max on non repeatable", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"c":{"type":"component","component":"shared.seo","min":1}}`)}, comp(t, "shared.seo", `{}`))
		}, CodeRepeatableBounds, SeverityWarning},
		{"repeatable min greater than max", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"c":{"type":"component","component":"shared.seo","repeatable":true,"min":3,"max":1}}`)}, comp(t, "shared.seo", `{}`))
		}, CodeBounds, SeverityWarning},
		{"number min greater than max", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"n":{"type":"integer","min":10,"max":1}}`)})
		}, CodeBounds, SeverityWarning},
		{"minLength greater than maxLength", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"s":{"type":"string","minLength":10,"maxLength":2}}`)})
		}, CodeBounds, SeverityWarning},
		{"uid target missing", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"slug":{"type":"uid","targetField":"title"}}`)})
		}, CodeUIDTarget, SeverityWarning},
		{"uid target wrong type", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"n":{"type":"integer"},"slug":{"type":"uid","targetField":"n"}}`)})
		}, CodeUIDTarget, SeverityWarning},
		{"media allowed type", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"m":{"type":"media","allowedTypes":["images","pdfs"]}}`)})
		}, CodeMediaAllowedTypes, SeverityWarning},
		{"localized attribute on non localized type", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"s":{"type":"string","pluginOptions":{"i18n":{"localized":true}}}}`)})
		}, CodeLocalizedAttribute, SeverityWarning},
		{"bidirectional relation in component", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "tag", `{"x":{"type":"relation","relation":"oneToOne","target":"api::tag.tag"}}`)},
				comp(t, "shared.link", `{"tag":{"type":"relation","relation":"oneToOne","target":"api::tag.tag","inversedBy":"x"}}`))
		}, CodeComponentRelationBi, SeverityWarning},
		{"identifier too long", func(t *testing.T) *Set {
			return set([]*ContentType{ct(t, "a", `{"aVeryLongAttributeNameThatGoesWellBeyondTheFiftyFiveCharacterLimit":{"type":"string"}}`)})
		}, CodeIdentifierTooLong, SeverityWarning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate(tt.set(t))
			if !hasIssue(r, tt.code, tt.sev) {
				t.Errorf("want %s (%s), got %v", tt.code, tt.sev, r.Issues)
			}
		})
	}
}

func TestValidateValidSetHasNoIssue(t *testing.T) {
	s := set([]*ContentType{
		ct(t, "post", `{"title":{"type":"string"},"slug":{"type":"uid","targetField":"title"},"tags":{"type":"relation","relation":"manyToMany","target":"api::tag.tag","inversedBy":"posts"},"author":{"type":"relation","relation":"manyToOne","target":"api::tag.tag","inversedBy":"owned"},"seo":{"type":"component","component":"shared.seo"},"blocks":{"type":"dynamiczone","components":["shared.seo"]},"stage":{"type":"enumeration","enum":["draft","in review"],"default":"draft"},"cover":{"type":"media","allowedTypes":["images"]}}`),
		ct(t, "tag", `{"posts":{"type":"relation","relation":"manyToMany","target":"api::post.post","mappedBy":"tags"},"owned":{"type":"relation","relation":"oneToMany","target":"api::post.post","mappedBy":"author"}}`),
	}, comp(t, "shared.seo", `{"title":{"type":"string"},"post":{"type":"relation","relation":"oneToOne","target":"api::post.post"}}`))
	if r := Validate(s); len(r.Issues) != 0 {
		t.Errorf("unexpected issues: %v", r.Issues)
	}
}

func TestReportListsAllErrors(t *testing.T) {
	s := set([]*ContentType{ct(t, "a", `{"x":{"type":"nope"},"c":{"type":"component","component":"no.pe"},"r":{"type":"relation","relation":"oneToOne","target":"api::z.z"}}`)})
	err := Validate(s).Err()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, code := range []string{CodeUnknownType, CodeUnknownComponent, CodeUnknownTarget} {
		if !strings.Contains(err.Error(), code) {
			t.Errorf("report misses %s:\n%v", code, err)
		}
	}
	if !strings.Contains(err.Error(), "3 errors") {
		t.Errorf("report should count 3 errors:\n%v", err)
	}
}
