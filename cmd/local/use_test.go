package local

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/userstate"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
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
	body := instancestest.Preamble + links
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
	if !strings.Contains(out.String(), "now uses prod") {
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

// lineOf returns the line of out whose NAME column is name, "" when none is.
func lineOf(out, name string) string {
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(strings.TrimPrefix(line, "*")); len(f) > 0 && f[0] == name {
			return line
		}
	}
	return ""
}

// Bare `astro use` off a terminal lists the links and marks the current one —
// nothing else. The resolution rule is in the help, and a running local
// Airflow is not something `astro use` can select, so neither is printed.
func TestBareUseListsTheLinksAndMarksTheCurrentOne(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	if err := savePin(dir, "prod"); err != nil {
		t.Fatal(err)
	}
	d, out, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if prod := lineOf(text, "prod"); !strings.HasPrefix(prod, "*") || strings.Contains(prod, "←") {
		t.Errorf("prod, your own selection, should carry the * and no label: %q\n%s", prod, text)
	}
	if dev := lineOf(text, "dev"); dev == "" || strings.HasPrefix(dev, "*") {
		t.Errorf("dev is listed and not current: %q\n%s", dev, text)
	}
	for _, gone := range []string{"LAYER", "resolves to", "ASTRO_DEPLOYMENT", "localhost:8080", "local "} {
		if strings.Contains(text, gone) {
			t.Errorf("the listing still prints %q:\n%s", gone, text)
		}
	}
}

// A row something other than your selection made current says what did, in
// the picker's words.
func TestBareUseLabelsARowItDidNotSelect(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if dev := lineOf(out.String(), "dev"); !strings.HasPrefix(dev, "*") || !strings.Contains(dev, "← default = true") {
		t.Errorf("dev, current by the manifest default, should say so: %q\n%s", dev, out)
	}

	t.Setenv(instances.EnvVar, "prod")
	d, out, _ = instanceDeps(t, dir)
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if prod := lineOf(out.String(), "prod"); !strings.HasPrefix(prod, "*") || !strings.Contains(prod, "← "+instances.EnvVar) {
		t.Errorf("prod, current by the env var, should say so: %q\n%s", prod, out)
	}
}

// Several links and no default: nothing is current, and the listing says why
// rather than leaving the reader to wonder where the * went.
func TestBareUseSaysWhyNothingIsCurrent(t *testing.T) {
	dir := instanceProject(t, "\n[tool.astro.deployments.dev]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n\n[tool.astro.deployments.prod]\ndeployment = 'clm2xk9dq000108l7a2b3c4d6'\n")
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No Deployment is current: several deployments are linked and none is the default") {
		t.Errorf("no reason given:\n%s", out)
	}
	if strings.Contains(out.String(), "*") {
		t.Errorf("a row is marked current with nothing resolving:\n%s", out)
	}
}

func TestBareUseJSONCarriesTheCurrentLinkAndWhy(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	t.Setenv(instances.EnvVar, "dev")
	d, out, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	for _, gone := range []string{"layers", "winner", "instances"} {
		if _, ok := got[gone]; ok {
			t.Errorf("the report still publishes %q:\n%s", gone, out)
		}
	}
	var res useListing
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Current != "dev" || res.From != "env" || res.Reason != "" {
		t.Fatalf("report = %+v", res)
	}
	if len(res.Deployments) != 2 || res.Deployments[0].Name != "dev" || !res.Deployments[0].Current || res.Deployments[1].Current {
		t.Fatalf("deployments = %+v, want the two links with dev current", res.Deployments)
	}
	if res.Deployments[0].Kind != "astro" || res.Deployments[0].Where != "clm2xk9dq000108l7a2b3c4d5" {
		t.Errorf("dev row = %+v", res.Deployments[0])
	}

	// Your own selection reads as one, never as the internal word for it.
	t.Setenv(instances.EnvVar, "")
	if err := savePin(dir, "prod"); err != nil {
		t.Fatal(err)
	}
	d, out, _ = instanceDeps(t, dir)
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Current != "prod" || res.From != "selection" {
		t.Errorf("report = %+v, want prod from selection", res)
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
	var res useListing
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(res.Deployments) != 1 {
		t.Fatalf("deployments = %+v", res.Deployments)
	}
	if row := res.Deployments[0]; row.URL != "https://airflow.staging.corp.dev" || row.AuthMethod != "token" || !row.Current {
		t.Fatalf("row = %+v", row)
	}
}

// Every name the listing prints is one `astro use` takes — checked by feeding
// each straight back, which is what a reader would do.
func TestEveryListedNameIsSelectable(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, out, _ := instanceDeps(t, dir, localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Port: 8080})
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var res useListing
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, row := range res.Deployments {
		sel, _, _ := instanceDeps(t, dir)
		if err := execute(t, sel, "use", row.Name); err != nil {
			t.Errorf("`astro use %s` refused a name the listing printed: %v", row.Name, err)
		}
	}
}

// A project with nothing linked says so rather than printing an empty table,
// in text and in json.
func TestBareUseSaysWhenNothingIsLinked(t *testing.T) {
	dir := instanceProject(t, "")
	d, out, _ := instanceDeps(t, dir)
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), noLinks) {
		t.Errorf("output does not explain the empty listing:\n%s", out)
	}

	d, out, _ = instanceDeps(t, dir)
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var res useListing
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || res.Deployments == nil || len(res.Deployments) != 0 {
		t.Errorf("json = %s, %v; want an empty deployments array", out, err)
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

// Bare `astro use` at a terminal asks which link to use and pins the answer.
// The none row that clears the pin is offered only once there is a pin to
// clear, so on a fresh project the row past the links is not a choice.
func TestBareUseAtATerminalPicksAndPins(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	pick := func(answer string) (string, error) {
		t.Helper()
		d, out, _ := instanceDeps(t, dir)
		d.Interactive = func() bool { return true }
		d.OutputTerminal = func() bool { return true }
		d.Stdin = strings.NewReader(answer)
		err := execute(t, d, "use")
		return out.String(), err
	}
	pin := func() string {
		t.Helper()
		state, err := userstate.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		return state.Instance
	}

	if _, err := pick("3\n"); !errors.Is(err, errInvalidLinkSelection) {
		t.Fatalf("a none row was offered with no pin to clear: %v", err)
	}
	out, err := pick("2\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Select the Deployment this project uses") || !strings.Contains(out, "now uses prod") {
		t.Errorf("output:\n%s", out)
	}
	// Before any selection, the manifest's default is current, and says so.
	if !lineWith(out, "dev", "← default = true") || !lineWith(out, "dev", "\033[1;32m") {
		t.Errorf("dev not marked as the default-chosen current row:\n%s", out)
	}
	if got := pin(); got != "prod" {
		t.Fatalf("pin = %q, want prod", got)
	}
	if out, err = pick("3\n"); err != nil {
		t.Fatal(err)
	}
	// Now the user's own selection is current: highlighted, once, with no label.
	if !lineWith(out, "prod", "\033[1;32m") || !lineWith(out, "prod", "clm2xk9dq000108l7a2b3c4d6") ||
		strings.Contains(out, "←") || strings.Count(out, "\033[1;32m") != 1 {
		t.Errorf("prod not marked as the selected current row:\n%s", out)
	}
	if !strings.Contains(out, "clear your selection") || pin() != "" {
		t.Errorf("none did not clear the pin (pin %q):\n%s", pin(), out)
	}
}

// Bare `astro use` reports rather than prompts when the picker could not be
// seen or would have nothing to offer: stdout piped away from a terminal
// stdin, or a project that links nothing.
func TestBareUseReportsWhenThePickerCannotHelp(t *testing.T) {
	for name, tc := range map[string]struct {
		links, want string
		stdoutTTY   bool
	}{
		"stdout piped": {twoLinkManifest, "← default = true", false},
		"no links":     {"", noLinks, true},
	} {
		t.Run(name, func(t *testing.T) {
			dir := instanceProject(t, tc.links)
			d, out, _ := instanceDeps(t, dir)
			d.Interactive = func() bool { return true }
			d.OutputTerminal = func() bool { return tc.stdoutTTY }
			d.Stdin = strings.NewReader("2\n")
			if err := execute(t, d, "use"); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) || strings.Contains(out.String(), "Select the Deployment") {
				t.Errorf("want the report:\n%s", out)
			}
		})
	}
}

// A pin that ASTRO_DEPLOYMENT outranks says so, rather than announcing a target
// the next command will not act on.
func TestUseWarnsWhenTheEnvOutranksThePin(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	t.Setenv(instances.EnvVar, "dev")
	d, _, errOut := instanceDeps(t, dir)
	if err := execute(t, d, "use", "prod"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), instances.EnvVar+"=dev still takes precedence") {
		t.Errorf("no warning:\n%s", errOut)
	}

	d, _, errOut = instanceDeps(t, dir)
	if err := execute(t, d, "use", "dev"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errOut.String(), "precedence") {
		t.Errorf("warned about an env var that agrees with the pin:\n%s", errOut)
	}
}

// lineWith reports whether one line of out holds both the name and the mark.
func lineWith(out, name, mark string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, " "+name+" ") && strings.Contains(line, mark) {
			return true
		}
	}
	return false
}

// A row ASTRO_DEPLOYMENT made current is marked with the variable, since
// selecting another row here will not move it.
func TestBareUseMarksAnEnvChosenRow(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	t.Setenv(instances.EnvVar, "prod")
	d, out, _ := instanceDeps(t, dir)
	d.Interactive = func() bool { return true }
	d.OutputTerminal = func() bool { return true }
	d.Stdin = strings.NewReader("2\n")
	if err := execute(t, d, "use"); err != nil {
		t.Fatal(err)
	}
	if !lineWith(out.String(), "prod", "← "+instances.EnvVar) {
		t.Errorf("prod not marked as env-chosen:\n%s", out)
	}
}

// --output json never prompts, terminal or not: it is the report, as before.
func TestBareUseWithJSONAtATerminalReports(t *testing.T) {
	dir := instanceProject(t, twoLinkManifest)
	d, out, _ := instanceDeps(t, dir)
	d.Interactive = func() bool { return true }
	d.OutputTerminal = func() bool { return true }
	d.Stdin = strings.NewReader("2\n")
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	var got useListing
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Current != "dev" {
		t.Errorf("current = %q, want dev", got.Current)
	}
	if state, _ := userstate.Load(dir); state.Instance != "" {
		t.Errorf("json run pinned %q", state.Instance)
	}
}
