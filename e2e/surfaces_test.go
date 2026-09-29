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

// Bare `astro use` off a terminal: the linked Deployments, which one is
// current, and what made it current.
//
// The two states are separate cases because they publish different fields and
// each hides the other's. A project resolving to nothing fills `reason` and
// leaves `current` empty; one that resolves fills `current` and `from` and
// leaves `reason` empty — they are the two arms of one branch in runUseShow,
// so a fixture that only reaches one silently pins the test to it.

// useListingPayload is the report, written out rather than imported: asking
// the implementation what it promises cannot catch a promise being broken.
type useListingPayload struct {
	Current     string `json:"current"`
	From        string `json:"from"`
	Reason      string `json:"reason"`
	Deployments []struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Where   string `json:"where"`
		Current bool   `json:"current"`
	} `json:"deployments"`
}

func useListing(t *testing.T, p *project) useListingPayload {
	t.Helper()
	var res useListingPayload
	p.run("use", "--output", "json").requireSuccess().requireJSON(&res)
	return res
}

func TestUseSaysWhyWhenNothingIsCurrent(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "unlinked").requireSuccess()

	res := useListing(t, p)
	if res.Current != "" {
		t.Errorf("nothing is linked, so nothing should be current; got %q", res.Current)
	}
	if res.Reason == "" {
		t.Error("with nothing current the report owes a reason, and published none")
	}
	if len(res.Deployments) != 0 {
		t.Errorf("an unlinked project listed deployments: %+v", res.Deployments)
	}
}

func TestUseNamesTheCurrentDeploymentAndWhy(t *testing.T) {
	tier(t, 0)

	p := linkedProject(t, "prod")
	res := useListing(t, p)

	if res.Current != "prod" || res.Reason != "" {
		t.Fatalf("a linked project should resolve to its default; current=%q reason=%q", res.Current, res.Reason)
	}
	// `from` is the half a bare name leaves out: which of the environment,
	// your selection and the manifest default supplied it.
	if res.From != "default" {
		t.Errorf("from = %q, want default", res.From)
	}
	if len(res.Deployments) != 1 || !res.Deployments[0].Current || res.Deployments[0].Where == "" {
		t.Errorf("deployments = %+v, want prod, current, with its coordinate", res.Deployments)
	}
}

func TestUseTextMarksTheCurrentDeployment(t *testing.T) {
	tier(t, 0)

	p := linkedProject(t, "prod")
	out := p.run("use").requireSuccess().Stdout

	// The header prints before any row, so a header-only assertion passes on
	// an empty listing. The marked row is what proves rows reached the writer.
	marked := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "*") && strings.Contains(line, "prod") && strings.Contains(line, "← default = true") {
			marked = true
		}
	}
	if !marked {
		t.Errorf("the listing should mark prod as current by the manifest default\n%s", out)
	}
	if strings.Contains(out, "LAYER") {
		t.Errorf("the listing still prints the resolution rule\n%s", out)
	}
}

// The payload is pinned in cmd/local/testdata/schema/use-listing.json.
// Holding real output against it catches a key this file does not decode
// being renamed or dropped.
func TestUseListingPublishesNoUnpinnedKeys(t *testing.T) {
	tier(t, 0)

	p := linkedProject(t, "prod")
	pinned := pinnedKeys(t, "use-listing")

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

// linkedProject scaffolds a project with one default deployment link, so
// there is something to resolve.
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
