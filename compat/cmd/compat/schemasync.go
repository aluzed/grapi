package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/migrate"
	"github.com/aluzed/grapi/internal/db/schemasync"
)

// schemaSync runs grapi's startup database steps for a fixture (migrations,
// then schema sync) and prints the sync status, so the bench can check them
// against databases created by Strapi and the reverse.
func schemaSync(args []string) error {
	fs := flag.NewFlagSet("schema-sync", flag.ExitOnError)
	fixture := fs.String("fixture", "", "fixture directory (the one holding src/)")
	client := fs.String("client", "sqlite", "sqlite or postgres")
	dsn := fs.String("db", "", "SQLite file or Postgres connection string")
	_ = fs.Parse(args)
	if *fixture == "" || *dsn == "" {
		return fmt.Errorf("-fixture and -db are required")
	}
	_, md, err := builtin.Load(os.DirFS(*fixture))
	if err != nil {
		return err
	}
	desired, err := md.Schema()
	if err != nil {
		return err
	}
	cfg := db.Config{Client: *client, Filename: *dsn}
	if *client == "postgres" {
		cfg = db.Config{Client: *client, ConnectionString: *dsn}
	}
	ctx := context.Background()
	d, err := db.Open(ctx, cfg, nil)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	var status schemasync.Status
	err = migrate.WithLock(ctx, d, func(ctx context.Context) error {
		ran, err := migrate.Run(ctx, d, migrate.Options{RunMigrations: true})
		if err != nil {
			return err
		}
		status, err = schemasync.Sync(ctx, d, desired, schemasync.Options{MigrationsRan: ran})
		return err
	})
	if err != nil {
		return err
	}
	fmt.Println(status)
	return nil
}
