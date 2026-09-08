package instances

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// fourKinds is one link of every kind the manifest can carry.
const fourKinds = `
[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d5'
default = true

[tool.astro.deployments.prod-mwaa]
target = 'mwaa'
environment = 'orders-prod'

[tool.astro.deployments.prod-composer]
target = 'composer'
environment = 'orders-composer'

[tool.astro.deployments.staging]
url = 'https://airflow.staging.corp.dev'
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }
`

func TestBuildCoversEveryLinkKind(t *testing.T) {
	set := Build(instancestest.Manifest(t, fourKinds))

	want := map[string]struct {
		kind  Kind
		where string
	}{
		"prod":          {KindAstro, "deployment clm2xk9dq000108l7a2b3c4d5"},
		"prod-mwaa":     {KindMWAA, "environment orders-prod"},
		"prod-composer": {KindComposer, "environment orders-composer"},
		"staging":       {KindEndpoint, "https://airflow.staging.corp.dev"},
	}
	if got := len(set.All()); got != len(want) {
		t.Fatalf("set has %d deployments, want %d: %v", got, len(want), set.Names())
	}
	for name, exp := range want {
		it, ok := set.Lookup(name)
		if !ok {
			t.Fatalf("%s missing from the set: %v", name, set.Names())
		}
		if it.Kind != exp.kind || it.Source != SourceManifest || it.Where != exp.where {
			t.Errorf("%s = {%s %s %q}, want {%s manifest %q}", name, it.Kind, it.Source, it.Where, exp.kind, exp.where)
		}
	}

	// Sorted by name, so every rendering is stable.
	names := set.Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("names are not sorted: %v", names)
		}
	}
}

// TestBuildHoldsDeploymentsOnly is the local split as a structural fact: the
// machine never enters the set, so nothing a top-level command resolves can
// point at localhost.
func TestBuildHoldsDeploymentsOnly(t *testing.T) {
	set := Build(instancestest.Manifest(t, twoLinks))
	if _, ok := set.Lookup(LocalName); ok {
		t.Error("the machine is in the deployment set")
	}
	for _, it := range set.All() {
		if it.Kind == KindLocal || it.Source == SourceRunning {
			t.Errorf("%s is a local Airflow, not a deployment: %+v", it.Name, it)
		}
	}
}

// TestLocalInstanceIsBuiltNotResolved: the machine's Airflow is addressed by
// spelling the command `astro local …`, and this is the instance that spelling
// acts on.
func TestLocalInstanceIsBuiltNotResolved(t *testing.T) {
	own := LocalInstance(Local{ProjectPath: "/w/orders", Port: 8080, AirflowMajor: "3"}, LocalName)
	if own.Name != LocalName || own.Kind != KindLocal || own.Source != SourceRunning {
		t.Fatalf("own = %+v", own)
	}
	if own.URL != "http://localhost:8080" || own.Where != own.URL || own.Project != "/w/orders" || own.AirflowMajor != "3" {
		t.Fatalf("own = %+v, want the record's own coordinates", own)
	}
	// A record with no port has no URL to offer, which Transport reports.
	if got := LocalInstance(Local{ProjectPath: "/w/orders"}, LocalName); got.URL != "" {
		t.Fatalf("a portless record built the URL %q", got.URL)
	}
}

// TestManifestRefusesTheReservedName pins the naming hygiene the split keeps:
// a link may not be called `local`, because that word means the machine and
// `astro local …` is how you say it. The manifest spells that name in its own
// package, so the test builds the link from LocalName — if the two ever
// disagree, this fails rather than leaving a link nobody can address.
func TestManifestRefusesTheReservedName(t *testing.T) {
	_, err := manifest.Parse([]byte(instancestest.Preamble +
		"\n[tool.astro.deployments." + LocalName + "]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n"))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v, want the reserved-name refusal", err)
	}
}

func TestBuildWithoutAManifest(t *testing.T) {
	if set := Build(nil); len(set.All()) != 0 {
		t.Fatalf("a nil manifest built %v", set.Names())
	}
}
