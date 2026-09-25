package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The Airflow version is the apache-airflow requirement in [project]
// dependencies, read through manifest.Manifest.Airflow. [tool.astro] airflow
// was a second place to state it, and two places that nothing kept in step is
// how a project came to run one Airflow in Docker and another in standalone.
//
// Deleting the field made the compiler catch every old reader. These two tests
// keep a second source from coming back: the typed [tool.astro] carries no
// Airflow field, and nothing outside pkg/manifest reads one off it.

func TestTheAstroTableStatesNoAirflowVersion(t *testing.T) {
	typ := reflect.TypeFor[manifest.Astro]()
	for i := range typ.NumField() {
		if name := typ.Field(i).Name; strings.Contains(strings.ToLower(name), "airflow") {
			t.Errorf("manifest.Astro has a field %s: the Airflow version is the requirement in [project] dependencies, "+
				"read with Manifest.Airflow(), and a second place to state it is the drift that removing "+
				"[tool.astro] airflow fixed", name)
		}
	}
}

func TestNothingReadsAnAirflowVersionOffTheAstroTable(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch {
			case strings.HasPrefix(d.Name(), ".") && rel != ".",
				d.Name() == "testdata", d.Name() == "node_modules",
				filepath.ToSlash(rel) == "pkg/manifest":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Errorf("parse %s: %v", rel, perr)
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !strings.HasPrefix(sel.Sel.Name, "Airflow") {
				return true
			}
			if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Astro" {
				t.Errorf("%s reads .Astro.%s: the Airflow version is Manifest.Airflow(), from the requirement", rel, sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
