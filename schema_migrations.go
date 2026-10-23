package grapi

import (
	"context"
	"fmt"
	"reflect"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

// Strapi's content type migrations (core/dist/migrations): the schema
// stored at the last start (core store strapi_content_types_schema) says
// which types turned draft and publish or i18n on or off since then.

var contentTypesSchemaKey = struct {
	scope store.Scope
	key   string
}{store.Scope{Type: "strapi"}, "content_types_schema"}

func readStoredSchema(ctx context.Context, d *db.DB) (map[string]any, error) {
	// Before the first sync there is no core store yet.
	if ok, err := schemasync.HasTable(ctx, d, "strapi_core_store_settings"); err != nil || !ok {
		return nil, err
	}
	var old map[string]any
	found, err := (store.Store{DB: d}).Get(ctx, contentTypesSchemaKey.scope, contentTypesSchemaKey.key, &old)
	if err != nil || !found {
		return nil, err
	}
	return old, nil
}

func dig(v any, path ...string) any {
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[p]
	}
	return v
}

// storedFlags are hasDraftAndPublish and isLocalizedContentType on a
// stored schema entry: strictly true.
func storedFlags(entry any) (dp, localized bool) {
	return dig(entry, "options", "draftAndPublish") == true, dig(entry, "pluginOptions", "i18n", "localized") == true
}

// changedTypes are the content types the stored schema knows, with their
// flags then and now.
type typeChange struct {
	uid                    string
	table                  string
	wasDP, isDP            bool
	wasLocalized, localize bool
}

func changedTypes(old map[string]any, p *Project) []typeChange {
	var out []typeChange
	for uid, ct := range p.Res.Set.ContentTypes.All() {
		entry, ok := old[uid]
		if !ok {
			continue
		}
		m, ok := p.MD.Model(uid)
		if !ok {
			continue
		}
		wasDP, wasLocalized := storedFlags(entry)
		out = append(out, typeChange{uid: uid, table: m.TableName, wasDP: wasDP, isDP: ct.DraftAndPublish(), wasLocalized: wasLocalized, localize: ct.Localized()})
	}
	return out
}

// disableMigrations runs before the schema sync, as Strapi's beforeSync:
// i18n turned off keeps the default locale's rows only, draft and publish
// turned off drops the drafts.
func (a *App) disableMigrations(ctx context.Context, p *Project) error {
	old, err := readStoredSchema(ctx, p.DB)
	if err != nil || old == nil {
		return err
	}
	q := p.DB.Dialect.Quote
	ph := p.DB.Dialect.Placeholder
	var defaultLocale any
	for _, c := range changedTypes(old, p) {
		if c.wasLocalized && !c.localize {
			if defaultLocale == nil {
				defaultLocale = document.New(&query.Engine{DB: p.DB, MD: p.MD}, p.Res.Set).DefaultLocale(ctx)
			}
			if defaultLocale == nil {
				return fmt.Errorf("cannot disable localization for %q: no default locale is set, so non-default rows cannot be identified", c.uid)
			}
			p.Log.Warn("localization turned off: rows of the other locales are deleted", "uid", c.uid, "keep", defaultLocale)
			if _, err := p.DB.Exec(ctx, fmt.Sprintf("delete from %s where %s <> %s", q(c.table), q("locale"), ph(1)), defaultLocale); err != nil {
				return err
			}
			if _, err := p.DB.Exec(ctx, fmt.Sprintf("update %s set %s = null where %s = %s", q(c.table), q("locale"), q("locale"), ph(1)), defaultLocale); err != nil {
				return err
			}
		}
		if c.wasDP && !c.isDP {
			p.Log.Warn("draft and publish turned off: the drafts are deleted", "uid", c.uid)
			if _, err := p.DB.Exec(ctx, fmt.Sprintf("delete from %s where %s is null", q(c.table), q("published_at"))); err != nil {
				return err
			}
		}
	}
	return nil
}

// enableMigrations runs after the sync, as Strapi's afterSync: i18n turned
// on puts the default locale on the rows without one, draft and publish
// turned on gives every published entry its draft (discardDraft).
func (a *App) enableMigrations(ctx context.Context, p *Project, docs *document.Service) error {
	old, err := readStoredSchema(ctx, p.DB)
	if err != nil || old == nil {
		return err
	}
	q := p.DB.Dialect.Quote
	for _, c := range changedTypes(old, p) {
		if !c.wasLocalized && c.localize {
			if def := docs.DefaultLocale(ctx); def != nil {
				if _, err := p.DB.Exec(ctx, fmt.Sprintf("update %s set %s = %s where %s is null", q(c.table), q("locale"), p.DB.Dialect.Placeholder(1), q("locale")), def); err != nil {
					return err
				}
			}
		}
		if c.wasDP || !c.isDP {
			continue
		}
		rows, err := p.DB.Query(ctx, fmt.Sprintf("select %s, %s from %s where %s is not null order by %s", q("document_id"), q("locale"), q(c.table), q("published_at"), q("id")))
		if err != nil {
			return err
		}
		type entry struct {
			documentID string
			locale     *string
		}
		var entries []entry
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.documentID, &e.locale); err != nil {
				_ = rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, e := range entries {
			params := document.Params{}
			if e.locale != nil {
				params["locale"] = *e.locale
			}
			if _, err := docs.DiscardDraft(ctx, c.uid, e.documentID, params); err != nil {
				return fmt.Errorf("draft and publish turned on for %s: %w", c.uid, err)
			}
		}
	}
	return nil
}

// storeFlags keeps the stored schema's flags in step with the content types,
// so that Strapi does not replay a migration grapi ran. Entries Strapi wrote
// are patched in place (both options and __schema__), the types it does not
// know get a minimal entry, and nothing is written when nothing changed.
func storeFlags(ctx context.Context, d *db.DB, set *schema.Set) error {
	old, err := readStoredSchema(ctx, d)
	if err != nil {
		return err
	}
	next := map[string]any{}
	for uid, entry := range old {
		next[uid] = entry
	}
	for uid, ct := range set.ContentTypes.All() {
		entry, _ := next[uid].(map[string]any)
		if dp, loc := storedFlags(entry); entry != nil && dp == ct.DraftAndPublish() && loc == ct.Localized() {
			continue
		}
		if entry == nil {
			entry = map[string]any{"uid": uid, "kind": ct.Kind, "modelType": "contentType"}
		} else {
			entry = deepCopy(entry).(map[string]any)
		}
		setFlags(entry, ct)
		if s, ok := entry["__schema__"].(map[string]any); ok {
			setFlags(s, ct)
		}
		next[uid] = entry
	}
	if reflect.DeepEqual(old, next) {
		return nil
	}
	return (store.Store{DB: d}).Set(ctx, contentTypesSchemaKey.scope, contentTypesSchemaKey.key, next)
}

func setFlags(entry map[string]any, ct *schema.ContentType) {
	options, _ := entry["options"].(map[string]any)
	if options == nil {
		options = map[string]any{}
	}
	options["draftAndPublish"] = ct.DraftAndPublish()
	entry["options"] = options
	plugin, _ := entry["pluginOptions"].(map[string]any)
	i18n, _ := plugin["i18n"].(map[string]any)
	if ct.Localized() {
		if plugin == nil {
			plugin = map[string]any{}
		}
		if i18n == nil {
			i18n = map[string]any{}
		}
		i18n["localized"] = true
		plugin["i18n"] = i18n
		entry["pluginOptions"] = plugin
	} else if i18n != nil {
		i18n["localized"] = false
	}
}

func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = deepCopy(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCopy(e)
		}
		return out
	}
	return v
}
