package instances

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// parseManifest builds a manifest from its real TOML, so the tests see the
// links exactly as a user's pyproject.toml produces them — kinds and auth
// defaults included.
func parseManifest(t *testing.T, body string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse([]byte("[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\nworkspace = 'ws_abc123'\n" + body))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

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

func TestBuildCoversEveryLinkKindAndTheRunningLocals(t *testing.T) {
	project := filepath.Join(t.TempDir(), "orders")
	other := filepath.Join(t.TempDir(), "billing")
	set := Build(Inputs{
		ProjectPath: project,
		Manifest:    parseManifest(t, fourKinds),
		Running: []Local{
			{ProjectPath: project, Port: 8080},
			{ProjectPath: other, Port: 8081},
		},
	})

	want := map[string]struct {
		kind   Kind
		source Source
		where  string
	}{
		"prod":          {KindAstro, SourceManifest, "deployment clm2xk9dq000108l7a2b3c4d5"},
		"prod-mwaa":     {KindMWAA, SourceManifest, "environment orders-prod"},
		"prod-composer": {KindComposer, SourceManifest, "environment orders-composer"},
		"staging":       {KindEndpoint, SourceManifest, "https://airflow.staging.corp.dev"},
		"local":         {KindLocal, SourceRunning, "http://localhost:8080"},
		"billing":       {KindLocal, SourceRunning, "http://localhost:8081"},
	}
	if got := len(set.All()); got != len(want) {
		t.Fatalf("set has %d instances, want %d: %v", got, len(want), set.Names())
	}
	for name, exp := range want {
		it, ok := set.Lookup(name)
		if !ok {
			t.Fatalf("%s missing from the set: %v", name, set.Names())
		}
		if it.Kind != exp.kind || it.Source != exp.source || it.Where != exp.where {
			t.Errorf("%s = {%s %s %q}, want {%s %s %q}", name, it.Kind, it.Source, it.Where, exp.kind, exp.source, exp.where)
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

// TestBuildReservesTheLocalName covers the collision the manifest cannot stop:
// it refuses a link named `local`, but nothing stops another project's
// directory from being called that.
func TestBuildReservesTheLocalName(t *testing.T) {
	project := filepath.Join(t.TempDir(), "orders")
	impostor := filepath.Join(t.TempDir(), "local")
	set := Build(Inputs{
		ProjectPath: project,
		Manifest:    parseManifest(t, twoLinks),
		Running:     []Local{{ProjectPath: impostor, Port: 8081}, {ProjectPath: project, Port: 8080}},
	})
	it, ok := set.Lookup(LocalName)
	if !ok {
		t.Fatal("no local instance")
	}
	if it.Project != project || it.Where != "http://localhost:8080" {
		t.Fatalf("local = %+v, want this project's own Airflow", it)
	}
}

// TestManifestRefusesTheReservedName pins the other half of the rule, which
// pkg/manifest enforces: a link may not take the name resolution keeps for the
// local Airflow. The manifest spells that name in its own package, so the test
// builds the link from LocalName — if the two ever disagree, this fails rather
// than leaving a link nobody can address.
func TestManifestRefusesTheReservedName(t *testing.T) {
	_, err := manifest.Parse([]byte("[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\nworkspace = 'ws_abc123'\n\n[tool.astro.deployments." + LocalName + "]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n"))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v, want the reserved-name refusal", err)
	}
}

func TestBuildNamesOtherProjectsAndKeepsTheFirstOnACollision(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a", "orders")
	b := filepath.Join(t.TempDir(), "b", "orders")
	first, second := a, b
	if b < a {
		first, second = b, a
	}
	set := Build(Inputs{
		ProjectPath: filepath.Join(t.TempDir(), "elsewhere"),
		Running:     []Local{{ProjectPath: second, Port: 8081}, {ProjectPath: first, Port: 8080}},
	})
	if got := set.Names(); len(got) != 1 || got[0] != "orders" {
		t.Fatalf("names = %v, want just orders", got)
	}
	it, _ := set.Lookup("orders")
	if it.Project != first {
		t.Fatalf("orders resolved to %s, want the first path %s", it.Project, first)
	}
}

func TestBuildWithoutAProjectOrAManifest(t *testing.T) {
	set := Build(Inputs{})
	if len(set.All()) != 0 {
		t.Fatalf("empty inputs built %v", set.Names())
	}
}
