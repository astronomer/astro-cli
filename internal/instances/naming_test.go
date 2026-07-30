package instances

import (
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// linkSet builds a manifest with the named links, for the naming cases where
// what a link points at does not matter.
func linkSet(names ...string) *manifest.Manifest {
	m := &manifest.Manifest{}
	m.Astro.Deployments = map[string]manifest.Link{}
	for _, n := range names {
		m.Astro.Deployments[n] = manifest.Link{Deployment: "clm" + n}
	}
	return m
}

// find returns one instance from everything the set can see, addressable or
// not, so a test can assert on a shadowed row.
func find(s Set, name string) (Instance, bool) {
	all := s.All()
	for i := range all {
		if all[i].Name == name {
			return all[i], true
		}
	}
	return Instance{}, false
}

// TestForeignProjectNamedLocalNeverTakesTheReservedName: a project directory
// can be called anything, including `local`. The reserved name means "this
// project's own Airflow" or it means nothing — resolving it to a neighbor's
// Airflow would act on the wrong machine's worth of DAGs.
func TestForeignProjectNamedLocalNeverTakesTheReservedName(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Manifest:    linkSet("dev"),
		Running:     []Local{{ProjectPath: "/home/ana/dev/local", Port: 8081}},
	})
	if _, ok := set.Lookup(LocalName); ok {
		t.Error("a foreign project answers to the reserved name")
	}
	// Not addressable, but not silently gone either.
	it, listed := find(set, LocalName)
	if !listed || it.Problem == "" {
		t.Fatalf("the foreign Airflow is missing from the listing: %+v", it)
	}
	if !strings.Contains(it.Problem, "--url http://localhost:8081") {
		t.Errorf("problem does not say how to reach it: %q", it.Problem)
	}

	// And nothing about it wins by default.
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "dev" {
		t.Fatalf("selected %s from %s, want the project's own default link", sel.Instance.Name, sel.From)
	}

	// The pin `astro use local` writes points at this project, so with nothing
	// running it is a start, not a neighbor.
	_, err = set.Select(Request{Pin: LocalName})
	var notRunning *NotRunningError
	if !errors.As(err, &notRunning) {
		t.Fatalf("pinned local resolved to %v", err)
	}
}

// TestPinnedLocalWithNothingRunningNamesTheRightFix: `astro use local` is
// allowed before anything runs, so the error every command then gives has to
// point at starting Airflow — clearing the pin is the fix for a different
// problem.
func TestPinnedLocalWithNothingRunningNamesTheRightFix(t *testing.T) {
	set := Build(Inputs{ProjectPath: "/w/orders", Manifest: linkSet("dev", "prod")})
	_, err := set.Select(Request{Pin: LocalName})
	var notRunning *NotRunningError
	if !errors.As(err, &notRunning) {
		t.Fatalf("err = %v, want a NotRunningError", err)
	}
	if !strings.Contains(err.Error(), "astro local start") {
		t.Errorf("message does not name the fix: %s", err)
	}
	// The pin is what asked, so clearing it is offered second.
	if !strings.Contains(err.Error(), "astro use") {
		t.Errorf("message does not offer the pin's own escape: %s", err)
	}

	// Asked for by flag instead, the pin's escape is not the fix and is left out.
	_, err = set.Select(Request{Flag: LocalName})
	if !errors.As(err, &notRunning) {
		t.Fatalf("err = %v, want a NotRunningError", err)
	}
	if strings.Contains(err.Error(), "--unset") {
		t.Errorf("a -i local run was told to clear a pin it did not use: %s", err)
	}
}

// TestSameBaseNameKeepsBothVisible: two projects whose directories share a
// name. Only one can answer to it, but `astro instance list` promises every
// running Airflow, so the other is listed with the reason and a way in.
func TestSameBaseNameKeepsBothVisible(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Running: []Local{
			{ProjectPath: "/a/etl", Port: 8081},
			{ProjectPath: "/b/etl", Port: 8082},
		},
	})
	if got := set.Names(); len(got) != 1 || got[0] != "etl" {
		t.Fatalf("addressable names = %v, want one etl", got)
	}
	it, _ := set.Lookup("etl")
	if it.Project != "/a/etl" {
		t.Errorf("the sorted-first project lost the name: %s", it.Project)
	}
	if got := len(set.All()); got != 2 {
		t.Fatalf("%d of 2 running Airflows are visible", got)
	}
	for _, other := range set.All() {
		if other.Project != "/b/etl" {
			continue
		}
		if !strings.Contains(other.Problem, "/a/etl") || !strings.Contains(other.Problem, "--url http://localhost:8082") {
			t.Errorf("the shadowed row does not explain itself: %q", other.Problem)
		}
	}
}

// TestManifestLinkKeepsItsNameOverAForeignAirflow: a link is what the team
// wrote down for this project; a neighbor's directory name is an accident.
func TestManifestLinkKeepsItsNameOverAForeignAirflow(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Manifest:    linkSet("etl"),
		Running:     []Local{{ProjectPath: "/a/etl", Port: 8081}},
	})
	it, ok := set.Lookup("etl")
	if !ok || it.Source != SourceManifest {
		t.Fatalf("etl = %+v, want the manifest link", it)
	}
	shadow, listed := find(set, "etl")
	if !listed {
		t.Fatal("the foreign Airflow vanished")
	}
	_ = shadow
	if got := len(set.All()); got != 2 {
		t.Fatalf("%d rows, want the link and the shadowed Airflow", got)
	}
}

// TestForeignAirflowsNeverWinByDefault: another project's Airflow is
// addressable by name and listed, but it is never what a bare command falls
// through to, and never what a prompt offers.
func TestForeignAirflowsNeverWinByDefault(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Running:     []Local{{ProjectPath: "/w/billing", Port: 8081}},
	})
	// Addressable by name.
	if sel, err := set.Select(Request{Flag: "billing"}); err != nil || sel.Instance.Name != "billing" {
		t.Fatalf("select by name = %+v, %v", sel, err)
	}
	// But nothing of this project's own exists, so a bare run says so rather
	// than quietly acting on the neighbor.
	_, err := set.Select(Request{})
	if !errors.Is(err, ErrNone) {
		t.Fatalf("err = %v, want ErrNone", err)
	}
	if !strings.Contains(err.Error(), "billing") {
		t.Errorf("message hides the Airflows that are running: %s", err)
	}
}

// TestOneOwnInstanceIsNeverAmbiguous: with a single instance to choose from
// there is nothing to ask about, whatever else is running on the machine.
func TestOneOwnInstanceIsNeverAmbiguous(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Manifest:    linkSet("dev"),
		Running:     []Local{{ProjectPath: "/w/billing", Port: 8081}},
	})
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "dev" {
		t.Fatalf("selected %s, want the one link", sel.Instance.Name)
	}
}

// TestPromptOffersOnlyThisProjectsInstances: the fall-through prompt writes its
// answer to the pin, so offering a neighbor's Airflow there would make it this
// project's standing default.
func TestPromptOffersOnlyThisProjectsInstances(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Manifest:    linkSet("dev", "prod"),
		Running:     []Local{{ProjectPath: "/w/billing", Port: 8081}},
	})
	_, err := set.Select(Request{})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want an AmbiguousError", err)
	}
	for _, name := range ambiguous.Choices {
		if name == "billing" {
			t.Fatalf("the prompt offers another project's Airflow: %v", ambiguous.Choices)
		}
	}
	if len(ambiguous.Choices) != 2 {
		t.Fatalf("choices = %v, want the two links", ambiguous.Choices)
	}
}

// TestOwnAirflowKeepsTheReservedNameOverAForeignOne covers both at once: a
// neighbor called `local` and this project's own Airflow running.
func TestOwnAirflowKeepsTheReservedNameOverAForeignOne(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Running: []Local{
			{ProjectPath: "/home/ana/dev/local", Port: 8081},
			{ProjectPath: "/w/orders", Port: 8080},
		},
	})
	it, ok := set.Lookup(LocalName)
	if !ok || it.Project != "/w/orders" {
		t.Fatalf("local = %+v, want this project's own Airflow", it)
	}
	sel, err := set.Select(Request{})
	if err != nil || sel.From != LayerRunning || sel.Instance.Project != "/w/orders" {
		t.Fatalf("select = %+v, %v", sel, err)
	}
}

// TestExplainSeparatesDetailFromFault: a layer naming something gone is a
// problem; a layer naming something real carries its coordinate. They are
// different columns because they mean opposite things.
func TestExplainSeparatesDetailFromFault(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: "/w/orders",
		Manifest:    linkSet("dev"),
		Running:     []Local{{ProjectPath: "/w/orders", Port: 8080}},
	})
	rows, _, _ := set.Explain(Request{Pin: "deleted"})
	for _, row := range rows {
		switch row.Layer {
		case LayerPin:
			if row.Problem == "" || row.Where != "" {
				t.Errorf("a pin naming nothing = %+v, want a problem and no coordinate", row)
			}
		case LayerRunning:
			if row.Where == "" || row.Problem != "" {
				t.Errorf("the running local = %+v, want its coordinate and no problem", row)
			}
		case LayerDefault, LayerEnv, LayerFlag, LayerURL:
		}
	}

	// A pin naming the reserved name with nothing running is its own problem,
	// not "names no instance".
	rows, _, _ = Build(Inputs{ProjectPath: "/w/orders", Manifest: linkSet("dev")}).Explain(Request{Pin: LocalName})
	for _, row := range rows {
		if row.Layer == LayerPin && !strings.Contains(row.Problem, "no local Airflow is running") {
			t.Errorf("pinned local with nothing running = %+v", row)
		}
	}
}
