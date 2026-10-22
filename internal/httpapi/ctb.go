package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aluzed/grapi/schema"
)

// The Content-Type Builder's content API: the schemas, read only.

func (s *Server) addCTBRoutes() {
	base := strings.TrimPrefix(s.cfg.Prefix, "/")
	ctb := "plugin::content-type-builder."
	s.routes = append(s.routes,
		route{"GET", []string{base, "content-type-builder", "content-types"}, ctb + "content-types", "getContentTypes", s.ctbContentTypes, "", false, false, nil},
		route{"GET", []string{base, "content-type-builder", "content-types", ":uid"}, ctb + "content-types", "getContentType", s.ctbContentType, "", false, false, nil},
		route{"GET", []string{base, "content-type-builder", "components"}, ctb + "components", "getComponents", s.ctbComponents, "", false, false, nil},
		route{"GET", []string{base, "content-type-builder", "components", ":uid"}, ctb + "components", "getComponent", s.ctbComponent, "", false, false, nil},
	)
}

func rawJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

// formatAttributes keeps the attributes the CTB shows: not id, documentId,
// publishedAt, the timestamps or the invisible ones.
func formatAttributes(attrs *schema.OrderedMap[*schema.Attribute]) map[string]any {
	out := map[string]any{}
	for name, a := range attrs.All() {
		switch name {
		case "id", schema.AttrDocumentID, schema.AttrPublishedAt, schema.AttrCreatedAt, schema.AttrUpdatedAt:
			continue
		}
		if !a.IsVisible() {
			continue
		}
		var m map[string]any
		b, _ := json.Marshal(a)
		_ = json.Unmarshal(b, &m)
		switch a.Type {
		case schema.TypeMedia:
			f := map[string]any{"type": "media", "multiple": a.IsMultiple(), "required": a.IsRequired(), "private": a.IsPrivate()}
			if a.Configurable != nil && !*a.Configurable {
				f["configurable"] = false
			}
			if a.AllowedTypes != nil {
				f["allowedTypes"] = a.AllowedTypes
			}
			if po := rawJSON(a.PluginOptions); po != nil {
				f["pluginOptions"] = po
			}
			m = f
		case schema.TypeRelation:
			delete(m, "configurable")
			if a.Configurable != nil && !*a.Configurable {
				m["configurable"] = false
			}
			var target any
			if a.InversedBy != "" {
				target = a.InversedBy
			} else if a.MappedBy != "" {
				target = a.MappedBy
			}
			m["targetAttribute"], m["required"], m["private"] = target, a.IsRequired(), a.IsPrivate()
		}
		out[name] = m
	}
	return out
}

func ctbVisible(ct *schema.ContentType) bool {
	var po struct {
		CTB struct {
			Visible *bool `json:"visible"`
		} `json:"content-type-builder"`
	}
	if len(ct.PluginOptions) > 0 && json.Unmarshal(ct.PluginOptions, &po) == nil && po.CTB.Visible != nil {
		return *po.CTB.Visible
	}
	return true
}

func formatContentType(ct *schema.ContentType) map[string]any {
	sch := map[string]any{"draftAndPublish": false}
	if ct.Options != nil {
		for k, v := range ct.Options.All() {
			sch[k] = rawJSON(v)
		}
	}
	sch["displayName"], sch["singularName"], sch["pluralName"] = ct.Info.DisplayName, ct.Info.SingularName, ct.Info.PluralName
	sch["description"] = ct.Info.Description
	if po := rawJSON(ct.PluginOptions); po != nil {
		sch["pluginOptions"] = po
	}
	sch["kind"], sch["collectionName"] = ct.Kind, ct.CollectionName
	sch["attributes"] = formatAttributes(&ct.Attributes)
	sch["visible"] = ctbVisible(ct)
	switch {
	case ct.UID == "admin::user":
		sch["restrictRelationsTo"] = []string{"oneWay", "manyWay"}
	case strings.HasPrefix(string(ct.UID), "strapi::") || ct.UID == fileUID || !ctbVisible(ct):
		sch["restrictRelationsTo"] = []string{}
	default:
		sch["restrictRelationsTo"] = nil
	}
	out := map[string]any{"uid": ct.UID, "apiID": ct.ModelName, "schema": sch}
	if ct.UID.Namespace() == schema.NamespacePlugin {
		out["plugin"] = ct.UID.Origin()
	} else if ct.UID.Namespace() == schema.NamespaceAdmin {
		out["plugin"] = "admin"
	}
	return out
}

func formatComponent(c *schema.Component) map[string]any {
	sch := map[string]any{"displayName": c.Info.DisplayName, "description": c.Info.Description, "collectionName": c.CollectionName, "attributes": formatAttributes(&c.Attributes)}
	if c.Info.Icon != "" {
		sch["icon"] = c.Info.Icon
	}
	return map[string]any{"uid": c.UID, "category": c.Category, "apiId": c.ModelName, "schema": sch}
}

func (s *Server) ctbContentTypes(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	kind, _ := rc.query["kind"].(string)
	if _, ok := rc.query["kind"]; ok && kind != schema.CollectionType && kind != schema.SingleType {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"name": "ValidationError", "message": "this must be one of the following values: singleType, collectionType",
			"details": map[string]any{"errors": []any{map[string]any{"path": []any{}, "message": "this must be one of the following values: singleType, collectionType", "name": "ValidationError", "value": rc.query["kind"]}}},
		}})
		return nil
	}
	data := []any{}
	for _, ct := range s.set.ContentTypes.All() {
		if kind == "" || ct.Kind == kind {
			data = append(data, formatContentType(ct))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
	return nil
}

func (s *Server) ctbContentType(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	ct, ok := s.set.ContentType(schema.UID(rc.params["uid"]))
	if !ok || !ctbVisible(ct) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "contentType.notFound"})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": formatContentType(ct)})
	return nil
}

func (s *Server) ctbComponents(w http.ResponseWriter, _ *http.Request, _ *requestContext) error {
	data := []any{}
	for _, c := range s.set.Components.All() {
		data = append(data, formatComponent(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
	return nil
}

func (s *Server) ctbComponent(w http.ResponseWriter, _ *http.Request, rc *requestContext) error {
	c, ok := s.set.Component(schema.UID(rc.params["uid"]))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "component.notFound"})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": formatComponent(c)})
	return nil
}
