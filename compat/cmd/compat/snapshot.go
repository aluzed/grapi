package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/aluzed/grapi/compat/corpus"
	"github.com/aluzed/grapi/internal/builtin"
)

// snapshot writes a read only corpus that lists every content type of a
// fixture, drafts and published, every locale, one level populated. Run
// against Strapi and grapi on the same database, it shows whether both
// engines read that data the same way.
func snapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	fixtureDir := fs.String("fixture-dir", "", "fixture directory (holding src/)")
	name := fs.String("name", "", "corpus fixture name")
	out := fs.String("out", "", "corpus file to write")
	_ = fs.Parse(args)
	if *fixtureDir == "" || *name == "" || *out == "" {
		return fmt.Errorf("-fixture-dir, -name and -out are required")
	}
	res, _, err := builtin.Load(os.DirFS(*fixtureDir))
	if err != nil {
		return err
	}
	var uids []string
	for uid := range res.Set.ContentTypes.All() {
		if strings.HasPrefix(uid, "api::") {
			uids = append(uids, uid)
		}
	}
	sort.Strings(uids)
	f := corpus.File{Fixture: *name}
	for _, uid := range uids {
		ct, _ := res.Set.ContentTypes.Get(uid)
		statuses := []string{""}
		if ct.DraftAndPublish() {
			statuses = []string{"draft", "published"}
		}
		for _, st := range statuses {
			q := []string{"populate=*"}
			if st != "" {
				q = append(q, "status="+st)
			}
			if ct.Localized() {
				q = append(q, "locale=*")
			}
			path := "/api/" + ct.Info.SingularName
			if !ct.IsSingleType() {
				path = "/api/" + ct.Info.PluralName
				q = append(q, "sort[0]=id:asc", "pagination[pageSize]=100")
			}
			f.Cases = append(f.Cases, corpus.Case{Name: "snapshot-" + ct.Info.SingularName + "-" + st, Path: path, Query: strings.Join(q, "&"), Token: "full"})
		}
	}
	f.Cases = append(f.Cases,
		corpus.Case{Name: "snapshot-users", Path: "/api/users", Query: "sort[0]=id:asc", Token: "full"},
		corpus.Case{Name: "snapshot-files", Path: "/api/upload/files", Query: "sort[0]=id:asc", Token: "full"},
	)
	b, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(*out, b, 0o600)
}
