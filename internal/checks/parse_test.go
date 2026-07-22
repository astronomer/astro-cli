package checks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeExec records what it was asked to run and returns a canned result.
type fakeExec struct {
	stdout []byte
	err    error

	gotName  string
	gotArgs  []string
	gotEnv   []string
	gotStdin []byte
}

func (f *fakeExec) Run(_ context.Context, _ string, env []string, name string, args []string, stdin []byte) ([]byte, error) {
	f.gotName, f.gotArgs, f.gotEnv, f.gotStdin = name, args, env, stdin
	return f.stdout, f.err
}

// runnerWithPython builds a VenvRunner whose venv Python is a file that exists,
// so Parse gets past the environment check and into exec.
func runnerWithPython(t *testing.T, ex Executor) *VenvRunner {
	t.Helper()
	python := filepath.Join(t.TempDir(), "python")
	require.NoError(t, os.WriteFile(python, []byte("#!/bin/sh\n"), 0o755))
	return &VenvRunner{
		Exec:       ex,
		pythonPath: func(string) string { return python },
		tempHome:   defaultTempHome,
	}
}

func TestParseMissingVenvIsEnvNotReady(t *testing.T) {
	r := &VenvRunner{
		Exec:       &fakeExec{},
		pythonPath: func(string) string { return filepath.Join(t.TempDir(), "does", "not", "exist") },
		tempHome:   defaultTempHome,
	}
	_, err := r.Parse(context.Background(), ParseInput{ProjectPath: "/p", DagsDir: "/p/dags"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvNotReady)
	assert.Contains(t, err.Error(), "astro local start")
}

func TestParseDecodesReportAndFeedsScriptOnStdin(t *testing.T) {
	ex := &fakeExec{stdout: []byte(`{"schema_version":1,"dags":[{"dag_id":"a","file":"dags/a.py"}],"import_errors":[],"files":[{"file":"dags/a.py","parse_time_seconds":0.3,"dag_ids":["a"]}]}`)}
	r := runnerWithPython(t, ex)

	report, err := r.Parse(context.Background(), ParseInput{ProjectPath: "/proj", DagsDir: "/proj/dags"})
	require.NoError(t, err)
	assert.Empty(t, report.Fatal)
	require.Len(t, report.Dags, 1)
	assert.Equal(t, "a", report.Dags[0].DagID)
	require.Len(t, report.Files, 1)
	assert.InDelta(t, 0.3, report.Files[0].ParseSeconds, 1e-9)

	// The script is piped in, not written to disk, and the project paths are
	// passed as arguments.
	assert.Equal(t, parseScript, ex.gotStdin)
	assert.Equal(t, []string{"-", "/proj", "/proj/dags"}, ex.gotArgs)
	assert.Contains(t, ex.gotEnv, "AIRFLOW__CORE__DAGS_FOLDER=/proj/dags")
	assert.True(t, hasEnvPrefix(ex.gotEnv, "AIRFLOW_HOME="), "AIRFLOW_HOME must be set to a scratch dir")
}

func TestParseFatalReportDecodesWithoutError(t *testing.T) {
	ex := &fakeExec{stdout: []byte(`{"fatal":"ModuleNotFoundError: No module named 'airflow'"}`)}
	r := runnerWithPython(t, ex)
	report, err := r.Parse(context.Background(), ParseInput{ProjectPath: "/p", DagsDir: "/p/dags"})
	require.NoError(t, err)
	assert.Contains(t, report.Fatal, "airflow")
}

func TestParseGarbledOutputIsAnError(t *testing.T) {
	ex := &fakeExec{stdout: []byte("not json")}
	r := runnerWithPython(t, ex)
	_, err := r.Parse(context.Background(), ParseInput{ProjectPath: "/p", DagsDir: "/p/dags"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding")
}

// TestScriptRunsUnderRealPython exercises the embedded script end to end with a
// real interpreter, proving it runs and emits JSON the Go side decodes. Airflow
// is not installed in the test environment, so the script reports that in its
// "fatal" field — which is exactly the decodable-output path we want to prove.
// When Airflow is present the same run would return a full report. Skipped when
// no python3 is on PATH; reported honestly either way.
func TestScriptRunsUnderRealPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH; skipping the embedded-script integration test")
	}
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, "dags"), 0o755))

	r := &VenvRunner{
		Exec:       execExecutor{},
		pythonPath: func(string) string { return python },
		tempHome:   defaultTempHome,
	}
	report, err := r.Parse(context.Background(), ParseInput{
		ProjectPath: project,
		DagsDir:     filepath.Join(project, "dags"),
	})
	require.NoError(t, err, "the script must always emit decodable JSON")
	if report.Fatal == "" {
		t.Logf("airflow is importable; got a full report with %d dags", len(report.Dags))
		return
	}
	assert.Contains(t, strings.ToLower(report.Fatal), "airflow",
		"with no Airflow installed the fatal field should say so")
}

func hasEnvPrefix(env []string, prefix string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}
