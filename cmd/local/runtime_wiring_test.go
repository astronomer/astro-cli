package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// These tests are about the wiring: that every run path actually calls the
// Dockerfile check and the runtime-build check, which are tested themselves in
// pkg/scaffold and pkg/runtimeversions.

// startRecorder is a runtime that records whether a start reached it, and
// reports a running Airflow in mode for restart to find.
type startRecorder struct {
	fakeRuntime
	started *bool
	mode    localrt.Mode
}

func (s startRecorder) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	*s.started = true
	return nil, localrt.ErrNotImplemented
}

func (s startRecorder) Attach(string) (localrt.Airflow, error) { return fakeAirflow{}, nil }

func (s startRecorder) ReadStatus(string) (localrt.Status, error) {
	if s.mode == "" {
		return localrt.Status{}, localrt.ErrNotImplemented
	}
	return localrt.Status{State: localrt.StateRunning, Mode: s.mode}, nil
}

// wiringProject writes a manifest, and a Dockerfile when from is set, and
// points d at them.
func wiringProject(t *testing.T, d *Deps, astro, from string) {
	t.Helper()
	dir := t.TempDir()
	m := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n" + astro
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if from != "" {
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM "+from+"\nRUN echo hi\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	isolateEnvSources(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

const mismatchedDockerfile = "dockerfile = 'Dockerfile'\n"

func isDockerfileMismatch(err error) bool {
	var ve *manifest.ValidationError
	return errors.As(err, &ve) && len(ve.Problems) == 1 && ve.Problems[0].Code == manifest.CodeDockerfileAirflowMismatch
}

// Start refuses in both modes, and restart before it stops anything.
func TestStartAndRestartRefuseADockerfileOfAnotherAirflow(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode // what restart finds running
	}{
		{name: "start", args: []string{"local", "start"}},
		{name: "start --docker", args: []string{"local", "start", "--docker"}},
		{name: "restart standalone", args: []string{"local", "restart"}, mode: localrt.ModeStandalone},
		{name: "restart docker", args: []string{"local", "restart"}, mode: localrt.ModeDocker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			started := false
			d.Runtime = startRecorder{started: &started, mode: tc.mode}
			wiringProject(t, &d, mismatchedDockerfile, "astrocrpublic.azurecr.io/runtime:3.3-8")

			err := execute(t, d, tc.args...)
			if !isDockerfileMismatch(err) {
				t.Fatalf("err = %v, want the Dockerfile mismatch", err)
			}
			if started {
				t.Error("the start reached the runtime")
			}
		})
	}
}

// A Dockerfile the check cannot compare starts as it did.
func TestStartGoesAheadWithADockerfileItCannotCompare(t *testing.T) {
	d, _ := testDeps(t)
	started := false
	d.Runtime = startRecorder{started: &started}
	wiringProject(t, &d, mismatchedDockerfile, "astrocrpublic.azurecr.io/runtime@sha256:0123")

	_ = execute(t, d, "local", "start", "--docker")
	if !started {
		t.Error("a digest-pinned base was refused")
	}
}

func TestCheckRefusesADockerfileOfAnotherAirflow(t *testing.T) {
	for _, args := range [][]string{
		{"local", "check"},
		{"local", "check", "--target", "astro"},
		{"local", "check", "--target", "mwaa"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, out := testDeps(t)
			wiringProject(t, &d, mismatchedDockerfile, "astrocrpublic.azurecr.io/runtime:3.3-8")

			err := execute(t, d, args...)
			var exit *cliout.ExitError
			if !errors.As(err, &exit) || exit.Code != checks.ExitEnvNotReady {
				t.Fatalf("err = %v, want exit %d", err, checks.ExitEnvNotReady)
			}
			if !strings.Contains(out.String(), "tool.astro.dockerfile") || !strings.Contains(out.String(), "runtime:3.3-8") {
				t.Errorf("stdout should name the Dockerfile and its FROM: %q", out.String())
			}
		})
	}
}

func TestPackageRefusesADockerfileOfAnotherAirflow(t *testing.T) {
	for _, target := range []string{"astro", "mwaa", "composer"} {
		t.Run(target, func(t *testing.T) {
			d, _ := testDeps(t)
			wiringProject(t, &d, mismatchedDockerfile, "astrocrpublic.azurecr.io/runtime:3.3-8")
			if err := execute(t, d, "package", target, "--out-dir", t.TempDir()); !isDockerfileMismatch(err) {
				t.Fatalf("err = %v, want the Dockerfile mismatch", err)
			}
		})
	}
}

// runtimeCheckRecorder is a Deps.RuntimeCheck that records its calls and
// answers with warnings and err.
type runtimeCheckRecorder struct {
	calls    [][2]string
	warnings []runtimeversions.Finding
	err      error
}

func (r *runtimeCheckRecorder) check(_ context.Context, runtime, pin string) ([]runtimeversions.Finding, error) {
	r.calls = append(r.calls, [2]string{runtime, pin})
	return r.warnings, r.err
}

// A Docker-mode start and restart hold the runtime build to the catalog before
// starting; standalone builds no image and does not.
func TestStartChecksTheRuntimeBuildInDockerModeOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		mode  localrt.Mode
		check bool
	}{
		{name: "start", args: []string{"local", "start"}},
		{name: "start --docker", args: []string{"local", "start", "--docker"}, check: true},
		{name: "restart standalone", args: []string{"local", "restart"}, mode: localrt.ModeStandalone},
		{name: "restart docker", args: []string{"local", "restart"}, mode: localrt.ModeDocker, check: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, stdout := testDeps(t)
			started := false
			d.Runtime = startRecorder{started: &started, mode: tc.mode}
			rec := &runtimeCheckRecorder{warnings: []runtimeversions.Finding{{Kind: runtimeversions.FindingYanked, Message: "runtime 3.1-12 is yanked"}}}
			d.RuntimeCheck = rec.check
			wiringProject(t, &d, "runtime = '3.1-12'\n", "")

			_ = execute(t, d, tc.args...)
			if !tc.check {
				if len(rec.calls) != 0 {
					t.Errorf("standalone checked the runtime build: %v", rec.calls)
				}
				return
			}
			if len(rec.calls) != 1 || rec.calls[0] != [2]string{"3.1-12", "3.1"} {
				t.Errorf("calls = %v, want one for 3.1-12 against 3.1", rec.calls)
			}
			if !strings.Contains(stdout.String(), "warning: pyproject.toml: tool.astro.runtime: runtime 3.1-12 is yanked") {
				t.Errorf("the warning was not shown: %q", stdout.String())
			}
			if !started {
				t.Error("a warning stopped the start")
			}
		})
	}
}

func TestADockerStartRefusedByTheRuntimeCheckStartsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode
	}{
		{name: "start", args: []string{"local", "start", "--docker"}},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeDocker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			started := false
			d.Runtime = startRecorder{started: &started, mode: tc.mode}
			blocking := &runtimeversions.RuntimeError{Finding: runtimeversions.Finding{Kind: runtimeversions.FindingSeriesMismatch, Blocking: true, Message: "another series"}}
			rec := &runtimeCheckRecorder{err: blocking}
			d.RuntimeCheck = rec.check
			wiringProject(t, &d, "runtime = '3.1-12'\n", "")

			err := execute(t, d, tc.args...)
			if !errors.Is(err, blocking) {
				t.Fatalf("err = %v, want the blocking finding", err)
			}
			if started {
				t.Error("the start reached the runtime")
			}
		})
	}
}

func TestPackageChecksTheRuntimeBuild(t *testing.T) {
	d, _ := testDeps(t)
	blocking := errors.New("another series")
	rec := &runtimeCheckRecorder{err: blocking}
	d.RuntimeCheck = rec.check
	wiringProject(t, &d, "runtime = '3.1-12'\n", "")

	if err := execute(t, d, "package", "astro"); !errors.Is(err, blocking) {
		t.Fatalf("err = %v, want the blocking finding", err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != [2]string{"3.1-12", "3.1"} {
		t.Errorf("calls = %v", rec.calls)
	}
}

// A Docker-mode start and restart of a project with its own Dockerfile warn
// about the per-machine files that build would copy into the image, and go on.
// Standalone builds no image, so it says nothing.
func TestStartWarnsAboutLocalFilesInTheImageInDockerModeOnly(t *testing.T) {
	const warning = "warning: the image built from Dockerfile would copy in these per-machine files: " +
		".astro/standalone. To keep them out, add it to .dockerignore"
	for _, tc := range []struct {
		name string
		args []string
		mode localrt.Mode
		warn bool
	}{
		{name: "start", args: []string{"local", "start"}},
		{name: "start --docker", args: []string{"local", "start", "--docker"}, warn: true},
		{name: "restart standalone", args: []string{"local", "restart"}, mode: localrt.ModeStandalone},
		{name: "restart docker", args: []string{"local", "restart"}, mode: localrt.ModeDocker, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, stdout := testDeps(t)
			started := false
			d.Runtime = startRecorder{started: &started, mode: tc.mode}
			wiringProject(t, &d, "dockerfile = 'Dockerfile'\n", "astrocrpublic.azurecr.io/runtime:3.1-12")
			dir, _ := d.WorkingDir()
			if err := os.MkdirAll(filepath.Join(dir, ".astro", "standalone"), 0o750); err != nil {
				t.Fatal(err)
			}

			_ = execute(t, d, tc.args...)
			if got := strings.Contains(stdout.String(), warning); got != tc.warn {
				t.Errorf("warned = %v, want %v: %q", got, tc.warn, stdout.String())
			}
			if !started {
				t.Error("the start did not reach the runtime")
			}
		})
	}
}
