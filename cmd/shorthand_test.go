package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/local"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// v1ShorthandExceptions are the flag names that held -i before the v2 query
// surface settled it for --instance. They are grandfathered, not
// endorsed: an earlier fix takes -i away from them, and when it does, the entry
// should be deleted rather than the test relaxed. Nothing may be added here —
// a new flag wanting -i is the collision this test exists to catch.
var v1ShorthandExceptions = map[string]bool{
	"image-name":            true, // astro deploy, astro remote deploy, software deploy
	"conn-id":               true, // astro deployment connection create/update
	"info":                  true, // astro deployment logs
	"wait":                  true, // astro deployment update
	"include":               true, // astro organization audit-logs
	"include-default-roles": true, // astro organization team/user list
}

// TestDashIMeansInstanceOutsideTheAllowlist guards the seam the per-tree test
// cannot see: cmd/local enforces one meaning per shorthand inside the v2 tree,
// but that tree mounts on the v1 root, where older commands spell -i several
// other ways. an earlier fix settled -i for --instance, so every new -i has to be
// that; the survivors are grandfathered by name above and go away at an earlier fix.
//
// This checks -i alone rather than every letter. The two trees disagree about
// plenty of shorthands (-d, -r, -c, -f, -t all mean one thing in v1 and another
// in the query surface), and reconciling those is its own decision with its own
// compatibility cost — not something to smuggle in under a test.
func TestDashIMeansInstanceOutsideTheAllowlist(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()

	v2 := map[string]string{}
	v2Names := map[string]bool{}
	for _, top := range local.AddCmds(local.NewDeps()) {
		v2Names[top.Name()] = true
		walkCmd(top, func(cmd *cobra.Command) {
			collect(cmd, func(f *pflag.Flag) { v2[f.Shorthand] = f.Name })
		})
	}
	if v2["i"] != "instance" {
		t.Fatalf("-i in the v2 tree is --%s, want --instance", v2["i"])
	}

	root := NewRootCmd()
	for _, top := range root.Commands() {
		if v2Names[top.Name()] {
			continue
		}
		walkCmd(top, func(cmd *cobra.Command) {
			collect(cmd, func(f *pflag.Flag) {
				if f.Shorthand != "i" || f.Name == "instance" || v1ShorthandExceptions[f.Name] {
					return
				}
				t.Errorf("%s: -i is --%s; an earlier fix settled -i for --instance", cmd.CommandPath(), f.Name)
			})
		})
	}
}

// TestV1ShorthandExceptionsAreAllStillReal keeps the allowlist honest: an entry
// whose flag no longer takes -i is a line to delete, and leaving it behind
// would quietly re-open the hole for a future flag of the same name.
func TestV1ShorthandExceptionsAreAllStillReal(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()
	seen := map[string]bool{}
	root := NewRootCmd()
	for _, top := range root.Commands() {
		walkCmd(top, func(cmd *cobra.Command) {
			collect(cmd, func(f *pflag.Flag) {
				if f.Shorthand == "i" {
					seen[f.Name] = true
				}
			})
		})
	}
	for name := range v1ShorthandExceptions {
		if !seen[name] {
			t.Errorf("--%s no longer takes -i; drop it from v1ShorthandExceptions", name)
		}
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
