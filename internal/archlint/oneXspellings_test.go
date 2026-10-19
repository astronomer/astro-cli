package archlint

import (
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// oneXSpellings are the 1.x flag names v2 replaced with --deployment and
// --workspace. Where a command has the v2 spelling, the 1.x one is a hidden
// alias that utils.ApplyAliases hands to it, and the command reads only the
// v2 flag. So a 1.x name is spelled out only where it is registered as an
// alias (an argument to utils.AddAlias) and at the sites oneXSpellingAllowed
// names.
var oneXSpellings = map[string]bool{"deployment-id": true, "deployment-name": true, "workspace-id": true}

// aliasRegistrar is the one function a 1.x spelling may be handed to anywhere.
const aliasRegistrar = "github.com/astronomer/astro-cli/cmd/utils.AddAlias"

// oneXSpellingAllowed is each site, other than an argument to AddAlias, that
// may spell a 1.x name, and why. A site is a file and the function, or the
// package-level var or const, the expression sits in ("file: name", a method
// as "Type.Method").
var oneXSpellingAllowed = map[string]string{
	"cmd/removed_flags.go: removedFlags": "the messages for 1.x flags v2 removed",
	"cmd/astro/dbt.go: dbtValueFlags":    "the removed astro dbt stub skips the values of 1.x's flags to find the subcommand typed",
	// APC's commands other than deploy have no --deployment or --workspace:
	// the 1.x spelling is the only one they take, registered where they are
	// built.
	"cmd/apc/deployment.go: newDeploymentRootCmd":                     "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment.go: newDeploymentUnadoptCmd":                  "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment.go: newDeploymentAirflowUpgradeCmd":           "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment.go: newDeploymentRuntimeUpgradeCmd":           "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment.go: newDeploymentRuntimeMigrateCmd":           "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_teams.go: newDeploymentTeamRootCmd":           "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_user.go: newDeploymentUserListCmd":            "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_user.go: newDeploymentUserAddCmd":             "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_user.go: newDeploymentUserRemoveCmd":          "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_user.go: newDeploymentUserUpdateCmd":          "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_service_account.go: newDeploymentSaCreateCmd": "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_service_account.go: newDeploymentSaListCmd":   "APC's own flag, with no v2 spelling",
	"cmd/apc/deployment_service_account.go: newDeploymentSaDeleteCmd": "APC's own flag, with no v2 spelling",
	"cmd/apc/workspace_user.go: newWorkspaceUserRootCmd":              "APC's own flag, with no v2 spelling",
	"cmd/apc/workspace_teams.go: newWorkspaceTeamRootCmd":             "APC's own flag, with no v2 spelling",
	"cmd/apc/workspace_service_account.go: newWorkspaceSaCreateCmd":   "APC's own flag, with no v2 spelling",
	"cmd/apc/workspace_service_account.go: newWorkspaceSaListCmd":     "APC's own flag, with no v2 spelling",
	"cmd/apc/workspace_service_account.go: newWorkspaceSaDeleteCmd":   "APC's own flag, with no v2 spelling",
}

// A command that took --deployment-id, --deployment-name or --workspace-id
// in 1.x and has --deployment or --workspace now reads only the new flag:
// nothing outside the alias registrations names the old one, so the two
// spellings cannot drift apart. The check is on constant values, through the
// type checker, so a 1.x name hidden in a named constant or spelled in pieces
// ("workspace" + "-id") is caught where it is used, not only where it is
// written.
func TestOnlyTheAliasRegistrationsSpellTheOneXNames(t *testing.T) {
	root := repoRoot(t)
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  root,
	}, "./cmd/...", "./internal/...")
	if err != nil {
		t.Fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("the packages under cmd/ and internal/ do not type-check")
	}
	var problems []string
	used := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			file, err := filepath.Rel(root, pkg.Fset.File(f.Pos()).Name())
			if err != nil {
				t.Fatal(err)
			}
			file = filepath.ToSlash(file)
			for _, site := range oneXSpellingSites(f, pkg.TypesInfo) {
				key := file + ": " + site.where
				if _, ok := oneXSpellingAllowed[key]; ok {
					used[key] = true
					continue
				}
				problems = append(problems, key+": "+site.value)
			}
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d expressions spell a 1.x flag name outside the alias registrations:\n\t%s\n\n"+
			"Read --deployment or --workspace instead: the 1.x spelling is an alias "+
			"utils.ApplyAliases hands to it. Register a 1.x spelling with utils.AddAlias "+
			"(or addDeploymentFlag/addWorkspaceFlag in cmd/astro). If the site has a reason "+
			"of its own, add it to oneXSpellingAllowed in internal/archlint/oneXspellings_test.go "+
			"and say why.", len(problems), strings.Join(problems, "\n\t"))
	}
	for key := range oneXSpellingAllowed {
		if !used[key] {
			t.Errorf("oneXSpellingAllowed names %s, which no longer spells a 1.x name: delete the entry", key)
		}
	}
}

// oneXSpellingSite is one expression whose constant value is a 1.x name.
type oneXSpellingSite struct {
	where string // the enclosing function, or package-level var or const
	value string
}

// oneXSpellingSites lists the outermost expressions in f whose constant value
// is a 1.x spelling, except an argument of a call to AddAlias.
func oneXSpellingSites(f *ast.File, info *types.Info) []oneXSpellingSite {
	var sites []oneXSpellingSite
	for _, decl := range f.Decls {
		for _, unit := range declUnits(decl) {
			var visit func(n ast.Node) bool
			visit = func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && callsAliasRegistrar(call, info) {
					// A 1.x name handed to AddAlias is the registration.
					for _, a := range call.Args {
						if !isOneXSpelling(info, a) {
							ast.Inspect(a, visit)
						}
					}
					return false
				}
				if e, ok := n.(ast.Expr); ok && isOneXSpelling(info, e) {
					sites = append(sites, oneXSpellingSite{where: unit.name, value: info.Types[e].Value.ExactString()})
					return false
				}
				return true
			}
			ast.Inspect(unit.node, visit)
		}
	}
	return sites
}

// declUnit is a top-level function, or one var or const spec, and its name.
type declUnit struct {
	name string
	node ast.Node
}

// declUnits splits a top-level declaration into named units: a function (a
// method as Type.Method), or each var or const spec by its first name.
func declUnits(decl ast.Decl) []declUnit {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		name := d.Name.Name
		if d.Recv != nil && len(d.Recv.List) > 0 {
			name = recvName(d.Recv.List[0].Type) + "." + name
		}
		return []declUnit{{name, d}}
	case *ast.GenDecl:
		var units []declUnit
		for _, spec := range d.Specs {
			if v, ok := spec.(*ast.ValueSpec); ok && len(v.Names) > 0 {
				units = append(units, declUnit{v.Names[0].Name, v})
			}
		}
		return units
	}
	return nil
}

func isOneXSpelling(info *types.Info, e ast.Expr) bool {
	tv, ok := info.Types[e]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.String && oneXSpellings[constant.StringVal(tv.Value)]
}

// callsAliasRegistrar reports whether call is to utils.AddAlias.
func callsAliasRegistrar(call *ast.CallExpr, info *types.Info) bool {
	var id *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		id = fn
	case *ast.SelectorExpr:
		id = fn.Sel
	default:
		return false
	}
	obj, ok := info.Uses[id].(*types.Func)
	return ok && obj.FullName() == aliasRegistrar
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}
