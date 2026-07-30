package local

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// instanceProject writes a project whose manifest carries the given deployment
// links, and points the cache root at a scratch dir so the pin and any runtime
// records stay out of the user's machine.
func instanceProject(t *testing.T, links string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "orders")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\nworkspace = 'ws_abc123'\n" + links
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	t.Setenv(instances.EnvVar, "")
	return dir
}

// instanceDeps points the commands at dir, with running holding whatever local
// Airflows the case wants alive.
func instanceDeps(t *testing.T, dir string, running ...localrt.Status) (d Deps, stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	d, _ = testDeps(t)
	d.Stdout = stdout
	d.Stderr = stderr
	d.Runtime = stubRuntime{list: running}
	d.WorkingDir = func() (string, error) { return dir, nil }
	return d, stdout, stderr
}

// twoLinkManifest has a default link and a second one, so every layer of the
// rule has something to point at.
const twoLinkManifest = `
[tool.astro.deployments.dev]
deployment = 'clm2xk9dq000108l7a2b3c4d5'
default = true

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d6'
`

func TestUsePinsAndUnsets(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, out, errOut := instanceDeps(t, dir)
	if err := execute(t, d, "use", "prod"); err != nil {
		t.Fatal(err)
	}
	state, err := userstate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Instance != "prod" {
		t.Fatalf("pin = %q, want prod", state.Instance)
	}
	if !strings.Contains(out.String(), "pinned prod") {
		t.Fatalf("stdout = %q", out)
	}
	// The one deliberate stderr line: what the pin now resolves to.
	if !strings.Contains(errOut.String(), "→ prod (astro deployment clm2xk9dq000108l7a2b3c4d6)") {
		t.Fatalf("stderr = %q", errOut)
	}

	d, out, _ = instanceDeps(t, dir)
	if err := execute(t, d, "use", "--unset"); err != nil {
		t.Fatal(err)
	}
	state, err = userstate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Instance != "" {
		t.Fatalf("pin = %q after --unset", state.Instance)
	}
	if !strings.Contains(out.String(), "cleared") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestUseRefusesANameTheProjectDoesNotKnow(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir)
	err := execute(t, d, "use", "nope")
	if err == nil || !strings.Contains(err.Error(), "known instances: dev, prod") {
		t.Fatalf("err = %v, want one listing the known names", err)
	}
}

func TestUseLocalPinsBeforeAnythingIsRunning(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use", "local"); err != nil {
		t.Fatalf("pinning the reserved name failed: %v", err)
	}
	state, err := userstate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Instance != instances.LocalName {
		t.Fatalf("pin = %q, want local", state.Instance)
	}
}

func TestBareUseShowsTheWholeRule(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	if err := savePin(dir, "prod"); err != nil {
		t.Fatal(err)
	}
	d, out, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"ASTRO_INSTANCE", "astro use", "running local Airflow", "manifest default link", "resolves to prod"} {
		if !strings.Contains(text, want) {
			t.Errorf("table does not show %q:\n%s", want, text)
		}
	}
}

func TestBareUseJSONCarriesEveryLayerAndTheWinner(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	t.Setenv(instances.EnvVar, "dev")
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Layers []struct {
			Layer, Value string
			Wins         bool
		}
		Winner string
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Winner != "dev" || len(got.Layers) != 4 {
		t.Fatalf("json = %+v", got)
	}
	if got.Layers[0].Layer != "env" || got.Layers[0].Value != "dev" || !got.Layers[0].Wins {
		t.Fatalf("env layer = %+v, want it winning with dev", got.Layers[0])
	}
}

func TestInstanceListMergesLinksAndRunningAirflows(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	other := filepath.Join(t.TempDir(), "billing")
	d, out, _ := instanceDeps(t, dir,
		localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080},
		localrt.Status{ProjectPath: other, State: localrt.StateRunning, Port: 8081},
		// A record whose runtime is gone is not a running instance.
		localrt.Status{ProjectPath: filepath.Join(t.TempDir(), "stale"), State: localrt.StateStopped, Port: 8082},
	)
	if err := execute(t, d, "instance", "list"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"dev", "prod", "local", "billing", "manifest", "running", "http://localhost:8080"} {
		if !strings.Contains(text, want) {
			t.Errorf("table does not show %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "stale") {
		t.Errorf("a stopped record is listed as an instance:\n%s", text)
	}
	// Nothing pinned and a local Airflow running: that is the current one, and
	// the only marked row.
	var marked []string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.HasPrefix(line, "*") {
			marked = append(marked, strings.Fields(line)[1])
		}
	}
	if len(marked) != 1 || marked[0] != "local" {
		t.Errorf("marked rows = %v, want just local:\n%s", marked, text)
	}
}

func TestInstanceListJSONIsOneObjectPerRow(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "instances", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one object per instance, got:\n%s", out)
	}
	var row struct {
		Name, Kind, Where, Source string
		Current                   bool
	}
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("json: %v", err)
	}
	if row.Name != "dev" || row.Kind != "astro" || row.Source != "manifest" || !row.Current {
		t.Fatalf("first row = %+v, want the default link marked current", row)
	}
}

func TestResolveInstanceWithoutATTYNamesEveryWayToDecide(t *testing.T) {
	// Two links, neither default, nothing running: resolution falls through,
	// and a run that cannot prompt must say how to decide.
	dir := instanceProject(t, ambiguousManifest)
	d, _, _ := instanceDeps(t, dir)
	c := &cli{d: d}
	_, err := c.resolveInstance("", "")
	if err == nil {
		t.Fatal("resolution succeeded with nothing to go on")
	}
	for _, want := range []string{"-i <name>", instances.EnvVar, "astro use <name>"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}
}

func TestPromptTakesANameAndRejectsAStranger(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("dev\n")
	c := &cli{d: d}
	name, err := c.promptForInstance([]string{"dev", "prod"})
	if err != nil || name != "dev" {
		t.Fatalf("prompt = %q, %v", name, err)
	}

	d.Stdin = strings.NewReader("nope\n")
	c = &cli{d: d}
	if _, err := c.promptForInstance([]string{"dev", "prod"}); err == nil {
		t.Fatal("a name outside the choices was accepted")
	}
}
