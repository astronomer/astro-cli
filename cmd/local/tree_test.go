package local

import (
	"testing"

	"github.com/spf13/cobra"
)

func walk(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		walk(sub, fn)
	}
}

// Every v2 command validates its args, carries the skip-pre-run annotation
// (so the v1 root never runs config or network work for it), and can reach
// an --output flag — the issue's day-one rules, checked structurally.
func TestTreeInvariants(t *testing.T) {
	d, _ := testDeps(t)
	for _, top := range AddCmds(d) {
		walk(top, func(cmd *cobra.Command) {
			if cmd.Args == nil {
				t.Errorf("%s has no Args validator", cmd.CommandPath())
			}
			if cmd.Annotations[skipPreRunAnnotation] != "true" {
				t.Errorf("%s is missing the skip-pre-run annotation", cmd.CommandPath())
			}
			if cmd.Runnable() && cmd.Name() != "dev" {
				if cmd.Flags().Lookup("output") == nil &&
					cmd.PersistentFlags().Lookup("output") == nil &&
					cmd.InheritedFlags().Lookup("output") == nil {
					t.Errorf("%s cannot reach an --output flag", cmd.CommandPath())
				}
			}
		})
	}
}

func TestLocalTreeHasEverySpecCommand(t *testing.T) {
	d, _ := testDeps(t)
	localCmd := NewLocalCmd(d)
	want := []string{"start", "stop", "restart", "status", "list", "logs", "run", "shell", "open", "reset", "check", "init"}
	have := map[string]bool{}
	for _, sub := range localCmd.Commands() {
		have[sub.Name()] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("astro local %s is missing", name)
		}
	}
}

func TestRootHasAliasesInitAndDev(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for _, name := range []string{"local", "init", "dev", "start", "stop", "logs"} {
		found := false
		for _, sub := range root.Commands() {
			if sub.Name() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("astro %s is missing from the v2 root", name)
		}
	}
}
