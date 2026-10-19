package archlint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// oneXSpellings are the 1.x flag names v2 replaced with --deployment and
// --workspace. Where a command has the v2 spelling, the 1.x one is a hidden
// alias registered with utils.AddAlias, and the command reads only the v2
// flag.
var oneXSpellings = map[string]bool{"deployment-id": true, "deployment-name": true, "workspace-id": true}

// oneXSpellingAllowed is each file that may write a 1.x name other than as an
// argument to AddAlias, and why.
var oneXSpellingAllowed = map[string]string{
	"cmd/removed_flags.go": "the messages for 1.x flags v2 removed",
	"cmd/astro/dbt.go":     "the removed astro dbt stub skips the values of 1.x's flags to find the subcommand typed",
	// APC's commands other than deploy have no --deployment or --workspace:
	// the 1.x spelling is the only one they take.
	"cmd/apc/deployment.go":                 "APC's own flags, with no v2 spelling",
	"cmd/apc/deployment_teams.go":           "APC's own flags, with no v2 spelling",
	"cmd/apc/deployment_user.go":            "APC's own flags, with no v2 spelling",
	"cmd/apc/deployment_service_account.go": "APC's own flags, with no v2 spelling",
	"cmd/apc/workspace_user.go":             "APC's own flags, with no v2 spelling",
	"cmd/apc/workspace_teams.go":            "APC's own flags, with no v2 spelling",
	"cmd/apc/workspace_service_account.go":  "APC's own flags, with no v2 spelling",
}

// A command that has --deployment or --workspace reads only that flag, never
// its 1.x spelling, so the two cannot drift apart. This catches an honest copy
// of the old pattern, a Changed("workspace-id") or a flag registered under a
// 1.x name, by scanning string literals under cmd/. It is not meant to stop
// deliberate evasion: a 1.x name in a named constant or spelled in pieces is
// out of scope.
func TestOnlyTheAliasHelpersNameTheOneXSpellings(t *testing.T) {
	root := repoRoot(t)
	var problems []string
	used := map[string]bool{}
	for _, file := range trackedGoFiles(t, root) {
		if !strings.HasPrefix(file, "cmd/") || !heldToPlainText(file) {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file)), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range oneXSpellingLiterals(f) {
			if _, ok := oneXSpellingAllowed[file]; ok {
				used[file] = true
				continue
			}
			problems = append(problems, file+": "+lit)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d string literals name a 1.x flag outside the alias helpers:\n\t%s\n\n"+
			"Read --deployment or --workspace instead, and register a 1.x spelling with "+
			"utils.AddAlias (or addDeploymentFlag/addWorkspaceFlag in cmd/astro). If the file "+
			"has a reason of its own, add it to oneXSpellingAllowed in "+
			"internal/archlint/oneXspellings_test.go and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for file := range oneXSpellingAllowed {
		if !used[file] {
			t.Errorf("oneXSpellingAllowed names %s, which no longer names a 1.x spelling: delete the entry", file)
		}
	}
}

// oneXSpellingLiterals is every string literal in f that is a 1.x name,
// except an argument to AddAlias.
func oneXSpellingLiterals(f *ast.File) []string {
	var lits []string
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "AddAlias" {
				return false
			}
		}
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil && oneXSpellings[s] {
			lits = append(lits, lit.Value)
		}
		return true
	})
	return lits
}
