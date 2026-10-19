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
// alias that utils.ApplyAliases hands to it, and the command reads only the
// v2 flag. So a 1.x name appears in the code only where it is registered as
// an alias (an argument to AddAlias), and in the files oneXSpellingAllowed
// names.
var oneXSpellings = []string{"deployment-id", "deployment-name", "workspace-id"}

// oneXSpellingAllowed is each file outside AddAlias calls that may name a
// 1.x spelling, and why.
var oneXSpellingAllowed = map[string]string{
	"cmd/astro/preferred_flags.go": "the Deployment helpers register the 1.x spellings as aliases and keep " +
		"the kind each one was given as",
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

// A command that took --deployment-id, --deployment-name or --workspace-id
// in 1.x and has --deployment or --workspace now reads only the new flag:
// nothing outside the alias helpers looks the old one up, so the two
// spellings cannot drift apart.
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
			problems = append(problems, file+": "+lit.Value)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d string literals name a 1.x flag outside the alias helpers:\n\t%s\n\n"+
			"Read --deployment or --workspace instead: the 1.x spelling is an alias "+
			"utils.ApplyAliases hands to it. Register a 1.x spelling with utils.AddAlias "+
			"(or addDeploymentFlag/addWorkspaceFlag in cmd/astro). If the file has a reason "+
			"of its own, add it to oneXSpellingAllowed in internal/archlint/oneXspellings_test.go "+
			"and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for file := range oneXSpellingAllowed {
		if !used[file] {
			t.Errorf("oneXSpellingAllowed names %s, which no longer names a 1.x spelling: delete the entry", file)
		}
	}
}

// oneXSpellingLiterals is every string literal in f naming a 1.x spelling,
// except as an argument to AddAlias.
func oneXSpellingLiterals(f *ast.File) []*ast.BasicLit {
	aliasArgs := map[*ast.BasicLit]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && calls(call, "AddAlias") {
			for _, a := range call.Args {
				if lit, ok := a.(*ast.BasicLit); ok {
					aliasArgs[lit] = true
				}
			}
		}
		return true
	})
	var lits []*ast.BasicLit
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || aliasArgs[lit] {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil && isOneXSpelling(s) {
			lits = append(lits, lit)
		}
		return true
	})
	return lits
}

func isOneXSpelling(s string) bool {
	for _, name := range oneXSpellings {
		if s == name {
			return true
		}
	}
	return false
}

// calls reports whether call is to a function named name, qualified or not.
func calls(call *ast.CallExpr, name string) bool {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name == name
	case *ast.SelectorExpr:
		return fn.Sel.Name == name
	}
	return false
}
