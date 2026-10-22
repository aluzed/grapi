// Package document is grapi's Document Service: Strapi 5's documents API
// (documentId, draft and publish, locales, components) on top of the query
// engine.
package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// ErrApplication is Strapi's ApplicationError (a 400 at the API).
var ErrApplication = errors.New("application error")

type appError struct{ msg string }

func (e *appError) Error() string { return e.msg }
func (e *appError) Unwrap() error { return ErrApplication }

// Components writes the components and dynamic zones of entries the way
// Strapi's document service does: each component gets its own row, linked
// to its parent as a relation, deleted and recreated on update unless its
// id is given back.
type Components struct {
	E   *query.Engine
	Set *schema.Set
}

func (c *Components) attributes(uid string) (*schema.OrderedMap[*schema.Attribute], error) {
	if ct, ok := c.Set.ContentType(schema.UID(uid)); ok {
		return &ct.Attributes, nil
	}
	if comp, ok := c.Set.Component(schema.UID(uid)); ok {
		return &comp.Attributes, nil
	}
	return nil, fmt.Errorf("model %s not found", uid)
}

func isComponentAttr(a *schema.Attribute) bool {
	return a.Type == schema.TypeComponent || a.Type == schema.TypeDynamicZone
}

// OmitComponentData removes the component and dynamic zone keys of data.
func (c *Components) OmitComponentData(uid string, data query.Data) (query.Data, error) {
	attrs, err := c.attributes(uid)
	if err != nil {
		return nil, err
	}
	out := query.Data{}
	for k, v := range data {
		if a, ok := attrs.Get(k); ok && isComponentAttr(a) {
			continue
		}
		out[k] = v
	}
	return out, nil
}

func pivot(field, componentType string) map[string]any {
	p := map[string]any{"field": field}
	if componentType != "" {
		p["component_type"] = componentType
	}
	return p
}

// Create creates the components of data and returns the relation values
// linking them to their parent.
func (c *Components) Create(ctx context.Context, uid string, data query.Data) (query.Data, error) {
	attrs, err := c.attributes(uid)
	if err != nil {
		return nil, err
	}
	body := query.Data{}
	for name, a := range attrs.All() {
		value, present := data[name]
		if !present || !isComponentAttr(a) {
			continue
		}
		if a.Type == schema.TypeComponent {
			if value == nil {
				continue
			}
			if a.Repeatable != nil && *a.Repeatable {
				list, ok := value.([]any)
				if !ok {
					return nil, errors.New("Expected an array to create repeatable component") //nolint:revive,staticcheck // Strapi's message
				}
				links := make([]any, 0, len(list))
				for _, v := range list {
					id, err := c.createComponent(ctx, a.Component, asData(v))
					if err != nil {
						return nil, err
					}
					links = append(links, map[string]any{"id": id, "__pivot": pivot(name, a.Component)})
				}
				body[name] = links
				continue
			}
			id, err := c.createComponent(ctx, a.Component, asData(value))
			if err != nil {
				return nil, err
			}
			body[name] = map[string]any{"id": id, "__pivot": pivot(name, a.Component)}
			continue
		}
		list, ok := value.([]any)
		if !ok {
			return nil, errors.New("Expected an array to create repeatable component") //nolint:revive,staticcheck // Strapi's message
		}
		links := make([]any, 0, len(list))
		for _, v := range list {
			d := asData(v)
			comp, _ := d["__component"].(string)
			id, err := c.createComponent(ctx, comp, d)
			if err != nil {
				return nil, err
			}
			links = append(links, map[string]any{"id": id, "__component": comp, "__pivot": pivot(name, "")})
		}
		body[name] = links
	}
	return body, nil
}

func asData(v any) query.Data {
	switch x := v.(type) {
	case map[string]any:
		return x
	case query.Row:
		out := query.Data{}
		for k, val := range x.All() {
			out[k] = val
		}
		return out
	}
	return query.Data{}
}

// assign is lodash's assign({}, componentData, omitComponentData(data)).
func (c *Components) assign(uid string, componentData, data query.Data) (query.Data, error) {
	rest, err := c.OmitComponentData(uid, data)
	if err != nil {
		return nil, err
	}
	out := query.Data{}
	for k, v := range componentData {
		out[k] = v
	}
	for k, v := range rest {
		out[k] = v
	}
	return out, nil
}

func (c *Components) createComponent(ctx context.Context, uid string, data query.Data) (any, error) {
	if _, ok := c.Set.Component(schema.UID(uid)); !ok {
		return nil, fmt.Errorf("component %q not found", uid)
	}
	componentData, err := c.Create(ctx, uid, data)
	if err != nil {
		return nil, err
	}
	withoutID := query.Data{}
	for k, v := range data {
		if k != "id" {
			withoutID[k] = v
		}
	}
	entry, err := c.assign(uid, componentData, withoutID)
	if err != nil {
		return nil, err
	}
	row, err := c.E.Create(ctx, uid, entry, query.Params{})
	if err != nil {
		return nil, err
	}
	id, _ := row.Get("id")
	return id, nil
}

// Update replaces the components of the fields present in data and returns
// the relation values for the parent.
func (c *Components) Update(ctx context.Context, uid string, entityID any, data query.Data) (query.Data, error) {
	attrs, err := c.attributes(uid)
	if err != nil {
		return nil, err
	}
	body := query.Data{}
	for name, a := range attrs.All() {
		value, present := data[name]
		if !present {
			continue
		}
		switch a.Type {
		case schema.TypeComponent:
			if err := c.deleteOld(ctx, uid, a.Component, entityID, name, value); err != nil {
				return nil, err
			}
			if a.Repeatable != nil && *a.Repeatable {
				list, ok := value.([]any)
				if !ok {
					return nil, errors.New("Expected an array to create repeatable component") //nolint:revive,staticcheck // Strapi's message
				}
				links := []any{}
				for _, v := range list {
					id, err := c.updateOrCreate(ctx, a.Component, v)
					if err != nil {
						return nil, err
					}
					if id != nil {
						links = append(links, map[string]any{"id": id, "__pivot": pivot(name, a.Component)})
					}
				}
				body[name] = links
				continue
			}
			id, err := c.updateOrCreate(ctx, a.Component, value)
			if err != nil {
				return nil, err
			}
			if id == nil {
				body[name] = nil
			} else {
				body[name] = map[string]any{"id": id, "__pivot": pivot(name, a.Component)}
			}
		case schema.TypeDynamicZone:
			if err := c.deleteOldDZ(ctx, uid, entityID, name, value); err != nil {
				return nil, err
			}
			list, ok := value.([]any)
			if !ok {
				return nil, errors.New("Expected an array to create repeatable component") //nolint:revive,staticcheck // Strapi's message
			}
			links := make([]any, 0, len(list))
			for _, v := range list {
				d := asData(v)
				comp, _ := d["__component"].(string)
				id, err := c.updateOrCreate(ctx, comp, v)
				if err != nil {
					return nil, err
				}
				links = append(links, map[string]any{"id": id, "__component": comp, "__pivot": pivot(name, "")})
			}
			body[name] = links
		}
	}
	return body, nil
}

func stringID(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprint(int64(x))
	}
	return fmt.Sprint(v)
}

func castArray(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case nil:
		return []any{nil}
	}
	return []any{v}
}

func idOf(v any) (any, bool) {
	switch x := v.(type) {
	case map[string]any:
		id, ok := x["id"]
		return id, ok
	case query.Row:
		return x.Get("id")
	}
	return nil, false
}

func componentOf(v any) string {
	switch x := v.(type) {
	case map[string]any:
		s, _ := x["__component"].(string)
		return s
	case query.Row:
		s, _ := x.Get("__component")
		str, _ := s.(string)
		return str
	}
	return ""
}

func (c *Components) previous(ctx context.Context, uid string, entityID any, name string) (any, error) {
	loaded, err := c.E.Load(ctx, uid, entityID, []string{name}, nil)
	if err != nil || loaded == nil {
		return nil, err
	}
	return loaded[name], nil
}

func notRelated(name string) error {
	return &appError{fmt.Sprintf("Some of the provided components in %s are not related to the entity", name)}
}

func (c *Components) deleteOld(ctx context.Context, uid, componentUID string, entityID any, name string, value any) error {
	prev, err := c.previous(ctx, uid, entityID, name)
	if err != nil {
		return err
	}
	var keep, all []string
	for _, v := range castArray(value) {
		if id, ok := idOf(v); ok {
			keep = append(keep, stringID(id))
		}
	}
	allSet := map[string]bool{}
	for _, v := range castArray(prev) {
		if id, ok := idOf(v); ok {
			all = append(all, stringID(id))
			allSet[stringID(id)] = true
		}
	}
	keepSet := map[string]bool{}
	for _, k := range keep {
		if !allSet[k] {
			return notRelated(name)
		}
		keepSet[k] = true
	}
	for _, id := range all {
		if !keepSet[id] {
			if err := c.DeleteComponent(ctx, componentUID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Components) deleteOldDZ(ctx context.Context, uid string, entityID any, name string, value any) error {
	prev, err := c.previous(ctx, uid, entityID, name)
	if err != nil {
		return err
	}
	type key struct{ id, comp string }
	var keep, all []key
	for _, v := range castArray(value) {
		if id, ok := idOf(v); ok {
			keep = append(keep, key{stringID(id), componentOf(v)})
		}
	}
	for _, v := range castArray(prev) {
		if id, ok := idOf(v); ok {
			all = append(all, key{stringID(id), componentOf(v)})
		}
	}
	has := func(list []key, k key) bool {
		for _, x := range list {
			if x == k {
				return true
			}
		}
		return false
	}
	for _, k := range keep {
		if !has(all, k) {
			return notRelated(name)
		}
	}
	for _, k := range all {
		if !has(keep, k) {
			if err := c.DeleteComponent(ctx, k.comp, k.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Components) updateOrCreate(ctx context.Context, uid string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data := asData(value)
	if id, ok := data["id"]; ok && id != nil {
		componentData, err := c.Update(ctx, uid, id, data)
		if err != nil {
			return nil, err
		}
		entry, err := c.assign(uid, componentData, data)
		if err != nil {
			return nil, err
		}
		row, err := c.E.Update(ctx, uid, map[string]any{"id": id}, entry, query.Params{})
		if err != nil || row == nil {
			return nil, err
		}
		v, _ := row.Get("id")
		return v, nil
	}
	return c.createComponent(ctx, uid, data)
}

// load reads the component and dynamic zone values of an entry.
func (c *Components) load(ctx context.Context, uid string, entityID any) (map[string]any, error) {
	attrs, err := c.attributes(uid)
	if err != nil {
		return nil, err
	}
	var names []string
	for name, a := range attrs.All() {
		if isComponentAttr(a) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return map[string]any{}, nil
	}
	loaded, err := c.E.Load(ctx, uid, entityID, names, nil)
	if loaded == nil {
		loaded = map[string]any{}
	}
	return loaded, err
}

// Delete deletes the components of an entry, nested ones first.
func (c *Components) Delete(ctx context.Context, uid string, entityID any) error {
	loaded, err := c.load(ctx, uid, entityID)
	if err != nil {
		return err
	}
	return c.deleteLoaded(ctx, uid, loaded)
}

func (c *Components) deleteLoaded(ctx context.Context, uid string, loaded map[string]any) error {
	attrs, err := c.attributes(uid)
	if err != nil {
		return err
	}
	for name, a := range attrs.All() {
		if !isComponentAttr(a) {
			continue
		}
		value := loaded[name]
		if value == nil {
			continue
		}
		for _, sub := range castArray(value) {
			id, ok := idOf(sub)
			if !ok {
				continue
			}
			compUID := a.Component
			if a.Type == schema.TypeDynamicZone {
				compUID = componentOf(sub)
			}
			if err := c.DeleteComponent(ctx, compUID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteComponent deletes a component row and its own components.
func (c *Components) DeleteComponent(ctx context.Context, uid string, id any) error {
	if _, ok := c.Set.Component(schema.UID(uid)); !ok {
		return nil
	}
	if err := c.Delete(ctx, uid, id); err != nil {
		return err
	}
	_, err := c.E.Delete(ctx, uid, map[string]any{"id": id}, query.Params{})
	return err
}

func marshalJSON(v any) ([]byte, error) { return json.Marshal(v) }
