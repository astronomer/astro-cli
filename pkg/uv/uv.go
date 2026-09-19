// Package uv drives the uv package manager (https://docs.astral.sh/uv/) for
// astro projects: creating the project venv, locking, syncing, and running
// commands inside the environment. It is a shared sub-module, so the rules
// from docs/v2-architecture.md apply: no printing, no exiting, stdlib only.
//
// uv operates on the project directory and reads pyproject.toml itself; this
// package never parses the manifest (that is pkg/manifest, one layer up) —
// it only needs the project path. The environment lives at <project>/.venv,
// where editors auto-detect it. The MVP locks against public PyPI only.
//
// The uv binary is discovered in three tiers: the ASTRO_UV_BIN environment
// variable (an explicit override that errors rather than falls through when
// unusable), then an embedder-supplied directory (desktop bundles a pinned
// uv inside its .app), then PATH — augmented with the common install
// locations a GUI-launched process loses to a stripped launchd PATH.
package uv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// MinVersion is the oldest uv this package accepts. uv is pre-1.0 and moves
// fast — flags appear and lockfile revisions bump between minors — so New
// checks the floor once instead of letting individual calls fail strangely
// later. 0.6.0 (February 2025) is old enough to be everywhere, and
// everything this package invokes (venv --allow-existing, lock, sync, run,
// --no-config) is stable in it.
const MinVersion = "0.6.0"

// EnvBin overrides discovery entirely: when set, its value is the uv binary
// to use, and a value that is not an executable file is an error rather than
// a fall-through.
const EnvBin = "ASTRO_UV_BIN"

// Options configures a Client.
type Options struct {
	// BinDir is an embedder-supplied directory to search after the EnvBin
	// override and before PATH. Desktop passes its bundled-uv directory;
	// the CLI passes nothing. A BinDir without a uv in it falls through to
	// PATH — it is a preference, unlike the env override.
	BinDir string

	// CacheDir becomes UV_CACHE_DIR for every invocation, so every
	// embedder sharing it pays the Python-toolchain and wheel downloads
	// once. Required; uv creates the directory itself.
	CacheDir string

	// NoConfig passes --no-config to every invocation. Without it uv
	// discovers configuration beyond the project — the user's uv.toml and
	// any [tool.uv] in parent directories — whose settings can break
	// resolution in ways that look like our bug: a user-level
	// exclude-newer quietly filters freshly published packages and lock
	// fails with "No solution found". The cost: --no-config also ignores
	// the project's own [tool.uv] table. Embedders that own the whole
	// invocation (desktop) set it; the CLI leaves it off so a project's
	// [tool.uv] settings keep working.
	NoConfig bool

	// HermeticEnv strips the inherited UV_* variables that steer resolution,
	// for an embedder that passes python, index, constraints and requirements
	// explicitly — where an ambient variable naming any of them can only
	// contradict what was asked for.
	//
	// NoConfig does not cover this. --no-config applies to configuration
	// FILES — a project's [tool.uv] table, a parent uv.toml — and not to the
	// environment, which still applies in full, so the exact hazard NoConfig
	// exists to stop arrives by a second route: UV_EXCLUDE_NEWER filters out
	// freshly published builds and resolution fails as "unsatisfiable".
	// UV_OFFLINE, UV_NO_INDEX, UV_INDEX, UV_FIND_LINKS, UV_CONSTRAINT,
	// UV_PYTHON and UV_NO_BINARY are load-bearing the same way. An embedder
	// wanting a managed install to be reproducible generally wants both.
	//
	// Deny by default against a small allowlist (operationalEnv) rather than a
	// denylist, so a resolution knob uv adds later is excluded until someone
	// decides otherwise, instead of quietly becoming a new way for a managed
	// install to fail. Nothing outside the UV_ prefix is touched, which is
	// what keeps HTTPS_PROXY, NO_PROXY, SSL_CERT_FILE and SSL_CERT_DIR
	// working.
	HermeticEnv bool

	// OnCertFallback, when set, is called after an invocation that only
	// succeeded once it was retried against the platform certificate store
	// (see command). The operation worked, so this is diagnostics rather than
	// an error path — and diagnostics worth having, because it is the only way
	// to tell a machine whose trust anchor is missing from uv's bundle from a
	// healthy one. A callback rather than a log line: this package does not
	// print.
	OnCertFallback func()
}

// Client is a resolved, version-checked uv binary plus the options every
// invocation shares. Construct with New.
type Client struct {
	bin     string
	version string
	opts    Options
	// waitDelay bounds how long Wait blocks on the output pipes after uv has
	// exited (see command). A field rather than a bare constant because the
	// behavior it guards only happens when a descendant outlives uv, and a
	// test that has to wait out the real delay to see it would be slow enough
	// that nobody runs it.
	waitDelay time.Duration
}

// New discovers uv, checks it against MinVersion, and returns a Client.
// Discovery failures satisfy errors.Is(err, ErrNotFound); an old binary
// surfaces as *VersionError.
func New(ctx context.Context, opts Options) (*Client, error) {
	if opts.CacheDir == "" {
		return nil, errors.New("uv: Options.CacheDir is required")
	}
	bin, err := discover(os.Getenv, opts.BinDir, defaultExtraDirs())
	if err != nil {
		return nil, err
	}
	version, err := queryVersion(ctx, bin)
	if err != nil {
		return nil, err
	}
	if compareVersions(version, MinVersion) < 0 {
		return nil, &VersionError{Bin: bin, Version: version, Min: MinVersion}
	}
	return &Client{bin: bin, version: version, opts: opts, waitDelay: defaultWaitDelay}, nil
}

// Bin returns the resolved uv binary path, for diagnostics.
func (c *Client) Bin() string { return c.bin }

// Version returns the discovered uv's version, e.g. "0.11.23".
func (c *Client) Version() string { return c.version }

// discover resolves the uv binary through the three tiers. getenv and
// extraDirs are parameters so tests can drive every tier.
func discover(getenv func(string) string, binDir string, extraDirs []string) (string, error) {
	if override := getenv(EnvBin); override != "" {
		if isExecutableFile(override) {
			return override, nil
		}
		return "", fmt.Errorf("%s is set to %q, which is not an executable file", EnvBin, override)
	}
	if binDir != "" {
		if p := filepath.Join(binDir, exeName()); isExecutableFile(p) {
			return p, nil
		}
	}
	dirs := append(filepath.SplitList(getenv("PATH")), extraDirs...)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if p := filepath.Join(dir, exeName()); isExecutableFile(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w on PATH or in the common install locations; install it (https://docs.astral.sh/uv/getting-started/installation/) or set %s", ErrNotFound, EnvBin)
}

// defaultExtraDirs lists where the common uv installers put the binary —
// Homebrew (arm and intel) and the standalone installer (~/.local/bin).
// Searched after PATH, so a GUI-launched embedder whose stripped launchd
// PATH lost them still finds a user-installed uv.
func defaultExtraDirs() []string {
	if runtime.GOOS == "windows" {
		// uv's Windows installers put it on the normal user PATH.
		return nil
	}
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return dirs
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "uv.exe"
	}
	return "uv"
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// queryVersion runs `uv --version` and returns the bare version number.
//
// WaitDelay for the same reason the runner sets it: Output() assigns buffers
// rather than files to both streams, so os/exec makes pipes and Wait blocks on
// every writer to them closing. New is on the path a consumer takes to start
// Airflow, and the context it is given carries no deadline of its own, so a uv
// that stalls here — a wrapper shim, a network-mounted install, an antivirus
// scan — would hang the start before any state was reported. The version query
// is one line of output, so nothing legitimate needs this long.
func queryVersion(ctx context.Context, bin string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.WaitDelay = defaultWaitDelay
	out, err := cmd.Output()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return "", fmt.Errorf("running %s --version: %w", bin, err)
	}
	// Output looks like "uv 0.11.23 (3cdf50e09 2026-06-19 aarch64-apple-darwin)".
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "uv" {
		return "", fmt.Errorf("unexpected output from %s --version: %q", bin, strings.TrimSpace(string(out)))
	}
	return fields[1], nil
}

// parseVersion parses "major.minor.patch" leniently: each segment counts
// only its leading digits, so a pre-release like "0.9.0rc1" still orders.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	segments := strings.SplitN(v, ".", 4)
	if len(segments) < 2 {
		return out, false
	}
	for i, seg := range segments {
		if i > 2 {
			break
		}
		digits := seg
		for j, r := range seg {
			if r < '0' || r > '9' {
				digits = seg[:j]
				break
			}
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return out, i > 0
		}
		out[i] = n
	}
	return out, true
}

// compareVersions orders two version strings; an unparseable version sorts
// lowest so it fails the floor check instead of sneaking past it.
func compareVersions(a, b string) int {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	if !aok || !bok {
		switch {
		case aok:
			return 1
		case bok:
			return -1
		default:
			return 0
		}
	}
	for i := range av {
		if av[i] != bv[i] {
			if av[i] < bv[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
