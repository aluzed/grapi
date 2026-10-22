package graphql

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/fixturetest"
)

// Review workflows is an Enterprise feature (clean-room): its types are
// left out of the reference schemas.
func enterprise(name string) bool { return strings.Contains(strings.ToLower(name), "reviewworkflows") }

func describeField(f *ast.FieldDefinition) string {
	var args []string
	for _, a := range f.Arguments {
		s := a.Name + ": " + a.Type.String()
		if a.DefaultValue != nil {
			s += " = " + a.DefaultValue.String()
		}
		args = append(args, s)
	}
	s := f.Name
	if len(args) > 0 {
		s += "(" + strings.Join(args, ", ") + ")"
	}
	s += ": " + f.Type.String()
	if f.DefaultValue != nil {
		s += " = " + f.DefaultValue.String()
	}
	return s
}

func describe(d *ast.Definition) []string {
	out := []string{string(d.Kind)}
	for _, f := range d.Fields {
		if strings.HasPrefix(f.Name, "__") || enterprise(f.Name) || enterprise(f.Type.Name()) {
			continue
		}
		out = append(out, describeField(f))
	}
	for _, v := range d.EnumValues {
		out = append(out, v.Name)
	}
	var members []string
	for _, m := range d.Types {
		if !enterprise(m) {
			members = append(members, m)
		}
	}
	if d.Name == "Query" || d.Name == "Mutation" || d.Kind == ast.Union {
		sort.Strings(out[1:])
		sort.Strings(members)
	}
	return append(out, members...)
}

func TestSchemaMatchesStrapi(t *testing.T) {
	for _, fx := range append([]string{"graphql"}, fixturetest.Names...) {
		t.Run(fx, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "sdl-"+fx+".graphql"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := gqlparser.LoadSchema(&ast.Source{Name: fx, Input: string(raw)})
			if err != nil {
				t.Fatal(err)
			}
			res, _, err := builtin.Load(os.DirFS(filepath.Join(fixturetest.Compat(), "fixtures", fx)))
			if err != nil {
				t.Fatal(err)
			}
			got, err := Build(res.Set)
			if err != nil {
				t.Fatal(err)
			}
			for name, wd := range want.Types {
				if wd.BuiltIn || enterprise(name) {
					continue
				}
				gd, ok := got.AST.Types[name]
				if !ok {
					t.Errorf("missing type %s", name)
					continue
				}
				w, g := describe(wd), describe(gd)
				if fmt.Sprint(w) != fmt.Sprint(g) {
					t.Errorf("type %s:\n want %s\n  got %s", name, strings.Join(w, "\n      "), strings.Join(g, "\n      "))
				}
			}
			for name, gd := range got.AST.Types {
				if _, ok := want.Types[name]; !ok && !gd.BuiltIn {
					t.Errorf("extra type %s", name)
				}
			}
		})
	}
}
