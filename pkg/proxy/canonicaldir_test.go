package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

// The same text, spelled the two ways Unicode allows.
const (
	nfcCafe = "café"  // é as a single rune
	nfdCafe = "café" // e followed by a combining acute
)

// canonicalDir agrees with rt.CanonicalPath about Unicode normalization.
//
// This file is a deliberate mirror of rt's canonicalizer — pkg/localrt depends
// on pkg/proxy, so the dependency cannot run the other way — and the mirror's
// note says it has to be kept rather than claimed. It was not, once: rt learned
// to respell a path to the filesystem's own capitalization and this did not,
// and the two then disagreed about what counts as one project. A route
// re-registered from the other spelling was treated as a stranger, handed a
// qualified hostname, and the original row was left pointing at a dead port.
//
// Normalization is the same lesson, so it is tested the same way, in both
// modules, on whichever filesystem the test is running on.
func TestCanonicalDirHandlesUnicodeNormalization(t *testing.T) {
	base := t.TempDir()

	probe := filepath.Join(base, "norm-"+nfcCafe)
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatalf("probing the filesystem: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(probe) })
	_, err := os.Stat(filepath.Join(base, "norm-"+nfdCafe))
	insensitive := err == nil
	t.Logf("filesystem under %s ignores Unicode normalization: %v", base, insensitive)

	if insensitive {
		actual := filepath.Join(base, nfcCafe)
		if err := os.Mkdir(actual, 0o755); err != nil {
			t.Fatal(err)
		}
		// Sorts ahead of the target, so an identity scan that follows links
		// or takes the first thing it can stat answers with this instead.
		if err := os.Symlink(actual, filepath.Join(base, "aaa-alias")); err != nil {
			t.Fatal(err)
		}

		fromActual := canonicalDir(actual)
		fromOther := canonicalDir(filepath.Join(base, nfdCafe))
		if fromActual != fromOther {
			t.Errorf("one directory, two canonical paths:\n  %q\n  %q", fromActual, fromOther)
		}
		if got := filepath.Base(fromOther); got != nfcCafe {
			t.Errorf("the filesystem's spelling should win, got %q", got)
		}
		return
	}

	// Byte-exact: two directories, and they must stay two. os.Mkdir rather
	// than MkdirAll, so a probe that misreported shows up here instead of as
	// a confusing failure below.
	one := filepath.Join(base, nfcCafe)
	two := filepath.Join(base, nfdCafe)
	if err := os.Mkdir(one, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(two, 0o755); err != nil {
		t.Fatalf("the probe said this volume distinguishes the spellings, but: %v", err)
	}
	if canonicalDir(one) == canonicalDir(two) {
		t.Error("two directories must not share one canonical path")
	}
}
