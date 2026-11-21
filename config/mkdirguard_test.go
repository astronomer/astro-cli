package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// directoryCreatorsOutsideHome are the root-module files that call os.MkdirAll
// or os.Mkdir directly, with why none of them can be the first to create the
// home config directory (~/.astro) at the wrong mode. Whichever writer creates
// that directory first decides its mode, so anything that can write under it
// goes through MkdirAllUnder; a file that needs to call os.MkdirAll itself
// belongs here, with a reason, and not by quietly bypassing the helper.
var directoryCreatorsOutsideHome = map[string]string{
	"config/config.go":                                   "MkdirAllUnder and the home config directory's own creation",
	"config/config_test_utils.go":                        "a project's .astro, in a test helper",
	"config/login.go":                                    "creates the home config directory itself, owner-only",
	"cmd/cliout/cliouttest/schema.go":                    "test golden files",
	"cmd/local/preflight.go":                             "the uv cache (XDG cache), not the home config directory",
	"internal/runtimecatalog/freeze.go":                  "the runtime constraints cache (XDG cache)",
	"internal/userstate/userstate.go":                    "per-project state under the cache directory",
	"internal/platform/astro/deploy/deploy.go":           "a project's dags directory",
	"internal/pack/tree.go":                              "the package output tree the user names",
	"internal/platform/astro/deployment/bundle_files.go": "the local directory the user names as the destination of a copy or sync out of a bundle",
	"internal/localenv/dotenv.go":                        "a project's .env",
	"pkg/fileutil/files.go":                              "copies a tree to a destination the caller names",
	"version/version.go":                                 "the release-check cache; owner-only (0700), so it creates ~/.astro that way too",
}

// A new os.MkdirAll that can land under ~/.astro has to use MkdirAllUnder,
// which makes the home config directory owner-only if it creates it.
func TestNoDirectoryIsCreatedUnderHomeBypassingTheHelper(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)

	var found []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(root, path, d) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && callsMkdir(path) {
			rel, relErr := filepath.Rel(root, path)
			require.NoError(t, relErr)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	}))

	sort.Strings(found)
	for _, f := range found {
		if _, ok := directoryCreatorsOutsideHome[f]; !ok {
			t.Errorf("%s calls os.MkdirAll or os.Mkdir: a directory that can be under the home config directory goes through config.MkdirAllUnder; add the file to directoryCreatorsOutsideHome with a reason if it cannot be", f)
		}
	}
}

// skipDir reports whether the walk leaves dir out: another module, or no source.
func skipDir(root, dir string, d fs.DirEntry) bool {
	if dir == root {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return true
	}
	switch d.Name() {
	case ".git", "node_modules", "vendor", ".claude":
		return true
	}
	return false
}

// callsMkdir reports whether the Go file at path calls os.MkdirAll or os.Mkdir.
func callsMkdir(path string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return false // a file that does not parse is the compiler's to report
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" && (sel.Sel.Name == "MkdirAll" || sel.Sel.Name == "Mkdir") {
			found = true
		}
		return true
	})
	return found
}
