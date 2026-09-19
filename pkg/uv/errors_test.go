package uv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture is stderr uv really wrote, captured under testdata. Shared with
// summary_test.go, which drives the same files through the summariser.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	return string(b)
}

func TestParseResolutionRealFixture(t *testing.T) {
	got := parseResolution(readFixture(t, "no-solution"))

	wantSummary := "Because apache-airflow==2.10.4 depends on flask>=2.2.1,<2.3 and your " +
		"project depends on apache-airflow==2.10.4, we can conclude that your " +
		"project depends on flask>=2.2.1,<2.3. " +
		"And because your project depends on flask>=3.1, we can conclude that " +
		"your project's requirements are unsatisfiable."
	if got.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", got.Summary, wantSummary)
	}
	if want := []string{"apache-airflow", "flask"}; strings.Join(got.Packages, " ") != strings.Join(want, " ") {
		t.Errorf("Packages = %v, want %v", got.Packages, want)
	}
	want := []string{
		"apache-airflow==2.10.4",
		"flask>=2.2.1,<2.3",
		"flask>=3.1",
	}
	if strings.Join(got.Constraints, " ") != strings.Join(want, " ") {
		t.Errorf("Constraints = %v, want %v", got.Constraints, want)
	}
}

func TestAsResolutionParseMissKeepsRawStderr(t *testing.T) {
	// A solver failure whose explanation block we do not recognize: the
	// typed fields stay empty and the error degrades to the plain
	// CommandError rendering — never worse than today.
	stderr := "error: No solution found when resolving dependencies\nsome shape this parser has never seen"
	cmdErr := &CommandError{Args: []string{"lock"}, ExitCode: 1, Stderr: stderr}

	err := asResolution("lock", error(cmdErr))

	var resErr *ResolutionError
	if !errors.As(err, &resErr) {
		t.Fatalf("asResolution() = %v, want *ResolutionError", err)
	}
	if resErr.Summary != "" || len(resErr.Packages) != 0 || len(resErr.Constraints) != 0 {
		t.Errorf("parse miss produced fields: %+v", resErr)
	}
	if resErr.Stderr != stderr {
		t.Errorf("Stderr = %q, want the raw stderr", resErr.Stderr)
	}
	if err.Error() != cmdErr.Error() {
		t.Errorf("Error() = %q, want the CommandError fallback %q", err.Error(), cmdErr.Error())
	}
}

func TestAsResolutionPassesOtherErrorsThrough(t *testing.T) {
	if err := asResolution("lock", nil); err != nil {
		t.Errorf("asResolution(nil) = %v, want nil", err)
	}
	plain := errors.New("context canceled")
	if err := asResolution("lock", plain); err != plain {
		t.Errorf("asResolution(plain) = %v, want it untouched", err)
	}
	cmdErr := &CommandError{Args: []string{"sync"}, ExitCode: 2, Stderr: "error: disk full"}
	if err := asResolution("sync", error(cmdErr)); err != error(cmdErr) {
		t.Errorf("asResolution(non-resolution CommandError) = %v, want it untouched", err)
	}
}

func TestCommandErrorRendering(t *testing.T) {
	err := &CommandError{
		Args:     []string{"--no-config", "lock"},
		ExitCode: 2,
		Stderr:   "warning: something\nerror: the real problem\n",
	}
	if want := "uv lock failed (exit 2): error: the real problem"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}

	bare := &CommandError{Args: []string{"sync"}, ExitCode: -1}
	if want := "uv sync failed (exit -1)"; bare.Error() != want {
		t.Errorf("Error() = %q, want %q", bare.Error(), want)
	}
}
