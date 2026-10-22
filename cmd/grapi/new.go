package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed template
var templates embed.FS

var projectName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func secret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// envEntry is one .env line; Value is empty in .env.example.
type envEntry struct{ Key, Value string }

func envFile(entries []envEntry, withValues bool) string {
	var b strings.Builder
	for _, e := range entries {
		v := ""
		if withValues {
			v = e.Value
		}
		b.WriteString(e.Key + "=" + v + "\n")
	}
	return b.String()
}

// newProject is `grapi new <name>`.
func newProject(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	dbClient := fs.String("db", "sqlite", "database: sqlite or postgres")
	dbHost := fs.String("db-host", "127.0.0.1", "postgres host")
	dbPort := fs.String("db-port", "5432", "postgres port")
	dbName := fs.String("db-name", "strapi", "postgres database")
	dbUser := fs.String("db-user", "strapi", "postgres user")
	dbPassword := fs.String("db-password", "", "postgres password")
	dbSSL := fs.Bool("db-ssl", false, "postgres over TLS")
	gitInit := fs.Bool("git-init", false, "run git init")
	framework := fs.Bool("framework", false, "add a main.go for the Go framework mode")
	var name string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return errors.New("usage: grapi new <name> [--db sqlite|postgres]")
	}
	if !projectName.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid project name %q: letters, digits, dot, dash and underscore only", name)
	}
	if *dbClient != "sqlite" && *dbClient != "postgres" {
		return fmt.Errorf("--db must be sqlite or postgres, not %q", *dbClient)
	}
	if _, err := os.Stat(name); err == nil { //nolint:gosec // name is checked against projectName
		return fmt.Errorf("%s already exists", name)
	}
	write := func(rel string, content []byte) error {
		p := filepath.Join(name, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { //nolint:gosec // name is checked, rel is ours
			return err
		}
		return os.WriteFile(p, content, 0o600) //nolint:gosec // name is checked, rel is ours
	}
	tpl := func(file string) []byte {
		b, _ := templates.ReadFile("template/" + file)
		return b
	}
	for _, f := range []string{"server", "admin", "api", "middlewares", "plugins"} {
		if err := write("config/"+f+".yaml", tpl(f+".yaml")); err != nil {
			return err
		}
	}
	if err := write("config/database.yaml", tpl("database-"+*dbClient+".yaml")); err != nil {
		return err
	}
	for _, keep := range []string{"src/api/.gitkeep", "src/components/.gitkeep", "src/extensions/.gitkeep", "public/uploads/.gitkeep", ".tmp/.gitkeep"} {
		if err := write(keep, nil); err != nil {
			return err
		}
	}
	if err := write(".gitignore", tpl("gitignore")); err != nil {
		return err
	}
	if *framework {
		if err := write("main.go", tpl("main.go.tmpl")); err != nil {
			return err
		}
	}
	env := []envEntry{
		{"HOST", "0.0.0.0"},
		{"PORT", "1337"},
		{"APP_KEYS", strings.Join([]string{secret(), secret(), secret(), secret()}, ",")},
		{"API_TOKEN_SALT", secret()},
		{"ADMIN_JWT_SECRET", secret()},
		{"TRANSFER_TOKEN_SALT", secret()},
		{"JWT_SECRET", secret()},
		{"ENCRYPTION_KEY", secret()},
	}
	if *dbClient == "sqlite" {
		env = append(env, envEntry{"DATABASE_FILENAME", ".tmp/data.db"})
	} else {
		env = append(env, envEntry{"DATABASE_HOST", *dbHost}, envEntry{"DATABASE_PORT", *dbPort}, envEntry{"DATABASE_NAME", *dbName},
			envEntry{"DATABASE_USERNAME", *dbUser}, envEntry{"DATABASE_PASSWORD", *dbPassword}, envEntry{"DATABASE_SSL", fmt.Sprint(*dbSSL)})
	}
	if err := write(".env", []byte(envFile(env, true))); err != nil {
		return err
	}
	if err := write(".env.example", []byte(envFile(env, false))); err != nil {
		return err
	}
	if *gitInit {
		cmd := exec.Command("git", "init", "-q", name) //nolint:gosec // name is checked against projectName
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git init: %w: %s", err, out)
		}
	}
	fmt.Fprintf(stdout, "Created %s. Next: cd %s && grapi develop\n", name, name)
	return nil
}
