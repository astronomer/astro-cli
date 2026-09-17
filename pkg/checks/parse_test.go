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
	// And the other half. A docker-mode project builds its environment in the
	// image and never writes a .venv, so "run astro local start" on its own
	// was advice that changes nothing: the user runs it, docker mode starts
	// fine, and the check fails again with the same sentence. This package
	// cannot tell the modes apart, so it names the command that works in
	// either.
	assert.Contains(t, err.Error(), "uv sync",
		"a docker-mode project needs a command that builds an interpreter without starting Airflow")
	// The sentinel leads. Wrapped at the end it landed after a colon, so the
	// line finished "…is what a docker-mode project needs: project environment
	// is not ready to check", reading as the object of that sentence — and
	// cmd/local prints this verbatim.
	assert.True(t, strings.HasPrefix(err.Error(), ErrEnvNotReady.Error()+":"),
		"the sentinel should lead, not trail a colon: %q", err.Error())
}

// A .venv that exists but cannot be used is a different problem, and neither
// suggested command fixes it — so it is reported rather than described as one
// that was never built.
func TestParseUnusableVenvSaysWhatWentWrong(t *testing.T) {
	dir := t.TempDir()
	// A symlink to nothing: it exists, and stat fails for a reason that is not
	// "never created".
	link := filepath.Join(dir, "python")
	require.NoError(t, os.Symlink(filepath.Join(dir, "gone"), link))

	r := &VenvRunner{
		Exec:       &fakeExec{},
		pythonPath: func(string) string { return link },
		tempHome:   defaultTempHome,
	}
	_, err := r.Parse(context.Background(), ParseInput{ProjectPath: "/p", DagsDir: "/p/dags"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvNotReady, "it is still an environment problem")
	// A dangling symlink stats as not-exist, so this one takes the ordinary
	// path; what matters is that the sentinel still leads and the advice is
	// there. The unusable branch is exercised by a stat error that is not
	// not-exist, which a test cannot portably manufacture without root.
	assert.True(t, strings.HasPrefix(err.Error(), ErrEnvNotReady.Error()+":"), err.Error())
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

// fileWritingExec mimics the real script: it writes the JSON result to the
// file named by ASTRO_PARSE_RESULT_FILE and returns whatever stdout spam it was
// given. It exists to prove Parse reads the result from the file and ignores a
// polluted stdout.
type fileWritingExec struct {
	result []byte
	stdout []byte
}

func (f *fileWritingExec) Run(_ context.Context, _ string, env []string, _ string, _ []string, _ []byte) ([]byte, error) {
	for _, kv := range env {
		if path, ok := strings.CutPrefix(kv, "ASTRO_PARSE_RESULT_FILE="); ok {
			if err := os.WriteFile(path, f.result, 0o600); err != nil {
				return nil, err
			}
		}
	}
	return f.stdout, nil
}

// TestParseReadsResultFileDespiteStdoutPollution reproduces the Airflow 3 bug:
// the DagBag writes log lines to stdout before, and (on an import timeout) after
// the result. The trailing line — a bare timestamp Go's JSON decoder reads as a
// number and then chokes on the '-' — is exactly what broke a stdout-only
// decode. Parse must take the result from the file and pass regardless.
func TestParseReadsResultFileDespiteStdoutPollution(t *testing.T) {
	spam := "[2026-07-20T12:00:00.000+0000] {dagbag.py:591} INFO - Filling up the DagBag\n" +
		"airflow.exceptions.AirflowTaskTimeout: DagBag import timeout for dags/slow.py after 30.0s\n" +
		"2026-07-20T12:00:00.200Z [info] scheduler shutting down\n"
	ex := &fileWritingExec{
		result: []byte(`{"schema_version":1,"dags":[{"dag_id":"a","file":"dags/a.py"}],"import_errors":[],"files":[]}`),
		stdout: []byte(spam),
	}
	r := runnerWithPython(t, ex)

	report, err := r.Parse(context.Background(), ParseInput{ProjectPath: "/proj", DagsDir: "/proj/dags"})
	require.NoError(t, err, "stdout spam around the JSON must not break decoding")
	require.Len(t, report.Dags, 1)
	assert.Equal(t, "a", report.Dags[0].DagID)
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

// The script's files_unavailable reaches the report. It is the only way the Go
// side can tell "no per-file stats" from "no DAG files".
func TestParseDecodesFilesUnavailable(t *testing.T) {
	ex := &fakeExec{stdout: []byte(`{"schema_version":1,"dags":[{"dag_id":"a","file":"dags/a.py"}],"import_errors":[],"files":[],"files_unavailable":true}`)}
	r := runnerWithPython(t, ex)

	report, err := r.Parse(context.Background(), ParseInput{ProjectPath: "proj", DagsDir: "dags"})
	require.NoError(t, err)
	assert.True(t, report.FilesUnavailable)
	assert.Empty(t, report.Files)

	// And absent means false rather than unknown, so an older-shaped payload
	// reads as "stats were fine".
	ex2 := &fakeExec{stdout: []byte(`{"schema_version":1,"dags":[],"import_errors":[],"files":[]}`)}
	report2, err := runnerWithPython(t, ex2).Parse(context.Background(), ParseInput{ProjectPath: "proj", DagsDir: "dags"})
	require.NoError(t, err)
	assert.False(t, report2.FilesUnavailable)
}
