//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// `astro af` against a project that links no deployment.
//
// The refusal is the feature. A project with no [tool.astro.deployments] has
// nothing for a deployment command to act on, and the two ways forward are
// not obvious: name an Airflow directly with --url, or use the machine's own
// tree. A bare "no deployment found" would leave somebody guessing at both.
//
// Tier 0: resolution fails before anything is contacted.
func TestAfWithNoLinksSaysWhatToDoInstead(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "unlinked").requireSuccess()

	out := p.run("af", "dags", "list").requireFailure().output()

	for _, want := range []string{
		// What is wrong, in the project's own vocabulary.
		"[tool.astro.deployments]",
		// The two ways out, both of which the message owes the reader. The
		// second is the command FAMILY, not the bare words "astro local":
		// the resolver's own sentence already says that much, and the half
		// that names `af dags` is appended by the command layer, which is
		// the only side that knows what this one is called. Asserting the
		// shorter string would pass with that half deleted.
		"--url",
		"astro local af dags",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal should mention %q\n%s", want, out)
		}
	}
}

// `local` is reserved, not unknown.
//
// Somebody who writes `astro use local` means this machine, and the machine
// has its own tree. Told "unknown deployment" they would go looking for one
// to create; the message sends them to `astro local` instead.
func TestUseRefusesLocalAsADeploymentName(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "pinned").requireSuccess()

	out := p.run("use", "local").requireFailure().output()

	if !strings.Contains(out, "is not a deployment") {
		t.Errorf("`local` is reserved rather than unknown, and the message should say so\n%s", out)
	}
	if !strings.Contains(out, "astro local start") {
		t.Errorf("the refusal should name the tree that does mean this machine\n%s", out)
	}
}

// The resolution ladder: every layer that could name a deployment, which one
// won, and where it came from.
//
// It is the answer to "why is it deploying to that": a single resolved name
// would leave somebody to guess which of the environment, the pin, and the
// manifest default had supplied it.
//
// The two states are separate cases because they publish different fields and
// each hides the other's. A project resolving to nothing fills `reason` and
// leaves `winner` empty; one that resolves fills `winner` and leaves `reason`
// empty — they are the two arms of one branch in runUse, so a fixture that
// only reaches one silently pins the test to it.

// ladderLayers are the three Explain always publishes, in precedence order.
// That order IS the contract: asserting merely that rows exist cannot fail,
// because Explain builds all three before it looks anything up.
var ladderLayers = []string{"env", "pin", "default"}

func TestUseLadderSaysWhyWhenNothingResolves(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "unlinked").requireSuccess()

	res := ladder(t, p)
	assertLadderLayers(t, res.Layers)

	if res.Winner != "" {
		t.Errorf("nothing is linked, so nothing should win; got %q", res.Winner)
	}
	if res.Reason == "" {
		t.Error("with no winner the ladder owes a reason, and published none")
	}
	for _, row := range res.Layers {
		if row.Wins {
			t.Errorf("layer %q claims to win while the winner is empty", row.Layer)
		}
	}
}

func TestUseLadderNamesTheWinnerAndWhereItCameFrom(t *testing.T) {
	tier(t, 0)

	p := linkedProject(t, "prod")
	res := ladder(t, p)
	assertLadderLayers(t, res.Layers)

	if res.Winner == "" {
		t.Fatalf("a linked project should resolve; reason=%q", res.Reason)
	}
	won := false
	for _, row := range res.Layers {
		if !row.Wins {
			continue
		}
		won = true
		if row.Value == "" {
			t.Errorf("the winning layer publishes no value: %+v", row)
		}
		// WHERE is the half that says which file or variable supplied it,
		// and it is exactly what a bare resolved name leaves out.
		if row.Where == "" {
			t.Errorf("the winning layer does not say where it came from: %+v", row)
		}
	}
	if !won {
		t.Errorf("a winner was named (%q) but no layer is marked as winning", res.Winner)
	}
}

func TestUseLadderTextCarriesTheSameAnswer(t *testing.T) {
	tier(t, 0)

	p := linkedProject(t, "prod")
	out := p.run("use").requireSuccess().Stdout

	for _, col := range []string{"LAYER", "VALUE", "WHERE"} {
		if !strings.Contains(out, col) {
			t.Errorf("the ladder should have a %s column\n%s", col, out)
		}
	}
	// The headers print before any row is rendered, so a header-only
	// assertion passes on an empty ladder. The resolved name is what proves
	// rows reached the writer.
	if !strings.Contains(out, "prod") {
		t.Errorf("the rendered ladder should carry the resolved deployment\n%s", out)
	}
}

// The payload is pinned in cmd/local/testdata/schema/use-resolution.json.
// Holding real output against it catches a key this file does not decode —
// problem, and the whole instances array — being renamed or dropped.
func TestUseLadderPublishesNoUnpinnedKeys(t *testing.T) {
	tier(t, 0)

	p := linkedProject(t, "prod")
	pinned := pinnedKeys(t, "use-resolution")

	var emitted map[string]any
	p.run("use", "--output", "json").requireSuccess().requireJSON(&emitted)
	if len(emitted) == 0 {
		t.Fatal("published an empty object; there is nothing to check")
	}
	for k := range emitted {
		if !pinned[k] {
			t.Errorf("`astro use --output json` published %q, which is not in the pinned contract", k)
		}
	}
}

// ladderRow is one line of the ladder, written out rather than imported:
// asking the implementation what it promises cannot catch a promise being
// broken.
type ladderRow struct {
	Layer   string `json:"layer"`
	Value   string `json:"value"`
	Where   string `json:"where"`
	Problem string `json:"problem"`
	Wins    bool   `json:"wins"`
}

type resolutionPayload struct {
	Layers []ladderRow `json:"layers"`
	Winner string      `json:"winner"`
	Reason string      `json:"reason"`
}

func ladder(t *testing.T, p *project) resolutionPayload {
	t.Helper()
	var res resolutionPayload
	p.run("use", "--output", "json").requireSuccess().requireJSON(&res)
	return res
}

func assertLadderLayers(t *testing.T, got []ladderRow) {
	t.Helper()
	if len(got) != len(ladderLayers) {
		t.Fatalf("the ladder should publish %d layers (%v), got %d", len(ladderLayers), ladderLayers, len(got))
	}
	for i, name := range ladderLayers {
		if got[i].Layer != name {
			t.Errorf("layer %d is %q, want %q — the order is the precedence", i, got[i].Layer, name)
		}
	}
}

// linkedProject scaffolds a project with one default deployment link, so the
// ladder has something to resolve.
func linkedProject(t *testing.T, name string) *project {
	t.Helper()
	p := newProject(t)
	p.run("init", "--name", "linked").requireSuccess()

	path := filepath.Join(p.Dir, "pyproject.toml")
	// A workspace is required on the link or as a [tool.astro] default; the
	// manifest refuses a link without one, which is the validation doing its
	// job on a fixture that left it out.
	write(t, path, read(t, path)+"\n[tool.astro.deployments."+name+"]\n"+
		"deployment = \"cm0000000000000000000000\"\n"+
		"workspace = \"cm1111111111111111111111\"\n"+
		"default = true\n")
	return p
}

// Every command the root menu shows belongs to exactly one group.
//
// Held at unit level already, and thoroughly: cmd's
// TestEveryVisibleTopLevelCommandIsClassified walks both the cloud and
// software roots and fails a command with no commandGroup entry, and
// root_test asserts the rendered help carries the five titles and no
// "Additional Commands:".
//
// What this adds is the shipped binary. Those build a root in-process from
// rootsUnderTest; this one runs main, so it covers the wiring between them —
// and it is what the plan asked for, on the grounds that the menu is the
// first thing a person sees and the only check of it ran somewhere the
// person never goes.
func TestHelpMenuGroupsEveryVisibleCommand(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	out := p.run("--help").requireSuccess().Stdout

	// Cobra collects an ungrouped command under a trailing "Additional
	// Commands:" heading. Its presence IS the failure: something reached the
	// menu without anybody deciding where it belongs.
	if strings.Contains(out, "Additional Commands:") {
		t.Errorf("a command reached the root menu with no group; cobra collected it under "+
			"\"Additional Commands\". Give it a commandGroup entry in cmd.\n%s", out)
	}

	// And the menu has commands in it. Cobra prints a group's title whether
	// or not anything is under it, so the titles alone would pass on an
	// entirely empty menu — which is how a negative assertion usually goes
	// wrong. One command per group is what says the groups have contents.
	for title, cmd := range map[string]string{
		"Develop locally:": "local",
		"Inspect Airflow:": "af",
		"Ship:":            "deploy",
		"Manage Astro:":    "workspace",
		"Set up the CLI:":  "version",
	} {
		if !strings.Contains(out, title) {
			t.Errorf("the root menu should carry the %q group\n%s", title, out)
			continue
		}
		if !strings.Contains(out, cmd) {
			t.Errorf("the %q group should list %q\n%s", title, cmd, out)
		}
	}
}
