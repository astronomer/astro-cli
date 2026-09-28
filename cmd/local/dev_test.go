package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// fakeRuntime fails every call; the engine does not exist in these tests.
type fakeRuntime struct{}

func (fakeRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return nil, localrt.ErrNotImplemented
}
func (fakeRuntime) Attach(string) (localrt.Airflow, error)    { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) LogSource(string) (localrt.Airflow, error) { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{}, localrt.ErrNotImplemented
}
func (fakeRuntime) List() ([]localrt.Status, error)       { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) PruneStale() ([]localrt.Status, error) { return nil, localrt.ErrNotImplemented }
func (fakeRuntime) Reset(context.Context, string) (localrt.ResetReport, error) {
	return localrt.ResetReport{}, localrt.ErrNotImplemented
}

func (fakeRuntime) Stopped(localrt.Plan) (localrt.Airflow, error) {
	return nil, localrt.ErrNotImplemented
}

func testDeps(t *testing.T) (d Deps, stdout *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	d = Deps{
		Stdin:      strings.NewReader(""),
		Stdout:     stdout,
		Stderr:     &bytes.Buffer{},
		Runtime:    fakeRuntime{},
		Checks:     stubParser{},
		WorkingDir: func() (string, error) { return t.TempDir(), nil },
		OpenURL:    func(string) error { return nil },
	}
	return d, stdout
}

func execute(t *testing.T, d Deps, args ...string) error {
	t.Helper()
	root := newRootCmd(d)
	root.SetArgs(args)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	return root.Execute()
}

// TestDevStubNamesTheReplacement drives the real table rather than a copy of
// it. The copy that used to live here covered whatever someone had pasted:
// a row added to devmap.go was tested nowhere, and a row corrected there
// failed here as though the correction were the regression.
func TestDevStubNamesTheReplacement(t *testing.T) {
	for _, m := range scaffold.DevReplacements() {
		t.Run(m.Command, func(t *testing.T) {
			d, _ := testDeps(t)
			err := execute(t, d, append([]string{"dev"}, strings.Fields(m.Command)...)...)
			if err == nil {
				t.Fatal("astro dev must fail")
			}
			msg := err.Error()
			if !strings.Contains(msg, "Use `"+m.Replacement+"` instead") {
				t.Errorf("error does not name the replacement %q:\n%s", m.Replacement, msg)
			}
			typed := "astro dev " + m.Command
			if !strings.Contains(msg, "`"+typed+"` was removed") {
				t.Errorf("error does not name the typed command %q:\n%s", typed, msg)
			}
		})
	}
}

func TestDevStubBareAndUnknown(t *testing.T) {
	d, _ := testDeps(t)
	err := execute(t, d, "dev")
	if err == nil || !strings.Contains(err.Error(), "astro dev was removed in Astro CLI v2") {
		t.Errorf("bare `astro dev` message wrong: %v", err)
	}

	err = execute(t, d, "dev", "upgrade-test")
	if err == nil {
		t.Fatal("unknown subcommand must still fail")
	}
	if !strings.Contains(err.Error(), "no direct replacement") {
		t.Errorf("unknown subcommand should say there is no direct replacement: %v", err)
	}
}

func TestDevStubV1Notice(t *testing.T) {
	writeDockerfile := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const notice = "astro v1 project (Dockerfile and .astro/)"

	t.Run("Dockerfile and .astro gets the v1 notice", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		if err := os.Mkdir(filepath.Join(dir, ".astro"), 0o700); err != nil {
			t.Fatal(err)
		}
		d, _ := testDeps(t)
		d.WorkingDir = func() (string, error) { return dir, nil }
		err := execute(t, d, "dev")
		if err == nil || !strings.Contains(err.Error(), notice) {
			t.Errorf("v1 dir should get the v1 notice: %v", err)
		}
	})
	t.Run("a pyproject that only configures tools keeps the v1 notice", func(t *testing.T) {
		d, _ := testDeps(t)
		d.WorkingDir = func() (string, error) { return v1ProjectWithToolsPyproject(t), nil }
		err := execute(t, d, "dev")
		if err == nil || !strings.Contains(err.Error(), notice) {
			t.Errorf("a v1 dir with a tools-only pyproject.toml should get the v1 notice: %v", err)
		}
		if err != nil && !strings.Contains(err.Error(), "Run `astro init` here to convert it in place") {
			t.Errorf("the v1 notice should point at astro init: %v", err)
		}
	})
	t.Run("Dockerfile alone gets no v1 notice", func(t *testing.T) {
		dir := t.TempDir()
		writeDockerfile(t, dir)
		d, _ := testDeps(t)
		d.WorkingDir = func() (string, error) { return dir, nil }
		err := execute(t, d, "dev")
		if err == nil {
			t.Fatal("astro dev must still fail")
		}
		if strings.Contains(err.Error(), "astro v1 project") {
			t.Errorf("a Dockerfile-only dir must not be called v1: %v", err)
		}
	})
}

// v1ProjectWithToolsPyproject is a classic v1 project that also keeps a
// pyproject.toml for tool settings only, which many v1 repositories do.
func v1ProjectWithToolsPyproject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"Dockerfile":       "FROM quay.io/astronomer/astro-runtime:3.1-12\n",
		"requirements.txt": "requests\n",
		project.Marker:     "[tool.ruff]\nline-length = 120\n\n[tool.pytest.ini_options]\ntestpaths = ['tests']\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, ".astro"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDevStubLeadsWithTheConversionInAV1Project(t *testing.T) {
	d, _ := testDeps(t)
	d.WorkingDir = func() (string, error) { return v1ProjectWithToolsPyproject(t), nil }

	err := execute(t, d, "dev", "pytest")
	if err == nil || !strings.Contains(err.Error(), "Convert with `astro init`, then use `uv run pytest`") {
		t.Errorf("a v1 project should be told to convert before using the replacement: %v", err)
	}

	err = execute(t, d, "dev", "init")
	if err == nil || !strings.Contains(err.Error(), "Use `astro init` instead") {
		t.Errorf("astro dev init should name astro init once, as its replacement: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "then use `astro init`") {
		t.Errorf("astro dev init should not be told to convert and then convert: %v", err)
	}
}

func TestDevStubJSONNamesTheConversion(t *testing.T) {
	convert := func(t *testing.T, dir string) (string, bool) {
		t.Helper()
		d, out := testDeps(t)
		d.WorkingDir = func() (string, error) { return dir, nil }
		if err := execute(t, d, "dev", "pytest", "--output", "json"); err == nil {
			t.Fatal("json mode must still fail")
		}
		var payload struct {
			Replacement string `json:"replacement"`
			Convert     string `json:"convert"`
			V1Project   bool   `json:"v1_project"`
		}
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("stdout is not one JSON object: %v\n%s", err, out.String())
		}
		if payload.Replacement != "uv run pytest" {
			t.Errorf("replacement = %q, want uv run pytest", payload.Replacement)
		}
		return payload.Convert, payload.V1Project
	}

	if got, v1 := convert(t, v1ProjectWithToolsPyproject(t)); got != "astro init" || !v1 {
		t.Errorf("v1 project: convert = %q, v1_project = %v; want astro init, true", got, v1)
	}
	if got, v1 := convert(t, t.TempDir()); got != "" || v1 {
		t.Errorf("empty dir: convert = %q, v1_project = %v; want neither", got, v1)
	}
}

func TestDevStubIgnoresFlagsWhenResolving(t *testing.T) {
	d, _ := testDeps(t)
	err := execute(t, d, "dev", "start", "--wait", "5m")
	if err == nil || !strings.Contains(err.Error(), "astro local start") {
		t.Errorf("flags must not hide the typed subcommand: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "`astro dev start` was removed") {
		t.Errorf("flag values must not leak into the echoed command: %v", err)
	}
}

func TestDevStartFitsTheProjectAndTheFlags(t *testing.T) {
	const waitNote = "--wait is now the ASTRO_LOCAL_HEALTH_TIMEOUT environment variable, a Go duration: ASTRO_LOCAL_HEALTH_TIMEOUT="
	const restartNote = "With nothing running, restart starts in standalone mode, which does not build the Dockerfile; use `astro local start --docker` then"
	withDockerfile := devContext{dockerfile: true, buildSecret: true, packageBuildSecret: true}
	cases := []struct {
		name  string
		args  []string
		dc    devContext
		want  string
		notes []string
	}{
		{name: "start", args: []string{"start"}, dc: withDockerfile, want: "astro local start --docker"},
		{
			name:  "start with a build secret and a wait",
			args:  []string{"start", "--build-secret", "id=netrc,env=NETRC_CONTENT", "--wait", "5m"},
			dc:    withDockerfile,
			want:  "astro local start --docker --build-secret id=netrc,env=NETRC_CONTENT",
			notes: []string{waitNote + "5m astro local start --docker --build-secret id=netrc,env=NETRC_CONTENT"},
		},
		{
			name:  "restart keeps its mode and takes the old plural flag",
			args:  []string{"restart", "--build-secrets=id=netrc,src=/home/me/.netrc"},
			dc:    withDockerfile,
			want:  "astro local restart --build-secret id=netrc,src=/home/me/.netrc",
			notes: []string{restartNote},
		},
		{
			name: "build carries the build secret to astro package",
			args: []string{"build", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			dc:   withDockerfile,
			want: "astro package --build-secret id=netrc,env=NETRC_CONTENT",
		},
		{
			name: "a package that takes no --build-secret is not given one",
			args: []string{"build", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			dc:   devContext{dockerfile: true, buildSecret: true},
			want: "astro package",
		},
		{
			name: "a spec with a space is not repeated",
			args: []string{"start", "--build-secret", "id=x,src=a b"},
			dc:   withDockerfile,
			want: "astro local start --docker --build-secret <spec>",
		},
		{
			name:  "a bare wait does not take the next flag as its value",
			args:  []string{"start", "--wait", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			dc:    withDockerfile,
			want:  "astro local start --docker --build-secret id=netrc,env=NETRC_CONTENT",
			notes: []string{waitNote + "10m astro local start --docker --build-secret id=netrc,env=NETRC_CONTENT"},
		},
		{
			name:  "a wait that is not a duration is not repeated",
			args:  []string{"start", "--wait=soon"},
			dc:    withDockerfile,
			want:  "astro local start --docker",
			notes: []string{waitNote + "10m astro local start --docker"},
		},
		{
			name:  "no Dockerfile, so no Docker mode and no build secret",
			args:  []string{"start", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			dc:    devContext{buildSecret: true},
			want:  "astro local start",
			notes: []string{"--build-secret applies only to a project that declares [tool.astro] dockerfile"},
		},
		{
			name: "a v1 project with a build secret converts to a Docker-mode build",
			args: []string{"start", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			dc:   devContext{v1: true, buildSecret: true},
			want: "astro local start --docker --build-secret id=netrc,env=NETRC_CONTENT",
		},
		{
			name: "a start that takes no --build-secret is not given one",
			args: []string{"start", "--build-secret", "id=netrc,env=NETRC_CONTENT"},
			dc:   devContext{dockerfile: true},
			want: "astro local start --docker",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := buildDevRemoved(devTypedSubcommand(tc.args), tc.args, tc.dc)
			if p.Replacement != tc.want {
				t.Errorf("replacement = %q, want %q", p.Replacement, tc.want)
			}
			if !slices.Equal(p.Notes, tc.notes) {
				t.Errorf("notes = %q, want %q", p.Notes, tc.notes)
			}
		})
	}
}

// The stub reads the project's manifest and the real command tree: a declared
// Dockerfile gives --docker, and a build secret is carried exactly when
// `astro local start` takes the flag.
func TestDevStartReadsTheProjectAndTheTree(t *testing.T) {
	dir := t.TempDir()
	pyproject := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n" +
		"[tool.astro]\ndockerfile = 'Dockerfile'\n"
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(pyproject), 0o600); err != nil {
		t.Fatal(err)
	}
	d, out := testDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }
	if err := execute(t, d, "dev", "start", "--build-secret", "id=netrc,env=NETRC_CONTENT", "--output", "json"); err == nil {
		t.Fatal("astro dev must fail")
	}
	var payload devRemoved
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out.String())
	}
	want := "astro local start --docker"
	if takesFlag(newRootCmd(d), []string{"local", nameStart}, "build-secret") {
		want += " --build-secret id=netrc,env=NETRC_CONTENT"
	}
	if payload.Replacement != want {
		t.Errorf("replacement = %q, want %q", payload.Replacement, want)
	}
}

func TestDevStubRendersNotesOnTheirOwnLine(t *testing.T) {
	p := buildDevRemoved("start", []string{"start", "--wait", "5m"}, devContext{})
	text := renderDevRemoved(p)
	if !strings.Contains(text, "instead.\n--wait is now the ASTRO_LOCAL_HEALTH_TIMEOUT environment variable, a Go duration: "+
		"ASTRO_LOCAL_HEALTH_TIMEOUT=5m astro local start\n") {
		t.Errorf("text does not carry the --wait note:\n%s", text)
	}
}

func TestDevStubJSONOutput(t *testing.T) {
	d, out := testDeps(t)
	err := execute(t, d, "dev", "ps", "--output", "json")
	if err == nil {
		t.Fatal("json mode must still fail")
	}
	var payload struct {
		Error       string `json:"error"`
		Typed       string `json:"typed_command"`
		Replacement string `json:"replacement"`
		Mapping     []struct {
			Command     string `json:"command"`
			Replacement string `json:"replacement"`
		} `json:"mapping"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &payload); jsonErr != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", jsonErr, out.String())
	}
	if payload.Replacement != "astro local status" {
		t.Errorf("replacement = %q, want astro local status", payload.Replacement)
	}
	if payload.Typed != "astro dev ps" {
		t.Errorf("typed_command = %q", payload.Typed)
	}
	if len(payload.Mapping) == 0 {
		t.Errorf("payload missing mapping: %+v", payload)
	}
}

func TestDevStubTextMatchesJSONData(t *testing.T) {
	p := buildDevRemoved("ps", nil, devContext{})
	text := renderDevRemoved(p)
	if !strings.Contains(text, p.Error) || !strings.Contains(text, p.Replacement) {
		t.Errorf("text rendering dropped payload data:\n%s", text)
	}
}

// The call site, not just the function: testing warnEnvValues directly cannot
// catch a missing call, so this drives the real command.
//
// fakeRuntime.Start returns ErrNotImplemented, so the command fails, and that
// is load-bearing: the warning is emitted before the runtime is asked for
// anything, so its presence on stdout shows the report happens on the way to
// starting rather than after a successful start.
func TestStartReportsEnvValueWarnings(t *testing.T) {
	d, stdout := testDeps(t)
	dir := t.TempDir()
	m := `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.env]
ASTRO_TEST_PORT = { type = 'port', default = '99999' }
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	isolateEnvSources(t, "ASTRO_TEST_PORT")

	// The error is the fake runtime refusing to start, not a rejected project.
	_ = execute(t, d, "local", "start")

	if !strings.Contains(stdout.String(), "warning: env var ASTRO_TEST_PORT:") {
		t.Errorf("start did not report the value warning; stdout was %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "99999") {
		t.Errorf("the warning does not name the offending value; stdout was %q", stdout.String())
	}
}

// The mirror: a conforming project prints no value warnings, so the common case
// stays quiet.
func TestStartReportsNothingWhenValuesConform(t *testing.T) {
	d, stdout := testDeps(t)
	dir := t.TempDir()
	m := `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.env]
ASTRO_TEST_PORT = { type = 'port', default = '8080' }
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	isolateEnvSources(t, "ASTRO_TEST_PORT")

	_ = execute(t, d, "local", "start")

	if strings.Contains(stdout.String(), "warning: env var") {
		t.Errorf("expected no value warning, got %q", stdout.String())
	}
}

// fakeAirflow is attachable and stoppable, which fakeRuntime's Airflow is not.
// runRestart attaches before it reports anything, so without this the restart
// path bails at attach and never reaches the warnings.
type fakeAirflow struct{}

func (fakeAirflow) Stop(context.Context, localrt.StopOptions) error    { return nil }
func (fakeAirflow) Status() (localrt.Status, error)                    { return localrt.Status{}, nil }
func (fakeAirflow) Logs(context.Context, localrt.LogOptions) error     { return nil }
func (fakeAirflow) Run(context.Context, []string, localrt.Stdio) error { return nil }
func (fakeAirflow) Shell(context.Context, localrt.Stdio) error         { return nil }
func (fakeAirflow) Env() ([]string, error)                             { return nil, nil }

// attachableRuntime is fakeRuntime with a working Attach and a running status.
// Both are needed to reach the reporting in runRestart: it reads the status
// first and falls back to a plain start when nothing is running, then attaches
// before it reports. Start still fails, which is fine — the warnings are
// emitted before the restart tries to start anything.
type attachableRuntime struct{ fakeRuntime }

func (attachableRuntime) Attach(string) (localrt.Airflow, error) { return fakeAirflow{}, nil }

func (attachableRuntime) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{State: localrt.StateRunning, Mode: localrt.ModeStandalone}, nil
}

// `astro local restart` reports value warnings too. warnEnvValues has two call
// sites, and a test of one says nothing about the other.
func TestRestartReportsEnvValueWarnings(t *testing.T) {
	d, stdout := testDeps(t)
	d.Runtime = attachableRuntime{}
	dir := t.TempDir()
	m := `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.env]
ASTRO_TEST_PORT = { type = 'port', default = '99999' }
`
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	isolateEnvSources(t, "ASTRO_TEST_PORT")

	_ = execute(t, d, "local", "restart")

	if !strings.Contains(stdout.String(), "warning: env var ASTRO_TEST_PORT:") {
		t.Errorf("restart did not report the value warning; stdout was %q", stdout.String())
	}
}

// isolateEnvSources cuts every ambient source the resolver consults above a
// manifest default: HOME and USERPROFILE for `~/.astro/env`, XDG_CACHE_HOME for
// user state, and the declared names themselves. testDeps sets none of them.
//
// Shell env outranks a manifest default, so any name a test relies on
// defaulting has to be unset. t.Setenv cannot unset, and setting "" is not
// absence, so the original is registered for restoration and then removed.
func isolateEnvSources(t *testing.T, declared ...string) {
	t.Helper()
	for _, n := range declared {
		if old, ok := os.LookupEnv(n); ok {
			t.Setenv(n, old) // registers the restore
		}
		if err := os.Unsetenv(n); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}
