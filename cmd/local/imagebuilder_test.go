package local

import (
	"go/ast"
	goparser "go/parser"
	"go/printer"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
	localrt "github.com/astronomer/astro-cli/pkg/localrt"
)

// The adapter forwards every field the two requests share.
//
// It is a hand-written field-by-field copy between two structs that are
// deliberately separate — localrt must not import imagebuild — and a copy like
// that fails silently: a field added to one side and forgotten here compiles,
// passes every unit test, and is simply absent at runtime.
//
// That is not hypothetical. Dockerfile and Context were missing, which is the
// whole of "bring your own Dockerfile" in docker mode. The engine leaves
// BaseImage empty when a Dockerfile is declared, because the file carries its
// own FROM, so the builder received neither a base nor a file, built nothing,
// and returned an empty tag. The compose file went out with `image:` blank and
// the start died on "services.db-migration.image must be a string" — a compose
// validation error, for a feature that had never run.
//
// Read out of the source rather than by calling Build, because what is being
// checked is the assignment list itself. Exercising the function would need a
// docker daemon to say anything, which is the tier that found this the
// expensive way.
func TestTheImageBuilderAdapterForwardsEveryField(t *testing.T) {
	assigned := assignedFields(t, "deps.go", "imagebuild", "Request")
	if len(assigned) == 0 {
		t.Fatal("found no imagebuild.Request literal; this test has stopped reading the source")
	}

	from := reflect.TypeOf(localrt.BuildRequest{})
	to := reflect.TypeOf(imagebuild.Request{})

	for i := range from.NumField() {
		name := from.Field(i).Name
		// Only the fields the destination actually has. One side carrying
		// something the other does not is a real difference between the two
		// types, not a dropped copy.
		if _, shared := to.FieldByName(name); !shared {
			continue
		}
		got, ok := assigned[name]
		if !ok {
			t.Errorf("localrt.BuildRequest.%s is never copied into imagebuild.Request", name)
			continue
		}
		// And copied from the field of the same name. Checking only that the
		// key appears would pass `Dockerfile: req.Context, Context:
		// req.Dockerfile` — a swap that is live rather than theoretical, since
		// rt.BuildRequest's own doc says the two are "only correct together"
		// and the engine sets them as a pair. It would build the project
		// against its Dockerfile as the context, and no unit test would say so.
		if want := "req." + name; got != want {
			t.Errorf("imagebuild.Request.%s is assigned %s, want %s", name, got, want)
		}
	}
}

// assignedFields maps each field a composite literal of <pkg>.<typeName>
// assigns to the expression it is assigned, rendered as source.
//
// The expression, not just the name, because the question is whether the copy
// is right and not only whether it is complete.
func assignedFields(t *testing.T, file, pkg, typeName string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	out := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != typeName {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != pkg {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			var rendered strings.Builder
			if err := printer.Fprint(&rendered, fset, kv.Value); err != nil {
				t.Fatalf("rendering the value assigned to %s: %v", key.Name, err)
			}
			out[key.Name] = rendered.String()
		}
		return true
	})
	return out
}
