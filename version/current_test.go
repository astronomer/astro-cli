package version

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"testing"
)

// withCurrVersion sets the linked-in version for one test and restores it.
func withCurrVersion(t *testing.T, v string) {
	t.Helper()
	prev := CurrVersion
	CurrVersion = v
	t.Cleanup(func() { CurrVersion = prev })
}

// withDerived replaces what the toolchain would report, for one test. The real
// [derive] reads this binary's own build info, which under `go test` is the test
// binary's, so the builds that matter cannot be produced from inside a test.
func withDerived(t *testing.T, v string) {
	t.Helper()
	prev := derive
	derive = func() string { return v }
	t.Cleanup(func() { derive = prev })
}

// The guarantee: the CLI can always say what it is.
//
// CurrVersion arrives from the Makefile's -ldflags and is empty in any build
// that skipped them, which `astro version` printed verbatim — the label and
// nothing after it.
func TestVersionLineAlwaysNamesAVersion(t *testing.T) {
	withCurrVersion(t, "")

	if got := Current(); got == "" {
		t.Fatal("Current() is empty, so `astro version` prints nothing after its label")
	}
	line := versionLine()
	if strings.TrimSpace(strings.TrimPrefix(line, cliCurrentVersion)) == "" {
		t.Errorf("versionLine() = %q, which is the label and nothing after it", line)
	}
	// Derived from this binary's own build info, so the exact value is the
	// toolchain's business. The shapes it can take are pinned on buildVersion
	// directly, below.
	t.Logf("derived version = %q", Current())
}

// The linker flag wins when it is there, because that is the release build and
// `make install`.
func TestCurrentPrefersTheLinkedVersion(t *testing.T) {
	withCurrVersion(t, "SNAPSHOT-deadbee")
	if got := Current(); got != "SNAPSHOT-deadbee" {
		t.Errorf("Current() = %q, want the linked value", got)
	}
}

// info builds the build info a given kind of build would carry.
func info(mainVersion string, settings ...string) *debug.BuildInfo {
	bi := &debug.BuildInfo{}
	bi.Main.Version = mainVersion
	for i := 0; i+1 < len(settings); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return bi
}

// The two things the toolchain can tell us apart, and the one that must not be
// believed.
//
// The case that drives the ordering is "a working copy, which Go also gave a
// pseudo-version": that version descends from the newest reachable tag, so
// reporting it has a dev build claiming a release it is not — to `astro
// version`, to telemetry, and to the platform API in x-astro-client-version,
// where a min-version or feature gate may act on it.
func TestBuildVersionNamesTheBuild(t *testing.T) {
	const rev = "7dba1525c1708f1a5f97fb50e21459be1021d03f"

	for _, tc := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			"a working copy is named after its revision",
			info("", "vcs.revision", rev, "vcs.modified", "false"),
			"SNAPSHOT-7dba152",
		},
		{
			"a modified working copy says so",
			info("", "vcs.revision", rev, "vcs.modified", "true"),
			"SNAPSHOT-7dba152-dirty",
		},
		{
			// Go stamps both, and the pseudo-version is the one to ignore.
			"a working copy Go also gave a pseudo-version",
			info("v1.46.0-nightly.20260819.0.20260916175924-7dba1525c170+dirty",
				"vcs.revision", rev, "vcs.modified", "true"),
			"SNAPSHOT-7dba152-dirty",
		},
		{
			// No VCS stamp: the source came from the module proxy, so this is a
			// real release, spelled the way goreleaser stamps it.
			"installed at a version",
			info("v1.45.0"),
			"1.45.0",
		},
		{
			"a build with nothing to go on",
			info("(devel)"),
			unknownVersion,
		},
		{
			"no module version either",
			info(""),
			unknownVersion,
		},
		{
			"a revision shorter than we would truncate to",
			info("", "vcs.revision", "abc12"),
			"SNAPSHOT-abc12",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildVersion(tc.info); got != tc.want {
				t.Errorf("buildVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A dev build must never be mistaken for a release, and a release must never be
// mistaken for a dev build.
func TestReleasedOnlyForARelease(t *testing.T) {
	for _, tc := range []struct {
		name    string
		curr    string
		derived string
		want    bool
	}{
		{name: "a stamped release", curr: "1.45.0", want: true},
		{name: "a stamped snapshot", curr: "SNAPSHOT-f2fd8ac", want: false},
		// The other convention for spelling a snapshot. The Makefile's VERSION
		// is an override, and a suffix is how build scripts usually write this.
		{name: "a snapshot spelled as a suffix", curr: "1.46.0-SNAPSHOT", want: false},
		{name: "the bare word", curr: "SNAPSHOT", want: false},
		// Nothing stamped, so the version was derived. A working copy derives a
		// snapshot; an install at a version derives a release, and it has
		// something to upgrade to like any other.
		{name: "a derived working copy", derived: "SNAPSHOT-7dba152-dirty", want: false},
		{name: "a derived unknown", derived: unknownVersion, want: false},
		{name: "a derived module version", derived: "1.45.0", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withCurrVersion(t, tc.curr)
			if tc.curr == "" {
				withDerived(t, tc.derived)
			}
			if got := Released(); got != tc.want {
				t.Errorf("Released() = %v, want %v (Current() = %q)", got, tc.want, Current())
			}
		})
	}
}

// failingTransport fails any request, so a test can prove whether one was made.
type failingTransport struct{ used bool }

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.used = true
	return nil, context.Canceled
}

// A local build asks the network nothing. Before, an empty version did not
// contain "SNAPSHOT", so every command fetched the release list and then failed
// to parse "" against it.
func TestCompareVersionsSkipsTheNetworkForALocalBuild(t *testing.T) {
	for _, tc := range []struct {
		name    string
		curr    string
		derived string
	}{
		{name: "nothing stamped, built from a working copy", derived: "SNAPSHOT-7dba152-dirty"},
		{name: "nothing stamped, nothing to go on", derived: unknownVersion},
		{name: "a stamped snapshot", curr: "SNAPSHOT-f2fd8ac"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withCurrVersion(t, tc.curr)
			if tc.curr == "" {
				withDerived(t, tc.derived)
			}
			tr := &failingTransport{}
			if err := CompareVersions(context.Background(), &http.Client{Transport: tr}); err != nil {
				t.Errorf("a local build must not report an upgrade-check error: %v", err)
			}
			if tr.used {
				t.Error("a local build fetched the release list; it has nothing to upgrade to")
			}
		})
	}
}

// The other side of that rule: an install straight from source at a real
// version is a release, and it does get told when a newer one exists. Gating
// this on the linker flag alone would have excluded it forever.
func TestCompareVersionsChecksAnInstallFromSource(t *testing.T) {
	withCurrVersion(t, "")
	withDerived(t, "1.45.0")

	tr := &failingTransport{}
	// The error is the transport refusing; what is being asserted is that the
	// check got as far as asking.
	if err := CompareVersions(context.Background(), &http.Client{Transport: tr}); err == nil {
		t.Error("expected the failing transport's error to surface")
	}
	if !tr.used {
		t.Error("a release installed from source never checked for a newer one")
	}
}
