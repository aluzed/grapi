package main

import (
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/aluzed/grapi"
)

// fixtureHooks are the fixtures' project code ported to Go, by fixture name.
// A private fixture registers its own from a zz_private_*.go file that
// compat/scripts/bench.sh copies here (GRAPI_PRIVATE_COMPAT).
var fixtureHooks = map[string]func(*grapi.App){}

// hookedApp sets up a full App on the bench database with a fixture's hooks,
// as its framework binary would.
func hookedApp(fixture, configDir, client, dsn string, register func(*grapi.App)) (*grapi.App, func(), error) {
	env := map[string]string{"DATABASE_CLIENT": client, "DATABASE_FILENAME": dsn}
	if client == "postgres" {
		// The bench's database.yaml reads the parts, not DATABASE_URL.
		u, err := url.Parse(dsn)
		if err != nil {
			return nil, nil, err
		}
		pw, _ := u.User.Password()
		env = map[string]string{
			"DATABASE_CLIENT": client, "DATABASE_HOST": u.Hostname(), "DATABASE_PORT": u.Port(),
			"DATABASE_NAME": strings.TrimPrefix(u.Path, "/"), "DATABASE_USERNAME": u.User.Username(), "DATABASE_PASSWORD": pw,
		}
	}
	for k, v := range env {
		if err := os.Setenv(k, v); err != nil {
			return nil, nil, err
		}
	}
	app := grapi.New(grapi.Options{Dir: fixture, ConfigDir: configDir, Stderr: io.Discard})
	register(app)
	srv, err := app.Setup()
	if err != nil {
		return nil, nil, err
	}
	return app, func() { _ = srv.Close() }, nil
}
