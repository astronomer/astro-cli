package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
)

// A misspelled key in a target section warns on both start and restart. Both
// call sites, because a test of one says nothing about the other, and the
// warning being printed at all shows the manifest loaded: it is not a refusal.
func TestStartAndRestartReportUnknownTargetKeys(t *testing.T) {
	const m = `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.targets.mwaa]
regoin = 'us-east-1'
`
	for _, tc := range []struct {
		cmd string
		rt  Runtime
	}{
		{nameStart, fakeRuntime{}},
		{nameRestart, attachableRuntime{}},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			d, stdout := testDeps(t)
			d.Runtime = tc.rt
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			isolateEnvSources(t)

			// The error is the fake runtime refusing to start.
			_ = execute(t, d, "local", tc.cmd)

			want := "warning: pyproject.toml: tool.astro.targets.mwaa.regoin: unknown key"
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("%s did not report the unknown target key; stdout was %q", tc.cmd, stdout.String())
			}
		})
	}
}
