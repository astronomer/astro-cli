package local

import (
	"context"
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

	sel, err := c.resolveDeployment(deploymentFlags{url: "https://airflow.corp.dev"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if sel.From != instances.LayerURL || sel.Instance.URL != "https://airflow.corp.dev" {
		t.Fatalf("selected %+v", sel)
	}
	// Opening the client is what announces the target, so the URL a run is
	// about to talk to is never invisible.
	if _, err := c.clientFor(context.Background(), sel.Instance); err != nil {
		t.Fatalf("client: %v", err)
	}
	if !strings.Contains(errOut.String(), "→ https://airflow.corp.dev") {
		t.Errorf("stderr = %q", errOut)
	}

	// The same directory without --url still reports the missing project, so
	// the escape hatch is the exception rather than a hole.
	if _, err := c.resolveDeployment(deploymentFlags{}); err == nil {
		t.Fatal("resolution outside a project succeeded")
	}
}

// TestDeploymentFlagsRegisterOnce pins the spelling every top-level
// Airflow-facing command inherits: -d is settled for --deployment,
// and --url takes no shorthand. One registration is what keeps the query
// surface from drifting into three spellings of the same idea.
func TestDeploymentFlagsRegisterOnce(t *testing.T) {
	cmd := &cobra.Command{Use: "dags"}
	f := &deploymentFlags{}
	addDeploymentFlags(cmd, f)

	deployment := cmd.PersistentFlags().Lookup("deployment")
	if deployment == nil || deployment.Shorthand != "d" {
		t.Fatalf("--deployment = %+v, want it registered with -d", deployment)
	}
	url := cmd.PersistentFlags().Lookup("url")
	if url == nil || url.Shorthand != "" {
		t.Fatalf("--url = %+v, want it registered with no shorthand", url)
	}
	if err := cmd.PersistentFlags().Parse([]string{"-d", "prod", "--url", "https://x"}); err != nil {
		t.Fatal(err)
	}
	if f.deployment != "prod" || f.url != "https://x" {
		t.Fatalf("parsed into %+v", f)
	}
}

func TestDeploymentAndURLTogetherAreRefused(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	c := &cli{d: d}
	_, err := c.resolveDeployment(deploymentFlags{deployment: "prod", url: "https://airflow.corp.dev"})
	if !errors.Is(err, instances.ErrMutuallyExclusive) {
		t.Fatalf("err = %v, want the mutually-exclusive refusal", err)
	}
}

// TestDeploymentClientOpensAClientOnWhatResolved covers the composition root
// the query commands call: one resolve, one transport, one client.
func TestDeploymentClientOpensAClientOnWhatResolved(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, errOut := instanceDeps(t, dir)
	d.Session = func(context.Context) (string, error) { return "Bearer t", nil }
	d.Locator = locatorFunc(func(context.Context, instances.Instance) (string, error) {
		return "https://airflow.example.com", nil
	})
	c := &cli{d: d}

	client, err := c.deploymentClient(context.Background(), deploymentFlags{})
	if err != nil {
		t.Fatalf("deploymentClient: %v", err)
	}
	if client == nil {
		t.Fatal("no client")
	}
	// Nothing said, so the manifest's default link answered — never the Airflow
	// running on this machine, which is a different command now.
	if !strings.Contains(errOut.String(), "→ dev") {
		t.Fatalf("stderr = %q, want the default link", errOut)
	}
}

// TestMachineInstanceIsThisProjectsOwnAirflow: `astro local <query>` acts on
// the Airflow this project has running, whatever else is on the machine and
// whatever the pin says.
func TestMachineInstanceIsThisProjectsOwnAirflow(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	if err := savePin(dir, "prod"); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "billing")
	d, _, _ := instanceDeps(t, dir,
		localrt.Status{ProjectPath: other, State: localrt.StateRunning, Port: 8081},
		localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080},
	)
	c := &cli{d: d}

	i, err := c.machineInstance()
	if err != nil {
		t.Fatalf("machineInstance: %v", err)
	}
	if i.Name != instances.LocalName || i.URL != "http://localhost:8080" {
		t.Fatalf("machine = %+v, want this project's own Airflow", i)
	}
}

// TestMachineInstanceNeedsSomethingRunning: with nothing up, the fix is to
// start Airflow. Pointing somewhere else is what the top-level spelling is for,
// so this error never offers one.
func TestMachineInstanceNeedsSomethingRunning(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := instanceDeps(t, dir)
	c := &cli{d: d}
	_, err := c.machineInstance()
	if !errors.Is(err, errNoLocalAirflow) {
		t.Fatalf("err = %v, want the start-it message", err)
	}
	if !strings.Contains(err.Error(), "astro local start") {
		t.Errorf("the fix named is %q", err)
	}
}

// TestSymlinkedProjectKeepsItsMachine: the record was written from the resolved
// path and the command runs from a symlinked one — the everyday macOS case,
// where /tmp is a link to /private/tmp. Both spellings key the same state
// directory, so they have to be the same project here too.
func TestSymlinkedProjectKeepsItsMachine(t *testing.T) {
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
	d, _, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: started, State: localrt.StateRunning, Port: 8080})
	c := &cli{d: d}
	i, err := c.machineInstance()
	if err != nil {
		t.Fatalf("the project's own Airflow was not recognized: %v", err)
	}
	if i.Name != instances.LocalName {
		t.Fatalf("machine = %+v", i)
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

	// Pinning reads the file before it writes it, so it meets the same wall and
	// has to name the same way out rather than surfacing a bare decode error.
	d, _, errOut := instanceDeps(t, dir)
	err = execute(t, d, "use", "prod")
	if err == nil || !strings.Contains(err.Error(), "astro use --unset") {
		t.Fatalf("err = %v, want the pin write to name the command that heals this", err)
	}
	if strings.Contains(errOut.String(), "→ prod") {
		t.Errorf("a pin that was never written was announced anyway: %q", errOut)
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

	sel, err := c.resolveDeployment(deploymentFlags{})
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
	sel, err = c2.resolveDeployment(deploymentFlags{})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if sel.From != instances.LayerPin || sel.Instance.Name != "prod" {
		t.Fatalf("second resolve = %+v, want the pin", sel)
	}
	if strings.Contains(errOut2.String(), "Which deployment") {
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

	name, err := c.promptForDeployment([]string{"dev", "prod"})
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

// TestPromptTakesANameOverAPosition: a deployment may legally be called "2".
// Reading the answer as a position first would pin whichever entry happened to
// sit there instead — here `prod`, the other one.
func TestPromptTakesANameOverAPosition(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("2\n")
	c := &cli{d: d}

	name, err := c.promptForDeployment([]string{"2", "prod"})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if name != "2" {
		t.Fatalf("answer = %q, want the deployment the user named", name)
	}
}

// A number still works where no deployment claims that spelling.
func TestPromptStillTakesANumber(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("2\n")
	c := &cli{d: d}

	name, err := c.promptForDeployment([]string{"dev", "prod"})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if name != "prod" {
		t.Fatalf("answer = %q, want the second choice", name)
	}
}

func TestPromptEndsOnAClosedStdin(t *testing.T) {
	d, _, _ := instanceDeps(t, t.TempDir())
	d.Stdin = strings.NewReader("")
	c := &cli{d: d}
	_, err := c.promptForDeployment([]string{"dev", "prod"})
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

	_, err := c.resolveDeployment(deploymentFlags{})
	var ambiguous *instances.AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want the ambiguous error rather than a prompt", err)
	}
	if strings.Contains(errOut.String(), "Which deployment") {
		t.Errorf("a json run prompted: %q", errOut)
	}
}

// A run with nothing to resolve names both worlds: the ways to say which
// deployment, and this family's own `astro local` form, which needs no decision
// at all. Driven through the real command, because the family's name is the
// point and only the registration knows it.
func TestFallThroughNamesTheLocalSpelling(t *testing.T) {
	for _, tc := range []struct{ name, links string }{
		{"nothing linked", ""},
		{"several linked, none default", ambiguousManifest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := instanceProject(t, tc.links)
			d, _, _ := instanceDeps(t, dir)
			err := execute(t, d, "dags", "list")
			if err == nil {
				t.Fatal("resolution succeeded with nothing to go on")
			}
			if !strings.Contains(err.Error(), "astro local dags") {
				t.Errorf("message does not spell out the machine's form: %s", err)
			}
		})
	}
}

// A pin an older release left behind names the machine's commands rather than
// reading as an unknown deployment — and says how to clear itself, since it
// fails every command until someone does.
func TestPinnedLocalPointsAtTheNewSpelling(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	if err := savePin(dir, instances.LocalName); err != nil {
		t.Fatal(err)
	}
	d, _, _ := instanceDeps(t, dir)
	c := &cli{d: d}
	_, err := c.resolveDeployment(deploymentFlags{})
	if !errors.Is(err, instances.ErrLocalNotADeployment) {
		t.Fatalf("err = %v, want the machine's own commands named", err)
	}
	if !strings.Contains(err.Error(), "astro use --unset") {
		t.Errorf("a stale pin was not told how to clear itself: %s", err)
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
