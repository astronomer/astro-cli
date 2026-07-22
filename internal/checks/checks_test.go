package checks

import (
	"context"
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

func TestEvaluateSameDagIDTwiceInOneFileIsNotADuplicate(t *testing.T) {
	// A dag_id listed twice for a single file is not a cross-file duplicate.
	res := evaluate(ParseReport{
		Files: []ReportFile{{File: "dags/a.py", DagIDs: []string{"x", "x"}}},
	})
	assert.Empty(t, res.Findings)
	assert.Equal(t, ExitOK, res.ExitCode(false))
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
	_, err := Run(context.Background(), Options{ProjectPath: "/proj"}, capture)
	require.NoError(t, err)
	assert.Equal(t, "/proj", got.ProjectPath)
	assert.Equal(t, "/proj/dags", got.DagsDir)
}

type parserFunc func(context.Context, ParseInput) (ParseReport, error)

func (f parserFunc) Parse(ctx context.Context, in ParseInput) (ParseReport, error) { return f(ctx, in) }
