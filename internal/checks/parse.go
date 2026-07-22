package checks

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// parseScript is the DAG-parse program, run with the project's own venv
// Python. It reports structured facts; this package decides what they mean.
//
//go:embed parse_dags.py
var parseScript []byte

// ParseInput is what the parser needs to inspect one project.
type ParseInput struct {
	ProjectPath string
	DagsDir     string
}

// ParseReport is the decoded output of parse_dags.py. Fatal is set when the
// script could not build a DagBag at all (no Airflow in the venv); the other
// fields are then empty.
type ParseReport struct {
	Fatal        string            `json:"fatal"`
	Dags         []ReportDag       `json:"dags"`
	ImportErrors []ReportImportErr `json:"import_errors"`
	Files        []ReportFile      `json:"files"`
}

// ReportDag is one DAG that loaded without an import error.
type ReportDag struct {
	DagID string `json:"dag_id"`
	File  string `json:"file"`
}

// ReportImportErr is one file that failed to import.
type ReportImportErr struct {
	File    string `json:"file"`
	Message string `json:"message"`
}

// ReportFile is one processed DAG file: its parse time and the dag_ids it
// defined.
type ReportFile struct {
	File         string   `json:"file"`
	ParseSeconds float64  `json:"parse_time_seconds"`
	DagIDs       []string `json:"dag_ids"`
}

// Executor runs the parse program and returns its stdout. It is a seam so
// tests never spawn a process. stderr is captured only to enrich an error.
type Executor interface {
	Run(ctx context.Context, dir string, env []string, name string, args []string, stdin []byte) (stdout []byte, err error)
}

// VenvRunner is the production Parser: it runs the embedded script with the
// project's .venv Python, AIRFLOW_HOME pointed at a throwaway directory so the
// parse never touches the user's project tree.
type VenvRunner struct {
	// Exec runs the process; execExecutor in production.
	Exec Executor
	// pythonPath resolves the venv interpreter for a project; a field so tests
	// can point it at a stub.
	pythonPath func(projectPath string) string
	// tempHome makes the throwaway AIRFLOW_HOME and a cleanup; a field for the
	// same reason.
	tempHome func() (dir string, cleanup func(), err error)
}

// NewVenvRunner builds the production runner.
func NewVenvRunner() *VenvRunner {
	return &VenvRunner{
		Exec:       execExecutor{},
		pythonPath: venvPython,
		tempHome:   defaultTempHome,
	}
}

// venvPython is the interpreter inside a project's .venv. The layout differs
// on Windows, so switch on GOOS for the value rather than build-tagging the
// file (docs/v2-architecture.md conventions).
func venvPython(projectPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(projectPath, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(projectPath, ".venv", "bin", "python")
}

func defaultTempHome() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "astro-check-home-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// Parse runs the embedded script and decodes its report. A missing venv
// interpreter is ErrEnvNotReady; the script itself is fed on stdin so nothing
// is written to the project.
func (r *VenvRunner) Parse(ctx context.Context, in ParseInput) (ParseReport, error) {
	python := r.pythonPath(in.ProjectPath)
	if _, err := os.Stat(python); err != nil {
		return ParseReport{}, fmt.Errorf(
			"no Python found at %s — run `astro local start` to build the project environment first: %w",
			python, ErrEnvNotReady,
		)
	}

	home, cleanup, err := r.tempHome()
	if err != nil {
		return ParseReport{}, fmt.Errorf("preparing a scratch AIRFLOW_HOME: %w", err)
	}
	defer cleanup()

	// The script writes its JSON here rather than to stdout: Airflow 3 spews
	// log lines to stdout while building a DagBag (and from a signal handler on
	// an import timeout, which no logging switch can mute), so stdout can't be
	// kept clean for the result. A private file in the scratch home can be.
	resultFile := filepath.Join(home, "parse-result.json")

	env := append(
		os.Environ(),
		"AIRFLOW_HOME="+home,
		"AIRFLOW__CORE__DAGS_FOLDER="+in.DagsDir,
		"AIRFLOW__CORE__LOAD_EXAMPLES=False",
		"ASTRO_PARSE_RESULT_FILE="+resultFile,
	)
	args := []string{"-", in.ProjectPath, in.DagsDir}
	stdout, err := r.Exec.Run(ctx, in.ProjectPath, env, python, args, parseScript)
	if err != nil {
		return ParseReport{}, fmt.Errorf("running the DAG parse: %w", err)
	}

	// Prefer the result file; fall back to stdout for a script that ignored the
	// env var (older embed, or a test's fake executor).
	raw := bytes.TrimSpace(stdout)
	if fromFile, ferr := os.ReadFile(resultFile); ferr == nil && len(bytes.TrimSpace(fromFile)) > 0 {
		raw = bytes.TrimSpace(fromFile)
	}

	var report ParseReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return ParseReport{}, fmt.Errorf("decoding the DAG parse output: %w", err)
	}
	return report, nil
}

// execExecutor is the production Executor, backed by os/exec.
type execExecutor struct{}

func (execExecutor) Run(ctx context.Context, dir string, env []string, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is the project's own venv Python, args are the embedded script and project paths
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && stderr.Len() > 0 {
			return stdout.Bytes(), fmt.Errorf("%w: %s", err, stderr.String())
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}
