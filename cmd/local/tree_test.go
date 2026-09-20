package local

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

func walk(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		walk(sub, fn)
	}
}

// Every v2 command validates its args, carries the skip-pre-run annotation,
// and can reach an --output flag — the issue's day-one rules, checked
// structurally.
//
// The annotation stops the v1 root's PersistentPreRunE: no logging setup, no
// platform pre-run, no telemetry hook, so no network work. It does not stop
// the config load, which happens in main before cobra has seen argv and has
// to, because cmd/root.go's detectRootOptions asks context.IsCloudContext()
// which subtree to mount. That read leaves nothing behind — see
// TestInitLeavesNoV1ConfigBehind in e2e.
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
				cmd.Name() != localrt.SuperviseSubcommand &&
				cmd.Name() != localrt.SessionWatchSubcommand &&
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
	want := []string{"start", "stop", "restart", "status", "list", "logs", "run", "shell", "open", "reset", "check", "api"}
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
	for _, name := range []string{"local", "init", "dev", "start", "stop", "logs", "use", "af"} {
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

// afGroup resolves the `af` node under root, or under the path given (the one
// caller passes "local"). It fails the test rather than returning nil, because
// every caller below is about what hangs off that node.
func afGroup(t *testing.T, root *cobra.Command, path ...string) *cobra.Command {
	t.Helper()
	cmd, _, err := root.Find(append(append([]string{}, path...), afName))
	if err != nil || cmd.Name() != afName {
		t.Fatalf("astro %s is missing: %v", strings.Join(append(append([]string{}, path...), afName), " "), err)
	}
	return cmd
}

// Both spellings of the group reach the same command, on both surfaces. The
// alias is the whole reason `af` can stay the short primary: a reader who never
// met that CLI spells it out and lands in the same place.
func TestAfGroupAnswersToAirflow(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for _, path := range [][]string{{}, {"local"}} {
		spelled, _, err := root.Find(append(append([]string{}, path...), afAlias, "dags"))
		if err != nil {
			t.Errorf("astro %s %s dags is missing: %v", strings.Join(path, " "), afAlias, err)
			continue
		}
		if short, _, _ := root.Find(append(append([]string{}, path...), afName, "dags")); short != spelled {
			t.Errorf("astro %s %s dags and astro %s %s dags are different commands",
				strings.Join(path, " "), afAlias, strings.Join(path, " "), afName)
		}
	}
}

// The `instance` and `instances` names stay unclaimed, so the noun a reader
// types means one thing: a deployment is a deployment, and the machine is
// `astro local`.
func TestTheInstanceNamesStayUnclaimed(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for _, name := range []string{"instance", "instances"} {
		if cmd, _, err := root.Find([]string{name}); err == nil && cmd.Name() == name {
			t.Errorf("astro %s still exists", name)
		}
	}
}

// querySurface is the query families and the verbs under each, spelled out. It
// is a table rather than a walk because these exact spellings are the contract,
// and a walk would pass whatever the tree happened to hold.
var querySurface = map[string][]string{
	"dags":        {"list", "get", "source", "stats", "pause", "unpause"},
	"runs":        {"list", "get", "tasks", "trigger", "delete", "clear"},
	"tasks":       {"list", "get", "instance", "logs", "clear"},
	"assets":      {"list", "events"},
	"connections": {"list", "get"},
	"variables":   {"list", "get"},
	"pools":       {"list", "get"},
	// health is a leaf rather than a family: it reads four things and prints
	// one report.
	"health": nil,
}

func TestQuerySurfaceHasEveryCommand(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	// Both spellings carry the whole surface: the machine's under
	// `astro local af`, a deployment's under `astro af`.
	for _, parent := range []*cobra.Command{afGroup(t, root), afGroup(t, root, "local")} {
		for family, verbs := range querySurface {
			cmd, _, err := parent.Find([]string{family})
			if err != nil || cmd.Name() != family {
				t.Errorf("%s %s is missing", parent.CommandPath(), family)
				continue
			}
			have := map[string]bool{}
			for _, sub := range cmd.Commands() {
				have[sub.Name()] = true
			}
			for _, verb := range verbs {
				if !have[verb] {
					t.Errorf("%s %s %s is missing", parent.CommandPath(), family, verb)
				}
			}
		}
	}
}

// TestBothRegistrationsAreTheSameCommands is the hard requirement of an earlier fix,
// checked structurally: every family exists twice — once against a deployment,
// once against this machine — and the two registrations are identical except
// for the selector flags, which only the deployment side carries. A forked
// command body shows up here as a flag, a verb, or a usage line that differs.
func TestBothRegistrationsAreTheSameCommands(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	top, machine := afGroup(t, root), afGroup(t, root, "local")
	// The flags the top-level registration adds and the machine's does not.
	selectors := map[string]bool{"deployment": true, "url": true}

	for family := range querySurface {
		topFamily, _, err := top.Find([]string{family})
		if err != nil {
			t.Errorf("astro af %s is missing", family)
			continue
		}
		machineFamily, _, err := machine.Find([]string{family})
		if err != nil {
			t.Errorf("astro local af %s is missing", family)
			continue
		}
		// The selectors sit on the top-level family and nowhere else.
		for name := range selectors {
			if topFamily.PersistentFlags().Lookup(name) == nil {
				t.Errorf("astro af %s cannot reach --%s", family, name)
			}
		}
		compareRegistrations(t, topFamily, machineFamily, selectors)
	}
}

// compareRegistrations walks two registrations of one command in step.
func compareRegistrations(t *testing.T, top, machine *cobra.Command, selectors map[string]bool) {
	t.Helper()
	where := machine.CommandPath()
	if got, want := flagNames(machine, nil), flagNames(top, selectors); !equalNames(got, want) {
		t.Errorf("%s flags = %v, want %v (the same flags modulo the selectors)", where, got, want)
	}
	for name := range selectors {
		if machine.PersistentFlags().Lookup(name) != nil || machine.Flags().Lookup(name) != nil {
			t.Errorf("%s takes --%s: the machine is not selectable", where, name)
		}
	}
	if top.Use != machine.Use {
		t.Errorf("%s: Use = %q, want %q", where, machine.Use, top.Use)
	}
	if top.Short != machine.Short {
		t.Errorf("%s: Short differs from the top-level registration", where)
	}
	subs := map[string]*cobra.Command{}
	for _, sub := range machine.Commands() {
		subs[sub.Name()] = sub
	}
	for _, sub := range top.Commands() {
		twin, ok := subs[sub.Name()]
		if !ok {
			t.Errorf("%s %s is missing", where, sub.Name())
			continue
		}
		delete(subs, sub.Name())
		compareRegistrations(t, sub, twin, selectors)
	}
	for name := range subs {
		t.Errorf("%s %s has no top-level twin", where, name)
	}
}

// flagNames is every flag a command declares, local and persistent, minus the
// ones the caller is willing to differ on.
//
// Name and shorthand are not enough. The builders take the target as an
// argument and already branch on it for help text, so a default or a
// description could just as easily be branched — and a `--limit` that defaults
// to 100 on one surface and 25 on the other is the same command in name only.
// Comparing DefValue and Usage catches that on the day it is written.
func flagNames(cmd *cobra.Command, skip map[string]bool) []string {
	var names []string
	collect := func(f *pflag.Flag) {
		if !skip[f.Name] {
			names = append(names, fmt.Sprintf("%s/%s default=%q usage=%q", f.Name, f.Shorthand, f.DefValue, f.Usage))
		}
	}
	cmd.Flags().VisitAll(collect)
	cmd.PersistentFlags().VisitAll(collect)
	sort.Strings(names)
	return names
}

func equalNames(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
}

// Every top-level command that acts on an Airflow reaches the same two flags,
// spelled the same way. One registration is what keeps the families from
// drifting into three spellings of one idea.
func TestQueryFamiliesShareTheSelectorFlags(t *testing.T) {
	d, _ := testDeps(t)
	top := afGroup(t, NewRootCmd(d))
	for _, family := range []string{"dags", "runs", "tasks", "assets", "connections", "variables", "pools", "health"} {
		cmd, _, err := top.Find([]string{family})
		if err != nil {
			t.Errorf("astro af %s is missing", family)
			continue
		}
		deployment := cmd.PersistentFlags().Lookup("deployment")
		if deployment == nil || deployment.Shorthand != "d" {
			t.Errorf("astro af %s: --deployment = %+v, want it registered with -d", family, deployment)
		}
		if cmd.PersistentFlags().Lookup("url") == nil {
			t.Errorf("astro af %s cannot reach --url", family)
		}
		for _, sub := range cmd.Commands() {
			if sub.InheritedFlags().Lookup("deployment") == nil {
				t.Errorf("astro af %s %s cannot reach -d", family, sub.Name())
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
	// The two that would break a consumer if they ever drifted: -o is the json
	// switch, and -d is the deployment selector everywhere, which is
	// why --dag-id gave the letter up.
	if got := meaning["o"]; got.flag != "output" {
		t.Errorf("-o is --%s, want --output", got.flag)
	}
	if got := meaning["d"]; got.flag != "deployment" {
		t.Errorf("-d is --%s, want --deployment", got.flag)
	}
}

// TestEveryReplacementNamesARealCommand ties the dev-removal table to the tree
// it points at. Nothing did before, so a row could name a command that was
// never built, or one that had since moved, and both compiled and passed.
//
// Airflow shipped exactly that bug and still has it: UPDATING.md says
// list_dags became dags list, the mapping table below it says list_dag, and
// the tombstones were generated from the table — so the command nobody types
// gets the helpful pointer and the real one gets a list of valid choices.
//
// This CLI has its own version of the same lesson. astro airflow was aliased
// to astro dev in 2019 with a deprecation notice, which worked; by the time
// astro airflow was removed the notice was gone, and it now answers with a
// bare unknown-command error.
func TestEveryReplacementNamesARealCommand(t *testing.T) {
	d, _ := testDeps(t)
	root := NewRootCmd(d)
	for _, m := range devReplacements() {
		if !strings.HasPrefix(m.Replacement, "astro ") {
			continue // uv run pytest, and anything else outside this tree
		}
		var path []string
		for _, word := range strings.Fields(strings.TrimPrefix(m.Replacement, "astro ")) {
			if strings.HasPrefix(word, "-") {
				break // astro local stop --clean
			}
			path = append(path, word)
		}
		found, _, err := root.Find(path)
		if err != nil || found == nil || found.CommandPath() != "astro "+strings.Join(path, " ") {
			t.Errorf("astro dev %s points at %q, which does not resolve; the table and the tree disagree",
				m.Command, m.Replacement)
		}
	}
}

// TestOldAirflowSpellingStillPoints covers the spelling this CLI reassigned.
// astro airflow meant the local project in 2019 and became astro dev; today it
// is the alias for the af group, so an old script asking for astro airflow
// start was answered with an unknown-command error naming astro af — a
// different thing entirely, and no route to astro local start.
func TestOldAirflowSpellingStillPoints(t *testing.T) {
	d, stdout := testDeps(t)
	err := execute(t, d, "airflow", "start")
	if err == nil {
		t.Fatal("astro airflow start should fail")
	}
	combined := stdout.String() + err.Error()
	if !strings.Contains(combined, "astro local start") {
		t.Errorf("astro airflow start should name astro local start; got %q", combined)
	}
}

// The e2e suite matches running proxy daemons on this spelling, and cannot
// import it: e2e is its own module and deliberately requires nothing, so that a
// dependency the suite takes can never reach the shipped binary.
//
// Pinned from this side instead. The suite's leak census hard-codes the literal
// as proxyServeArg to find daemons a run started and failed to reap, and a
// rename would fail nothing over there — pgrep would simply stop matching, and
// the axis would report zero daemons forever. Renaming the constant is fine;
// changing what it spells means changing the census to match.
func TestServeSubcommandSpellingIsWhatE2EMatches(t *testing.T) {
	const spelledInE2E = "__proxy-serve"
	if proxydaemon.ServeSubcommand != spelledInE2E {
		t.Fatalf("ServeSubcommand is %q, but the e2e leak census matches proxy daemons on %q — "+
			"update its proxyServeArg or that axis silently stops finding anything",
			proxydaemon.ServeSubcommand, spelledInE2E)
	}
}
