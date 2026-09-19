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
	"regexp"
	"slices"
	"sort"
	"strings"
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

// ErrNoInterpreter is the narrower case: the project has no interpreter at
// all, as opposed to one whose Airflow will not import.
//
// The two are worth telling apart because only the first is safe to answer by
// building an environment. A project whose own .venv is broken would otherwise
// be checked against a freshly resolved one and reported clean, while the
// Airflow it actually runs stays broken — a green check for a project that
// cannot start.
//
// It wraps ErrEnvNotReady, so a caller that only cares that no verdict was
// reached keeps working unchanged.
var ErrNoInterpreter = fmt.Errorf("%w: no interpreter", ErrEnvNotReady)

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
	return run(ctx, opts, parser, buildRemedy)
}

// buildRemedy is what to do about an environment the check could not use.
//
// Both commands, because this package cannot tell whether the project runs in
// docker mode and only one of them is the answer in either — the same reason
// the sibling case in ParseWith names both.
//
// The mode qualifies the SECOND command, and is written to read that way. It
// used to trail the whole sentence — "…and `uv sync` rebuilds it without
// starting Airflow, which is what a docker-mode project needs" — where it
// scanned as a condition on the advice as a whole, and the first question it
// drew was whether any of this applied outside Docker. It applies to every
// project; only the choice between the two commands depends on the mode.
const buildRemedy = " — `astro local start` rebuilds it, or `uv sync` if this project builds in Docker"

// run is Run with the remedy chosen by the caller, so that the one path where
// it would be wrong can say so.
func run(ctx context.Context, opts Options, parser Parser, remedy string) (Result, error) {
	in := ParseInput{
		ProjectPath: opts.ProjectPath,
		DagsDir:     DefaultDagsDir(opts.ProjectPath),
	}
	report, err := parser.Parse(ctx, in)
	if err != nil {
		return Result{}, err
	}
	if report.Fatal != "" {
		// Nothing to judge. Whether that is worth a remedy depends on which
		// of the two fatals it is: Airflow out of reach is answered by
		// building an environment, and anything DagBag raised is the
		// project's own and is not. See ParseReport.FatalIsEnvironment.
		if report.FatalIsEnvironment {
			return Result{}, fmt.Errorf("%s: %w%s", report.Fatal, ErrEnvNotReady, remedy)
		}
		return Result{}, fmt.Errorf("%s: %w", report.Fatal, ErrEnvNotReady)
	}
	return evaluate(report), nil
}

// ProvisionInput is what a provisioned check needs about the project: enough
// to build an environment equivalent to the one the project would build for
// itself.
type ProvisionInput struct {
	ProjectPath string
	DagsDir     string
	// Pin is the manifest's [tool.astro] airflow, used when the dependencies
	// do not name Airflow themselves.
	Pin string
	// Deps is the manifest's [project] dependencies.
	Deps []string
	// RequiresPython is the manifest's [project] requires-python, passed to the
	// provisioner as its interpreter request.
	RequiresPython string
}

// RunProvisioned inspects a project's DAGs with an interpreter it builds, for
// a project that has none of its own.
//
// A project has a .venv because something built one, and only `astro local
// start` in standalone mode does: a docker-mode project keeps its dependencies
// in the image and never writes one, so a check there had nothing to parse
// with. The environment is the same cached scratch venv a target check uses,
// through the same Provisioner, so uv is discovered the way every other uv in
// this CLI is. Nothing is written into the project.
//
// It builds the spec here rather than taking one, so that this path and
// Preflight cannot drift on the two decisions that matter: that Airflow is in
// the requirement set at all, and that the set is sorted so the cache key over
// it is stable.
func RunProvisioned(ctx context.Context, opts Options, in ProvisionInput, prov Provisioner, parser TargetParser, progress func(string)) (Result, error) {
	// A consumer with nowhere to stream notes passes nil, the way Preflight
	// allows: Astro Desktop has no text renderer.
	if progress == nil {
		progress = func(string) {}
	}
	python, err := prov.EnsureVenv(ctx, VenvSpec{
		Airflow: in.Pin,
		Python:  in.RequiresPython,
		Reqs:    projectRequirements(in.Pin, in.Deps),
	}, progress)
	if err != nil {
		return Result{}, err
	}
	// No remedy here: the environment under inspection is the one this call
	// just built, in a cache directory neither command touches. Telling
	// somebody who has watched the CLI build an environment to go and build
	// one is the kind of advice that makes a reader doubt the rest.
	return run(ctx, opts, withInterpreter{python: python, parser: parser}, "")
}

// projectRequirements is what to install to reproduce the project's own
// environment: its dependencies as written, with Airflow added when they do
// not name it.
//
// As written, because the manifest already states Airflow in the shape the
// project means — "apache-airflow==3.1.*" — and rebuilding that from the pin
// would ask for "apache-airflow==3.1", which is not a release. Added when
// absent, because a manifest need not name Airflow at all: a docker project
// declaring its own Dockerfile builds the image from that file, and the
// dependency list stops describing it. Installing nothing called Airflow would
// spend a long download to arrive at "Airflow is not importable".
//
// Sorted for the same reason requirementSet sorts: the provisioner's cache key
// hashes this slice in order, so two runs that differ only in the order of
// pyproject.toml's lines would otherwise miss the cache and rebuild.
func projectRequirements(pin string, deps []string) []string {
	out := append([]string{}, deps...)
	if !slices.ContainsFunc(out, func(d string) bool { return distName(d) == airflowDist }) {
		out = append(out, airflowRequirement(pin))
	}
	sort.Strings(out)
	return out
}

// airflowRequirement is the requirement that installs the Airflow a pin names.
// A partial pin ("3", "3.1") is a series, so it becomes a prefix match; a full
// one is exact. It mirrors pkg/scaffold.airflowRequirement, which writes the
// entry this reads back — the two stay separate rather than couple a cmd-layer
// package to the scaffold.
func airflowRequirement(pin string) string {
	if strings.Count(pin, ".") < 2 {
		return airflowDist + "==" + pin + ".*"
	}
	return airflowDist + "==" + pin
}

// withInterpreter adapts a TargetParser and a chosen interpreter into the
// Parser Run takes, so a provisioned check runs through the same evaluation as
// every other one rather than a second copy of it.
type withInterpreter struct {
	python string
	parser TargetParser
}

func (w withInterpreter) Parse(ctx context.Context, in ParseInput) (ParseReport, error) {
	return w.parser.ParseWith(ctx, w.python, in)
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
	// Duplicates are collected rather than emitted here. Airflow files one
	// import error per REFUSED FILE, so three copies of a dag_id arrive as two
	// errors — emitting a finding each reported one problem twice, counted two
	// errors for it, and gave each finding half the file list. They are keyed by
	// dag_id and merged with the cross-file check below, which is also where
	// they belong in the output: evaluate groups findings by check.
	dupFiles := map[string][]string{}
	for _, ie := range importErrs {
		if id, files, ok := duplicateFromImportError(ie, report.Dags); ok {
			dupFiles[id] = append(dupFiles[id], files...)
			continue
		}
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

	dups := duplicateDagIDs(report.Files, dupFiles)
	// Name the copy Airflow actually loaded. Before these were reclassified the
	// row carried Airflow's own sentence, which said which file it ignored, and
	// a reader who lost that would know a dag_id is duplicated without knowing
	// which definition is the live one — the first thing they need in order to
	// delete the right file.
	for i := range dups {
		for _, d := range report.Dags {
			if d.DagID == dups[i].DagID {
				dups[i].File = d.File
				break
			}
		}
	}
	res.Findings = append(res.Findings, dups...)

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

// fromImportErrors carries the duplicates Airflow reported itself, keyed by
// dag_id, so they merge with the ones found here instead of being reported
// twice by two code paths that noticed the same thing.
func duplicateDagIDs(files []ReportFile, fromImportErrors map[string][]string) []Finding {
	sources := map[string][]string{}
	for _, file := range files {
		for _, id := range file.DagIDs {
			sources[id] = append(sources[id], file.File)
		}
	}
	// A dag_id Airflow refused appears here even when only one file survived to
	// list it, because the refusal is itself proof of a second definition.
	for id, locs := range fromImportErrors {
		sources[id] = append(sources[id], locs...)
	}
	var findings []Finding
	for id, locs := range sources {
		unique := dedupeSorted(locs)
		// One file is not a duplicate — unless Airflow said so, in which case
		// there was a second definition it declined to load and the count of
		// surviving files understates the problem.
		if len(unique) < 2 && len(fromImportErrors[id]) == 0 {
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

// dupImportErrRe picks the duplicated dag_id out of the error Airflow raises
// for one, rendered from AirflowDagDuplicatedIdException:
//
//	AirflowDagDuplicatedIdException: Ignoring DAG x from /p/b.py - also found in /p/a.py
//
// Only the id is taken. The sentence also carries both paths, but absolute and
// as Airflow spelled them, while every path this package reports is relative to
// the project — so they are looked up instead, which keeps the finding
// consistent with the rest and leaves less of the wording load-bearing.
var dupImportErrRe = regexp.MustCompile(`AirflowDagDuplicatedIdException: Ignoring DAG (\S+) `)

// duplicateFromImportError reports a duplicate dag_id that reached us as an
// import error, which is how every real one does.
//
// duplicateDagIDs below cannot see these, and that is not a gap in it: it
// compares the dag_ids each file successfully registered, and a DagBag never
// registers the same id twice. Airflow notices first, refuses the second file,
// and files the whole thing under import_errors — so KindDuplicateDagID never
// reached anyone, and a consumer filtering on it missed every duplicate there
// has ever been. `astro local check --output json` is the contract
// Astro Desktop reads, and "this kind exists but never occurs" is the worst
// shape a contract can take.
//
// The text is matched because it is all Airflow hands back: import_errors is a
// map of path to rendered string and the exception object is long gone. When
// the wording changes the match fails and the finding stays an import error —
// today's behavior exactly — so this can improve on the status quo and cannot
// regress it.
//
// It returns the id and the files it can attribute to it, for the caller to
// merge by id: Airflow files one import error per REFUSED file, so a dag_id in
// three places arrives here twice and is one problem either way.
//
// The losing file is the one the error is filed under; the winner is whichever
// file the DAG did load from, which the report already lists.
func duplicateFromImportError(ie ReportImportErr, dags []ReportDag) (dagID string, files []string, ok bool) {
	m := dupImportErrRe.FindStringSubmatch(ie.Message)
	if m == nil {
		return "", nil, false
	}
	dagID = m[1]
	files = []string{ie.File}
	for _, d := range dags {
		if d.DagID == dagID && d.File != "" {
			files = append(files, d.File)
		}
	}
	return dagID, files, true
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
