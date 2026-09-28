package checks

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeParser returns a canned report so evaluate and Run are exercised without
// running Python.
type fakeParser struct {
	report ParseReport
	err    error
}

func (p *fakeParser) Parse(context.Context, ParseInput) (ParseReport, error) {
	return p.report, p.err
}

func TestEvaluateImportErrorsAreFailures(t *testing.T) {
	res := evaluate(ParseReport{
		Dags:         []ReportDag{{DagID: "ok", File: "dags/ok.py"}},
		ImportErrors: []ReportImportErr{{File: "dags/bad.py", Message: "SyntaxError"}},
		Files: []ReportFile{
			{File: "dags/ok.py", ParseSeconds: 0.2, DagIDs: []string{"ok"}},
		},
	})
	assert.Equal(t, 1, res.DagCount)
	assert.Equal(t, 1, res.Errors)
	assert.Equal(t, 0, res.Warnings)
	require.Len(t, res.Findings, 1)
	assert.Equal(t, KindImportError, res.Findings[0].Kind)
	assert.Equal(t, SeverityError, res.Findings[0].Severity)
	assert.Equal(t, ExitChecksFailed, res.ExitCode(false))
}

func TestEvaluateDetectsDuplicateDagIDs(t *testing.T) {
	res := evaluate(ParseReport{
		Files: []ReportFile{
			{File: "dags/a.py", DagIDs: []string{"shared", "only_a"}},
			{File: "dags/b.py", DagIDs: []string{"shared"}},
		},
	})
	require.Len(t, res.Findings, 1)
	f := res.Findings[0]
	assert.Equal(t, KindDuplicateDagID, f.Kind)
	assert.Equal(t, SeverityError, f.Severity)
	assert.Equal(t, "shared", f.DagID)
	assert.Equal(t, []string{"dags/a.py", "dags/b.py"}, f.Files)
	assert.Equal(t, ExitChecksFailed, res.ExitCode(false))
}

// How a real duplicate arrives: as an import error, because Airflow refuses the
// second file before our own cross-file comparison could ever see the id twice.
//
// Reported as an import error it was invisible to anyone filtering on
// duplicate_dag_id — which is the whole reason that kind exists.
//
// The message is verbatim from `astro local check` against Airflow 3.1.8.
func TestEvaluateReportsAirflowsOwnDuplicateAsADuplicate(t *testing.T) {
	res := evaluate(ParseReport{
		// The DAG loaded from dupe_copy.py; exampledag.py was refused.
		Dags: []ReportDag{{DagID: "example_dag", File: "dags/dupe_copy.py"}},
		ImportErrors: []ReportImportErr{{
			File: "dags/exampledag.py",
			Message: "AirflowDagDuplicatedIdException: Ignoring DAG example_dag from " +
				"/proj/dags/exampledag.py - also found in /proj/dags/dupe_copy.py",
		}},
		Files: []ReportFile{{File: "dags/dupe_copy.py", DagIDs: []string{"example_dag"}}},
	})

	require.Len(t, res.Findings, 1, "one problem, one finding")
	f := res.Findings[0]
	assert.Equal(t, KindDuplicateDagID, f.Kind)
	assert.Equal(t, SeverityError, f.Severity)
	assert.Equal(t, "example_dag", f.DagID)
	// Both files, relative to the project like every other path this package
	// reports — taken from the report rather than from Airflow's sentence,
	// which spells them absolute.
	assert.Equal(t, []string{"dags/dupe_copy.py", "dags/exampledag.py"}, f.Files)
	assert.Equal(t, 1, res.Errors)
	assert.Equal(t, ExitChecksFailed, res.ExitCode(false))
}

// Three copies of a dag_id are one problem, not two.
//
// Airflow files an import error per REFUSED file, so it reports two here — one
// for b.py, one for c.py. Emitting a finding each counted two errors for one
// duplicated id and gave each finding half the file list, so neither named all
// three places the id is defined.
func TestEvaluateMergesRepeatedDuplicatesByDagID(t *testing.T) {
	res := evaluate(ParseReport{
		Dags: []ReportDag{{DagID: "shared", File: "dags/a.py"}},
		ImportErrors: []ReportImportErr{
			{File: "dags/b.py", Message: "AirflowDagDuplicatedIdException: Ignoring DAG shared from /p/dags/b.py - also found in /p/dags/a.py"},
			{File: "dags/c.py", Message: "AirflowDagDuplicatedIdException: Ignoring DAG shared from /p/dags/c.py - also found in /p/dags/a.py"},
		},
		Files: []ReportFile{{File: "dags/a.py", DagIDs: []string{"shared"}}},
	})

	require.Len(t, res.Findings, 1, "one duplicated id is one finding")
	f := res.Findings[0]
	assert.Equal(t, KindDuplicateDagID, f.Kind)
	assert.Equal(t, "shared", f.DagID)
	assert.Equal(t, []string{"dags/a.py", "dags/b.py", "dags/c.py"}, f.Files,
		"every file defining the id, not the half one import error knew about")
	assert.Equal(t, "dags/a.py", f.File, "the copy Airflow actually loaded")
	assert.Equal(t, 1, res.Errors, "one problem counts once")
}

// Both detection paths noticing the same id is still one finding: the
// cross-file backstop and Airflow's own refusal are two ways of seeing it.
func TestEvaluateDoesNotReportTheSameDuplicateTwice(t *testing.T) {
	res := evaluate(ParseReport{
		Dags: []ReportDag{{DagID: "shared", File: "dags/a.py"}},
		ImportErrors: []ReportImportErr{
			{File: "dags/b.py", Message: "AirflowDagDuplicatedIdException: Ignoring DAG shared from /p/b.py - also found in /p/a.py"},
		},
		// An Airflow that kept both copies in its per-file stats, which is the
		// case the cross-file check exists for.
		Files: []ReportFile{
			{File: "dags/a.py", DagIDs: []string{"shared"}},
			{File: "dags/b.py", DagIDs: []string{"shared"}},
		},
	})
	require.Len(t, res.Findings, 1)
	assert.Equal(t, []string{"dags/a.py", "dags/b.py"}, res.Findings[0].Files)
	assert.Equal(t, 1, res.Errors)
}

// An import error that is not a duplicate is untouched.
func TestEvaluateLeavesOrdinaryImportErrorsAlone(t *testing.T) {
	res := evaluate(ParseReport{ImportErrors: []ReportImportErr{{
		File:    "dags/bad.py",
		Message: "Traceback (most recent call last):\nModuleNotFoundError: No module named 'x'",
	}}})
	require.Len(t, res.Findings, 1)
	assert.Equal(t, KindImportError, res.Findings[0].Kind)
}

// When Airflow rewords the exception the match fails and the finding stays an
// import error — today's behavior, so the wording cannot break the check, only
// sharpen it.
func TestEvaluateFallsBackWhenTheDuplicateWordingChanges(t *testing.T) {
	res := evaluate(ParseReport{ImportErrors: []ReportImportErr{{
		File:    "dags/exampledag.py",
		Message: "AirflowDagDuplicatedIdException: duplicate dag_id example_dag",
	}}})
	require.Len(t, res.Findings, 1)
	assert.Equal(t, KindImportError, res.Findings[0].Kind)
	assert.Equal(t, ExitChecksFailed, res.ExitCode(false), "still an error either way")
}

// With no surviving DAG to pair it with — every copy refused — the finding still
// names the id and the file it does know.
func TestEvaluateDuplicateWithNoSurvivingDag(t *testing.T) {
	res := evaluate(ParseReport{ImportErrors: []ReportImportErr{{
		File:    "dags/exampledag.py",
		Message: "AirflowDagDuplicatedIdException: Ignoring DAG example_dag from /p/a.py - also found in /p/b.py",
	}}})
	require.Len(t, res.Findings, 1)
	assert.Equal(t, KindDuplicateDagID, res.Findings[0].Kind)
	assert.Equal(t, []string{"dags/exampledag.py"}, res.Findings[0].Files)
}

func TestEvaluateSameDagIDTwiceInOneFileIsNotADuplicate(t *testing.T) {
	// A dag_id listed twice for a single file is not a cross-file duplicate.
	res := evaluate(ParseReport{
		Files: []ReportFile{{File: "dags/a.py", DagIDs: []string{"x", "x"}}},
	})
	assert.Empty(t, res.Findings)
	assert.Equal(t, ExitOK, res.ExitCode(false))
}

// The warning has to arrive before the failure it warns about.
//
// Set equal to Airflow's import timeout — as it was — the finding is
// unreachable: Airflow abandons the import at that same moment and reports an
// error, so the warning never appears without one, "slow is not broken"
// describes a state the default configuration cannot produce, and --strict's
// effect on it cannot be observed. Every other test here feeds evaluate a
// ParseSeconds relative to the threshold, so all of them pass either way; this
// is the one that pins the gap.
func TestSlowParseWarnsBeforeAirflowGivesUp(t *testing.T) {
	assert.Less(t, ParseTimeWarnThreshold, defaultAirflowImportTimeout,
		"a file slow enough to warn must still be one Airflow finished importing")

	// A margin, not a hair: the point is to flag a file heading for the limit
	// while there is still something to do about it.
	margin := defaultAirflowImportTimeout - ParseTimeWarnThreshold
	assert.GreaterOrEqual(t, margin, defaultAirflowImportTimeout/4,
		"the gap is what makes the warning actionable")

	// And the reachable band really does produce a warning and nothing else.
	res := evaluate(ParseReport{Files: []ReportFile{{
		File:         "dags/slow.py",
		ParseSeconds: (ParseTimeWarnThreshold + defaultAirflowImportTimeout).Seconds() / 2,
		DagIDs:       []string{"s"},
	}}})
	require.Len(t, res.Findings, 1)
	assert.Equal(t, SeverityWarning, res.Findings[0].Severity)
	assert.Equal(t, 0, res.Errors)
}

// The threshold tracks the timeout the run was actually subject to, because a
// project can move it. Fixed at Airflow's default, a project that raises the
// timeout gets warned about files nowhere near its limit, and one that lowers it
// gets a warning it can never reach — the same defect, in both directions.
func TestSlowParseThresholdFollowsTheReportedTimeout(t *testing.T) {
	for _, tc := range []struct {
		name          string
		timeout       float64
		parseSeconds  float64
		wantFindings  int
		wantThreshold float64
	}{
		{
			// 40s under a 120s timeout is not close to anything.
			name:    "a raised timeout does not warn about a file well inside it",
			timeout: 120, parseSeconds: 40, wantFindings: 0,
		},
		{
			name:    "a raised timeout warns as its own limit approaches",
			timeout: 120, parseSeconds: 100, wantFindings: 1, wantThreshold: 80,
		},
		{
			// 12s would pass at the default threshold and is already most of
			// the way to this project's limit.
			name:    "a lowered timeout warns where the default would not",
			timeout: 15, parseSeconds: 12, wantFindings: 1, wantThreshold: 10,
		},
		{
			// No value reported: fall back to Airflow's default.
			name:    "an unreported timeout falls back to the default",
			timeout: 0, parseSeconds: 25, wantFindings: 1, wantThreshold: ParseTimeWarnThreshold.Seconds(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := evaluate(ParseReport{
				ImportTimeoutSeconds: tc.timeout,
				Files:                []ReportFile{{File: "dags/slow.py", ParseSeconds: tc.parseSeconds, DagIDs: []string{"s"}}},
			})
			require.Len(t, res.Findings, tc.wantFindings)
			if tc.wantFindings == 0 {
				return
			}
			assert.Equal(t, KindSlowParse, res.Findings[0].Kind)
			assert.InDelta(t, tc.wantThreshold, res.Findings[0].ThresholdSeconds, 0.001,
				"the reported threshold must be the one the verdict used")
		})
	}
}

func TestEvaluateSlowParseIsWarningStrictMakesItFail(t *testing.T) {
	res := evaluate(ParseReport{
		Files: []ReportFile{
			{File: "dags/slow.py", ParseSeconds: ParseTimeWarnThreshold.Seconds() + 5, DagIDs: []string{"s"}},
			{File: "dags/fast.py", ParseSeconds: 0.5, DagIDs: []string{"f"}},
		},
	})
	require.Len(t, res.Findings, 1)
	assert.Equal(t, KindSlowParse, res.Findings[0].Kind)
	assert.Equal(t, SeverityWarning, res.Findings[0].Severity)
	assert.Equal(t, 1, res.Warnings)
	assert.Equal(t, 0, res.Errors)
	assert.Equal(t, ExitOK, res.ExitCode(false))
	assert.Equal(t, ExitChecksFailed, res.ExitCode(true))
	assert.True(t, res.Passed(false))
	assert.False(t, res.Passed(true))
}

func TestExitCodesAreDistinctIntegers(t *testing.T) {
	// The three outcomes must be different integers (an earlier fix: no substring
	// overlap, no ambiguity).
	assert.NotEqual(t, ExitOK, ExitChecksFailed)
	assert.NotEqual(t, ExitOK, ExitEnvNotReady)
	assert.NotEqual(t, ExitChecksFailed, ExitEnvNotReady)
}

func TestRunFatalReportIsEnvNotReady(t *testing.T) {
	_, err := Run(context.Background(), Options{ProjectPath: "/p"}, &fakeParser{
		report: ParseReport{Fatal: "ModuleNotFoundError: No module named 'airflow'"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvNotReady)
	assert.Contains(t, err.Error(), "airflow")
}

// A blocked check says how to stop being blocked — when that is a question it
// can answer.
//
// A venv holding a Python and no Airflow is what an interrupted `astro local
// start` leaves, and the reader got the Python exception and the sentinel and
// nothing else, while the no-interpreter case beside it has named both
// remedies all along.
func TestAnEnvironmentFatalNamesAWayOut(t *testing.T) {
	_, err := Run(context.Background(), Options{ProjectPath: "/p"}, &fakeParser{
		report: ParseReport{
			Fatal:              "ModuleNotFoundError: No module named 'airflow'",
			FatalIsEnvironment: true,
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvNotReady)

	for _, want := range []string{"astro local start", "uv sync"} {
		assert.Contains(t, err.Error(), want,
			"a reader who cannot check needs to know what would let them")
	}
	// What is wrong still comes before what to do about it.
	assert.Contains(t, err.Error(), "ModuleNotFoundError")
}

// Everything DagBag raised is the project's own, and gets no remedy.
//
// The script's fatal used to come from one except around the Airflow import
// AND the DagBag construction, so a malformed airflow.cfg, a plugins package
// that will not import and a dags directory that cannot be read were all
// indistinguishable from a missing Airflow. Attaching "rebuild the
// environment" to the lot of them would send their authors to rebuild
// something that was never the problem — the guess ParseWith's sibling case
// explicitly refuses to make.
func TestAProjectFatalIsReportedWithoutARemedy(t *testing.T) {
	_, err := Run(context.Background(), Options{ProjectPath: "/p"}, &fakeParser{
		report: ParseReport{Fatal: "AirflowConfigException: error at airflow.cfg line 3"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvNotReady)

	assert.Contains(t, err.Error(), "AirflowConfigException")
	for _, absent := range []string{"astro local start", "uv sync", "rebuilds"} {
		assert.NotContains(t, err.Error(), absent,
			"neither command fixes a config file, and saying so wastes a rebuild")
	}
}

// And a check running in an environment the CLI built for it gets no remedy
// either, whichever fatal it hit.
//
// RunProvisioned inspects a venv in a cache directory that neither command
// touches. Telling somebody who has just watched the CLI build an environment
// to go and build one is the kind of advice that makes a reader doubt the
// rest of the output.
func TestAProvisionedRunOffersNoRemedy(t *testing.T) {
	_, err := RunProvisioned(context.Background(), Options{ProjectPath: "/p"},
		ProvisionInput{ProjectPath: "/p", Pin: "3.1"},
		&fakeProvisioner{python: "/tmp/venv/bin/python"},
		&fakeTargetParser{report: ParseReport{
			Fatal:              "ImportError: cannot import name 'x'",
			FatalIsEnvironment: true,
		}},
		nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEnvNotReady)

	assert.Contains(t, err.Error(), "ImportError")
	assert.NotContains(t, err.Error(), "astro local start",
		"the environment under inspection is the one this call just built")
}

func TestRunPropagatesParserError(t *testing.T) {
	_, err := Run(context.Background(), Options{ProjectPath: "/p"}, &fakeParser{err: ErrEnvNotReady})
	assert.ErrorIs(t, err, ErrEnvNotReady)
}

func TestRunPassesDagsDirToParser(t *testing.T) {
	var got ParseInput
	capture := parserFunc(func(_ context.Context, in ParseInput) (ParseReport, error) {
		got = in
		return ParseReport{}, nil
	})
	// A relative project path with no separator in it. The assertion below has
	// to build the expected dags dir with filepath.Join — Run derives it that
	// way, so a hardcoded "/proj/dags" asserts a separator only Unix uses — and
	// a Join argument that already contains one is its own lint finding.
	// Nothing here needs the path to be absolute.
	_, err := Run(context.Background(), Options{ProjectPath: "proj"}, capture)
	require.NoError(t, err)
	assert.Equal(t, "proj", got.ProjectPath)
	// Stdlib rather than DefaultDagsDir, so the expectation restates the intent
	// — the project path plus a dags child — instead of re-running the code
	// under test.
	assert.Equal(t, filepath.Join("proj", "dags"), got.DagsDir)
}

func TestRunPassesEnvToParser(t *testing.T) {
	var got ParseInput
	capture := parserFunc(func(_ context.Context, in ParseInput) (ParseReport, error) {
		got = in
		return ParseReport{}, nil
	})
	env := []string{"ENV=sandbox"}
	_, err := Run(context.Background(), Options{ProjectPath: "proj", Env: env}, capture)
	require.NoError(t, err)
	assert.Equal(t, env, got.Env)
}

type parserFunc func(context.Context, ParseInput) (ParseReport, error)

func (f parserFunc) Parse(ctx context.Context, in ParseInput) (ParseReport, error) { return f(ctx, in) }

// A run that could not perform a check does not report clean.
//
// dagbag_stats is an Airflow implementation detail and two checks derive from
// it. Without this signal an empty Files slice is indistinguishable from a
// project with nothing wrong, so a checker that stopped checking passes.
func TestFilesUnavailableIsReportedRatherThanPassingClean(t *testing.T) {
	res, err := Run(context.Background(), Options{ProjectPath: "proj"}, &fakeParser{
		report: ParseReport{
			Dags:             []ReportDag{{DagID: "a", File: "dags/a.py"}},
			FilesUnavailable: true,
		},
	})
	require.NoError(t, err)

	require.Len(t, res.Findings, 1)
	assert.Equal(t, KindChecksIncomplete, res.Findings[0].Kind)
	assert.Equal(t, SeverityWarning, res.Findings[0].Severity)
	assert.Contains(t, res.Findings[0].Message, "did not run")
	assert.Equal(t, 1, res.Warnings)
	assert.Equal(t, 0, res.Errors)

	// A warning, so an ordinary run still passes — nothing is known to be
	// wrong. Under --strict it fails, which is what strict is for.
	assert.True(t, res.Passed(false))
	assert.False(t, res.Passed(true))
}

// The ordinary case stays silent: a project with per-file stats and nothing
// wrong reports no findings at all.
func TestFilesAvailableAddsNoIncompleteFinding(t *testing.T) {
	res, err := Run(context.Background(), Options{ProjectPath: "proj"}, &fakeParser{
		report: ParseReport{
			Dags:  []ReportDag{{DagID: "a", File: "dags/a.py"}},
			Files: []ReportFile{{File: "dags/a.py", ParseSeconds: 0.2, DagIDs: []string{"a"}}},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, res.Findings)
}
