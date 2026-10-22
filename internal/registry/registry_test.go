package registry

import (
	"errors"
	"sync"
	"testing"
)

func TestValidUID(t *testing.T) {
	for _, ok := range []string{"api::post.post", "plugin::users-permissions.user", "global::is-owner", "admin::user", "strapi::core-store", "api::post.is-owner", "plugin::upload.file"} {
		if !ValidUID(ok) {
			t.Errorf("%s should be valid", ok)
		}
	}
	for _, bad := range []string{"api:foo", "plugin::", "Api::post.post", "api::Post.post", "foo::bar", "post.post"} {
		if ValidUID(bad) {
			t.Errorf("%s should be invalid", bad)
		}
	}
}

func TestAddTwiceFails(t *testing.T) {
	r := New[int]("service")
	if err := r.Add("api::a.a", 1); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("api::a.a", 2); err == nil {
		t.Error("second Add must fail")
	}
}

type (
	controller interface{ Find() string }
	base       struct{}
)

func (base) Find() string { return "base" }

type override struct{ super controller }

func (o override) Find() string { return "custom+" + o.super.Find() }

func TestExtendKeepsSuper(t *testing.T) {
	r := New[controller]("controller")
	_ = r.Add("api::a.a", base{})
	if err := r.Extend("api::a.a", func(prev controller) controller { return override{prev} }); err != nil {
		t.Fatal(err)
	}
	c, _ := r.Get("api::a.a")
	if got := c.Find(); got != "custom+base" {
		t.Errorf("Find = %q", got)
	}
	if err := r.Extend("api::missing.missing", func(p controller) controller { return p }); err == nil {
		t.Error("Extend of a missing uid must fail")
	}
}

func TestLazyFactoriesCanDependOnEachOther(t *testing.T) {
	r := New[string]("service")
	_ = r.AddLazy("api::a.a", func() string { b, _ := r.Get("api::b.b"); return "a(" + b + ")" })
	_ = r.AddLazy("api::b.b", func() string { return "b" })
	if v, _ := r.Get("api::a.a"); v != "a(b)" {
		t.Errorf("got %q", v)
	}
}

func TestResolveOrder(t *testing.T) {
	r := New[string]("policy")
	_ = r.Add("global::is-owner", "global")
	_ = r.Add("api::post.is-owner", "api")
	_ = r.Add("plugin::up.is-owner", "plugin")
	_ = r.Add("plugin::other.only-plugin", "other")
	tests := []struct {
		name  string
		scope Scope
		want  string
	}{
		{"is-owner", Scope{API: "post"}, "api"},
		{"is-owner", Scope{Plugin: "up", API: "post"}, "plugin"},
		{"is-owner", Scope{API: "comment"}, "global"},
		{"plugin::other.only-plugin", Scope{API: "post"}, "other"},
	}
	for _, tt := range tests {
		got, _, err := r.Resolve(tt.name, tt.scope)
		if err != nil || got != tt.want {
			t.Errorf("Resolve(%s, %+v) = %q, %v; want %q", tt.name, tt.scope, got, err, tt.want)
		}
	}
	if _, _, err := r.Resolve("nope", Scope{API: "post"}); err == nil {
		t.Error("unknown policy must fail")
	}
}

func TestFreezeAndConcurrentReads(t *testing.T) {
	r := New[int]("service")
	_ = r.AddLazy("api::a.a", func() int { return 42 })
	r.Freeze()
	var frozen ErrFrozen
	if err := r.Add("api::b.b", 1); !errors.As(err, &frozen) {
		t.Errorf("Add after Freeze = %v", err)
	}
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, ok := r.Get("api::a.a"); !ok || v != 42 {
				t.Errorf("Get = %d, %v", v, ok)
			}
			_ = r.Keys("api::")
		}()
	}
	wg.Wait()
}
