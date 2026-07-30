package local

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// TestURLTargetNeedsNoProject is the escape hatch's whole point: an Airflow
// nobody declared, reached from a directory that is not a project at all.
func TestURLTargetNeedsNoProject(t *testing.T) {
	outside := t.TempDir() // no pyproject.toml anywhere above it
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(instances.EnvVar, "")
	d, _, errOut := instanceDeps(t, outside)
	c := &cli{d: d}

	sel, err := c.resolveInstance("", "https://airflow.corp.dev")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if sel.From != instances.LayerURL || sel.Instance.URL != "https://airflow.corp.dev" {
		t.Fatalf("selected %+v", sel)
	}
	if !strings.Contains(errOut.String(), "→ https://airflow.corp.dev") {
		t.Errorf("stderr = %q", errOut)
	}

	// The same directory without --url still reports the missing project, so
	// the escape hatch is the exception rather than a hole.
	if _, err := c.resolveInstance("", ""); err == nil {
		t.Fatal("resolution outside a project succeeded")
	}
}

// TestInstanceFlagsRegisterOnce pins the spelling every Airflow-facing command
// will inherit: -i is settled for --instance, and --url takes no
// shorthand. One registration is what keeps the query surface from drifting
// into three spellings of the same idea.
func TestInstanceFlagsRegisterOnce(t *testing.T) {
	cmd := &cobra.Command{Use: "dags"}
	f := &instanceFlags{}
	addInstanceFlags(cmd, f)

	instance := cmd.PersistentFlags().Lookup("instance")
	if instance == nil || instance.Shorthand != "i" {
		t.Fatalf("--instance = %+v, want it registered with -i", instance)
	}
	url := cmd.PersistentFlags().Lookup("url")
	if url == nil || url.Shorthand != "" {
		t.Fatalf("--url = %+v, want it registered with no shorthand", url)
	}
	if err := cmd.PersistentFlags().Parse([]string{"-i", "prod", "--url", "https://x"}); err != nil {
		t.Fatal(err)
	}
	if f.instance != "prod" || f.url != "https://x" {
		t.Fatalf("parsed into %+v", f)
	}
}

func TestInstanceAndURLTogetherAreRefused(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	c := &cli{d: d}
	_, err := c.resolveInstance("prod", "https://airflow.corp.dev")
	if !errors.Is(err, instances.ErrMutuallyExclusive) {
		t.Fatalf("err = %v, want the mutually-exclusive refusal", err)
	}
}

// TestInstanceClientOpensAClientOnWhatResolved covers the composition root the
// query commands will call: one resolve, one transport, one client.
func TestInstanceClientOpensAClientOnWhatResolved(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	c := &cli{d: d}

	sel, client, err := c.instanceClient(context.Background(), instanceFlags{})
	if err != nil {
		t.Fatalf("instanceClient: %v", err)
	}
	if sel.Instance.Name != instances.LocalName || client == nil {
		t.Fatalf("selection = %+v, client = %v", sel, client)
	}
}

// TestSymlinkedProjectKeepsItsLocal: the record was written from the resolved
// path and the command runs from a symlinked one — the everyday macOS case,
// where /tmp is a link to /private/tmp. Both spellings key the same state
// directory, so they have to be the same project here too.
func TestSymlinkedProjectKeepsItsLocal(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(target, "orders"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	dir := filepath.Join(link, "orders")
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\n\n[tool.astro]\nairflow = '3.1'\nworkspace = 'ws_abc123'\n" + twoLinkManifest
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	t.Setenv(instances.EnvVar, "")

	started := filepath.Join(target, "orders")
	d, out, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: started, State: localrt.StateRunning, Port: 8080})
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "resolves to local") {
		t.Fatalf("the project's own Airflow was not recognized:\n%s", out)
	}
}

// TestUseUnsetHealsACorruptState: --unset is the documented way out of a bad
// pin, so it cannot be the command that a bad state file breaks.
func TestUseUnsetHealsACorruptState(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	sd, err := userstate.Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, "state.json"), []byte(`{"instance": `), 0o600); err != nil {
		t.Fatal(err)
	}
	// Any other command says what to run, rather than leaving the way out to be
	// discovered.
	d, _, _ := instanceDeps(t, dir)
	err = execute(t, d, "use")
	if err == nil || !strings.Contains(err.Error(), "astro use --unset") {
		t.Fatalf("err = %v, want it to name the command that heals this", err)
	}

	d, _, _ = instanceDeps(t, dir)
	if err := execute(t, d, "use", "--unset"); err != nil {
		t.Fatalf("--unset could not clear state it cannot parse: %v", err)
	}
	state, err := userstate.Load(dir)
	if err != nil {
		t.Fatalf("state is still unreadable: %v", err)
	}
	if state.Instance != "" {
		t.Fatalf("pin = %q after --unset", state.Instance)
	}
}

// TestPromptPinsAndIsNeverAskedTwice drives the fall-through end to end: an
// ambiguous project, an answer, the pin on disk, and a second run that resolves
// in silence.
func TestPromptPinsAndIsNeverAskedTwice(t *testing.T) {
	dir := instanceProject(t, ambiguousManifest)
	d, _, errOut := instanceDeps(t, dir)
	d.Stdin = strings.NewReader("2\n")
	d.Interactive = func() bool { return true }
	c := &cli{d: d}

	sel, err := c.resolveInstance("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if sel.Instance.Name != "prod" {
		t.Fatalf("resolved to %s, want the answer", sel.Instance.Name)
	}
	if !strings.Contains(errOut.String(), "picked prod — pinned for this project") {
		t.Errorf("the pick was not announced: %q", errOut)
	}
	state, err := userstate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Instance != "prod" {
		t.Fatalf("pin = %q, want the answer written", state.Instance)
	}

	// Second run: the pin answers, and nothing is asked.
	d2, _, errOut2 := instanceDeps(t, dir)
	d2.Stdin = strings.NewReader("")
	d2.Interactive = func() bool { return true }
	c2 := &cli{d: d2}
	sel, err = c2.resolveInstance("", "")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if sel.From != instances.LayerPin || sel.Instance.Name != "prod" {
		t.Fatalf("second resolve = %+v, want the pin", sel)
	}
	if strings.Contains(errOut2.String(), "Which instance") {
		t.Errorf("asked a second time: %q", errOut2)
	}
}

// TestPromptNeedsAnExplicitChoice: Enter is not an answer. Picking for someone
// who did not choose is exactly the wrong-Airflow failure the prompt exists to
// prevent.
func TestPromptNeedsAnExplicitChoice(t *testing.T) {
	d, _, errOut := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("\n\n2\n")
	c := &cli{d: d}

	name, err := c.promptForInstance([]string{"dev", "prod"})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if name != "prod" {
		t.Fatalf("answer = %q, want the explicit choice", name)
	}
	if strings.Count(errOut.String(), "Choose 1-2") < 3 {
		t.Errorf("empty answers were not re-asked: %q", errOut)
	}
}

func TestPromptEndsOnAClosedStdin(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("")
	c := &cli{d: d}
	_, err := c.promptForInstance([]string{"dev", "prod"})
	var ambiguous *instances.AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want the message naming every way to decide", err)
	}
}

// TestJSONRunsNeverPrompt: a prompt on stderr with the run blocked on stdin is
// not a stream a program can parse, so json output decides for itself that it
// cannot ask.
func TestJSONRunsNeverPrompt(t *testing.T) {
	dir := instanceProject(t, ambiguousManifest)
	d, _, errOut := instanceDeps(t, dir)
	d.Stdin = strings.NewReader("2\n")
	d.Interactive = func() bool { return true }
	c := &cli{d: d, output: string(FormatJSON)}

	_, err := c.resolveInstance("", "")
	var ambiguous *instances.AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want the ambiguous error rather than a prompt", err)
	}
	if strings.Contains(errOut.String(), "Which instance") {
		t.Errorf("a json run prompted: %q", errOut)
	}
}

// TestEmptyInstanceListSaysSoInJSON: zero instances is zero NDJSON lines, which
// is correct and indistinguishable from a crash, so the explanation goes to
// stderr where it cannot corrupt the stream.
func TestEmptyInstanceListSaysSoInJSON(t *testing.T) {
	dir := instanceProject(t, "")
	d, out, errOut := instanceDeps(t, dir)
	if err := execute(t, d, "instance", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should stay an empty stream, got %q", out)
	}
	if !strings.Contains(errOut.String(), "No instances") {
		t.Errorf("stderr does not explain the empty stream: %q", errOut)
	}
}

// TestInstanceListShowsAShadowedAirflow: two projects with the same directory
// name. One answers to it; the other is listed with the reason and a way in,
// because the command promises every Airflow running on this machine.
func TestInstanceListShowsAShadowedAirflow(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	root := t.TempDir()
	first := filepath.Join(root, "a", "etl")
	second := filepath.Join(root, "b", "etl")
	for _, p := range []string{first, second} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	d, out, _ := instanceDeps(t, dir,
		localrt.Status{ProjectPath: first, State: localrt.StateRunning, Port: 8081},
		localrt.Status{ProjectPath: second, State: localrt.StateRunning, Port: 8082},
	)
	if err := execute(t, d, "instance", "list"); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "etl") < 2 {
		t.Errorf("only one of the two same-named Airflows is listed:\n%s", out)
	}
	if !strings.Contains(out.String(), "--url http://localhost:8082") {
		t.Errorf("the shadowed row does not say how to reach it:\n%s", out)
	}
}

// TestInstanceListJSONCarriesURLAndAuthMethod: a consumer needs to tell a
// resolved address from a coordinate that still needs a lookup, and to know how
// a link proves itself before it tries.
func TestInstanceListJSONCarriesURLAndAuthMethod(t *testing.T) {
	dir := instanceProject(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'AF_TOKEN' }\n")
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "instance", "list", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var row struct {
		Name, Kind, Where, URL, Source string
		AuthMethod                     string `json:"auth_method"`
		Current                        bool
	}
	if err := json.Unmarshal(out.Bytes(), &row); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if row.URL != "https://airflow.staging.corp.dev" || row.AuthMethod != "token" {
		t.Fatalf("row = %+v", row)
	}
}

// TestUseLocalSaysNothingIsRunningYet: pinning the reserved name before
// starting Airflow is allowed and normal, and saying so beats silence.
func TestUseLocalSaysNothingIsRunningYet(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, errOut := instanceDeps(t, dir)
	if err := execute(t, d, "use", "local"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "astro local start") {
		t.Errorf("stderr = %q, want it to say nothing is running yet", errOut)
	}
}

// TestPinnedLocalNamesTheRightFix: every command between `astro use local` and
// `astro local start` fails, and has to point at starting Airflow.
func TestPinnedLocalNamesTheRightFix(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use", "local"); err != nil {
		t.Fatal(err)
	}
	d, _, _ = instanceDeps(t, dir)
	c := &cli{d: d}
	_, err := c.resolveInstance("", "")
	if err == nil {
		t.Fatal("resolution succeeded with the pin naming nothing running")
	}
	if !strings.Contains(err.Error(), "astro local start") {
		t.Errorf("the fix named is %q", err)
	}
}

// ambiguousManifest declares two links and marks neither, the only shape that
// reaches the prompt.
const ambiguousManifest = `
[tool.astro.deployments.dev]
deployment = 'clm2xk9dq000108l7a2b3c4d5'

[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d6'
`
