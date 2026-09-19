package rt

// Plain stdlib testing, like its neighbors: pkg/localrt is a shared
// sub-module and keeps its dependency list near-empty
// (docs/v2-architecture.md), and mixing styles inside one package is the
// thing that rule exists to stop.

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

// caseInsensitiveFS reports whether the filesystem under dir ignores case,
// asked rather than assumed: a case-sensitive volume can be mounted under a
// case-insensitive one, and GOOS does not know which it got.
func caseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatalf("probing the filesystem: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(probe) })

	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// Case, on whichever filesystem the test is running on.
//
// One test with two branches rather than two that each skip, so that
// something always asserts. Two skipping tests can both skip — if the probe
// above ever misreports, the package goes green having checked nothing.
//
// The two directions are different properties, and the second is the
// dangerous one:
//
//   - ignore case, and one directory reached two ways must be ONE project.
//     macOS ignores case, neither Abs nor EvalSymlinks touches it, and
//     ~/work/analytics and ~/Work/analytics used to be two project ids, two
//     state records, two Airflows.
//   - respect case, and two directories must stay TWO projects. Folding
//     case unconditionally would merge somebody's separate projects into one
//     state record, which is worse than the bug.
func TestCanonicalPathHandlesCase(t *testing.T) {
	base := t.TempDir()
	insensitive := caseInsensitiveFS(t, base)
	t.Logf("filesystem under %s ignores case: %v", base, insensitive)

	if insensitive {
		t.Run("one directory, two spellings, one project", func(t *testing.T) {
			actual := filepath.Join(base, "Analytics")
			mustMkdir(t, actual)
			wrong := filepath.Join(base, "analytics")

			fromActual := mustCanonical(t, actual)
			fromWrong := mustCanonical(t, wrong)
			if fromActual != fromWrong {
				t.Errorf("one directory, two canonical paths:\n  %s\n  %s", fromActual, fromWrong)
			}
			if got := filepath.Base(fromWrong); got != "Analytics" {
				t.Errorf("the filesystem's spelling should win, got %q", got)
			}

			// Which is the point: the id is what state is keyed on.
			if mustID(t, actual) != mustID(t, wrong) {
				t.Error("one directory must not be two projects")
			}
		})
		return
	}

	t.Run("two directories stay two projects", func(t *testing.T) {
		upper := filepath.Join(base, "Analytics")
		lower := filepath.Join(base, "analytics")
		mustMkdir(t, upper)
		mustMkdir(t, lower)

		cUpper, cLower := mustCanonical(t, upper), mustCanonical(t, lower)
		if cUpper == cLower {
			t.Fatalf("two directories collapsed into one: %s", cUpper)
		}
		if got := filepath.Base(cUpper); got != "Analytics" {
			t.Errorf("upper = %q", got)
		}
		if got := filepath.Base(cLower); got != "analytics" {
			t.Errorf("lower = %q", got)
		}
		if mustID(t, upper) == mustID(t, lower) {
			t.Error("two projects must not share one state record")
		}
	})
}

// A correctly spelled path comes back unchanged, on either kind of
// filesystem — so no existing project's id moves.
func TestCanonicalPathLeavesACorrectSpellingAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Analytics")
	mustMkdir(t, dir)

	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustCanonical(t, dir); got != want {
		t.Errorf("CanonicalPath = %q, want %q", got, want)
	}
}

// The rule the filesystem here cannot exercise.
//
// An exact match must beat a case-insensitive one, which only shows itself
// when a directory holds two names differing just by case — something a
// case-insensitive filesystem cannot represent, so on macOS removing the
// exact pass changes no behavior and no test. Handed the names directly, the
// rule is checkable anywhere.
func TestPickSpellingPrefersAnExactMatch(t *testing.T) {
	both := []string{"Analytics", "analytics"}

	if got := pickSpelling(both, "analytics"); got != "analytics" {
		t.Errorf("the name asked for is the name meant, got %q", got)
	}
	if got := pickSpelling(both, "Analytics"); got != "Analytics" {
		t.Errorf("got %q", got)
	}
	// Order must not decide it.
	if got := pickSpelling([]string{"analytics", "Analytics"}, "analytics"); got != "analytics" {
		t.Errorf("order decided the answer, got %q", got)
	}
}

// With no exact match, the filesystem's spelling wins — the
// case-insensitive situation.
func TestPickSpellingFallsBackToCaseInsensitive(t *testing.T) {
	if got := pickSpelling([]string{"Analytics"}, "analytics"); got != "Analytics" {
		t.Errorf("got %q", got)
	}
	if got := pickSpelling([]string{"other", "Analytics"}, "ANALYTICS"); got != "Analytics" {
		t.Errorf("got %q", got)
	}
}

// A name the directory does not hold comes back untouched, so a path
// pointing at something not yet created is not mangled.
func TestPickSpellingKeepsAnUnknownName(t *testing.T) {
	if got := pickSpelling([]string{"Analytics"}, "missing"); got != "missing" {
		t.Errorf("got %q", got)
	}
	if got := pickSpelling(nil, "anything"); got != "anything" {
		t.Errorf("got %q", got)
	}
}

// A parent that exists but cannot be listed keeps the spelling it was
// given, rather than failing a command over a display detail.
//
// This is the fallback that actually fires: a missing directory never
// reaches trueCase, because CanonicalPath runs EvalSymlinks first and that
// errors on any absent component. An unreadable one does — macOS gates
// ~/Documents and ~/Desktop per application, so one tool can list a parent
// that another cannot.
func TestTrueCaseKeepsASpellingItCannotVerify(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can list anything")
	}
	if goruntime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
	base := t.TempDir()
	parent := filepath.Join(base, "Locked")
	child := filepath.Join(parent, "Project")
	mustMkdir(t, parent)
	mustMkdir(t, child)

	if err := os.Chmod(parent, 0o111); err != nil { // traversable, not listable
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	// The child's own name cannot be checked, so it stays as given.
	got := trueCase(filepath.Join(parent, "project"))
	if filepath.Base(got) != "project" {
		t.Errorf("an unlistable parent should leave the spelling alone, got %q", got)
	}
}

// The walk must not mangle the shapes it is handed.
func TestTrueCaseLeavesOddInputsAlone(t *testing.T) {
	sep := string(filepath.Separator)
	if got := trueCase(sep); got != sep {
		t.Errorf("root = %q", got)
	}
	// A relative path is not a shape the walk understands; Abs runs first in
	// CanonicalPath, so this is defense rather than a live case.
	if got := trueCase("relative/path"); got != "relative/path" {
		t.Errorf("relative = %q", got)
	}
}

// Every component is respelled, not only the last: the wrong case can be
// anywhere in the path.
func TestTrueCaseRespellsEveryComponent(t *testing.T) {
	base := t.TempDir()
	if !caseInsensitiveFS(t, base) {
		t.Skip("case-sensitive filesystem")
	}
	mustMkdir(t, filepath.Join(base, "Outer", "Inner"))

	got := trueCase(filepath.Join(base, "outer", "inner"))
	if !strings.HasSuffix(got, filepath.Join("Outer", "Inner")) {
		t.Errorf("both components should be respelled, got %q", got)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustCanonical(t *testing.T, path string) string {
	t.Helper()
	got, err := CanonicalPath(path)
	if err != nil {
		t.Fatalf("CanonicalPath(%s): %v", path, err)
	}
	return got
}

func mustID(t *testing.T, path string) string {
	t.Helper()
	id, err := ProjectID(path)
	if err != nil {
		t.Fatalf("ProjectID(%s): %v", path, err)
	}
	return id
}
