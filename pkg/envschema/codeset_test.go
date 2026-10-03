package envschema

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"
)

// problemCodes lists every code that is declared.
//
// The list is written by hand — Go cannot enumerate the members of a
// string-constant type — and a hand-written list of everything is one that
// goes stale the first time somebody adds a rule and stops there. Every other
// test about the set reads problemCodes, so a code left out of it is a code
// that silently opts out of all of them: out of the distinctness check, and
// out of TestEveryCodeIsReachable, which is the one that would have noticed
// the rule was never exercised.
//
// So this reads the declarations back out of the source and compares. It is
// the one test here that cannot be satisfied by editing the list.
func TestProblemCodesListsEveryDeclaredCode(t *testing.T) {
	declared := declaredProblemCodes(t)
	if len(declared) == 0 {
		t.Fatal("found no ProblemCode constants; this test has stopped reading the source")
	}

	listed := map[ProblemCode]bool{}
	for _, c := range problemCodes {
		listed[c] = true
	}
	for name, value := range declared {
		if !listed[value] {
			t.Errorf("%s (%q) is declared but missing from problemCodes", name, value)
		}
	}
	if len(declared) != len(problemCodes) {
		t.Errorf("%d codes declared in the source, problemCodes has %d", len(declared), len(problemCodes))
	}
}

// declaredProblemCodes reads the package's own source and returns every
// `Name ProblemCode = "value"` constant in it, by name.
func declaredProblemCodes(t *testing.T) map[string]ProblemCode {
	t.Helper()
	out := map[string]ProblemCode{}
	fset := token.NewFileSet()
	for _, file := range []string{"envschema.go", "parse.go", "validate.go", "conform.go", "name.go"} {
		f, err := goparser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "ProblemCode" {
				return true
			}
			for i, name := range vs.Names {
				out[name.Name] = constString(t, file, vs, i)
			}
			return true
		})
	}
	return out
}

// constString is the string a ValueSpec's i'th value is, with its quotes off.
func constString(t *testing.T, file string, vs *ast.ValueSpec, i int) ProblemCode {
	t.Helper()
	if i >= len(vs.Values) {
		t.Fatalf("%s: %s is declared with no value", file, vs.Names[i].Name)
	}
	lit, ok := vs.Values[i].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Fatalf("%s: %s is not a string literal; codes are written out on purpose", file, vs.Names[i].Name)
	}
	return ProblemCode(lit.Value[1 : len(lit.Value)-1])
}
