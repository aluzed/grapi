// Package store is Strapi's core store: JSON values in
// strapi_core_store_settings, keyed "<type>_<name>_<key>".
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/db"
)

// Scope is the part of a key shared by a plugin's or the core's settings.
type Scope struct {
	// Type is "core" or "plugin". Get defaults to core; Set and Delete
	// keep it as given, like Strapi (an empty type gives "undefined_").
	Type        string
	Name        string
	Environment string
	Tag         string
}

// Store reads and writes the core store.
type Store struct{ DB *db.DB }

func (s Scope) key(key string, defaultType string) string {
	t := s.Type
	if t == "" {
		t = defaultType
	}
	prefix := t
	if s.Name != "" {
		prefix += "_" + s.Name
	}
	return prefix + "_" + key
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (st Store) where(scope Scope, key string, defaultType string) (string, []any) {
	ph := st.DB.Dialect.Placeholder
	q := `"key" = ` + ph(1)
	args := []any{scope.key(key, defaultType)}
	for _, c := range []struct {
		col string
		val string
	}{{"environment", scope.Environment}, {"tag", scope.Tag}} {
		if c.val == "" {
			q += ` and "` + c.col + `" is null`
			continue
		}
		args = append(args, c.val)
		q += ` and "` + c.col + `" = ` + ph(len(args))
	}
	return q, args
}

// Get decodes the value into v; found is false when the key is absent or
// of an unknown type.
func (st Store) Get(ctx context.Context, scope Scope, key string, v any) (found bool, err error) {
	where, args := st.where(scope, key, "core")
	var value, typ sql.NullString
	err = st.DB.QueryRow(ctx, `select "value", "type" from "strapi_core_store_settings" where `+where+` order by "id" limit 1`, args...).Scan(&value, &typ)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch typ.String {
	case "object", "array", "boolean", "string":
		if json.Unmarshal([]byte(value.String), v) != nil {
			// Strapi falls back to new Date(value).
			t, perr := time.Parse(time.RFC3339Nano, value.String)
			if perr != nil {
				return true, nil
			}
			b, _ := json.Marshal(t)
			return true, json.Unmarshal(b, v)
		}
		return true, nil
	case "number":
		f, perr := strconv.ParseFloat(strings.TrimSpace(value.String), 64)
		if perr != nil {
			return true, nil
		}
		b, _ := json.Marshal(f)
		return true, json.Unmarshal(b, v)
	}
	return false, nil
}

// jsType is JavaScript's typeof for a JSON value.
func jsType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int64, float64, json.Number:
		return "number"
	case nil:
		return "object"
	}
	return "object"
}

// Set stores value, JSON encoded, with Strapi's type (typeof value).
func (st Store) Set(ctx context.Context, scope Scope, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	where, args := st.where(scope, key, "undefined")
	var id int64
	err = st.DB.QueryRow(ctx, `select "id" from "strapi_core_store_settings" where `+where+` order by "id" limit 1`, args...).Scan(&id)
	ph := st.DB.Dialect.Placeholder
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = st.DB.Exec(ctx, `insert into "strapi_core_store_settings" ("key", "value", "type", "environment", "tag") values (`+ph(1)+`, `+ph(2)+`, `+ph(3)+`, `+ph(4)+`, `+ph(5)+`)`,
			scope.key(key, "undefined"), string(b), jsType(value), nullable(scope.Environment), nullable(scope.Tag))
		return err
	case err != nil:
		return err
	}
	_, err = st.DB.Exec(ctx, `update "strapi_core_store_settings" set "value" = `+ph(1)+`, "type" = `+ph(2)+` where "id" = `+ph(3), string(b), jsType(value), id)
	return err
}

// Delete removes the key.
func (st Store) Delete(ctx context.Context, scope Scope, key string) error {
	where, args := st.where(scope, key, "undefined")
	_, err := st.DB.Exec(ctx, `delete from "strapi_core_store_settings" where `+where, args...)
	return err
}
