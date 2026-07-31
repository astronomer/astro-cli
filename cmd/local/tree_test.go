package local

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/internal/localdocker"
	"github.com/astronomer/astro-cli/internal/localstandalone/supervise"
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
			// The dev stub and the internal supervisor, session-watcher, and
			// proxy server render no data, so the --output rule does not apply.
			if cmd.Runnable() && cmd.Name() != "dev" &&
				cmd.Name() != supervise.Subcommand &&
				cmd.Name() != localdocker.SessionWatchSubcommand &&
				cmd.Name() != proxydaemon.ServeSubcommand {
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
	for _, name := range []string{"local", "init", "dev", "start", "stop", "logs", "use", "instance"} {
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

// The query surface spelled out: the families at the top level and the verbs
// under each. It is a table rather than a walk because these exact spellings
// are the contract, and a walk would pass whatever the tree happened to hold.
func TestQuerySurfaceHasEveryCommand(t *testing.T) {
	want := map[string][]string{
		"dags":        {"list", "get", "source", "stats", "pause", "unpause"},
		"runs":        {"list", "get", "trigger", "delete", "clear"},
		"tasks":       {"list", "get", "instance", "logs", "clear"},
		"assets":      {"list", "events"},
		"connections": {"list", "get"},
		"variables":   {"list", "get"},
		"pools":       {"list", "get"},
		// list reads the inventory; the other three describe whichever Airflow
		// resolution picked.
		"instance": {"list", "config", "version", "health"},
	}
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for family, verbs := range want {
		cmd, _, err := root.Find([]string{family})
		if err != nil || cmd.Name() != family {
			t.Errorf("astro %s is missing from the v2 root", family)
			continue
		}
		have := map[string]bool{}
		for _, sub := range cmd.Commands() {
			have[sub.Name()] = true
		}
		for _, verb := range verbs {
			if !have[verb] {
				t.Errorf("astro %s %s is missing", family, verb)
			}
		}
	}
}

// Every command that acts on an Airflow reaches the same two flags, spelled the
// same way. One registration is what keeps the families from drifting into
// three spellings of one idea.
func TestQueryFamiliesShareTheInstanceFlags(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for _, family := range []string{"dags", "runs", "tasks", "assets", "connections", "variables", "pools"} {
		cmd, _, err := root.Find([]string{family})
		if err != nil {
			t.Errorf("astro %s is missing", family)
			continue
		}
		instance := cmd.PersistentFlags().Lookup("instance")
		if instance == nil || instance.Shorthand != "i" {
			t.Errorf("astro %s: --instance = %+v, want it registered with -i", family, instance)
		}
		if cmd.PersistentFlags().Lookup("url") == nil {
			t.Errorf("astro %s cannot reach --url", family)
		}
		for _, sub := range cmd.Commands() {
			if sub.InheritedFlags().Lookup("instance") == nil {
				t.Errorf("astro %s %s cannot reach -i", family, sub.Name())
			}
		}
	}
}

// One shorthand means one thing across the v2 tree — the commands AddCmds
// returns, not the v1 surface those mount alongside, which has its own older
// spellings (see TestV2ShorthandsDoNotCollideWithV1 in package cmd). `af`
// reused -t for tags, task ids, and try numbers in commands that sit next to
// each other; carrying that over would make -t unreadable, and -o would shadow
// --output outright. Nothing enforces this in cobra, so it is enforced here.
func TestShorthandsMeanOneThingEachAcrossTheV2Tree(t *testing.T) {
	d, _ := testDeps(t)
	// meaning maps a shorthand to the flag name that claimed it, and where.
	type claim struct{ flag, where string }
	meaning := map[string]claim{}
	for _, top := range AddCmds(d) {
		walk(top, func(cmd *cobra.Command) {
			check := func(f *pflag.Flag) {
				if f.Shorthand == "" {
					return
				}
				prior, seen := meaning[f.Shorthand]
				if !seen {
					meaning[f.Shorthand] = claim{f.Name, cmd.CommandPath()}
					return
				}
				if prior.flag != f.Name {
					t.Errorf("-%s is --%s in %q but --%s in %q",
						f.Shorthand, prior.flag, prior.where, f.Name, cmd.CommandPath())
				}
			}
			cmd.Flags().VisitAll(check)
			cmd.PersistentFlags().VisitAll(check)
		})
	}
	// The one that would break every json consumer if it ever drifted.
	if got := meaning["o"]; got.flag != "output" {
		t.Errorf("-o is --%s, want --output", got.flag)
	}
}
