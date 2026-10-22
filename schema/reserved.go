package schema

import (
	"regexp"
	"strings"
)

// Rejected when a schema is loaded: these collide with columns grapi manages.
var loadReserved = map[string]bool{"id": true, "document_id": true}

// Rejected by the content-type builder only (Strapi loads them with a warning
// at most), compared in snake case. A trailing * is a prefix match.
var builderReservedAttributes = []string{
	"id", "document_id", "created_at", "updated_at", "published_at", "created_by_id",
	"updated_by_id", "created_by", "updated_by", "entry_id", "localizations", "meta", "locale",
	"__component", "__contentType", "strapi*", "_strapi*", "__strapi*",
}

var builderReservedDraftAndPublish = []string{"status"}

var builderReservedModels = []string{
	"boolean", "date", "date_time", "time", "upload", "document", "then",
	"strapi*", "_strapi*", "__strapi*",
}

func isLoadReserved(name string) bool { return loadReserved[snakeCase(name)] }

// IsReservedAttributeName reports whether the content-type builder must refuse
// name. With draftAndPublish, "status" is reserved too.
func IsReservedAttributeName(name string, draftAndPublish bool) bool {
	// Like Strapi, compare in snake case only: leading underscores are dropped,
	// so "__component" is not caught here but by the builder's name pattern.
	s := snakeCase(name)
	return matchesReserved(s, builderReservedAttributes) ||
		(draftAndPublish && matchesReserved(s, builderReservedDraftAndPublish))
}

// IsReservedModelName reports whether the content-type builder must refuse a model name.
func IsReservedModelName(name string) bool {
	return matchesReserved(snakeCase(name), builderReservedModels)
}

func matchesReserved(name string, list []string) bool {
	for _, entry := range list {
		if prefix, ok := strings.CutSuffix(entry, "*"); ok {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if name == entry {
			return true
		}
	}
	return false
}

var kebabCase = regexp.MustCompile(`^([a-z][a-z0-9]*)(-[a-z0-9]+)*$`)

func isKebabCase(s string) bool { return kebabCase.MatchString(s) }
