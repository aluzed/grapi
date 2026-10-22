// Package migrate runs database migrations before the schema sync, with the
// bookkeeping of Strapi 5: user migrations in strapi_migrations, internal ones
// in strapi_migrations_internal, one transaction per migration.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/db"
)

// Tables Strapi records migrations in.
const (
	UserTable     = "strapi_migrations"
	InternalTable = "strapi_migrations_internal"
)

// StrapiInternal are the internal migrations Strapi 5.59 records, in the order
// a fresh database gets them. grapi never runs them: they only matter for a
// database coming from Strapi 4, which grapi refuses.
var StrapiInternal = []string{
	"5.0.0-rename-identifiers-longer-than-max-length",
	"5.0.0-02-created-document-id",
	"5.0.0-03-created-locale",
	"5.0.0-04-created-published-at",
	"5.0.0-05-drop-slug-fields-index",
	"5.0.0-06-add-document-id-indexes",
	"core::5.0.0-discard-drafts",
	"admin::migrate-prefered-language-dk-to-da",
	"upload::unsign-richtext-and-blocks-urls",
}

// Migration is a migration grapi can run: a user SQL file, a Go migration
// registered in framework mode, or an internal grapi migration (named
// "grapi::<version>-<nn>-<name>").
type Migration struct {
	Name string
	Up   func(ctx context.Context, d *db.DB) error
}

// Options of Run.
type Options struct {
	// Dir holds the user migrations (database/migrations), nil for none.
	Dir fs.FS
	// Go are the user migrations registered in code, sorted with the files.
	Go []Migration
	// Internal are grapi's own migrations, run in order after the user ones.
	Internal []Migration
	// RunMigrations is Strapi's settings.runMigrations (true by default):
	// when false, user migrations are not run.
	RunMigrations bool
	Log           *slog.Logger
}

// ErrStrapi4 is returned for a database created by Strapi 4.
var ErrStrapi4 = errors.New("this database was created by Strapi 4 (no document_id column): migrate it with Strapi's v4 to v5 tool first, then open it with grapi")

// Run applies the pending migrations, user ones first like Strapi, and
// reports whether any ran (the schema sync must then do a full diff).
func Run(ctx context.Context, d *db.DB, opts Options) (ran bool, err error) {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if err := checkNotStrapi4(ctx, d); err != nil {
		return false, err
	}
	fresh, err := isFresh(ctx, d)
	if err != nil {
		return false, err
	}
	// Strapi creates the table even when user migrations are disabled.
	if err := ensureTable(ctx, d, UserTable); err != nil {
		return false, err
	}
	if opts.RunMigrations {
		user, err := userMigrations(opts.Dir, opts.Go)
		if err != nil {
			return false, err
		}
		n, err := runPending(ctx, d, UserTable, user, opts.Log)
		if err != nil {
			return false, err
		}
		ran = ran || n > 0
	}
	internal, err := internalMigrations(ctx, d, fresh, opts.Internal)
	if err != nil {
		return ran, err
	}
	n, err := runPending(ctx, d, InternalTable, internal, opts.Log)
	return ran || n > 0, err
}

// internalMigrations returns Strapi's internal migrations followed by grapi's.
// On a database grapi creates, Strapi's are recorded as done without running,
// as they would be on a fresh Strapi database; on an existing one, a missing
// Strapi migration means the database comes from an older Strapi version.
func internalMigrations(ctx context.Context, d *db.DB, fresh bool, own []Migration) ([]Migration, error) {
	if err := ensureTable(ctx, d, InternalTable); err != nil {
		return nil, err
	}
	done, err := Executed(ctx, d, InternalTable)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, name := range StrapiInternal {
		if !slices.Contains(done, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 && !fresh {
		return nil, fmt.Errorf("the database misses Strapi internal migrations %s: open it once with Strapi 5.59 so that they run, then with grapi", strings.Join(missing, ", "))
	}
	out := make([]Migration, 0, len(missing)+len(own))
	for _, name := range missing {
		out = append(out, Migration{Name: name, Up: func(context.Context, *db.DB) error { return nil }})
	}
	return append(out, own...), nil
}

func userMigrations(dir fs.FS, goMigrations []Migration) ([]Migration, error) {
	out := append([]Migration(nil), goMigrations...)
	if dir != nil {
		entries, err := fs.ReadDir(dir, ".")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || e.IsDir() {
				continue
			}
			switch {
			case strings.HasSuffix(name, ".sql"):
				out = append(out, sqlMigration(dir, name))
			case strings.HasSuffix(name, ".js"):
				out = append(out, Migration{Name: name, Up: func(context.Context, *db.DB) error {
					return fmt.Errorf("JavaScript migrations are not supported yet (planned with JS hooks): rewrite %s as a .sql file or a Go migration", name)
				}})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	for i := 1; i < len(out); i++ {
		if out[i].Name == out[i-1].Name {
			return nil, fmt.Errorf("two migrations are named %s", out[i].Name)
		}
	}
	return out, nil
}

// sqlMigration runs a whole file as one statement batch, like Strapi's
// knex.raw on the file content.
func sqlMigration(dir fs.FS, name string) Migration {
	return Migration{Name: name, Up: func(ctx context.Context, d *db.DB) error {
		b, err := fs.ReadFile(dir, path.Clean(name))
		if err != nil {
			return err
		}
		_, err = d.Exec(ctx, string(b))
		return err
	}}
}

func runPending(ctx context.Context, d *db.DB, table string, migrations []Migration, log *slog.Logger) (int, error) {
	if err := ensureTable(ctx, d, table); err != nil {
		return 0, err
	}
	done, err := Executed(ctx, d, table)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range migrations {
		if slices.Contains(done, m.Name) {
			continue
		}
		start := time.Now()
		log.Info("migrating", "name", m.Name)
		err := d.Tx(ctx, func(ctx context.Context) error {
			if err := m.Up(ctx, d); err != nil {
				return err
			}
			return logMigration(ctx, d, table, m.Name)
		})
		if err != nil {
			return n, fmt.Errorf("migration %s (up) failed: %w", m.Name, err)
		}
		log.Info("migrated", "name", m.Name, "durationSeconds", time.Since(start).Seconds())
		n++
	}
	return n, nil
}

// Status lists the migrations of a table with whether they ran.
type Status struct {
	Name     string
	Executed bool
}

// UserStatus returns the user migrations and whether each ran.
func UserStatus(ctx context.Context, d *db.DB, dir fs.FS, goMigrations []Migration) ([]Status, error) {
	all, err := userMigrations(dir, goMigrations)
	if err != nil {
		return nil, err
	}
	if err := ensureTable(ctx, d, UserTable); err != nil {
		return nil, err
	}
	done, err := Executed(ctx, d, UserTable)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(all))
	for _, m := range all {
		out = append(out, Status{Name: m.Name, Executed: slices.Contains(done, m.Name)})
	}
	return out, nil
}
