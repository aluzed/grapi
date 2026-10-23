package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/aluzed/grapi/internal/adminapi"
	"github.com/aluzed/grapi/internal/db/query"
)

const (
	adminUserUID = "admin::user"
	adminRoleUID = "admin::role"
	sessionUID   = "admin::session"
)

// readPassword takes --password, or the first line of stdin with
// --password-stdin, so that it stays out of the shell history.
func readPassword(flag string, fromStdin bool, stdin io.Reader) (string, error) {
	if !fromStdin {
		return flag, nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// adminCreateUser is `grapi admin:create-user`: a Super Admin, active.
func adminCreateUser(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("admin:create-user")
	email := fs.String("email", "", "e-mail (required)")
	password := fs.String("password", "", "password (prefer --password-stdin)")
	fromStdin := fs.Bool("password-stdin", false, "read the password from the first line of stdin")
	firstname := fs.String("firstname", "", "first name (required)")
	lastname := fs.String("lastname", "", "last name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pw, err := readPassword(*password, *fromStdin, os.Stdin)
	if err != nil {
		return err
	}
	*email = strings.ToLower(strings.TrimSpace(*email))
	if *email == "" || *firstname == "" {
		return errors.New("--email and --firstname are required")
	}
	if err := adminapi.ValidatePassword(pw); err != nil {
		return err
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	ctx := context.Background()
	if n, err := e.Count(ctx, adminUserUID, query.Params{Where: map[string]any{"email": *email}}); err != nil {
		return err
	} else if n > 0 {
		return fmt.Errorf("an admin with the e-mail %s already exists", *email)
	}
	role, err := e.FindOne(ctx, adminRoleUID, query.Params{Where: map[string]any{"code": adminapi.SuperAdminCode}})
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("the Super Admin role does not exist yet: start grapi once")
	}
	hash, err := adminapi.HashPassword(pw)
	if err != nil {
		return err
	}
	roleID, _ := role.Get("id")
	data := query.Data{"email": *email, "firstname": *firstname, "password": hash, "isActive": true, "blocked": false, "roles": []any{roleID}}
	if *lastname != "" {
		data["lastname"] = *lastname
	}
	row, err := e.Create(ctx, adminUserUID, data, query.Params{})
	if err != nil {
		return err
	}
	id, _ := row.Get("id")
	fmt.Fprintf(stdout, "Super Admin %s created (id %v).\n", *email, id)
	return nil
}

// adminResetPassword is `grapi admin:reset-user-password`: only the
// password of that admin changes, and their sessions end.
func adminResetPassword(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("admin:reset-user-password")
	email := fs.String("email", "", "e-mail (required)")
	password := fs.String("password", "", "new password (prefer --password-stdin)")
	fromStdin := fs.Bool("password-stdin", false, "read the password from the first line of stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pw, err := readPassword(*password, *fromStdin, os.Stdin)
	if err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email is required")
	}
	if err := adminapi.ValidatePassword(pw); err != nil {
		return err
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	ctx := context.Background()
	user, err := e.FindOne(ctx, adminUserUID, query.Params{Where: map[string]any{"email": strings.ToLower(strings.TrimSpace(*email))}})
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("no admin with the e-mail %s", *email)
	}
	hash, err := adminapi.HashPassword(pw)
	if err != nil {
		return err
	}
	id, _ := user.Get("id")
	if _, err := e.Update(ctx, adminUserUID, map[string]any{"id": id}, query.Data{"password": hash}, query.Params{}); err != nil {
		return err
	}
	if _, err := e.DeleteMany(ctx, sessionUID, query.Params{Where: map[string]any{"userId": fmt.Sprint(id), "origin": "admin"}}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Password of %s reset; their sessions were closed.\n", *email)
	return nil
}

// adminListUsers is `grapi admin:list-users`.
func adminListUsers(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("admin:list-users")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, e, err := openEngine(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	rows, err := e.FindMany(context.Background(), adminUserUID, query.Params{OrderBy: map[string]any{"id": "asc"}, Populate: map[string]any{"roles": true}})
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEMAIL\tNAME\tACTIVE\tBLOCKED\tROLES")
	for _, r := range rows {
		get := func(k string) any {
			v, _ := r.Get(k)
			if v == nil {
				return ""
			}
			return v
		}
		var roles []string
		switch list := get("roles").(type) {
		case []query.Row:
			for _, role := range list {
				n, _ := role.Get("name")
				roles = append(roles, fmt.Sprint(n))
			}
		case []any:
			for _, e := range list {
				if role, ok := e.(query.Row); ok {
					n, _ := role.Get("name")
					roles = append(roles, fmt.Sprint(n))
				}
			}
		}
		name := strings.TrimSpace(fmt.Sprint(get("firstname"), " ", get("lastname")))
		fmt.Fprintf(w, "%v\t%v\t%s\t%v\t%v\t%s\n", get("id"), get("email"), name, get("isActive"), get("blocked"), strings.Join(roles, ", "))
	}
	return w.Flush()
}
