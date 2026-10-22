package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
)

func openSQLite(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "test.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(context.Background(), "create table items (id integer primary key autoincrement, name varchar(255))"); err != nil {
		t.Fatal(err)
	}
	return db
}

func count(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), "select count(*) from items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTxCommitRollbackAndNesting(t *testing.T) {
	db := openSQLite(t)
	ctx := context.Background()

	if err := db.Tx(ctx, func(ctx context.Context) error {
		_, err := db.Exec(ctx, "insert into items (name) values (?)", "a")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	committed, rolledBack := 0, 0
	err := db.Tx(ctx, func(outer context.Context) error {
		if _, err := db.Exec(outer, "insert into items (name) values (?)", "b"); err != nil {
			return err
		}
		return db.Tx(outer, func(inner context.Context) error {
			if !db.InTx(inner) {
				t.Error("inner call must join the outer transaction")
			}
			OnCommit(inner, func() { committed++ })
			OnRollback(inner, func() { rolledBack++ })
			if _, err := db.Exec(inner, "insert into items (name) values (?)", "c"); err != nil {
				return err
			}
			var n int
			if err := db.QueryRow(inner, "select count(*) from items").Scan(&n); err != nil || n != 3 {
				t.Errorf("inner transaction must see uncommitted rows: %d %v", n, err)
			}
			return boom
		})
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, db); n != 1 {
		t.Errorf("rollback must undo both nested inserts, %d rows", n)
	}
	if committed != 0 || rolledBack != 1 {
		t.Errorf("callbacks: committed=%d rolledBack=%d", committed, rolledBack)
	}

	_ = db.Tx(ctx, func(outer context.Context) error {
		return db.Tx(outer, func(inner context.Context) error {
			OnCommit(inner, func() { committed++ })
			return nil
		})
	})
	if committed != 1 {
		t.Errorf("OnCommit must run once after the outer commit, ran %d", committed)
	}
}

func TestSQLiteConcurrentWrites(t *testing.T) {
	db := openSQLite(t)
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.Tx(context.Background(), func(ctx context.Context) error {
				_, err := db.Exec(ctx, "insert into items (name) values (?)", fmt.Sprint(i))
				return err
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := count(t, db); n != 100 {
		t.Errorf("%d rows, want 100", n)
	}
}

type goldenSchema struct {
	Tables []struct {
		Name    string   `json:"name"`
		Columns []Column `json:"columns"`
	} `json:"tables"`
}

func loadGoldenSchema(t *testing.T, path string) goldenSchema {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no golden schema: %v", err)
	}
	var s goldenSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Column types must be the ones knex wrote in the reference SQLite database.
func TestSQLiteColumnTypesMatchStrapi(t *testing.T) {
	dirs, _ := filepath.Glob("../../compat/golden/*-sqlite/schema")
	if len(dirs) == 0 {
		t.Skip("no sqlite golden schema")
	}
	colDef := regexp.MustCompile("`([^`]+)` ([a-z]+(?:\\(\\d+\\))?)")
	compared := 0
	defer func() {
		if compared < 100 {
			t.Errorf("only %d columns compared", compared)
		}
		t.Logf("%d columns compared", compared)
	}()
	for _, dir := range dirs {
		schema := loadGoldenSchema(t, filepath.Join(dir, "database-schema.json"))
		raw, err := os.ReadFile(filepath.Join(dir, "physical-sqlite-master.json"))
		if err != nil {
			t.Fatal(err)
		}
		var master []struct{ Type, Name, SQL string }
		_ = json.Unmarshal(raw, &master)
		physical := map[string]map[string]string{}
		for _, m := range master {
			if m.Type != "table" {
				continue
			}
			physical[m.Name] = map[string]string{}
			for _, def := range colDef.FindAllStringSubmatch(m.SQL, -1) {
				physical[m.Name][def[1]] = def[2]
			}
		}
		for _, tbl := range schema.Tables {
			for _, c := range tbl.Columns {
				want, ok := physical[tbl.Name][c.Name]
				if !ok {
					continue
				}
				compared++
				got, err := sqlite{}.ColumnType(c)
				if err != nil || got != want {
					t.Errorf("%s %s.%s (%s %v) = %q, %v; knex wrote %q", filepath.Base(filepath.Dir(dir)), tbl.Name, c.Name, c.Type, c.Args, got, err, want)
				}
			}
		}
	}
}
