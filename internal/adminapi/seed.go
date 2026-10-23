package adminapi

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
)

const (
	explorer       = "plugin::content-manager.explorer."
	creatorFilter  = permissions.IsCreator
	settingsAdmin  = permissions.SectionSettings
	pluginsSection = permissions.SectionPlugins
)

// ContentManagerTypes are the content types the Content Manager shows.
func (s *Server) ContentManagerTypes() []string {
	var out []string
	for uid, ct := range s.set.ContentTypes.All() {
		if !strings.HasPrefix(uid, "api::") && !strings.HasPrefix(uid, "plugin::") {
			continue
		}
		var opts struct {
			CM struct {
				Visible *bool `json:"visible"`
			} `json:"content-manager"`
		}
		if len(ct.PluginOptions) > 0 {
			_ = json.Unmarshal(ct.PluginOptions, &opts)
		}
		if opts.CM.Visible != nil && !*opts.CM.Visible {
			continue
		}
		out = append(out, uid)
	}
	sort.Strings(out)
	return out
}

func (s *Server) registerActions() {
	subjects := s.ContentManagerTypes()
	ct := func(uid, name string, props ...string) permissions.Action {
		return permissions.Action{UID: uid, DisplayName: name, PluginName: "content-manager", Section: permissions.SectionContentTypes, Subjects: subjects, ApplyToProperties: props}
	}
	s.actions.Register(
		ct("explorer.create", "Create", "fields", "locales"),
		ct("explorer.read", "Read", "fields", "locales"),
		ct("explorer.update", "Update", "fields", "locales"),
		ct("explorer.delete", "Delete", "locales"),
		ct("explorer.publish", "Publish", "locales"),
	)
	plugin := func(plugin, uid, name, category string) permissions.Action {
		return permissions.Action{UID: uid, DisplayName: name, PluginName: plugin, Section: pluginsSection, Category: category}
	}
	s.actions.Register(
		plugin("content-manager", "single-types.configure-view", "Configure view", "single types"),
		plugin("content-manager", "collection-types.configure-view", "Configure view", "collection types"),
		plugin("content-manager", "components.configure-layout", "Configure Layout", "components"),
		plugin("content-type-builder", "read", "Read", "content type builder"),
		plugin("upload", "read", "Access the Media Library", "media library"),
		plugin("upload", "configure-view-settings", "Configure view", "media library"),
		plugin("upload", "assets.create", "Create (upload)", "media library"),
		plugin("upload", "assets.update", "Update (crop, details, replace) + delete", "media library"),
		plugin("upload", "assets.download", "Download", "media library"),
		plugin("upload", "assets.copy-link", "Copy link", "media library"),
	)
	setting := func(plugin, uid, name, category string) permissions.Action {
		return permissions.Action{UID: uid, DisplayName: name, PluginName: plugin, Section: settingsAdmin, Category: category}
	}
	for _, a := range []string{"create", "read", "update", "delete"} {
		name := strings.ToUpper(a[:1]) + a[1:]
		s.actions.Register(
			setting("admin", "webhooks."+a, name, "webhooks"),
			setting("admin", "users."+a, name, "users and roles"),
			setting("admin", "roles."+a, name, "users and roles"),
			setting("admin", "api-tokens."+a, name, "api tokens"),
			setting("i18n", "locale."+a, name, "Internationalization"),
			setting("users-permissions", "roles."+a, name, "roles"),
		)
	}
	s.actions.Register(
		setting("admin", "api-tokens.access", "Access the API tokens settings page", "api tokens"),
		setting("admin", "api-tokens.regenerate", "Regenerate", "api tokens"),
		setting("admin", "project-settings.read", "Read the project level settings", "project"),
		setting("admin", "project-settings.update", "Update the project level settings", "project"),
		setting("upload", "settings.read", "Access the Media Library settings page", "media library"),
		setting("email", "settings.read", "Access the Email Settings page", "email"),
		setting("users-permissions", "providers.read", "Read", "providers"),
		setting("users-permissions", "providers.update", "Edit", "providers"),
		setting("users-permissions", "email-templates.read", "Read", "email templates"),
		setting("users-permissions", "email-templates.update", "Edit", "email templates"),
		setting("users-permissions", "advanced-settings.read", "Read", "advanced settings"),
		setting("users-permissions", "advanced-settings.update", "Edit", "advanced settings"),
	)
}

func (s *Server) allPermissions() []Permission {
	var out []Permission
	for _, a := range s.actions.All() {
		if len(a.Subjects) == 0 {
			out = append(out, Permission{Action: a.ActionID(), Properties: map[string]any{}, Conditions: []string{}})
			continue
		}
		for _, subj := range a.Subjects {
			out = append(out, Permission{Action: a.ActionID(), Subject: &subj, Properties: map[string]any{}, Conditions: []string{}})
		}
	}
	return out
}

// seedRoles creates Super Admin, Editor and Author when no role exists.
// Editor and Author get the Content Manager and Media Library permissions,
// Author on its own entries only and without publish.
func (s *Server) seedRoles(ctx context.Context) error {
	n, err := s.e.Count(ctx, roleUID, query.Params{})
	if err != nil || n > 0 {
		return err
	}
	roles := []struct{ name, code, desc string }{
		{"Super Admin", SuperAdminCode, "Super Admins can access and manage all features and settings."},
		{"Editor", editorCode, "Editors can manage and publish contents including those of other users."},
		{"Author", authorCode, "Authors can manage the content they have created."},
	}
	ids := map[string]any{}
	for _, r := range roles {
		row, err := s.e.Create(ctx, roleUID, query.Data{"name": r.name, "code": r.code, "description": r.desc}, query.Params{})
		if err != nil {
			return err
		}
		ids[r.code] = get(row, "id")
	}
	var batch []query.Data
	add := func(role, action string, subject any, conditions []string) {
		conds := make([]any, len(conditions))
		for i, c := range conditions {
			conds[i] = c
		}
		batch = append(batch, query.Data{"action": action, "actionParameters": map[string]any{}, "subject": subject, "properties": map[string]any{}, "conditions": conds, "role": ids[role]})
	}
	for _, subj := range s.ContentManagerTypes() {
		if strings.HasPrefix(subj, "plugin::") {
			continue
		}
		for _, a := range []string{"create", "read", "update", "delete", "publish"} {
			add(editorCode, explorer+a, subj, []string{})
			if a != "publish" {
				add(authorCode, explorer+a, subj, []string{creatorFilter})
			}
		}
	}
	for _, a := range []string{"read", "assets.create", "assets.update", "assets.download", "assets.copy-link"} {
		add(editorCode, "plugin::upload."+a, nil, []string{})
		cond := []string{}
		if a == "assets.update" {
			cond = []string{creatorFilter}
		}
		add(authorCode, "plugin::upload."+a, nil, cond)
	}
	for _, d := range batch {
		if _, err := s.e.Create(ctx, permissionUID, d, query.Params{}); err != nil {
			return err
		}
	}
	return nil
}
