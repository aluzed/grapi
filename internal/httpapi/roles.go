package httpapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// The content API actions of the built-in plugins' controllers, as
// users-permissions' getActions lists them.
//
//go:embed up_actions.json
var pluginActionsJSON []byte

const permissionUID = "plugin::users-permissions.permission"

var errRoleNotFound = newError(http.StatusNotFound, "NotFoundError", "Role not found")

func (s *Server) addRoleRoutes() {
	base := strings.TrimPrefix(s.cfg.Prefix, "/")
	up := "plugin::users-permissions."
	s.routes = append(s.routes,
		route{"GET", []string{base, "users-permissions", "permissions"}, up + "permissions", "getPermissions", s.permissionsTree, "", false, false, nil},
		route{"GET", []string{base, "users-permissions", "roles", ":id"}, up + "role", "findOne", s.roleFindOne, "", false, false, nil},
		route{"GET", []string{base, "users-permissions", "roles"}, up + "role", "find", s.roleFind, "", false, false, nil},
		route{"POST", []string{base, "users-permissions", "roles"}, up + "role", "createRole", s.roleCreate, "", false, false, nil},
		route{"PUT", []string{base, "users-permissions", "roles", ":role"}, up + "role", "updateRole", s.roleUpdate, "", false, false, nil},
		route{"DELETE", []string{base, "users-permissions", "roles", ":role"}, up + "role", "deleteRole", s.roleDelete, "", false, false, nil},
	)
}

type actionTree map[string]map[string]map[string]map[string]map[string]any

// actions is getActions: every content API action, disabled.
func (s *Server) actions() actionTree {
	tree := actionTree{}
	add := func(group, controller, action string) {
		if tree[group] == nil {
			tree[group] = map[string]map[string]map[string]map[string]any{"controllers": {}}
		}
		c := tree[group]["controllers"]
		if c[controller] == nil {
			c[controller] = map[string]map[string]any{}
		}
		c[controller][action] = map[string]any{"enabled": false, "policy": ""}
	}
	for uid, ct := range s.set.ContentTypes.All() {
		u := schema.UID(uid)
		if u.Namespace() != schema.NamespaceAPI {
			continue
		}
		_, controller, _ := strings.Cut(uid, ".")
		actions := []string{"find", "findOne", "create", "update", "delete"}
		if ct.IsSingleType() {
			actions = []string{"find", "update", "delete"}
		}
		for _, a := range actions {
			add("api::"+u.Origin(), controller, a)
		}
	}
	// Go routes of the project carry their action in their scope.
	for _, rt := range s.routes {
		if strings.HasPrefix(rt.scope, "api::") {
			if parts := strings.SplitN(rt.scope, ".", 3); len(parts) == 3 {
				add(parts[0], parts[1], parts[2])
			}
		}
	}
	var plugins actionTree
	_ = json.Unmarshal(pluginActionsJSON, &plugins)
	for group, g := range plugins {
		for controller, actions := range g["controllers"] {
			for a := range actions {
				add(group, controller, a)
			}
		}
	}
	return tree
}

func (s *Server) permissionsTree(w http.ResponseWriter, _ *http.Request, _ *requestContext) error {
	writeJSON(w, http.StatusOK, map[string]any{"permissions": s.actions()})
	return nil
}

// roleOutput is the role sanitized like sanitizeLocalizationFields.
func roleOutput(row query.Row) map[string]any {
	out := map[string]any{}
	for k, v := range row.All() {
		if k != schema.AttrLocale && k != schema.AttrLocalizations && k != "users" {
			out[k] = v
		}
	}
	return out
}

func (s *Server) roleWithPermissions(ctx context.Context, id any) (query.Row, error) {
	return s.docs.E.FindOne(ctx, roleUID, query.Params{Where: map[string]any{"id": id}, Populate: map[string]any{"permissions": true}})
}

func permissionRows(role query.Row) []query.Row {
	v, _ := role.Get("permissions")
	return query.Rows(v)
}

func (s *Server) roleFindOne(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	role, err := s.roleWithPermissions(r.Context(), rc.params["id"])
	if err != nil {
		return err
	}
	if role == nil {
		return errRoleNotFound
	}
	tree := s.actions()
	for _, p := range permissionRows(role) {
		action, _ := p.Get("action")
		parts := strings.SplitN(fmt.Sprint(action), ".", 3)
		if len(parts) != 3 {
			continue
		}
		if tree[parts[0]] == nil {
			tree[parts[0]] = map[string]map[string]map[string]map[string]any{"controllers": {}}
		}
		c := tree[parts[0]]["controllers"]
		if c[parts[1]] == nil {
			c[parts[1]] = map[string]map[string]any{}
		}
		c[parts[1]][parts[2]] = map[string]any{"enabled": true, "policy": ""}
	}
	out := roleOutput(role)
	out["permissions"] = tree
	writeJSON(w, http.StatusOK, map[string]any{"role": out})
	return nil
}

func (s *Server) roleFind(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	ctx := r.Context()
	// Strapi passes `sort` to db.query, which ignores it: no ORDER BY.
	roles, err := s.docs.E.FindMany(ctx, roleUID, query.Params{})
	if err != nil {
		return err
	}
	out := make([]any, 0, len(roles))
	for _, role := range roles {
		id, _ := role.Get("id")
		n, err := s.docs.E.Count(ctx, userUID, query.Params{Where: map[string]any{"role": map[string]any{"id": id}}})
		if err != nil {
			return err
		}
		o := roleOutput(role)
		o["nb_users"] = n
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": out})
	return nil
}

// enabledActions flattens a permissions tree to the enabled action ids.
func enabledActions(permissions any) []string {
	var out []string
	groups, _ := permissions.(map[string]any)
	names := func(m map[string]any) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	for _, group := range names(groups) {
		g, _ := groups[group].(map[string]any)
		controllers, _ := g["controllers"].(map[string]any)
		for _, controller := range names(controllers) {
			actions, _ := controllers[controller].(map[string]any)
			for _, action := range names(actions) {
				a, _ := actions[action].(map[string]any)
				if a["enabled"] == true {
					out = append(out, group+"."+controller+"."+action)
				}
			}
		}
	}
	return out
}

func (s *Server) roleBody(r *http.Request) (map[string]any, error) {
	body, _, err := readBody(r)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, validationError("Request body cannot be empty", nil)
	}
	return body, nil
}

func (s *Server) roleCreate(w http.ResponseWriter, r *http.Request, _ *requestContext) error {
	ctx := r.Context()
	body, err := s.roleBody(r)
	if err != nil {
		return err
	}
	if t, _ := body["type"].(string); t == "" {
		name, _ := body["name"].(string)
		body["type"] = schema.SnakeCase(strings.ToLower(name))
	}
	data := query.Data{}
	attrs := s.attrsOf(roleUID)
	for k, v := range body {
		if _, ok := attrs.Get(k); ok && k != "users" && k != "permissions" {
			data[k] = v
		}
	}
	role, err := s.docs.E.Create(ctx, roleUID, data, query.Params{})
	if err != nil {
		return err
	}
	id, _ := role.Get("id")
	for _, action := range enabledActions(body["permissions"]) {
		if _, err := s.docs.E.Create(ctx, permissionUID, query.Data{"action": action, "role": id}, query.Params{}); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

func (s *Server) roleUpdate(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	body, err := s.roleBody(r)
	if err != nil {
		return err
	}
	role, err := s.roleWithPermissions(ctx, rc.params["role"])
	if err != nil {
		return err
	}
	if role == nil {
		return errRoleNotFound
	}
	id, _ := role.Get("id")
	data := query.Data{}
	for _, k := range []string{"name", "description"} {
		if v, ok := body[k]; ok {
			data[k] = v
		}
	}
	if _, err := s.docs.E.Update(ctx, roleUID, map[string]any{"id": id}, data, query.Params{}); err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, a := range enabledActions(body["permissions"]) {
		wanted[a] = true
	}
	have := map[string]bool{}
	for _, p := range permissionRows(role) {
		action, _ := p.Get("action")
		a := fmt.Sprint(action)
		have[a] = true
		if !wanted[a] {
			pid, _ := p.Get("id")
			if _, err := s.docs.E.Delete(ctx, permissionUID, map[string]any{"id": pid}, query.Params{}); err != nil {
				return err
			}
		}
	}
	for _, a := range enabledActions(body["permissions"]) {
		if !have[a] {
			if _, err := s.docs.E.Create(ctx, permissionUID, query.Data{"action": a, "role": id}, query.Params{}); err != nil {
				return err
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}

func (s *Server) roleDelete(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	roleID := rc.params["role"]
	public, err := s.docs.E.FindOne(ctx, roleUID, query.Params{Where: map[string]any{"type": "public"}})
	if err != nil {
		return err
	}
	var publicID any
	if public != nil {
		publicID, _ = public.Get("id")
	}
	if fmt.Sprint(publicID) == roleID {
		return appError("Cannot delete public role")
	}
	role, err := s.docs.E.FindOne(ctx, roleUID, query.Params{Where: map[string]any{"id": roleID}, Populate: map[string]any{"users": true, "permissions": true}})
	if err != nil {
		return err
	}
	if role == nil {
		return errRoleNotFound
	}
	users, _ := role.Get("users")
	for _, u := range query.Rows(users) {
		uid, _ := u.Get("id")
		if _, err := s.docs.E.Update(ctx, userUID, map[string]any{"id": uid}, query.Data{"role": publicID}, query.Params{}); err != nil {
			return err
		}
	}
	for _, p := range permissionRows(role) {
		pid, _ := p.Get("id")
		if _, err := s.docs.E.Delete(ctx, permissionUID, map[string]any{"id": pid}, query.Params{}); err != nil {
			return err
		}
	}
	id, _ := role.Get("id")
	if _, err := s.docs.E.Delete(ctx, roleUID, map[string]any{"id": id}, query.Params{}); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	return nil
}
