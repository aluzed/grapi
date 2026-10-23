package permissions

import (
	"reflect"
	"testing"
)

const (
	update = "plugin::content-manager.explorer.update"
	read   = "plugin::content-manager.explorer.read"
	post   = "api::post.post"
)

func TestCanField(t *testing.T) {
	a := Build([]Permission{{Action: update, Subject: post, Fields: []string{"body", "seo.title"}}}, User{ID: 1}, NewConditions())
	cases := map[string]bool{"": true, "body": true, "title": false, "seo.title": true, "seo.description": false, "seo": false}
	for field, want := range cases {
		if got := a.Can(update, post, field); got != want {
			t.Errorf("Can(%q) = %v, want %v", field, got, want)
		}
	}
	if a.Can(read, post, "") {
		t.Error("read was not granted")
	}
	if a.Can(update, "api::page.page", "") {
		t.Error("another subject was granted")
	}
}

func TestFieldPatterns(t *testing.T) {
	a := Build([]Permission{{Action: update, Subject: post, Fields: []string{"blocks.*.title", "seo"}}}, User{}, NewConditions())
	for field, want := range map[string]bool{"blocks.0.title": true, "blocks.0.body": false, "seo.title": true} {
		if got := a.Can(update, post, field); got != want {
			t.Errorf("Can(%q) = %v, want %v", field, got, want)
		}
	}
}

func TestIsCreatorFilter(t *testing.T) {
	a := Build([]Permission{{Action: read, Subject: post, Conditions: []string{IsCreator}}}, User{ID: 7}, NewConditions())
	f, ok := a.Filter(read, post)
	want := map[string]any{"createdBy": map[string]any{"id": map[string]any{"$eq": 7}}}
	if !ok || !reflect.DeepEqual(f, want) {
		t.Fatalf("Filter = %v %v, want %v", f, ok, want)
	}
	if !a.Can(read, post, "") {
		t.Error("a conditional permission still allows the action")
	}
}

func TestPermissionsCombineInOr(t *testing.T) {
	perms := []Permission{
		{Action: read, Subject: post, Conditions: []string{IsCreator}},
		{Action: read, Subject: post, Conditions: []string{HasSameRoleAsCreator}},
	}
	a := Build(perms, User{ID: 7, RoleIDs: []any{2}}, NewConditions())
	f, _ := a.Filter(read, post)
	or, ok := f.(map[string]any)["$or"].([]any)
	if !ok || len(or) != 2 {
		t.Fatalf("Filter = %v, want a $or of two", f)
	}
}

func TestConditionsOfOnePermissionAreAlternatives(t *testing.T) {
	a := Build([]Permission{{Action: read, Subject: post, Conditions: []string{IsCreator, HasSameRoleAsCreator}}}, User{ID: 7}, NewConditions())
	f, _ := a.Filter(read, post)
	if or, ok := f.(map[string]any)["$or"].([]any); !ok || len(or) != 2 {
		t.Fatalf("Filter = %v, want a $or of two", f)
	}
}

func TestUnconditionalWins(t *testing.T) {
	perms := []Permission{
		{Action: read, Subject: post, Conditions: []string{IsCreator}},
		{Action: read, Subject: post},
	}
	f, ok := Build(perms, User{ID: 1}, NewConditions()).Filter(read, post)
	if !ok || f != nil {
		t.Fatalf("Filter = %v %v, want no filter", f, ok)
	}
}

func TestBooleanConditions(t *testing.T) {
	c := NewConditions()
	c.Register(Condition{ID: "plugin::x.yes", Handler: func(User) any { return true }}, Condition{ID: "plugin::x.no", Handler: func(User) any { return false }})
	a := Build([]Permission{{Action: read, Subject: post, Conditions: []string{"plugin::x.no"}}}, User{}, c)
	if a.Can(read, post, "") {
		t.Error("a false condition drops the permission")
	}
	a = Build([]Permission{{Action: read, Subject: post, Conditions: []string{IsCreator, "plugin::x.yes"}}}, User{ID: 1}, c)
	if f, ok := a.Filter(read, post); !ok || f != nil {
		t.Errorf("a true condition lifts the restriction, got %v", f)
	}
	a = Build([]Permission{{Action: read, Subject: post, Conditions: []string{"plugin::x.unknown"}}}, User{}, c)
	if f, ok := a.Filter(read, post); !ok || f != nil {
		t.Errorf("unknown conditions are ignored, got %v %v", f, ok)
	}
}

func TestLocales(t *testing.T) {
	a := Build([]Permission{{Action: read, Subject: post, Locales: []string{"fr"}}}, User{}, NewConditions())
	if !a.CanLocale(read, post, "fr") || a.CanLocale(read, post, "en") {
		t.Error("only fr is allowed")
	}
	f, _ := a.Filter(read, post)
	want := map[string]any{"locale": map[string]any{"$in": []any{"fr"}}}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("Filter = %v, want %v", f, want)
	}
	a = Build([]Permission{{Action: read, Subject: post, Locales: []string{"fr"}, Conditions: []string{IsCreator}}}, User{ID: 3}, NewConditions())
	f, _ = a.Filter(read, post)
	if and, ok := f.(map[string]any)["$and"].([]any); !ok || len(and) != 2 {
		t.Errorf("Filter = %v, want a $and of condition and locale", f)
	}
}

func TestSubjectLessAndSuperAdmin(t *testing.T) {
	a := Build([]Permission{{Action: "admin::webhooks.read"}}, User{}, NewConditions())
	if !a.Can("admin::webhooks.read", "", "") || a.Can("admin::webhooks.create", "", "") {
		t.Error("subject-less permission")
	}
	s := SuperAdmin()
	if !s.Can("anything", post, "x") {
		t.Error("super admin")
	}
	if f, ok := s.Filter(read, post); !ok || f != nil || s.Fields(read, post) != nil {
		t.Error("super admin has no restriction")
	}
}

func TestFields(t *testing.T) {
	perms := []Permission{
		{Action: read, Subject: post, Fields: []string{"title"}},
		{Action: read, Subject: post, Fields: []string{"title", "body"}},
	}
	a := Build(perms, User{}, NewConditions())
	if got := a.Fields(read, post); !reflect.DeepEqual(got, []string{"title", "body"}) {
		t.Errorf("Fields = %v", got)
	}
	if got := a.Fields(update, post); got == nil || len(got) != 0 {
		t.Errorf("no permission gives no field, got %v", got)
	}
}

func TestActionID(t *testing.T) {
	r := &Actions{}
	r.Register(Action{UID: "users.read", PluginName: "admin"}, Action{UID: "explorer.read", PluginName: "content-manager"})
	if _, ok := r.Get("admin::users.read"); !ok {
		t.Error("admin action")
	}
	if _, ok := r.Get("plugin::content-manager.explorer.read"); !ok {
		t.Error("plugin action")
	}
}
