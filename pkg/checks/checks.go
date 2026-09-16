// Package checks validates a project's DAGs without starting Airflow: the
// engine behind `astro local check`. It replaces v1's `astro dev parse` and
// the pytest integrity-test file the v1 CLI wrote into the user's project
// ("DO NOT EDIT" stamp and all) — the checks now run from an embedded copy
// (parse_dags.py) against the project's own uv-managed .venv Python, and this
// package never writes the user's tree.
//
// Following the layer rules in docs/v2-architecture.md, nothing here prints
// or exits: Run returns a typed Result and the cmd layer renders it and maps
// the exit code. Exit codes are plain integers compared as integers, distinct
// for "environment not ready" and "checks failed" — an earlier fix failures-report-
// as-passing bug came from reading meaning into a substring of an exit status,
// so nothing here does.
package checks

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Exit codes, distinct so a caller (and CI) can tell the two failure classes
// apart. Compared as integers, never as strings or substrings.
const (
	// ExitOK means every check passed.
	ExitOK = 0
	// ExitChecksFailed means at least one DAG failed a check (or, under
	// --strict, raised a warning).
	ExitChecksFailed = 1
	// ExitEnvNotReady means no verdict was reached, so nothing is known about
	// the DAGs either way.
	//
	// The environment is the case it is named for and the one this package
	// raises: no .venv, or no Airflow in it. A caller signals the same code for
	// anything else that stops a check before it can judge — not being in a
	// project directory, a manifest that will not load, an unknown target — and
	// should, because the distinction CI needs is "the DAGs are bad" (1) versus
	// "I could not tell" (2), not which of the second kind it was. The name is
	// narrower than the meaning for compatibility: it is public API in a
	// sub-module other tools build against.
	ExitEnvNotReady = 2
)

// defaultAirflowImportTimeout is Airflow's own default dagbag_import_timeout:
// the point at which a slow DAG file stops being slow and becomes an error,
// because Airflow abandons the import and reports the failure itself.
//
// A fallback, not an assumption. The setting is configurable and the parse
// reports the value actually in force (ParseReport.ImportTimeoutSeconds); this
// is what the threshold is derived from only when that is unavailable.
const defaultAirflowImportTimeout = 30 * time.Second

// warnFractionOfTimeout places the slow-parse warning below the timeout it
// warns about. See ParseTimeWarnThreshold.
const warnFractionOfTimeout = 2.0 / 3.0

// warnThreshold is the slow-parse threshold for one run: two thirds of the
// import timeout that run was subject to.
//
// Derived per run rather than fixed, because the timeout is a project's to set
// and the warning means nothing except relative to it. A project that raises
// dagbag_import_timeout to 120 would otherwise be warned about files nowhere
// near its limit; one that lowers it to 15 would get a 20s warning it can never
// reach, which is the defect a fixed threshold introduced in the first place.
func warnThreshold(report ParseReport) float64 {
	timeout := report.ImportTimeoutSeconds
	if timeout <= 0 {
		timeout = defaultAirflowImportTimeout.Seconds()
	}
	return timeout * warnFractionOfTimeout
}

// ParseTimeWarnThreshold flags a DAG file whose import took longer than this:
// slow enough to risk a scheduler timeout in a real deployment, while still
// importing. The finding is a warning, not a failure — slow is not broken —
// unless --strict is set.
//
// Set BELOW airflowImportTimeout, deliberately, and that is the whole point of
// the warning. It used to equal it, which made it unreachable: a file slow
// enough to trip it had already been abandoned by Airflow, so the run reported
// an import error and the warning only ever appeared alongside one. "Slow is
// not broken" described a state the default configuration could not produce,
// and --strict's effect on this finding could not be observed at all.
//
// A warning is worth having only if it arrives before the thing it warns about.
// Two thirds leaves a real margin — ten seconds at Airflow's default — without
// firing on projects that are merely not instant; a DAG taking twenty seconds
// to import is already worth looking at.
// It is the threshold for a run whose timeout is Airflow's default. A run
// reporting a different one is judged against that instead — see warnThreshold.
// This stays exported because other tools build against this sub-module.
const ParseTimeWarnThreshold = time.Duration(float64(defaultAirflowImportTimeout) * warnFractionOfTimeout)

// ErrEnvNotReady reports that the project environment is not in a state the
// check can inspect: the .venv is missing, or Airflow is not installed in it.
// The caller maps it to ExitEnvNotReady.
var ErrEnvNotReady = errors.New("project environment is not ready to check")

// Severity ranks a finding. Errors fail the run; warnings fail it only under
// --strict.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Kind names what a finding is about, so a machine reader keys off a stable
// value rather than parsing the message.
type Kind string

const (
	KindImportError    Kind = "import_error"
	KindDuplicateDagID Kind = "duplicate_dag_id"
	KindSlowParse      Kind = "slow_parse"
	// KindChecksIncomplete reports that a check could not run, as opposed to
	// running and finding nothing. Without it a checker that stopped checking
	// is indistinguishable from a clean project.
	KindChecksIncomplete Kind = "checks_incomplete"
)

// Finding is one thing the check noticed about one DAG or file. It is the unit
// the cmd layer renders — one per line in --output json (NDJSON), one row in
// the text table. Fields not relevant to a Kind stay zero and omit from JSON.
type Finding struct {
	Kind     Kind     `json:"kind"`
	Severity Severity `json:"severity"`
	// File is the DAG file the finding is about, relative to the project root.
	File string `json:"file,omitempty"`
	// DagID is set on findings about a specific DAG (duplicate ids).
	DagID string `json:"dag_id,omitempty"`
	// Files lists every file defining a duplicated dag_id.
	Files []string `json:"files,omitempty"`
	// Message carries an import error's text.
	Message string `json:"message,omitempty"`
	// ParseSeconds and ThresholdSeconds describe a slow-parse finding.
	ParseSeconds     float64 `json:"parse_time_seconds,omitempty"`
	ThresholdSeconds float64 `json:"threshold_seconds,omitempty"`
}

// Result is the outcome of a check run: every finding plus the counts the
// caller needs to render a summary and pick an exit code.
type Result struct {
	Findings []Finding
	// DagCount is how many DAGs loaded without an import error.
	DagCount int
	Errors   int
	Warnings int
}

// ExitCode maps a result to a process exit code. Under strict, warnings count
// as failures.
func (r Result) ExitCode(strict bool) int {
	if r.Errors > 0 || (strict && r.Warnings > 0) {
		return ExitChecksFailed
	}
	return ExitOK
}

// Passed reports whether the run cleared the bar (strict included).
func (r Result) Passed(strict bool) bool {
	return r.ExitCode(strict) == ExitOK
}

// Options controls one check run.
type Options struct {
	// ProjectPath is the project root (holds pyproject.toml, dags/, .venv).
	ProjectPath string
	// Strict turns warnings into failures.
	Strict bool
}

// Parser runs the embedded DAG-parse script and returns its structured report.
// It is the seam tests replace so no real Python runs in a unit test.
type Parser interface {
	Parse(ctx context.Context, in ParseInput) (ParseReport, error)
}

// Run inspects a project's DAGs and returns the findings. It returns a wrapped
// ErrEnvNotReady when the environment cannot be inspected; any other non-nil
// error is an unexpected failure to run the parse at all.
func Run(ctx context.Context, opts Options, parser Parser) (Result, error) {
	in := ParseInput{
		ProjectPath: opts.ProjectPath,
		DagsDir:     DefaultDagsDir(opts.ProjectPath),
	}
	report, err := parser.Parse(ctx, in)
	if err != nil {
		return Result{}, err
	}
	if report.Fatal != "" {
		// Airflow missing or unimportable in the venv — nothing to judge.
		return Result{}, fmt.Errorf("%s: %w", report.Fatal, ErrEnvNotReady)
	}
	return evaluate(report), nil
}

// evaluate turns a raw parse report into findings and counts. It is pure so it
// can be tested without running anything. Findings are grouped by check and
// sorted by file within each group, so the same project always reports the
// same order regardless of the parser's file-iteration order.
func evaluate(report ParseReport) Result {
	var res Result
	res.DagCount = len(report.Dags)

	importErrs := append([]ReportImportErr(nil), report.ImportErrors...)
	sort.Slice(importErrs, func(i, j int) bool { return importErrs[i].File < importErrs[j].File })
	for _, ie := range importErrs {
		res.Findings = append(res.Findings, Finding{
			Kind:     KindImportError,
			Severity: SeverityError,
			File:     ie.File,
			Message:  ie.Message,
		})
	}

	// Reported before the checks that depend on it, so the reason their
	// findings are absent reads ahead of their absence. A warning, not an
	// error: nothing is known to be wrong, which is exactly the problem.
	if report.FilesUnavailable {
		res.Findings = append(res.Findings, Finding{
			Kind:     KindChecksIncomplete,
			Severity: SeverityWarning,
			Message:  "Airflow reported no per-file statistics, so the duplicate dag_id and slow-parse checks did not run",
		})
	}

	res.Findings = append(res.Findings, duplicateDagIDs(report.Files)...)

	slow := make([]Finding, 0)
	threshold := warnThreshold(report)
	for _, file := range report.Files {
		if file.ParseSeconds > threshold {
			slow = append(slow, Finding{
				Kind:         KindSlowParse,
				Severity:     SeverityWarning,
				File:         file.File,
				ParseSeconds: file.ParseSeconds,
				// The threshold this run was judged against, so the rendered
				// message and the json agree with the verdict even on a project
				// that moved its import timeout.
				ThresholdSeconds: threshold,
			})
		}
	}
	sort.Slice(slow, func(i, j int) bool { return slow[i].File < slow[j].File })
	res.Findings = append(res.Findings, slow...)

	for _, f := range res.Findings {
		switch f.Severity {
		case SeverityError:
			res.Errors++
		case SeverityWarning:
			res.Warnings++
		}
	}
	return res
}

// duplicateDagIDs finds any dag_id defined in more than one file. Airflow's
// own DagBag reports most cross-file duplicates as import errors already; this
// is the structural backstop for the versions that keep both copies with only
// a log line, so a duplicate never slips through as a pass. Output is sorted
// so findings are deterministic.
func duplicateDagIDs(files []ReportFile) []Finding {
	sources := map[string][]string{}
	for _, file := range files {
		for _, id := range file.DagIDs {
			sources[id] = append(sources[id], file.File)
		}
	}
	var findings []Finding
	for id, locs := range sources {
		unique := dedupeSorted(locs)
		if len(unique) < 2 {
			continue
		}
		findings = append(findings, Finding{
			Kind:     KindDuplicateDagID,
			Severity: SeverityError,
			DagID:    id,
			Files:    unique,
		})
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].DagID < findings[j].DagID })
	return findings
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
