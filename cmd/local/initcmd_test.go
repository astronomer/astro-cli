package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/scaffold"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// stubLocker stands in for a uv-backed Locker in cmd tests, so the import path
// runs without a real uv binary.
type stubLocker struct{ err error }

func (l stubLocker) Lock(context.Context, string, uv.Stdio) error { return l.err }

// useLocker points newImportLocker at a stub for the duration of a test.
func useLocker(t *testing.T, locker scaffold.Locker, err error) {
	t.Helper()
	prev := newImportLocker
	newImportLocker = func(context.Context) (scaffold.Locker, error) { return locker, err }
	t.Cleanup(func() { newImportLocker = prev })
}

// plainRepo writes a minimal plain-Airflow repo and returns its path.
func plainRepo(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "dags"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dags", "d.py"), []byte("# dag\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "requirements.txt"), []byte("flask==2.0\nrequests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

// initDeps pins WorkingDir to one directory (testDeps mints a fresh temp dir
// per call, which init tests cannot use).
func initDeps(t *testing.T) (d Deps, dir string, stdout *strings.Builder) {
	t.Helper()
	d, _ = testDeps(t)
	dir = t.TempDir()
	stdout = &strings.Builder{}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Stdout = stdout
	return d, dir, stdout
}

func TestInitScaffoldsTheWorkingDir(t *testing.T) {
	d, dir, stdout := initDeps(t)
	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("astro init: %v", err)
	}
	for _, f := range []string{"pyproject.toml", ".gitignore", "AGENTS.md", "dags", "include", "plugins", "tests"} {
		if _, err := os.Lstat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	out := stdout.String()
	if !strings.Contains(out, "Created astro project") || !strings.Contains(out, replaceStart) {
		t.Errorf("text output incomplete:\n%s", out)
	}
}

func TestInitScaffoldsARelativeDirectory(t *testing.T) {
	d, dir, _ := initDeps(t)
	if err := execute(t, d, "init", "pipelines"); err != nil {
		t.Fatalf("astro init pipelines: %v", err)
	}
	m, err := os.ReadFile(filepath.Join(dir, "pipelines", "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// The surgical TOML editor may quote with ' or ".
	if !strings.Contains(string(m), `name = 'pipelines'`) && !strings.Contains(string(m), `name = "pipelines"`) {
		t.Errorf("manifest does not carry the derived name:\n%s", m)
	}
}

func TestInitJSONOutput(t *testing.T) {
	d, dir, stdout := initDeps(t)
	if err := execute(t, d, "init", "--output", "json"); err != nil {
		t.Fatalf("astro init --output json: %v", err)
	}
	var payload struct {
		Dir     string   `json:"dir"`
		Name    string   `json:"name"`
		Airflow string   `json:"airflow"`
		Created []string `json:"created"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout.String())
	}
	if payload.Dir != dir {
		t.Errorf("dir = %q, want %q", payload.Dir, dir)
	}
	if payload.Name != filepath.Base(dir) {
		t.Errorf("name = %q, want %q", payload.Name, filepath.Base(dir))
	}
	if payload.Airflow == "" || len(payload.Created) == 0 {
		t.Errorf("payload missing airflow or created: %+v", payload)
	}
}

func TestInitRefusesAnExistingProject(t *testing.T) {
	d, dir, _ := initDeps(t)
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := execute(t, d, "init")
	if err == nil || !strings.Contains(err.Error(), "already has a pyproject.toml") {
		t.Errorf("want a clear refusal, got: %v", err)
	}
}

func TestInitFromImportsRepo(t *testing.T) {
	d, dir, stdout := initDeps(t)
	useLocker(t, stubLocker{}, nil)
	src := plainRepo(t)

	if err := execute(t, d, "init", "--from", src); err != nil {
		t.Fatalf("astro init --from: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Errorf("target missing pyproject.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dags", "d.py")); err != nil {
		t.Errorf("dag was not copied: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Imported") || !strings.Contains(out, "lock: resolved") {
		t.Errorf("import output incomplete:\n%s", out)
	}
}

func TestInitFromJSONOutput(t *testing.T) {
	d, dir, stdout := initDeps(t)
	useLocker(t, stubLocker{}, nil)
	src := plainRepo(t)

	if err := execute(t, d, "init", "--from", src, "--output", "json"); err != nil {
		t.Fatalf("astro init --from --output json: %v", err)
	}
	var payload struct {
		Source       string `json:"source"`
		Dir          string `json:"dir"`
		Dependencies int    `json:"dependencies"`
		Dags         int    `json:"dags"`
		Lock         struct {
			Attempted bool `json:"attempted"`
			Locked    bool `json:"locked"`
		} `json:"lock"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout.String())
	}
	if payload.Dir != dir || payload.Dependencies != 2 || payload.Dags != 1 {
		t.Errorf("unexpected payload: %+v", payload)
	}
	if !payload.Lock.Attempted || !payload.Lock.Locked {
		t.Errorf("lock not reported as done: %+v", payload.Lock)
	}
}

func TestInitFromReportsLockFailure(t *testing.T) {
	d, _, stdout := initDeps(t)
	useLocker(t, stubLocker{err: &uv.ResolutionError{Op: "lock", Summary: "no way to satisfy flask and werkzeug"}}, nil)
	src := plainRepo(t)

	// A lock failure leaves the command succeeding: the project is scaffolded.
	if err := execute(t, d, "init", "--from", src); err != nil {
		t.Fatalf("import should not fail on lock error: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "did not resolve") {
		t.Errorf("lock failure not reported:\n%s", out)
	}
}

func TestInitFromSkipsLockWithoutUV(t *testing.T) {
	d, _, stdout := initDeps(t)
	useLocker(t, nil, uv.ErrNotFound)
	src := plainRepo(t)

	if err := execute(t, d, "init", "--from", src); err != nil {
		t.Fatalf("import should run without uv: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "skipped uv lock") {
		t.Errorf("missing-uv note absent:\n%s", out)
	}
}

func TestLocalInitAliasWorks(t *testing.T) {
	d, dir, _ := initDeps(t)
	if err := execute(t, d, "local", "init"); err != nil {
		t.Fatalf("astro local init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Errorf("missing pyproject.toml: %v", err)
	}
}
