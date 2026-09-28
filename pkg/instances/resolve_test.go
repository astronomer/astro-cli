package instances

import (
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// twoLinks is the shape that reaches every layer: a default link and a second
// one.
const twoLinks = `
[tool.astro.deployments.dev]
deployment = 'clm2xk9dq000108l7a2b3c4d5'
default = true

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d6'
`

// fullSet is every layer at once: two links, one of them the default, so each
// precedence test only has to say what it passes.
func fullSet(t *testing.T) Set {
	t.Helper()
	return Build(instancestest.Manifest(t, twoLinks))
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
		{"nothing said falls to the default link", Request{}, "dev", LayerDefault},
		{"the pin beats the default link", Request{Pin: "prod"}, "prod", LayerPin},
		{"the env beats the pin", Request{Pin: "prod", Env: "dev"}, "dev", LayerEnv},
		{"the flag beats the env", Request{Pin: "dev", Env: "dev", Flag: "prod"}, "prod", LayerFlag},
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

// TestLocalIsNotASelectableName covers the layer that left the rule. Every way
// of saying `local` — a flag, the exported variable, a pin an older release
// wrote — is answered with where the machine went, not with a name listing.
func TestLocalIsNotASelectableName(t *testing.T) {
	set := fullSet(t)
	for _, req := range []Request{{Flag: LocalName}, {Env: LocalName}, {Pin: LocalName}} {
		_, err := set.Select(req)
		if !errors.Is(err, ErrLocalNotADeployment) {
			t.Fatalf("select(%+v) = %v, want the machine's own commands named", req, err)
		}
	}
	if !strings.Contains(ErrLocalNotADeployment.Error(), "astro local af dags list") {
		t.Errorf("the refusal does not show the new spelling: %s", ErrLocalNotADeployment)
	}
}

func TestLoneLinkIsItsOwnDefault(t *testing.T) {
	set := Build(instancestest.Manifest(t, "\n[tool.astro.deployments.only]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n"))
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "only" || sel.From != LayerDefault {
		t.Fatalf("selected %s from %s, want only from the default link", sel.Instance.Name, sel.From)
	}
}

func TestSelectRefusesDeploymentAndURLTogether(t *testing.T) {
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
		{Request{Env: "nope"}, `no deployment named "nope" (from ASTRO_DEPLOYMENT); known deployments: dev, prod`},
		{Request{Pin: "nope"}, `no deployment named "nope" (pinned for this project; clear it with ` + "`astro use --unset`" + `); known deployments: dev, prod`},
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

// -d takes a Deployment id as well as a link name, as `astro deploy
// --deployment` does. A link pointing at that Deployment answers for it; an id
// no link carries is the Deployment alone, reached with the Astro login.
func TestFlagFallsThroughToADeploymentID(t *testing.T) {
	set := fullSet(t)

	sel, err := set.Select(Request{Flag: "clm2xk9dq000108l7a2b3c4d6"})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "prod" || sel.From != LayerFlag {
		t.Fatalf("selected %s from %s, want the prod link from the flag", sel.Instance.Name, sel.From)
	}

	sel, err = set.Select(Request{Flag: "cexampledeployment0000002"})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	got := sel.Instance
	if got.Name != "cexampledeployment0000002" || got.Kind != KindAstro || got.Source != SourceDeploymentID ||
		got.Link.Deployment != "cexampledeployment0000002" || got.Link.Auth.Method != manifest.AuthAstro {
		t.Fatalf("an unlinked id resolved to %+v", got)
	}
}

func TestAmbiguousNamesEveryWayToDecide(t *testing.T) {
	// Two links, neither marked default: the fall-through.
	set := Build(instancestest.Manifest(t, `
[tool.astro.deployments.dev]
deployment = 'clm2xk9dq000108l7a2b3c4d5'

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d6'
`))
	_, err := set.Select(Request{})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("select: %v, want an AmbiguousError", err)
	}
	if got := ambiguous.Choices; len(got) != 2 || got[0] != "dev" || got[1] != "prod" {
		t.Fatalf("choices = %v, want dev and prod", got)
	}
	// The machine's spelling is left to the command layer here: this project
	// has real deployments to choose between, and the caller appends its own
	// `astro local <family>` form (see localForm in cmd/local).
	for _, want := range []string{"-d <name>", EnvVar, "astro use <name>"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}
}

// TestNothingToActOnNamesBothSpellings: a project with no links resolves to
// nothing, and the message has to name the machine's spelling as well as the
// deployment one — the split's whole promise is that neither world is reached
// by accident, so neither may be hidden either.
func TestNothingToActOnNamesBothSpellings(t *testing.T) {
	set := Build(instancestest.Manifest(t, ""))
	_, err := set.Select(Request{})
	if !errors.Is(err, ErrNone) {
		t.Fatalf("select: %v, want ErrNone", err)
	}
	for _, want := range []string{"astro local", "--url", "[tool.astro.deployments]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
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

// TestExplainSeparatesDetailFromFault: a layer naming something gone is a
// problem; a layer naming something real carries its coordinate. They are
// different columns because they mean opposite things.
func TestExplainSeparatesDetailFromFault(t *testing.T) {
	set := fullSet(t)
	rows, _, err := set.Explain(Request{Pin: "gone"})
	if err == nil {
		t.Fatal("a pin naming nothing resolved")
	}
	for _, row := range rows {
		switch row.Layer {
		case LayerPin:
			if row.Problem == "" || row.Where != "" || row.Wins {
				t.Errorf("a pin naming nothing = %+v, want a problem, no coordinate, and no win", row)
			}
		case LayerDefault:
			if row.Where == "" || row.Problem != "" {
				t.Errorf("the default link = %+v, want its coordinate and no problem", row)
			}
		case LayerEnv, LayerFlag, LayerURL:
		}
	}

	// A pin naming the reserved word is its own problem, with its own fix — not
	// "names no deployment".
	rows, _, _ = set.Explain(Request{Pin: LocalName})
	for _, row := range rows {
		if row.Layer == LayerPin && !strings.Contains(row.Problem, "astro local") {
			t.Errorf("a pin naming the machine = %+v", row)
		}
	}
}
