package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	semver "github.com/Masterminds/semver/v3"

	"github.com/astronomer/astro-cli/pkg/ansi"
)

// CurrVersion is stamped in at link time by the Makefile and by goreleaser, and
// is empty in any build that did not pass the -ldflags. Read it through
// [Current], which supplies a version when it is empty.
//
// The proxy daemon handshake in airflow/proxy reads this directly, and on
// purpose: an unstamped build must not force a running daemon to restart, and
// an empty version is how it recognizes itself as one.
var CurrVersion string

const (
	cliCurrentVersion  = "Astro CLI Version: "
	astroCLIReleaseURL = "https://updates.astronomer.io/astro-cli"
	// snapshotMarker appears in the version of every build that is not a
	// release, and [Released] is the one rule that reads it. The Makefile
	// stamps "SNAPSHOT-<short sha>" and a derived dev version is built to the
	// same shape, but the bare word is what we match: "1.46.0-SNAPSHOT" is the
	// other convention for spelling this, and a VERSION override using it
	// should suppress the upgrade check just the same.
	snapshotMarker = "SNAPSHOT"
	snapshotPrefix = snapshotMarker + "-"
	// unknownVersion is for a build nothing can identify. Deliberately a
	// snapshot: an unidentifiable build is a local one as far as the upgrade
	// check is concerned, and treating it as a release means fetching the
	// release list only to fail parsing an empty string against it.
	unknownVersion = snapshotPrefix + "unknown"
	// shortRevLen matches `git rev-parse --short`, which is what the Makefile
	// passes, so the two spellings of a dev build look alike.
	shortRevLen = 7
)

// Current is the version to report and to compare against the release list.
//
// The linker flag first, because a release build and `make install` both set
// it. Failing that, whatever the toolchain recorded about this binary — see
// [buildVersion] for the two things that can be.
//
// Without this, any build that skipped the -ldflags reported nothing at all:
// `astro version` printed its label and stopped, which is a CLI unable to say
// what it is. Installing straight from source is the path that gets there, and
// the upgrade check then parsed that empty string against semver on every
// command and logged the failure.
func Current() string {
	if CurrVersion != "" {
		return CurrVersion
	}
	return derive()
}

// derive is the build-info read, memoized: what the toolchain recorded cannot
// change while the process runs, and [Current] is called once per API request
// (twice — the client header and the user agent), once per telemetry event and
// once per pre-deploy. debug.ReadBuildInfo re-parses the binary's whole
// embedded module list on every call, so an `astro deploy` would otherwise pay
// for that dozens of times to learn the same answer.
//
// It is also the seam a test replaces to stand a different build in for this
// one.
var derive = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownVersion
	}
	return buildVersion(info)
})

// Released reports whether this version is one of our releases, which is the
// only kind with anything to upgrade to. A snapshot is a local build, and so is
// anything we could not identify.
//
// This reads [Current], so it covers a version we derived as well as one
// stamped in. That is safe because every derived version is either a snapshot
// or a module version, and [buildVersion] is careful about which: a build from
// a working copy is named after its revision, never after the release its
// pseudo-version descends from. Without that care a developer's own build would
// parse as a release and nag them to upgrade to whatever the release list
// happens to name.
func Released() bool {
	return !strings.Contains(Current(), snapshotMarker)
}

// buildVersion names this binary from what the toolchain stamped into it.
//
// The VCS stamp comes first, because the presence of it is what tells the two
// cases apart. Building in a working copy records vcs.revision, and Go also
// synthesizes a pseudo-version for Main.Version from it — descended from the
// newest reachable tag, which is a release this build is emphatically not, and
// carrying no marker to say so. A working copy is therefore named after its
// revision instead, in the shape the Makefile stamps.
//
// Main.Version is then left meaning what it means with no VCS stamp to explain
// it: this binary was installed at a version, from the module proxy, and that
// version is a real release. Go mints a pseudo-version only out of VCS
// information, so one cannot reach here.
func buildVersion(info *debug.BuildInfo) string {
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" {
		// Worth knowing when this surprises you: building from a git worktree
		// nested inside the main checkout stamps the *main* checkout's
		// revision and dirty flag, because Go walks past the worktree's .git
		// file to the .git directory above it. Go's to fix; either way what
		// comes out is a snapshot, which is the part that has to be right.
		if len(rev) > shortRevLen {
			rev = rev[:shortRevLen]
		}
		if dirty {
			// Uncommitted changes, so the revision alone would name a tree this
			// is not. Worth saying, since this is the build people bisect with.
			return snapshotPrefix + rev + "-dirty"
		}
		return snapshotPrefix + rev
	}
	// "(devel)" is what a build with nothing to go on reports, and it says no
	// more than the empty string does.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		// Trimmed because goreleaser stamps {{ .Version }}, which drops the v.
		// One release must not have two spellings depending on how it arrived.
		return strings.TrimPrefix(v, "v")
	}
	// No VCS stamp and no module version: built outside a repository, from an
	// unpacked archive, or with -buildvcs=false.
	return unknownVersion
}

type astroCLIRelease struct {
	Version string `json:"version"`
}

type astroCLIReleaseResponse struct {
	AvailableReleases []astroCLIRelease `json:"available_releases"`
}

// versionLine is what [PrintVersion] prints, split out so a test can hold it to
// naming a version: the bug this replaced printed the label and nothing after.
func versionLine() string {
	return cliCurrentVersion + Current()
}

// PrintVersion writes the `astro version` line to w. Its bytes are a contract:
// astronomer/deploy-action runs `astro version | awk '{print $4}'`, so the
// version has to stay the fourth whitespace-separated field.
func PrintVersion(w io.Writer) error {
	_, err := fmt.Fprintln(w, versionLine())
	return err
}

func getCLIReleases(ctx context.Context, client *http.Client, url string) (*astroCLIReleaseResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("astro-cli releases endpoint request failed with status: %d", response.StatusCode)
	}

	var releases astroCLIReleaseResponse
	err = json.NewDecoder(response.Body).Decode(&releases)
	if err != nil {
		return nil, err
	}

	return &releases, nil
}

func getLatestRelease(ctx context.Context, client *http.Client, url string) (*semver.Version, error) {
	releases, err := getCLIReleases(ctx, client, url)
	if err != nil {
		return nil, err
	}

	if len(releases.AvailableReleases) == 0 {
		return nil, errors.New("astro-cli releases endpoint returned 0 results")
	}

	var latest *semver.Version
	for _, r := range releases.AvailableReleases {
		// discard any versions that we cannot parse
		v, err := semver.NewVersion(r.Version)
		if err == nil && (latest == nil || v.GreaterThan(latest)) {
			latest = v
		}
	}

	if latest == nil {
		return nil, errors.New("astro-cli releases endpoint returned 0 valid versions")
	}

	return latest, nil
}

func CompareVersions(ctx context.Context, client *http.Client) error {
	// Only a release has a version worth comparing. A snapshot, and anything we
	// could not identify, is a local build: there is nothing for it to upgrade
	// to, and asking costs a request on every command.
	if !Released() {
		return nil
	}

	// Get the latest release
	latestSemver, err := getLatestRelease(ctx, client, astroCLIReleaseURL)
	if err != nil {
		return err
	}

	// Parse the current and latest versions into semver objects
	currentSemver, err := semver.NewVersion(Current())
	if err != nil {
		return err
	}

	// Compare the versions and print a message to the user if the current version is outdated
	if currentSemver.LessThan(latestSemver) {
		fmt.Fprintf(os.Stderr, "\nA newer version of Astro CLI is available: %s\nPlease see https://www.astronomer.io/docs/astro/cli/upgrade-cli for information on how to update the Astro CLI\n\n", latestSemver)
		fmt.Fprint(os.Stderr, ansi.Cyan("\nTo learn more about what's new in this version, please see https://www.astronomer.io/docs/astro/cli/release-notes\n\n"))
		fmt.Fprintf(os.Stderr, "If you don't want to see this message again run 'astro config set -g upgrade_message false' or pass '2>/dev/null' to print this text to stderr\n\n")
	}

	return nil
}
