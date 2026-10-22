package normalize

import (
	"regexp"
	"sort"
	"strings"
)

var (
	oneOfPattern     = regexp.MustCompile(`(must be one of the following values: )(.+)$`)
	componentUIDItem = regexp.MustCompile(`^[a-z0-9-]+\.[a-z0-9-]+$`)
)

// sortComponentUIDLists sorts "must be one of the following values" lists made
// only of component uids: Strapi builds them from its component registry, whose
// order follows the filesystem. Enum lists keep their schema order.
func sortComponentUIDLists(s string) string {
	return oneOfPattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := oneOfPattern.FindStringSubmatch(m)
		items := strings.Split(parts[2], ", ")
		for _, it := range items {
			if !componentUIDItem.MatchString(it) {
				return m
			}
		}
		sort.Strings(items)
		return parts[1] + strings.Join(items, ", ")
	})
}
