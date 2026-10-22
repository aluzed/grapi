package schemasync

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/metadata"
)

// Morph type columns Strapi repairs after a sync that changed the schema.
const (
	ComponentTypePivot = "component_type"
	RelatedTypePivot   = "related_type"
)

// RemoveOrphanMorphTypes deletes the rows of morph link tables whose type
// column names a model that no longer exists (a deleted component or content
// type), like Strapi's removeOrphanMorphType repair.
func RemoveOrphanMorphTypes(ctx context.Context, d *db.DB, md *metadata.Metadata, pivot string, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	seen := map[string]bool{}
	for _, m := range md.Models() {
		for _, a := range m.Attributes.All() {
			jt := a.JoinTable
			if a.Type != "relation" || jt == nil || !slices.Contains(jt.PivotColumns, pivot) || seen[jt.Name+"\x00"+pivot] {
				continue
			}
			seen[jt.Name+"\x00"+pivot] = true
			types, err := queryStrings(ctx, d, fmt.Sprintf("select distinct %s from %s where %s is not null",
				d.Dialect.Quote(pivot), d.Dialect.Quote(jt.Name), d.Dialect.Quote(pivot)))
			if err != nil {
				return err
			}
			for _, t := range types {
				if _, ok := md.Model(t); ok {
					continue
				}
				log.Debug("removing orphan morph type", "table", jt.Name, "type", t)
				if _, err := d.Exec(ctx, fmt.Sprintf("delete from %s where %s = %s",
					d.Dialect.Quote(jt.Name), d.Dialect.Quote(pivot), d.Dialect.Placeholder(1)), t); err != nil {
					log.Error("failed to remove orphan morph type", "table", jt.Name, "type", t, "error", err)
				}
			}
		}
	}
	return nil
}
