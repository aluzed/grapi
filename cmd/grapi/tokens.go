package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/auth/apitoken"
	"github.com/aluzed/grapi/internal/db/query"
)

const (
	apiTokenUID     = "admin::api-token"
	apiTokenPermUID = "admin::api-token-permission"
	upRoleUID       = "plugin::users-permissions.role"
	upPermissionUID = "plugin::users-permissions.permission"
)

func openEngine(dir, configDir string, stderr io.Writer) (*grapi.Project, *query.Engine, error) {
	p, err := grapi.LoadProject(dir, configDir, stderr)
	if err != nil {
		return nil, nil, err
	}
	return p, &query.Engine{DB: p.DB, MD: p.MD}, nil
}

// apiTokenCreate is `grapi api-token:create`: a content API token, shown once.
func apiTokenCreate(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("api-token:create")
	name := fs.String("name", "", "token name (required, unique)")
	desc := fs.String("description", "", "description")
	typ := fs.String("type", "read-only", "read-only, full-access or custom")
	perms := fs.String("permissions", "", "custom tokens: comma separated actions (api::article.article.find,...)")
	days := fs.Int("lifespan", 0, "days: 7, 30 or 90 (unlimited when 0)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	switch *typ {
	case "read-only", "full-access", "custom":
	default:
		return fmt.Errorf("--type must be read-only, full-access or custom")
	}
	if !apitoken.Lifespans[*days] {
		return fmt.Errorf("--lifespan must be 7, 30 or 90")
	}
	var actions []string
	for _, a := range strings.Split(*perms, ",") {
		if a = strings.TrimSpace(a); a != "" {
			actions = append(actions, a)
		}
	}
	if *typ == "custom" && len(actions) == 0 {
		return errors.New("a custom token needs --permissions")
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	salt, _ := p.Conf.String("admin.apiToken.salt")
	if salt == "" {
		salt = p.Conf.Env["API_TOKEN_SALT"]
	}
	if salt == "" {
		return errors.New("admin.apiToken.salt (API_TOKEN_SALT) is not set")
	}
	ctx := context.Background()
	if n, err := e.Count(ctx, apiTokenUID, query.Params{Where: map[string]any{"name": *name}}); err != nil || n > 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("a token named %q already exists", *name)
	}
	accessKey, keys, err := newAccessKey(p, salt)
	if err != nil {
		return err
	}
	data := query.Data{"name": *name, "type": *typ, "kind": "content-api"}
	for k, v := range keys {
		data[k] = v
	}
	if *desc != "" {
		data["description"] = *desc
	}
	if *days > 0 {
		life := time.Duration(*days) * apitoken.Day
		data["lifespan"] = strconv.FormatInt(life.Milliseconds(), 10)
		data["expiresAt"] = time.Now().Add(life)
	}
	row, err := e.Create(ctx, apiTokenUID, data, query.Params{})
	if err != nil {
		return err
	}
	id, _ := row.Get("id")
	for _, a := range actions {
		if *typ != "custom" {
			break
		}
		if _, err := e.Create(ctx, apiTokenPermUID, query.Data{"action": a, "token": id}, query.Params{}); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "Token %q created (id %v). Copy it now, it is not shown again:\n%s\n", *name, id, accessKey)
	return nil
}

// apiTokenList is `grapi api-token:list`.
func apiTokenList(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("api-token:list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	rows, err := e.FindMany(context.Background(), apiTokenUID, query.Params{OrderBy: map[string]any{"id": "asc"}})
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tKIND\tTYPE\tEXPIRES\tLAST USED")
	for _, r := range rows {
		get := func(k string) any {
			v, _ := r.Get(k)
			if v == nil {
				return "-"
			}
			return v
		}
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\n", get("id"), get("name"), get("kind"), get("type"), get("expiresAt"), get("lastUsedAt"))
	}
	return w.Flush()
}

// newAccessKey is the api-token service's key, see apitoken.New.
func newAccessKey(p *grapi.Project, salt string) (string, query.Data, error) {
	secret, _ := p.Conf.String("admin.secrets.encryptionKey")
	key, cols, err := apitoken.New(salt, secret)
	return key, query.Data(cols), err
}

// apiTokenRegenerate is `grapi api-token:regenerate --id N`: a new key for
// the token, the old one stops working (the admin's regenerate).
func apiTokenRegenerate(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("api-token:regenerate")
	id := fs.Int("id", 0, "token id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("--id is required")
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	salt, _ := p.Conf.String("admin.apiToken.salt")
	if salt == "" {
		salt = p.Conf.Env["API_TOKEN_SALT"]
	}
	if salt == "" {
		return errors.New("admin.apiToken.salt (API_TOKEN_SALT) is not set")
	}
	accessKey, keys, err := newAccessKey(p, salt)
	if err != nil {
		return err
	}
	row, err := e.Update(context.Background(), apiTokenUID, map[string]any{"id": *id}, keys, query.Params{})
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("no token %d", *id)
	}
	fmt.Fprintf(stdout, "Token %d regenerated. Copy it now, it is not shown again:\n%s\n", *id, accessKey)
	return nil
}

func apiTokenDelete(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("api-token:delete")
	id := fs.Int("id", 0, "token id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id <= 0 {
		return errors.New("--id is required")
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	ctx := context.Background()
	if _, err := e.DeleteMany(ctx, apiTokenPermUID, query.Params{Where: map[string]any{"token": map[string]any{"id": *id}}}); err != nil {
		return err
	}
	row, err := e.Delete(ctx, apiTokenUID, map[string]any{"id": *id}, query.Params{})
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("no token %d", *id)
	}
	fmt.Fprintf(stdout, "Token %d deleted.\n", *id)
	return nil
}

// permissions is `grapi permissions:list|grant|revoke <role type> [actions...]`
// on users-permissions roles (public, authenticated...).
func permissions(cmd string, args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags(cmd)
	var rest []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		rest, args = append(rest, args[0]), args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest = append(rest, fs.Args()...)
	if len(rest) == 0 {
		return fmt.Errorf("usage: grapi %s <role type> [actions...]", cmd)
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	ctx := context.Background()
	role, err := e.FindOne(ctx, upRoleUID, query.Params{Where: map[string]any{"type": rest[0]}})
	if err != nil {
		return err
	}
	if role == nil {
		return fmt.Errorf("no users-permissions role of type %q", rest[0])
	}
	roleID, _ := role.Get("id")
	perms, err := e.FindMany(ctx, upPermissionUID, query.Params{Where: map[string]any{"role": map[string]any{"id": roleID}}})
	if err != nil {
		return err
	}
	have := map[string]any{}
	for _, r := range perms {
		a, _ := r.Get("action")
		id, _ := r.Get("id")
		have[fmt.Sprint(a)] = id
	}
	switch cmd {
	case "permissions:list":
		names := make([]string, 0, len(have))
		for a := range have {
			names = append(names, a)
		}
		sort.Strings(names)
		for _, a := range names {
			fmt.Fprintln(stdout, a)
		}
	case "permissions:grant":
		for _, a := range rest[1:] {
			if _, ok := have[a]; ok {
				continue
			}
			if _, err := e.Create(ctx, upPermissionUID, query.Data{"action": a, "role": roleID}, query.Params{}); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "granted %s to %s\n", a, rest[0])
		}
	case "permissions:revoke":
		for _, a := range rest[1:] {
			id, ok := have[a]
			if !ok {
				continue
			}
			if _, err := e.Delete(ctx, upPermissionUID, map[string]any{"id": id}, query.Params{}); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "revoked %s from %s\n", a, rest[0])
		}
	}
	return nil
}
