//go:build integration

package db

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openPostgres(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("GRAPI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("GRAPI_TEST_PG_DSN not set")
	}
	db, err := Open(context.Background(), Config{Client: "postgres", ConnectionString: dsn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Creates every table of the golden schema with ColumnType and compares
// information_schema with what Strapi produced.
func TestPostgresColumnTypesMatchStrapi(t *testing.T) {
	db := openPostgres(t)
	ctx := context.Background()
	dirs, _ := filepath.Glob("../../compat/golden/*-postgres/schema")
	for _, dir := range dirs {
		schema := loadGoldenSchema(t, filepath.Join(dir, "database-schema.json"))
		raw, _ := os.ReadFile(filepath.Join(dir, "physical-columns.json"))
		var want []struct {
			Table     string `json:"table_name"`
			Column    string `json:"column_name"`
			DataType  string `json:"data_type"`
			MaxLength *int   `json:"character_maximum_length"`
			Precision *int   `json:"numeric_precision"`
			Scale     *int   `json:"numeric_scale"`
		}
		_ = json.Unmarshal(raw, &want)
		_, _ = db.Exec(ctx, "drop schema if exists grapi_types cascade")
		if _, err := db.Exec(ctx, "create schema grapi_types"); err != nil {
			t.Fatal(err)
		}
		for _, tbl := range schema.Tables {
			var cols []string
			for _, c := range tbl.Columns {
				typ, err := postgres{}.ColumnType(c)
				if err != nil {
					t.Fatal(err)
				}
				cols = append(cols, postgres{}.Quote(c.Name)+" "+typ)
			}
			q := "create table grapi_types." + postgres{}.Quote(tbl.Name) + " (" + strings.Join(cols, ", ") + ")"
			if _, err := db.Exec(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		rows, err := db.Query(ctx, `select table_name, column_name, data_type, character_maximum_length, numeric_precision, numeric_scale from information_schema.columns where table_schema = 'grapi_types'`)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for rows.Next() {
			var tbl, col, typ string
			var maxLen, prec, scale *int
			if err := rows.Scan(&tbl, &col, &typ, &maxLen, &prec, &scale); err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal([]any{typ, maxLen, prec, scale})
			got[tbl+"."+col] = string(b)
		}
		rows.Close()
		for _, w := range want {
			b, _ := json.Marshal([]any{w.DataType, w.MaxLength, w.Precision, w.Scale})
			if g, ok := got[w.Table+"."+w.Column]; ok && g != string(b) {
				t.Errorf("%s.%s = %s, Strapi has %s", w.Table, w.Column, g, b)
			}
		}
		_, _ = db.Exec(ctx, "drop schema grapi_types cascade")
	}
}
