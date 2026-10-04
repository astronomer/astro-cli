package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// A standalone start and restart build the venv on the Python the project's
// image runs, read from the runtime catalog; a docker start leaves the choice
// to the image build.
func TestStandaloneStartRunsTheImagesPython(t *testing.T) {
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
    "3.1-12": {"metadata": {"airflowVersion": "3.1.8", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.12"}}
  }}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		args     []string
		mode     localrt.Mode
		requires string
		want     string
		warning  string
	}{
		{name: "start, the build's default", args: []string{"local", "start"}, requires: ">=3.10", want: "3.12"},
		{name: "start, another Python", args: []string{"local", "start"}, requires: "==3.13.*", want: "3.13"},
		{name: "restart", args: []string{"local", "restart"}, mode: localrt.ModeStandalone, requires: "==3.13.*", want: "3.13"},
		{name: "docker start", args: []string{"local", "start", "--docker"}, requires: "==3.13.*", want: ""},
		{
			name: "no Python the build ships", args: []string{"local", "start"}, requires: "==3.11.*", want: "",
			warning: "requires-python ==3.11.* in pyproject.toml admits none of the Pythons runtime 3.1-12 ships (3.12, 3.13)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, stdout := testDeps(t)
			var plans []localrt.Plan
			d.Runtime = planRecorder{plans: &plans, mode: tc.mode}
			d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return c }
			stderr := &bytes.Buffer{}
			d.Stderr = stderr
			dir := t.TempDir()
			m := "[project]\nname = 'demo'\nrequires-python = '" + tc.requires + "'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			isolateEnvSources(t)
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			_ = execute(t, d, tc.args...)
			if len(plans) != 1 {
				t.Fatalf("started %d times, want 1", len(plans))
			}
			if plans[0].PythonVersion != tc.want {
				t.Errorf("PythonVersion = %q, want %q", plans[0].PythonVersion, tc.want)
			}
			out := stdout.String() + stderr.String()
			if tc.warning != "" && !strings.Contains(out, tc.warning) {
				t.Errorf("output does not warn %q:\n%s", tc.warning, out)
			}
			if tc.warning == "" && strings.Contains(out, "admits none") {
				t.Errorf("unexpected Python warning:\n%s", out)
			}
		})
	}
}
