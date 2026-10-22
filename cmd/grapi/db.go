package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/schemasync"
)

// dbSync is `grapi db:sync --dry-run`: what the start-up sync would do on
// this database, without writing.
func dbSync(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("db:sync")
	dry := fs.Bool("dry-run", false, "print the changes without applying them")
	printSchema := fs.Bool("print-schema", false, "print the schema grapi computes, as Strapi stores it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *printSchema {
		p, err := grapi.LoadProject(*dir, *configDir, stderr)
		if err != nil {
			return err
		}
		defer func() { _ = p.DB.Close() }()
		b, err := metadata.Marshal(p.Desired)
		if err != nil {
			return err
		}
		_, err = stdout.Write(append(b, '\n'))
		return err
	}
	if !*dry {
		return fmt.Errorf("db:sync only runs with --dry-run; grapi start applies the sync")
	}
	p, err := grapi.LoadProject(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	hashMatches, changes, err := schemasync.Plan(context.Background(), p.DB, p.Desired, schemasync.Options{})
	if err != nil {
		return err
	}
	status := changes.Status
	if hashMatches {
		fmt.Fprintln(stdout, "stored schema hash: matches (grapi start skips the sync)")
	} else {
		fmt.Fprintln(stdout, "stored schema hash: differs (grapi start runs the sync)")
	}
	fmt.Fprintln(stdout, string(status))
	for _, t := range changes.Added {
		fmt.Fprintln(stdout, "  add table", t.Name)
	}
	for _, t := range changes.Removed {
		fmt.Fprintln(stdout, "  drop table", t.Name)
	}
	for _, t := range changes.Updated {
		for _, c := range t.Columns.Added {
			fmt.Fprintf(stdout, "  %s: add column %s\n", t.Name, c.Name)
		}
		for _, c := range t.Columns.Updated {
			fmt.Fprintf(stdout, "  %s: alter column %s\n", t.Name, c.Name)
		}
		for _, c := range t.Columns.Removed {
			fmt.Fprintf(stdout, "  %s: drop column %s\n", t.Name, c.Name)
		}
		for _, i := range t.Indexes.Added {
			fmt.Fprintf(stdout, "  %s: add index %s\n", t.Name, i.Name)
		}
		for _, i := range t.Indexes.Updated {
			fmt.Fprintf(stdout, "  %s: alter index %s\n", t.Name, i.Name)
		}
		for _, i := range t.Indexes.Removed {
			fmt.Fprintf(stdout, "  %s: drop index %s\n", t.Name, i.Name)
		}
		for _, f := range t.ForeignKeys.Added {
			fmt.Fprintf(stdout, "  %s: add foreign key %s\n", t.Name, f.Name)
		}
		for _, f := range t.ForeignKeys.Updated {
			fmt.Fprintf(stdout, "  %s: alter foreign key %s\n", t.Name, f.Name)
		}
		for _, f := range t.ForeignKeys.Removed {
			fmt.Fprintf(stdout, "  %s: drop foreign key %s\n", t.Name, f.Name)
		}
	}
	return nil
}

var cuid2 = regexp.MustCompile(`^[a-z0-9]{24}$`)

// dbCheck is `grapi db:check`: the consistency report of DB-011, read only.
func dbCheck(args []string, stdout, stderr io.Writer) error {
	fs, dir, configDir := projectFlags("db:check")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := grapi.LoadProject(*dir, *configDir, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = p.DB.Close() }()
	ctx := context.Background()
	var problems []string
	report := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	_, changes, err := schemasync.Plan(ctx, p.DB, p.Desired, schemasync.Options{})
	if err != nil {
		return err
	}
	for _, t := range changes.Added {
		report("missing table %s", t.Name)
	}
	for _, t := range changes.Updated {
		for _, c := range t.Columns.Added {
			report("%s: missing column %s", t.Name, c.Name)
		}
		for _, c := range t.Columns.Updated {
			report("%s: column %s differs from the schema", t.Name, c.Name)
		}
	}

	q := p.DB.Dialect.Quote
	count := func(sql string, args ...any) (int64, error) {
		var n int64
		err := p.DB.QueryRow(ctx, sql, args...).Scan(&n)
		return n, err
	}
	tables := map[string]*metadata.Table{}
	for _, t := range p.Desired.Tables {
		tables[t.Name] = t
	}
	names := make([]string, 0, len(tables))
	for n := range tables {
		names = append(names, n)
	}
	sort.Strings(names)
	added := map[string]bool{}
	for _, t := range changes.Added {
		added[t.Name] = true
	}
	for _, name := range names {
		t := tables[name]
		if added[name] {
			continue
		}
		hasDoc := false
		for _, c := range t.Columns {
			hasDoc = hasDoc || c.Name == "document_id"
		}
		if hasDoc {
			rows, err := p.DB.Query(ctx, `select `+q("document_id")+` from `+q(name))
			if err != nil {
				return err
			}
			bad := 0
			for rows.Next() {
				var doc *string
				if err := rows.Scan(&doc); err != nil {
					_ = rows.Close()
					return err
				}
				if doc == nil || !cuid2.MatchString(*doc) {
					bad++
				}
			}
			_ = rows.Close()
			if bad > 0 {
				report("%s: %d rows without a cuid2 document_id", name, bad)
			}
		}
		if !strings.HasSuffix(name, "_lnk") && !strings.HasSuffix(name, "_mph") && !strings.HasSuffix(name, "_cmps") {
			continue
		}
		for _, fk := range t.ForeignKeys {
			if len(fk.Columns) != 1 || len(fk.ReferencedColumns) != 1 {
				continue
			}
			col, ref, refCol := fk.Columns[0], fk.ReferencedTable, fk.ReferencedColumns[0]
			n, err := count(`select count(*) from ` + q(name) + ` l where l.` + q(col) + ` is not null and not exists (select 1 from ` + q(ref) + ` r where r.` + q(refCol) + ` = l.` + q(col) + `)`)
			if err != nil {
				return err
			}
			if n > 0 {
				report("%s: %d rows point to a missing %s.%s", name, n, ref, refCol)
			}
		}
	}
	for _, table := range []string{"up_users", "admin_users"} {
		if _, ok := tables[table]; !ok || added[table] {
			continue
		}
		n, err := count(`select count(*) from ` + q(table) + ` where ` + q("password") + ` is not null and ` + q("password") + ` not like '$2a$%' and ` + q("password") + ` not like '$2b$%' and ` + q("password") + ` not like '$2y$%'`)
		if err != nil {
			return err
		}
		if n > 0 {
			report("%s: %d passwords bcrypt cannot read", table, n)
		}
	}
	if len(problems) == 0 {
		fmt.Fprintln(stdout, "OK")
		return nil
	}
	for _, pr := range problems {
		fmt.Fprintln(stdout, pr)
	}
	return fmt.Errorf("%d problems", len(problems))
}
