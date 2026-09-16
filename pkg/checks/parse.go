package checks

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// Env is the environment the DAGs are imported under, in os.Environ form.
	// Empty inherits this process's, which is what the CLI wants: it runs from
	// the project directory and its own environment is the user's shell.
	//
	// A caller whose environment is NOT the user's shell has to supply it, or
	// the parse and the real Airflow disagree. A DAG that reads a variable at
	// module scope — `os.environ["SNOWFLAKE_ACCOUNT"]`, a Variable.get default
	// — imports fine under an Airflow started with the project's environment
	// and raises here without it, which reports a working DAG as broken. A GUI
	// that composes the environment itself (project .env, a vault, declared
	// defaults) is exactly that caller.
	//
	// AIRFLOW_HOME, the DAGs folder, the examples switch and the result-file
	// path are appended after this and win: they are what makes the parse
	// side-effect-free, so a caller cannot accidentally point it at a real
	// Airflow home.
	Env []string
}

// ParseReport is the decoded output of parse_dags.py. Fatal is set when the
// script could not build a DagBag at all (no Airflow in the venv); the other
// fields are then empty.
type ParseReport struct {
	Fatal        string            `json:"fatal"`
	Dags         []ReportDag       `json:"dags"`
	ImportErrors []ReportImportErr `json:"import_errors"`
	Files        []ReportFile      `json:"files"`
	// FilesUnavailable reports that Airflow gave no per-file statistics, so
	// the checks derived from them did not run. Distinct from Files being
	// empty, which is a project with no DAG files.
	FilesUnavailable bool `json:"files_unavailable"`
	// ImportTimeoutSeconds is the dagbag_import_timeout this parse ran under:
	// the point at which Airflow abandons a slow file and reports an import
	// error instead. Zero when the parser could not read it, and the
	// slow-parse threshold then falls back to Airflow's documented default.
	//
	// Reported rather than assumed because a project can set it, and the
	// warning is only useful relative to the value actually in force.
	ImportTimeoutSeconds float64 `json:"import_timeout_seconds"`
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

// ParseProgram is the DAG-parse program itself, for a caller that has to run
// it somewhere this process cannot reach.
//
// VenvRunner hands it to a local interpreter on stdin. A caller whose Python
// lives inside a container does the same through its own engine, and needs the
// program, its arguments, its environment and its decoder as separate pieces.
// These four exports are what parseWith itself uses, so there is one definition
// of each instead of a copy per runner — the JSON contract and the environment
// that makes the parse side-effect-free cannot drift between them.
func ParseProgram() []byte { return bytes.Clone(parseScript) }

// ParseArgs are the program's arguments. It takes the project root and the DAGs
// directory and puts both on sys.path, the way a running Airflow does, so
// `include`- and `plugins`-style imports resolve.
//
// The paths are the ones the INTERPRETER will see. A container caller passes
// container paths; the program reports every file relative to projectRoot, so
// its report needs no translation on the way back.
func ParseArgs(projectRoot, dagsDir string) []string {
	return []string{"-", projectRoot, dagsDir}
}

// ParseEnvironment is the environment that keeps the parse side-effect-free:
// a throwaway AIRFLOW_HOME, an explicit DAGs folder, and no examples. These
// must win over anything a caller supplies, so they are applied last.
//
// A map rather than os.Environ's KEY=VALUE pairs, because a caller that has to
// deliver these somewhere else — as container flags, say — would otherwise
// split them apart again, and the obvious spelling of that split is wrong: a
// value may legitimately contain "=", so splitting on anything but the first
// one truncates it. Nothing here holds an "=" today, which is exactly what
// would make the bug wait for the first caller that layers project env through
// the same path. parseWith renders the pairs it needs.
//
// resultFile names the file the program writes its JSON to. Empty means it
// writes to stdout instead, which it keeps clean for the purpose — it moves the
// real stdout aside before importing Airflow and points fd 1 at stderr, so log
// lines (including from a signal handler on an import timeout) cannot reach the
// result. A caller reading stdout must therefore keep the two streams apart:
// one pseudo-terminal for both would merge them and corrupt the JSON.
func ParseEnvironment(airflowHome, dagsDir, resultFile string) map[string]string {
	env := map[string]string{
		"AIRFLOW_HOME":                 airflowHome,
		"AIRFLOW__CORE__DAGS_FOLDER":   dagsDir,
		"AIRFLOW__CORE__LOAD_EXAMPLES": "False",
	}
	if resultFile != "" {
		env["ASTRO_PARSE_RESULT_FILE"] = resultFile
	}
	return env
}

// DecodeParseReport decodes the program's output.
func DecodeParseReport(raw []byte) (ParseReport, error) {
	var report ParseReport
	if err := json.Unmarshal(bytes.TrimSpace(raw), &report); err != nil {
		return ParseReport{}, fmt.Errorf("decoding the DAG parse output: %w", err)
	}
	return report, nil
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

// venvPython is the interpreter inside a project's .venv.
func venvPython(projectPath string) string {
	return VenvInterpreter(filepath.Join(projectPath, ".venv"))
}

func defaultTempHome() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "astro-check-home-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil //nolint:errcheck // best-effort cleanup of the temp home
}

// Parse runs the embedded script with the project's own .venv interpreter and
// decodes its report. A missing venv interpreter is ErrEnvNotReady; the script
// itself is fed on stdin so nothing is written to the project.
func (r *VenvRunner) Parse(ctx context.Context, in ParseInput) (ParseReport, error) {
	python := r.pythonPath(in.ProjectPath)
	if _, err := os.Stat(python); err != nil {
		return ParseReport{}, fmt.Errorf(
			"no Python found at %s — run `astro local start` to build the project environment first: %w",
			python, ErrEnvNotReady,
		)
	}
	return r.parseWith(ctx, python, in)
}

// ParseWith runs the embedded script with an explicit interpreter, for a
// pre-flight check whose scratch venv lives outside the project. The caller
// owns provisioning that interpreter, so this does not check the project .venv.
func (r *VenvRunner) ParseWith(ctx context.Context, python string, in ParseInput) (ParseReport, error) {
	return r.parseWith(ctx, python, in)
}

// parseWith is the shared body: run the script with the given interpreter,
// AIRFLOW_HOME pointed at a throwaway directory, and decode the JSON result.
func (r *VenvRunner) parseWith(ctx context.Context, python string, in ParseInput) (ParseReport, error) {
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

	base := in.Env
	if len(base) == 0 {
		base = os.Environ()
	}
	// Appended last so they win over anything the caller supplied: they are what
	// keep the parse from touching a real Airflow home or the project tree.
	// os/exec keeps the last duplicate of a key, so order is the mechanism.
	env := append([]string(nil), base...)
	owned := ParseEnvironment(home, in.DagsDir, resultFile)
	for _, k := range slices.Sorted(maps.Keys(owned)) {
		env = append(env, k+"="+owned[k])
	}
	args := ParseArgs(in.ProjectPath, in.DagsDir)
	stdout, err := r.Exec.Run(ctx, in.ProjectPath, env, python, args, ParseProgram())
	if err != nil {
		return ParseReport{}, fmt.Errorf("running the DAG parse: %w", err)
	}

	// Prefer the result file; fall back to stdout for a script that ignored the
	// env var (older embed, or a test's fake executor).
	// DecodeParseReport trims, so neither branch needs to.
	raw := stdout
	if fromFile, ferr := os.ReadFile(resultFile); ferr == nil && len(bytes.TrimSpace(fromFile)) > 0 {
		raw = fromFile
	}

	return DecodeParseReport(raw)
}

// execExecutor is the production Executor, backed by os/exec.
type execExecutor struct{}

func (execExecutor) Run(ctx context.Context, dir string, env []string, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
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
