package grapi_test

import (
	"go/importer"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestNoInternalTypesInPublicAPI(t *testing.T) {
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	for _, path := range []string{"github.com/aluzed/grapi", "github.com/aluzed/grapi/admin", "github.com/aluzed/grapi/document", "github.com/aluzed/grapi/hook", "github.com/aluzed/grapi/schema"} {
		pkg, err := imp.Import(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		check := func(where string, typ types.Type) {
			if s := types.TypeString(typ, nil); strings.Contains(s, "/internal/") {
				t.Errorf("%s: %s", where, s)
			}
		}
		scope := pkg.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			where := path + "." + name
			switch o := obj.(type) {
			case *types.TypeName:
				named, ok := o.Type().(*types.Named)
				if !ok {
					check(where, o.Type())
					continue
				}
				if st, ok := named.Underlying().(*types.Struct); ok {
					for i := 0; i < st.NumFields(); i++ {
						if f := st.Field(i); f.Exported() {
							check(where+"."+f.Name(), f.Type())
						}
					}
				} else {
					check(where, named.Underlying())
				}
				for i := 0; i < named.NumMethods(); i++ {
					if m := named.Method(i); m.Exported() {
						check(where+"."+m.Name(), m.Type())
					}
				}
			default:
				check(where, obj.Type())
			}
		}
	}
}
