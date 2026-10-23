// Command grapi runs the grapi server and its admin CLI.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: grapi <command>\n\ncommands:\n  new <name>          create a project (--db sqlite|postgres, --framework)\n  develop             serve and reload when src/ or config/ changes (--open: the admin in the browser)\n  start               serve the project in the current directory\n  db:sync --dry-run   print what the schema sync would change\n  db:check            check that the database is consistent\n  health              exit 0 when the local server answers /_health (container healthcheck)\n  report              versions and project facts, without secrets\n  openapi [-o file]   write the content API's OpenAPI document\n  ts:generate-types   write types/generated/*.d.ts for @strapi/strapi (-o dir)\n  generate content-type|component  write a schema (--name, --attr name:type[:required], ...)\n  routes:list         list the content API routes (--details: handler and policies)\n  content-types:list  list the content types (components:list, the components)\n  middlewares:list    list the built-in middlewares (hooks:list, the hooks)\n  configuration:dump  write the plugins' settings of the core store (-f file, -p)\n  configuration:restore  load them back (-f file, -s replace|merge|keep)\n  api-token:create    create a content API token (--name, --type, --permissions, --lifespan)\n  api-token:list      list the API tokens\n  api-token:delete    delete an API token (--id)\n  api-token:regenerate  give an API token a new key (--id)\n  permissions:list|grant|revoke <role> [actions...]   users-permissions role permissions\n  admin:create-user   create a Super Admin (--email, --firstname, --lastname, --password-stdin)\n  admin:reset-user-password  set an admin's password and end their sessions (--email, --password-stdin)\n  admin:list-users    list the admins\n  admin:delete-user   delete an admin and end their sessions (--email)\n  admin:active-user   activate or deactivate an admin (--email, --active=false)\n  admin:block-user    block an admin (--email, --unblock to lift it)\n  version             print the grapi version\n\nflags of every project command: -dir, -config, -env, -log-level")
		return 2
	}
	switch args[0] {
	case "new":
		if err := newProject(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "develop":
		if err := develop(args[1:], stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "report":
		if err := report(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "api-token:create", "api-token:list", "api-token:delete", "api-token:regenerate":
		cmd := map[string]func([]string, io.Writer, io.Writer) error{"api-token:create": apiTokenCreate, "api-token:list": apiTokenList, "api-token:delete": apiTokenDelete, "api-token:regenerate": apiTokenRegenerate}[args[0]]
		if err := cmd(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "admin:create-user", "admin:reset-user-password", "admin:list-users", "admin:delete-user", "admin:active-user", "admin:block-user":
		cmd := map[string]func([]string, io.Writer, io.Writer) error{
			"admin:create-user": adminCreateUser, "admin:reset-user-password": adminResetPassword, "admin:list-users": adminListUsers,
			"admin:delete-user": adminDeleteUser, "admin:active-user": adminActiveUser, "admin:block-user": adminBlockUser,
		}[args[0]]
		if err := cmd(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "permissions:list", "permissions:grant", "permissions:revoke":
		if err := permissions(args[0], args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "start":
		if err := start(args[1:], stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "db:sync", "db:check":
		cmd := map[string]func([]string, io.Writer, io.Writer) error{"db:sync": dbSync, "db:check": dbCheck}[args[0]]
		if err := cmd(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "configuration:dump", "config:dump":
		if err := configurationDump(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "configuration:restore", "config:restore":
		if err := configurationRestore(args[1:], os.Stdin, stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "content-types:list", "components:list":
		if err := listCommand(strings.TrimSuffix(args[0], ":list"), args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "middlewares:list", "hooks:list":
		if err := staticList(strings.TrimSuffix(args[0], ":list"), stdout); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "generate":
		if err := generate(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "routes:list":
		if err := routesList(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "ts:generate-types":
		if err := tsGenerateTypes(args[1:], stdout); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "openapi":
		if err := openAPI(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "health":
		if err := health(); err != nil {
			fmt.Fprintln(stderr, "grapi:", err)
			return 1
		}
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	default:
		fmt.Fprintf(stderr, "grapi: unknown command %q\n", args[0])
		return 2
	}
}
