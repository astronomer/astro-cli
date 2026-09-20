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

	if got, ok := pickSpelling(both, "analytics"); got != "analytics" || !ok {
		t.Errorf("the name asked for is the name meant, got %q, %v", got, ok)
	}
	if got, ok := pickSpelling(both, "Analytics"); got != "Analytics" || !ok {
		t.Errorf("got %q, %v", got, ok)
	}
	// Order must not decide it.
	if got, ok := pickSpelling([]string{"analytics", "Analytics"}, "analytics"); got != "analytics" || !ok {
		t.Errorf("order decided the answer, got %q, %v", got, ok)
	}
}

// With no exact match, the filesystem's spelling wins — the
// case-insensitive situation.
func TestPickSpellingFallsBackToCaseInsensitive(t *testing.T) {
	if got, ok := pickSpelling([]string{"Analytics"}, "analytics"); got != "Analytics" || !ok {
		t.Errorf("got %q, %v", got, ok)
	}
	if got, ok := pickSpelling([]string{"other", "Analytics"}, "ANALYTICS"); got != "Analytics" || !ok {
		t.Errorf("got %q, %v", got, ok)
	}
}

// A name the directory does not hold comes back untouched, so a path
// pointing at something not yet created is not mangled — and reported as a
// miss, which is what sends the caller on to ask the filesystem by identity.
//
// The bool matters more than it looks: without it a miss and an exact match
// are the same answer, and the identity scan that catches Unicode
// normalization would run for every component or none.
func TestPickSpellingKeepsAnUnknownName(t *testing.T) {
	if got, ok := pickSpelling([]string{"Analytics"}, "missing"); got != "missing" || ok {
		t.Errorf("got %q, %v", got, ok)
	}
	if got, ok := pickSpelling(nil, "anything"); got != "anything" || ok {
		t.Errorf("got %q, %v", got, ok)
	}
}

// A name that differs only by Unicode normalization is a miss for
// pickSpelling, deliberately.
//
// Nothing here folds normalization, because folding it in text is the
// approach this stopped using: the list of equivalences a volume honors is
// the volume's to know. pickSpelling reports that it found nothing, and the
// identity scan settles it against the filesystem.
func TestPickSpellingDoesNotFoldNormalizationItself(t *testing.T) {
	if got, ok := pickSpelling([]string{nfcCafe}, nfdCafe); got != nfdCafe || ok {
		t.Errorf("got %q, %v — normalization belongs to the filesystem, not to this", got, ok)
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

// The same text, spelled the two ways Unicode allows. macOS hands out NFD
// from some APIs and NFC from others — a name typed in Terminal arrives one
// way, the same name from Finder or a `git clone` the other — so both reach
// the CLI for one directory.
const (
	nfcCafe = "caf\u00e9"  // é as a single rune
	nfdCafe = "cafe\u0301" // e followed by a combining acute
)

// normalizationInsensitiveFS reports whether the filesystem under dir treats
// the two spellings as one name. Asked rather than assumed, for the same
// reason caseInsensitiveFS is: the answer belongs to the volume, not to GOOS.
func normalizationInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "norm-"+nfcCafe)
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatalf("probing the filesystem: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(probe) })

	_, err := os.Stat(filepath.Join(dir, "norm-"+nfdCafe))
	return err == nil
}

// Unicode normalization, on whichever filesystem the test is running on.
//
// The third member of the family trueCase exists for, after symlinks and
// case: one directory reachable by more than one spelling. The id is a hash
// of a spelling, so every equivalence the filesystem recognizes and the hash
// does not becomes two project ids for one directory — two state records, two
// Airflows, `astro local list` showing it twice and `stop` ending one.
//
// Two branches rather than two skipping tests, so something always asserts,
// and because the directions are different properties. The second is again
// the dangerous one: folding normalization unconditionally would merge two
// genuinely separate directories on a byte-exact filesystem, where `café`
// spelled two ways really is two projects.
func TestCanonicalPathHandlesUnicodeNormalization(t *testing.T) {
	base := t.TempDir()
	insensitive := normalizationInsensitiveFS(t, base)
	t.Logf("filesystem under %s ignores Unicode normalization: %v", base, insensitive)

	if insensitive {
		t.Run("one directory, two spellings, one project", func(t *testing.T) {
			actual := filepath.Join(base, nfcCafe)
			mustMkdir(t, actual)
			other := filepath.Join(base, nfdCafe)

			// Two siblings that sort ahead of it, each killing a different
			// wrong answer. A plain directory catches a scan that returns
			// whatever it can stat — without it the target is listed first and
			// that mutation passes by luck. A symlink TO the target catches a
			// scan that follows links: os.Stat does, so the alias compared
			// equal to the real entry and its name became the canonical
			// spelling, which is the wrong id and a path with an unresolved
			// symlink in it after CanonicalPath has promised there are none.
			mustMkdir(t, filepath.Join(base, "aaa-decoy"))
			if err := os.Symlink(actual, filepath.Join(base, "aaa-alias")); err != nil {
				t.Fatal(err)
			}

			fromActual := mustCanonical(t, actual)
			fromOther := mustCanonical(t, other)
			if fromActual != fromOther {
				t.Errorf("one directory, two canonical paths:\n  %q\n  %q", fromActual, fromOther)
			}
			if mustID(t, actual) != mustID(t, other) {
				t.Error("one directory must not be two projects")
			}
			// And the spelling that wins is the filesystem's own, not merely a
			// consistent one — the assertion the case test makes too. An
			// answer that agreed with itself while naming a sibling would
			// satisfy everything above.
			if got := filepath.Base(fromOther); got != nfcCafe {
				t.Errorf("the filesystem's spelling should win, got %q", got)
			}
		})
		return
	}

	t.Run("two directories, two projects", func(t *testing.T) {
		one := filepath.Join(base, nfcCafe)
		two := filepath.Join(base, nfdCafe)
		// os.Mkdir, not MkdirAll: this branch's premise is that these are two
		// directories, and MkdirAll succeeds silently when they are one. If
		// the probe above ever misreports, that turns a correct
		// implementation into a confusing failure on the assertion below
		// rather than a clear one here.
		if err := os.Mkdir(one, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(two, 0o755); err != nil {
			t.Fatalf("the probe said this volume distinguishes the spellings, but: %v", err)
		}

		if mustID(t, one) == mustID(t, two) {
			t.Error("two directories must not share one project id")
		}
	})
}

// The identity scan, called directly, so it is exercised on every platform.
//
// Through spellingOnDisk it is reachable only where the volume conflates two
// spellings of one name — macOS here, and nowhere on a Linux runner — because
// anywhere else a component that exists is in the listing and the exact pass
// answers first. That left the whole scan dead code on half of CI.
// pickSpelling was pulled out of the I/O for exactly this reason: what cannot
// be reached cannot be falsified.
//
// Calling it with an entries list sidesteps that, and a hard link gives it a
// genuine second claimant to be wrong about anywhere.
func TestSameEntrySpellingRefusesToGuessBetweenTwoClaimants(t *testing.T) {
	base := t.TempDir()
	leaf := filepath.Join(base, "leaf.env")
	if err := os.WriteFile(leaf, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Sorts first, so a scan that takes the earliest match rather than
	// refusing an ambiguous one answers with this name.
	if err := os.Link(leaf, filepath.Join(base, "aaa-hardlink")); err != nil {
		t.Skipf("this filesystem does not support hard links: %v", err)
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}

	// Two names for one file, so there is no answer to give: the spelling asked
	// for comes back untouched rather than replaced by a sibling that happens
	// to share an inode.
	if got := sameEntrySpelling(base, entries, "leaf.env"); got != "leaf.env" {
		t.Errorf("sameEntrySpelling answered %q for a file with two names", got)
	}
}

// And a single claimant of the right kind is answered.
//
// The other half of the above: refusing every time would be an easy way to
// pass it, and would quietly undo the normalization fix.
func TestSameEntrySpellingAnswersWhenOneEntryMatches(t *testing.T) {
	base := t.TempDir()
	mustMkdir(t, filepath.Join(base, "target"))
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}

	if got := sameEntrySpelling(base, entries, "target"); got != "target" {
		t.Errorf("sameEntrySpelling(target) = %q, want the entry it matched", got)
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
