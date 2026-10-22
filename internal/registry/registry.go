package registry

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Namespaces of registry uids.
const (
	NSAPI    = "api"
	NSPlugin = "plugin"
	NSAdmin  = "admin"
	NSGlobal = "global"
	NSStrapi = "strapi"
)

var uidPattern = regexp.MustCompile(`^(api|plugin|admin|global|strapi)::[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-zA-Z0-9-]*)*$`)

// ValidUID reports whether uid has the <namespace>::<name>[.<sub>] form.
func ValidUID(uid string) bool { return uidPattern.MatchString(uid) }

// Registry holds values of one kind (services, controllers, policies...) by uid.
// Values may be lazy factories, built on first Get, which lets services depend
// on each other in any order as in Strapi. A frozen registry rejects writes.
type Registry[T any] struct {
	kind      string
	mu        sync.RWMutex
	factories map[string]func() T
	values    map[string]T
	frozen    bool
}

// New returns an empty registry; kind names it in errors ("service", "policy").
func New[T any](kind string) *Registry[T] {
	return &Registry[T]{kind: kind, factories: map[string]func() T{}, values: map[string]T{}}
}

// ErrFrozen is returned by writes after Freeze.
type ErrFrozen struct{ Kind, UID string }

func (e ErrFrozen) Error() string {
	return fmt.Sprintf("%s registry is frozen: cannot register %s after bootstrap", e.Kind, e.UID)
}

// Add registers a value. Registering an existing uid is an error.
func (r *Registry[T]) Add(uid string, v T) error {
	return r.AddLazy(uid, func() T { return v })
}

// AddLazy registers a factory called on the first Get.
func (r *Registry[T]) AddLazy(uid string, factory func() T) error {
	if !ValidUID(uid) {
		return fmt.Errorf("invalid %s uid %q", r.kind, uid)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrFrozen{r.kind, uid}
	}
	if _, ok := r.factories[uid]; ok {
		return fmt.Errorf("%s %s has already been registered", r.kind, uid)
	}
	r.factories[uid] = factory
	return nil
}

// Extend wraps the registered value: fn receives the previous implementation,
// which is how an override keeps access to the original (Strapi's super).
func (r *Registry[T]) Extend(uid string, fn func(prev T) T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrFrozen{r.kind, uid}
	}
	prev, ok := r.factories[uid]
	if !ok {
		return fmt.Errorf("%s %s does not exist", r.kind, uid)
	}
	delete(r.values, uid)
	r.factories[uid] = func() T { return fn(prev()) }
	return nil
}

// Get returns the value for uid, building it on first use.
func (r *Registry[T]) Get(uid string) (T, bool) {
	r.mu.RLock()
	v, ok := r.values[uid]
	factory, known := r.factories[uid]
	r.mu.RUnlock()
	if ok || !known {
		return v, ok
	}
	// Built outside the lock: a factory may Get other entries of this registry.
	built := factory()
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.values[uid]; ok {
		return existing, true
	}
	r.values[uid] = built
	return built, true
}

// Has reports whether uid is registered.
func (r *Registry[T]) Has(uid string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[uid]
	return ok
}

// Keys returns the registered uids starting with prefix ("" for all), sorted.
func (r *Registry[T]) Keys(prefix string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for uid := range r.factories {
		if strings.HasPrefix(uid, prefix) {
			out = append(out, uid)
		}
	}
	sort.Strings(out)
	return out
}

// Freeze rejects every later write.
func (r *Registry[T]) Freeze() {
	r.mu.Lock()
	r.frozen = true
	r.mu.Unlock()
}

// Unfreeze reopens the registry for a controlled reload (develop mode, content-type builder).
func (r *Registry[T]) Unfreeze() {
	r.mu.Lock()
	r.frozen = false
	r.mu.Unlock()
}

// Scope tells where a short name is resolved from: the plugin or api that
// declares the route using it.
type Scope struct {
	Plugin string
	API    string
}

// Resolve finds a policy or middleware by name: the full uid first, then
// plugin::<plugin>.<name>, api::<api>.<name> and global::<name>.
func (r *Registry[T]) Resolve(name string, s Scope) (T, string, error) {
	candidates := []string{name}
	if !strings.Contains(name, "::") {
		candidates = nil
		if s.Plugin != "" {
			candidates = append(candidates, "plugin::"+s.Plugin+"."+name)
		}
		if s.API != "" {
			candidates = append(candidates, "api::"+s.API+"."+name)
		}
		candidates = append(candidates, "global::"+name)
	}
	for _, uid := range candidates {
		if v, ok := r.Get(uid); ok {
			return v, uid, nil
		}
	}
	var zero T
	return zero, "", fmt.Errorf("%s %q not found (tried %s)", r.kind, name, strings.Join(candidates, ", "))
}
