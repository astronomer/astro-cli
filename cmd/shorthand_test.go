package cmd

import (
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/local"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// v1DashDExceptions are the shell flags that spell -d as something other than a
// deployment. They are grandfathered, not endorsed: an earlier fix settled -d for
// --deployment across the core tree, harmonizing with the shell's own --deployment-id,
// and every one of these predates that. Nothing may be added — a new flag
// wanting -d for something else is the collision this test exists to catch,
// and an entry that goes away is a line to delete.
//
// The exemption is by flag name, not by command, which is the known weakness:
// a new `--description` taking -d anywhere in the shell tree inherits this pass
// without anyone deciding it should. Keying on command path instead would mean
// listing every one of the dozen-odd places `--description` already appears,
// and re-listing them whenever one moves. Named here so the next person to
// widen the tree knows what this does not catch.
var v1DashDExceptions = map[string]bool{
	"dags":        true, // astro deploy (and software deploy, where houston allows)
	"description": true, // deployment, workspace, team, and token create/update
	"domain":      true, // astro auth token
	"for":         true, // astro deployment hibernate, astro deployment wake-up
}

// TestDashDMeansDeploymentOutsideTheAllowlist guards the seam the per-tree test
// cannot see: cmd/local enforces one meaning per shorthand inside the core tree,
// but that tree mounts on the shell root, where older commands spell -d several
// other ways. an earlier fix settled -d for the deployment selector, matching the shell's
// own --deployment-id, so every new -d has to be one of those two; the
// survivors are grandfathered by name above.
//
// This checks -d alone rather than every letter. The two trees disagree about
// plenty of shorthands (-r, -c, -f, -t all mean one thing in the shell and another in
// the query surface), and reconciling those is its own decision with its own
// compatibility cost — not something to smuggle in under a test.
func TestDashDMeansDeploymentOutsideTheAllowlist(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()

	v2 := map[string]string{}
	v2Names := map[string]bool{}
	for _, top := range local.AddCmds(local.NewDeps()) {
		v2Names[top.Name()] = true
		walkCmd(top, func(cmd *cobra.Command) {
			collect(cmd, func(f *pflag.Flag) { v2[f.Shorthand] = f.Name })
		})
	}
	if v2["d"] != "deployment" {
		t.Fatalf("-d in the core tree is --%s, want --deployment", v2["d"])
	}

	for platform, root := range rootsUnderTest(t) {
		for _, top := range root.Commands() {
			if v2Names[top.Name()] {
				continue
			}
			walkCmd(top, func(cmd *cobra.Command) {
				collect(cmd, func(f *pflag.Flag) {
					if f.Shorthand != "d" || f.Name == "deployment" || f.Name == "deployment-id" || v1DashDExceptions[f.Name] {
						return
					}
					t.Errorf("[%s] %s: -d is --%s; an earlier fix settled -d for the deployment selector", platform, cmd.CommandPath(), f.Name)
				})
			})
		}
	}
}

// TestV1DashDExceptionsAreAllStillReal keeps the allowlist honest: an entry
// whose flag no longer takes -d is a line to delete, and leaving it behind
// would quietly re-open the hole for a future flag of the same name.
func TestV1DashDExceptionsAreAllStillReal(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()
	seen := map[string]bool{}
	// The union across both branches: an entry reachable in only one of them
	// is still real, and building one root would call it stale.
	for _, root := range rootsUnderTest(t) {
		for _, top := range root.Commands() {
			walkCmd(top, func(cmd *cobra.Command) {
				collect(cmd, func(f *pflag.Flag) {
					if f.Shorthand == "d" {
						seen[f.Name] = true
					}
				})
			})
		}
	}
	var stale []string
	for name := range v1DashDExceptions {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("--%s no longer takes -d; drop it from v1DashDExceptions", name)
	}
}

func walkCmd(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		walkCmd(sub, fn)
	}
}

// collect visits every flag a command declares, local and persistent, skipping
// the ones with no shorthand to argue about.
func collect(cmd *cobra.Command, fn func(*pflag.Flag)) {
	visit := func(f *pflag.Flag) {
		if f.Shorthand != "" {
			fn(f)
		}
	}
	cmd.Flags().VisitAll(visit)
	cmd.PersistentFlags().VisitAll(visit)
}
