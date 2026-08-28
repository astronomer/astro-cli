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
	if err == nil || !strings.Contains(err.Error(), "known deployments: dev, prod") {
		t.Fatalf("err = %v, want one listing the known names", err)
	}
}

// `local` is reserved for the machine, so pinning it must refuse — and the
// refusal has to teach the spelling that works rather than read as a typo.
func TestUseRefusesTheReservedName(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	err := execute(t, d, "use", instances.LocalName)
	if err == nil {
		t.Fatal("the reserved name was pinned")
	}
	for _, want := range []string{"astro local start", "astro local af dags list"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %s", want, err)
		}
	}
	if state, _ := userstate.Load(dir); state.Instance != "" {
		t.Errorf("a refused pin was written anyway: %q", state.Instance)
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
	for _, want := range []string{"ASTRO_DEPLOYMENT", "astro use", "manifest default link", "resolves to prod"} {
		if !strings.Contains(text, want) {
			t.Errorf("table does not show %q:\n%s", want, text)
		}
	}
	// The running local Airflow is no longer a layer of the rule — it is not on
	// the ladder at all, only in the inventory below it.
	if strings.Contains(text, "running local Airflow") {
		t.Errorf("the machine is still a resolution layer:\n%s", text)
	}
}

// Bare `astro use` answers both halves of one question: what this project is
// pointed at, and what else it could be pointed at — its links, and every local
// Airflow on the machine, told apart by the source column.
func TestBareUseListsDeploymentsAndTheMachine(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	other := filepath.Join(t.TempDir(), "billing")
	d, out, _ := instanceDeps(t, dir,
		localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080},
		localrt.Status{ProjectPath: other, State: localrt.StateRunning, Port: 8081},
		// A record whose runtime is gone is not a running Airflow.
		localrt.Status{ProjectPath: filepath.Join(t.TempDir(), "stale"), State: localrt.StateStopped, Port: 8082},
	)
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"dev", "prod", "local", "billing", "manifest", "running", "http://localhost:8080"} {
		if !strings.Contains(text, want) {
			t.Errorf("listing does not show %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "stale") {
		t.Errorf("a stopped record is listed as running:\n%s", text)
	}
}

// Two names can want the same word: a neighboring checkout called `local`,
// and one called `billing` when the manifest already links a `billing`. Before
// the split, instances.Build arbitrated this; the inventory re-derives names
// and has to carry the rule. Every row keeps a distinct name, the reserved word
// stays this machine's, and nothing shown is a name `astro use` would refuse
// without saying so.
func TestInventoryNamesNeverCollide(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest+"\n[tool.astro.deployments.billing]\ndeployment = 'clm2xk9dq000108l7a2b3c4d7'\n")
	elsewhere := t.TempDir()
	foreignLocal := filepath.Join(elsewhere, "local")
	foreignBilling := filepath.Join(elsewhere, "billing")

	d, out, _ := instanceDeps(t, dir,
		localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080},
		localrt.Status{ProjectPath: foreignLocal, State: localrt.StateRunning, Port: 8081},
		localrt.Status{ProjectPath: foreignBilling, State: localrt.StateRunning, Port: 8082},
	)
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Instances []struct {
			Name, Kind, URL, Source, Note string
		} `json:"instances"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}

	seen := map[string]int{}
	for _, row := range report.Instances {
		seen[row.Name]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%d rows are named %q — a consumer keying on name cannot tell them apart:\n%s", n, name, out)
		}
	}
	for _, row := range report.Instances {
		switch {
		case row.Name == instances.LocalName && row.URL != "http://localhost:8080":
			t.Errorf("a neighbor answers to %q: %+v", instances.LocalName, row)
		case row.Source == "running" && row.Note == "":
			// `astro use <name>` pins deployments only, so a running row that
			// said nothing would be a name the next command refuses.
			t.Errorf("running row %q carries no note saying how to reach it: %+v", row.Name, row)
		case row.Source == "manifest" && row.Note != "":
			t.Errorf("deployment %q carries a note it does not need: %q", row.Name, row.Note)
		}
	}
	// The two neighbors are reachable only as bare URLs, and the listing says so.
	for _, want := range []string{"--url http://localhost:8081", "--url http://localhost:8082"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("listing does not say %q:\n%s", want, out)
		}
	}
}

// Every name the listing prints is either pinnable or carries the note that
// says what to do instead — checked by feeding each one straight back to
// `astro use`, which is what a reader would do.
func TestEveryListedNameIsPinnableOrExplained(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	other := filepath.Join(t.TempDir(), "billing")
	d, out, _ := instanceDeps(t, dir,
		localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080},
		localrt.Status{ProjectPath: other, State: localrt.StateRunning, Port: 8081},
	)
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Instances []struct {
			Name, Source, Note string
		} `json:"instances"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, row := range report.Instances {
		pin, _, _ := instanceDeps(t, dir)
		err := execute(t, pin, "use", row.Name)
		if row.Source == "manifest" {
			if err != nil {
				t.Errorf("`astro use %s` refused a deployment the listing printed: %v", row.Name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("`astro use %s` pinned a running Airflow; only deployments are pinnable", row.Name)
		}
		if row.Note == "" {
			t.Errorf("row %q is not pinnable and does not say what to do instead", row.Name)
		}
	}
}

func TestBareUseJSONCarriesEveryLayerTheWinnerAndTheInventory(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	t.Setenv(instances.EnvVar, "dev")
	d, out, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Layers []struct {
			Layer, Value string
			Wins         bool
		}
		Winner    string
		Instances []struct {
			Name, Kind, Where, URL, Source string
			AuthMethod                     string `json:"auth_method"`
			Current                        bool
		}
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Winner != "dev" || len(got.Layers) != 3 {
		t.Fatalf("json = %+v", got)
	}
	if got.Layers[0].Layer != "env" || got.Layers[0].Value != "dev" || !got.Layers[0].Wins {
		t.Fatalf("env layer = %+v, want it winning with dev", got.Layers[0])
	}
	if len(got.Instances) != 3 {
		t.Fatalf("instances = %+v, want the two links and the machine", got.Instances)
	}
	if got.Instances[0].Name != "dev" || got.Instances[0].Source != "manifest" || !got.Instances[0].Current {
		t.Errorf("first row = %+v, want the winning link marked current", got.Instances[0])
	}
	// The machine is listed, and never current: it is not something resolution
	// can pick.
	machine := got.Instances[2]
	if machine.Name != instances.LocalName || machine.Kind != "local" || machine.Source != "running" || machine.Current {
		t.Errorf("machine row = %+v", machine)
	}
	if machine.URL != "http://localhost:8080" {
		t.Errorf("the machine row has no address: %+v", machine)
	}
}

// A link's auth method rides in the listing so a consumer knows how it proves
// itself before it tries, and its URL tells a resolved address from a
// coordinate that still needs a lookup.
func TestBareUseJSONCarriesURLAndAuthMethod(t *testing.T) {
	dir := instanceProject(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'AF_TOKEN' }\n")
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Instances []struct {
			URL        string
			AuthMethod string `json:"auth_method"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(got.Instances) != 1 {
		t.Fatalf("instances = %+v", got.Instances)
	}
	if got.Instances[0].URL != "https://airflow.staging.corp.dev" || got.Instances[0].AuthMethod != "token" {
		t.Fatalf("row = %+v", got.Instances[0])
	}
}

// A project with nothing to act on says so rather than printing an empty table.
func TestBareUseSaysWhenThereIsNothingToActOn(t *testing.T) {
	dir := instanceProject(t, "")
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), emptyInventory) {
		t.Errorf("output does not explain the empty listing:\n%s", out)
	}
}

func TestPromptTakesANameAndRejectsAStranger(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("dev\n")
	c := &cli{d: d}
	name, err := c.promptForDeployment([]string{"dev", "prod"})
	if err != nil || name != "dev" {
		t.Fatalf("prompt = %q, %v", name, err)
	}

	d.Stdin = strings.NewReader("nope\n")
	c = &cli{d: d}
	if _, err := c.promptForDeployment([]string{"dev", "prod"}); err == nil {
		t.Fatal("a name outside the choices was accepted")
	}
}
