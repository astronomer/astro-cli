package instances

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// twoLinks is the shape that reaches every layer: a default link, a second
// link, and room for a running local Airflow.
const twoLinks = `
[tool.astro.deployments.dev]
deployment = 'clm2xk9dq000108l7a2b3c4d5'
default = true

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d6'
`

// fullSet is every layer at once: two links (one default) plus this project's
// running Airflow, so each precedence test only has to say what it passes.
func fullSet(t *testing.T) Set {
	t.Helper()
	project := filepath.Join(t.TempDir(), "orders")
	return Build(Inputs{
		ProjectPath: project,
		Manifest:    parseManifest(t, twoLinks),
		Running:     []Local{{ProjectPath: project, Port: 8080}},
	})
}

// TestPrecedence walks the rule one step at a time: each case supplies one
// more layer than the last and must beat it.
func TestPrecedence(t *testing.T) {
	set := fullSet(t)
	cases := []struct {
		name string
		req  Request
		want string
		from Layer
	}{
		{"nothing said falls to the running local", Request{}, LocalName, LayerRunning},
		{"the pin beats what is running", Request{Pin: "prod"}, "prod", LayerPin},
		{"the env beats the pin", Request{Pin: "prod", Env: "dev"}, "dev", LayerEnv},
		{"the flag beats the env", Request{Pin: "prod", Env: "dev", Flag: LocalName}, LocalName, LayerFlag},
		{"--url beats everything and needs no name", Request{Pin: "prod", Env: "dev", URL: "https://airflow.corp.dev"}, "https://airflow.corp.dev", LayerURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sel, err := set.Select(tc.req)
			if err != nil {
				t.Fatalf("select: %v", err)
			}
			if sel.Instance.Name != tc.want || sel.From != tc.from {
				t.Fatalf("selected %s from %s, want %s from %s", sel.Instance.Name, sel.From, tc.want, tc.from)
			}
		})
	}
}

func TestDefaultLinkIsTheFloor(t *testing.T) {
	project := filepath.Join(t.TempDir(), "orders")
	// Nothing running: the marked default link is what is left.
	set := Build(Inputs{ProjectPath: project, Manifest: parseManifest(t, twoLinks)})
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "dev" || sel.From != LayerDefault {
		t.Fatalf("selected %s from %s, want dev from the default link", sel.Instance.Name, sel.From)
	}
}

func TestLoneLinkIsItsOwnDefault(t *testing.T) {
	set := Build(Inputs{
		ProjectPath: filepath.Join(t.TempDir(), "orders"),
		Manifest:    parseManifest(t, "\n[tool.astro.deployments.only]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n"),
	})
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "only" || sel.From != LayerDefault {
		t.Fatalf("selected %s from %s, want only from the default link", sel.Instance.Name, sel.From)
	}
}

func TestSelectRefusesInstanceAndURLTogether(t *testing.T) {
	set := fullSet(t)
	if _, err := set.Select(Request{Flag: "dev", URL: "https://airflow.corp.dev"}); !errors.Is(err, ErrMutuallyExclusive) {
		t.Fatalf("select: %v, want the mutually-exclusive error", err)
	}
}

func TestUnknownNamesPointAtTheLayerThatHoldsThem(t *testing.T) {
	set := fullSet(t)
	cases := []struct {
		req  Request
		want string
	}{
		{Request{Flag: "nope"}, `no instance named "nope"; known instances: dev, local, prod`},
		{Request{Env: "nope"}, `no instance named "nope" (from ASTRO_INSTANCE); known instances: dev, local, prod`},
		{Request{Pin: "nope"}, `no instance named "nope" (pinned for this project; clear it with ` + "`astro use --unset`" + `); known instances: dev, local, prod`},
	}
	for _, tc := range cases {
		_, err := set.Select(tc.req)
		var unknown *UnknownError
		if !errors.As(err, &unknown) {
			t.Fatalf("select: %v, want an UnknownError", err)
		}
		if err.Error() != tc.want {
			t.Errorf("message =\n  %s\nwant\n  %s", err, tc.want)
		}
	}
}

func TestAmbiguousNamesEveryWayToDecide(t *testing.T) {
	// Two links, neither marked default, nothing running: the fall-through.
	set := Build(Inputs{
		ProjectPath: filepath.Join(t.TempDir(), "orders"),
		Manifest: parseManifest(t, `
[tool.astro.deployments.dev]
deployment = 'clm2xk9dq000108l7a2b3c4d5'

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d6'
`),
	})
	_, err := set.Select(Request{})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("select: %v, want an AmbiguousError", err)
	}
	if got := ambiguous.Choices; len(got) != 2 || got[0] != "dev" || got[1] != "prod" {
		t.Fatalf("choices = %v, want dev and prod", got)
	}
	for _, want := range []string{"-i <name>", EnvVar, "astro use <name>"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}
}

func TestNothingToActOnAtAll(t *testing.T) {
	set := Build(Inputs{ProjectPath: filepath.Join(t.TempDir(), "orders"), Manifest: parseManifest(t, "")})
	if _, err := set.Select(Request{}); !errors.Is(err, ErrNone) {
		t.Fatalf("select: %v, want ErrNone", err)
	}
}

func TestExplainShowsEveryLayerAndTheWinner(t *testing.T) {
	set := fullSet(t)
	rows, sel, err := set.Explain(Request{Pin: "prod"})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if sel.Instance.Name != "prod" {
		t.Fatalf("selection = %s, want prod", sel.Instance.Name)
	}
	want := []struct {
		layer Layer
		value string
		wins  bool
	}{
		{LayerEnv, "", false},
		{LayerPin, "prod", true},
		{LayerRunning, LocalName, false},
		{LayerDefault, "dev", false},
	}
	if len(rows) != len(want) {
		t.Fatalf("explained %d layers, want %d", len(rows), len(want))
	}
	for i, exp := range want {
		if rows[i].Layer != exp.layer || rows[i].Value != exp.value || rows[i].Wins != exp.wins {
			t.Errorf("row %d = %+v, want {%s %q wins=%v}", i, rows[i], exp.layer, exp.value, exp.wins)
		}
	}
}

func TestExplainFlagsALayerNamingSomethingUnknown(t *testing.T) {
	set := fullSet(t)
	rows, _, err := set.Explain(Request{Pin: "gone"})
	if err == nil {
		t.Fatal("a pin naming nothing resolved")
	}
	for _, row := range rows {
		if row.Layer != LayerPin {
			continue
		}
		if row.Problem == "" || row.Wins {
			t.Fatalf("stale pin row = %+v, want a note and no win", row)
		}
	}
}
