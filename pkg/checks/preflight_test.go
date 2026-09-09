package checks

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeProvisioner records what it was asked to build and returns canned
// outcomes, so a preflight test runs no uv and no install.
type fakeProvisioner struct {
	gotSpec     VenvSpec
	python      string
	ensureErr   error
	gotReqs     []string
	gotURL      string
	gotPyVer    string
	resolveErr  error
	ensureCalls int
}

func (f *fakeProvisioner) EnsureVenv(_ context.Context, spec VenvSpec, progress func(string)) (string, error) {
	f.gotSpec = spec
	f.ensureCalls++
	progress("provisioning")
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	return f.python, nil
}

func (f *fakeProvisioner) ResolveConstraints(_ context.Context, reqs []string, url, pythonVersion string) error {
	f.gotReqs = reqs
	f.gotURL = url
	f.gotPyVer = pythonVersion
	return f.resolveErr
}

// fakeTargetParser returns a canned report for the interpreter it is handed.
type fakeTargetParser struct {
	report     ParseReport
	err        error
	gotPython  string
	parseCalls int
}

func (f *fakeTargetParser) ParseWith(_ context.Context, python string, _ ParseInput) (ParseReport, error) {
	f.gotPython = python
	f.parseCalls++
	return f.report, f.err
}

func cleanReport() ParseReport {
	return ParseReport{
		Dags:  []ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}
}

func noteProgress(string) {}

func TestPreflightComposerResolvesCleanly(t *testing.T) {
	prov := &fakeProvisioner{python: "/scratch/bin/python"}
	parser := &fakeTargetParser{report: cleanReport()}
	in := PreflightInput{Pin: "3.1", Deps: []string{"apache-airflow==3.1.*", "pandas"}}

	rep := Preflight(context.Background(), TargetComposer, in, prov, parser, false, noteProgress)

	if rep.OpError != "" {
		t.Fatalf("clean composer check should not error: %s", rep.OpError)
	}
	if rep.AirflowChecked != "3.1.8" {
		t.Errorf("composer 3.1 should check against 3.1.8, got %q", rep.AirflowChecked)
	}
	if rep.MappedFrom != "" {
		t.Errorf("composer offers 3.1.x, so no downgrade: got mapped_from %q", rep.MappedFrom)
	}
	if rep.ExitCode(false) != ExitOK {
		t.Errorf("clean check should exit 0, got %d", rep.ExitCode(false))
	}
	if rep.Constraints != nil {
		t.Error("composer has no constraints step")
	}
	// The scratch venv gets the mapped Airflow, project's airflow pin dropped.
	if got := strings.Join(prov.gotSpec.Reqs, " "); !strings.Contains(got, "apache-airflow==3.1.8") || strings.Contains(got, "3.1.*") {
		t.Errorf("scratch reqs should pin the mapped Airflow and drop the project pin: %q", got)
	}
	if parser.gotPython != "/scratch/bin/python" {
		t.Errorf("parser should run against the provisioned interpreter, got %q", parser.gotPython)
	}
}

func TestPreflightMWAAMapsDownAndNotes(t *testing.T) {
	prov := &fakeProvisioner{python: "/scratch/bin/python"}
	parser := &fakeTargetParser{report: cleanReport()}
	in := PreflightInput{Pin: "3.1", Deps: []string{"apache-airflow==3.1.*"}}

	rep := Preflight(context.Background(), TargetMWAA, in, prov, parser, false, noteProgress)

	if rep.AirflowChecked != "3.0.6" {
		t.Errorf("MWAA 3.1 should map down to 3.0.6, got %q", rep.AirflowChecked)
	}
	if rep.MappedFrom != "3.1" {
		t.Errorf("mapped_from should name the pin 3.1, got %q", rep.MappedFrom)
	}
	if len(rep.Notes) == 0 || !strings.Contains(strings.Join(rep.Notes, " "), "3.0.6") {
		t.Errorf("a prominent note should name both versions: %v", rep.Notes)
	}
	// MWAA provisions python 3.12.
	if prov.gotSpec.Python != "3.12" {
		t.Errorf("MWAA 3.x runs py3.12; got %q", prov.gotSpec.Python)
	}
}

func TestPreflightMWAAConstraints(t *testing.T) {
	cases := []struct {
		name       string
		resolveErr error
		wantOK     bool
		wantSkip   bool
		wantConf   bool
	}{
		{"clean", nil, true, false, false},
		{"conflict", &ConstraintConflict{Summary: "pandas>=2 and pandas<1 cannot both hold"}, false, false, true},
		{"offline", ErrConstraintsUnavailable, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := &fakeProvisioner{python: "/p", resolveErr: tc.resolveErr}
			parser := &fakeTargetParser{report: cleanReport()}
			in := PreflightInput{Pin: "3.0.6", Deps: []string{"apache-airflow==3.0.6", "pandas"}}

			rep := Preflight(context.Background(), TargetMWAA, in, prov, parser, false, noteProgress)

			if rep.Constraints == nil {
				t.Fatal("MWAA should report a constraints outcome")
			}
			c := rep.Constraints
			if c.OK != tc.wantOK {
				t.Errorf("OK = %v, want %v", c.OK, tc.wantOK)
			}
			if (c.Skipped != "") != tc.wantSkip {
				t.Errorf("Skipped = %q, want skip=%v", c.Skipped, tc.wantSkip)
			}
			if (c.Conflict != "") != tc.wantConf {
				t.Errorf("Conflict = %q, want conflict=%v", c.Conflict, tc.wantConf)
			}
			// A constraints outcome, whatever it is, never fails the check on its
			// own; a conflict is reported but the DAG parse was clean here.
			if tc.wantConf && rep.ExitCode(false) != ExitOK {
				t.Error("a constraints conflict is a note on a clean parse, not a DAG failure")
			}
			// The resolver sees the deps with apache-airflow dropped.
			if got := strings.Join(prov.gotReqs, " "); strings.Contains(got, "apache-airflow==") {
				t.Errorf("constraints resolve should drop apache-airflow: %q", got)
			}
			if prov.gotPyVer != "3.12" {
				t.Errorf("constraints resolve should target py3.12, got %q", prov.gotPyVer)
			}
		})
	}
}

func TestPreflightPinBelowFloorErrors(t *testing.T) {
	prov := &fakeProvisioner{python: "/p"}
	parser := &fakeTargetParser{report: cleanReport()}
	// 2.5 is below MWAA's lowest offered (2.7.2).
	rep := Preflight(context.Background(), TargetMWAA, PreflightInput{Pin: "2.5"}, prov, parser, false, noteProgress)

	if rep.OpError == "" {
		t.Fatal("a pin below the floor should be an operational error")
	}
	if rep.ExitCode(false) != ExitEnvNotReady {
		t.Errorf("exit code should be operational (2), got %d", rep.ExitCode(false))
	}
	if prov.ensureCalls != 0 {
		t.Error("no venv should be built when nothing maps")
	}
}

func TestPreflightEmptyPinErrors(t *testing.T) {
	rep := Preflight(context.Background(), TargetMWAA, PreflightInput{Pin: ""}, &fakeProvisioner{}, &fakeTargetParser{}, false, noteProgress)
	if rep.OpError == "" {
		t.Error("an empty pin should be an operational error, not a crash")
	}
}

func TestPreflightImportErrorFails(t *testing.T) {
	prov := &fakeProvisioner{python: "/p"}
	parser := &fakeTargetParser{report: ParseReport{
		ImportErrors: []ReportImportErr{{File: "dags/bad.py", Message: "boom"}},
	}}
	rep := Preflight(context.Background(), TargetComposer, PreflightInput{Pin: "3.1", Deps: nil}, prov, parser, false, noteProgress)

	if rep.Errors != 1 || len(rep.Findings) != 1 {
		t.Fatalf("an import error should be one finding: %+v", rep)
	}
	if rep.ExitCode(false) != ExitChecksFailed {
		t.Errorf("findings should exit 1, got %d", rep.ExitCode(false))
	}
}

func TestPreflightVenvFailureIsOperational(t *testing.T) {
	prov := &fakeProvisioner{ensureErr: errors.New("uv install blew up")}
	rep := Preflight(context.Background(), TargetComposer, PreflightInput{Pin: "3.1"}, prov, &fakeTargetParser{}, false, noteProgress)
	if rep.ExitCode(false) != ExitEnvNotReady {
		t.Errorf("a venv build failure is operational (2), got %d", rep.ExitCode(false))
	}
	if !strings.Contains(rep.OpError, "uv install blew up") {
		t.Errorf("op error should carry the cause: %q", rep.OpError)
	}
}

func TestRequirementSetDropsProjectAirflow(t *testing.T) {
	got := requirementSet("3.0.6", []string{"apache-airflow==3.1.*", "apache-airflow-providers-standard", "pandas"})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "apache-airflow==3.0.6") {
		t.Errorf("mapped Airflow missing: %q", joined)
	}
	if strings.Contains(joined, "3.1.*") {
		t.Errorf("project's own airflow pin should be dropped: %q", joined)
	}
	if !strings.Contains(joined, "apache-airflow-providers-standard") {
		t.Errorf("providers must survive dropAirflow: %q", joined)
	}
}

// A consumer with nowhere to stream notes passes nil progress, and Preflight
// promises never to return an error — so it must not panic on one either.
// Astro Desktop is the first such caller: it has no text renderer.
//
// The MWAA path is the one that matters: mwaaConstraints calls progress
// unconditionally, so this reaches the deref rather than short-circuiting.
func TestPreflightToleratesANilProgress(t *testing.T) {
	prov := &fakeProvisioner{python: "scratch-python"}
	parser := &fakeTargetParser{}
	rep := Preflight(context.Background(), TargetMWAA, PreflightInput{
		ProjectPath: "proj", Pin: "2.10.5", Deps: []string{"requests"},
	}, prov, parser, false, nil)
	if rep.Target != TargetMWAA {
		t.Errorf("target = %q, want %q", rep.Target, TargetMWAA)
	}
	if rep.OpError != "" {
		t.Errorf("OpError = %q, want none", rep.OpError)
	}
}
