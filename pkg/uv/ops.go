package uv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Stdio carries optional live output destinations for a uv invocation.
// Output is always also captured internally (the runner tees, never
// stream-only), so a failure carries uv's stderr on the error value even
// when the caller watched it scroll by.
type Stdio struct {
	// In feeds the process's stdin; nil means no input. Only Run reads it.
	In io.Reader
	// Out and Err receive the process's stdout and stderr as they happen;
	// nil discards the live stream (stderr capture still happens).
	Out, Err io.Writer
}

// Venv creates <project>/.venv without installing anything, tolerating an
// existing one. python selects the interpreter version; "" lets uv pick one
// satisfying the project's requires-python.
func (c *Client) Venv(ctx context.Context, project, python string, stdio Stdio) error {
	args := []string{"venv", "--allow-existing"}
	if python != "" {
		args = append(args, "--python", python)
	}
	return c.fetch(ctx, project, stdio, args...)
}

// Lock resolves the project's dependencies and writes <project>/uv.lock.
// A solver failure surfaces as *ResolutionError.
func (c *Client) Lock(ctx context.Context, project string, stdio Stdio) error {
	return asResolution("lock", c.fetch(ctx, project, stdio, "lock"))
}

// VenvAt creates a standalone venv at dir — not the <project>/.venv Venv
// makes — for a scratch environment that lives outside any project (a
// pre-flight check against a platform's Airflow version). python selects the
// interpreter version, e.g. "3.12"; uv provisions a managed CPython when the
// host has none. "" lets uv choose.
func (c *Client) VenvAt(ctx context.Context, dir, python string, stdio Stdio) error {
	args := []string{"venv", dir, "--allow-existing"}
	if python != "" {
		args = append(args, "--python", python)
	}
	return c.fetch(ctx, "", stdio, args...)
}

// PipInstall installs reqs into the venv whose interpreter is pythonBin, using
// `uv pip install`. constraint, when set, is passed as --constraint (a path or
// URL). A solver failure surfaces as *ResolutionError. It backs a scratch
// venv that is not driven from a pyproject.toml, so it takes an explicit
// requirement list rather than a project directory.
func (c *Client) PipInstall(ctx context.Context, pythonBin string, reqs []string, constraint string, stdio Stdio) error {
	args := []string{"pip", "install", "--python", pythonBin}
	if constraint != "" {
		args = append(args, "--constraint", constraint)
	}
	args = append(args, reqs...)
	return asResolution("pip install", c.fetch(ctx, "", stdio, args...))
}

// PipCompile resolves reqs (read from stdio.In as a requirements list on
// stdin) against an optional constraint file, without installing anything —
// the resolver runs, the output is discarded. constraint may be a path or a
// URL; pythonVersion, when set, targets the resolve at that interpreter's
// markers (e.g. "3.12") so a platform's Python-specific constraints resolve
// faithfully. A conflict surfaces as *ResolutionError. It is how a caller asks
// "does this dependency set solve under these constraints?" without an install.
func (c *Client) PipCompile(ctx context.Context, constraint, pythonVersion string, stdio Stdio) error {
	args := []string{"pip", "compile", "-", "--no-header", "--no-annotate"}
	if constraint != "" {
		args = append(args, "--constraint", constraint)
	}
	if pythonVersion != "" {
		args = append(args, "--python-version", pythonVersion)
	}
	return asResolution("pip compile", c.fetch(ctx, "", stdio, args...))
}

// Sync makes <project>/.venv match the project's lockfile, locking first
// when the lockfile is missing or stale — so it too can fail with
// *ResolutionError. python is as for Venv.
func (c *Client) Sync(ctx context.Context, project, python string, stdio Stdio) error {
	args := []string{"sync"}
	if python != "" {
		args = append(args, "--python", python)
	}
	return asResolution("sync", c.fetch(ctx, project, stdio, args...))
}

// Run executes argv inside the project environment via `uv run`, which
// first brings the environment up to date (and so can also fail with
// *ResolutionError).
func (c *Client) Run(ctx context.Context, project string, argv []string, stdio Stdio) error {
	args := append([]string{"run", "--"}, argv...)
	return asResolution("run", c.command(ctx, project, stdio, args...))
}

// markerName marks a fully synced venv. uv tracks installed packages inside
// the venv (dist-info), so partial state from an interrupted install poisons
// later syncs with metadata errors; the marker's absence is how we tell.
const markerName = ".install-complete"

// markerMode is owner-only: the marker says this venv finished installing, and
// a reader that trusts it skips the sync that would have rebuilt it.
const markerMode = 0o600

// EnsureSynced provisions <project>/.venv from the project's pyproject and
// lockfile, recovering from poisoned venvs: a venv without the completion
// marker is wiped up front, and a failed sync gets one wipe-and-retry before
// the error stands. The marker is dropped while syncing so an interruption
// mid-flight leaves the venv marked incomplete.
func (c *Client) EnsureSynced(ctx context.Context, project, python string, stdio Stdio) error {
	venv := filepath.Join(project, ".venv")
	marker := filepath.Join(venv, markerName)

	if _, err := os.Stat(venv); err == nil {
		if _, err := os.Stat(marker); err != nil {
			if rmErr := os.RemoveAll(venv); rmErr != nil {
				return fmt.Errorf("removing half-installed venv: %w", rmErr)
			}
		}
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing install marker: %w", err)
	}

	if err := c.Sync(ctx, project, python, stdio); err != nil {
		if ctx.Err() != nil {
			// A canceled sync leaves the venv unfinished rather than
			// unreadable, and the retry below would run under the same dead
			// context.
			return err
		}
		// Retried only for the failure a clean tree actually fixes. Everything
		// else — a typo in pyproject.toml, a 401 from a private index, a full
		// disk — survives a wipe unchanged, so deleting the environment only
		// spends a full reinstall on the way to the identical error, and costs
		// the environment to get there.
		if !isPoisonedVenv(stderrOf(err)) {
			return err
		}
		if rmErr := os.RemoveAll(venv); rmErr != nil {
			return errors.Join(err, fmt.Errorf("removing venv for retry: %w", rmErr))
		}
		// After the delete and before the second sync: a delete that failed
		// returns above, so this never announces a retry that does not happen,
		// and the expensive half is still ahead, so a consumer hears about the
		// wait while there is still a wait to explain.
		if c.opts.OnSyncRetry != nil {
			c.opts.OnSyncRetry(err)
		}
		if err := c.Sync(ctx, project, python, stdio); err != nil {
			return err
		}
	}
	if err := os.WriteFile(marker, nil, markerMode); err != nil {
		return fmt.Errorf("writing install marker: %w", err)
	}
	return nil
}

// stderrTailLimit bounds captured stderr so a chatty command cannot grow an
// error value without limit; uv's diagnostics fit comfortably.
const stderrTailLimit = 64 << 10

// defaultWaitDelay bounds how long Wait blocks on the output pipes after uv
// itself has exited. See command for why they can outlive the process.
const defaultWaitDelay = 10 * time.Second

// command runs uv exactly once, with the shared environment, project as the
// working directory, and stderr teed into a bounded capture. A non-zero exit
// comes back as *CommandError carrying that capture.
//
// Run uses this rather than fetch, and the difference is not stylistic: its
// argv is the caller's own program, so executing it a second time is a side
// effect rather than a retry — `uv run -- airflow db migrate` would migrate
// twice. The trust-failure classifier reads the child's stderr, which for Run
// is that program's output, and "tls handshake" or "certificate verify failed"
// are things an application logs about its own connections.
func (c *Client) command(ctx context.Context, project string, stdio Stdio, args ...string) error {
	args = c.globalArgs(args)
	stderr, err := c.run(ctx, project, c.childEnv(), stdio, args)
	return commandError(args, stderr, err)
}

// globalArgs prefixes the flags every invocation carries.
//
// --color never keeps the captured stderr plain text: uv emits ANSI color even
// when its output is not a terminal, so without it CommandError.Stderr — and
// the message built from its last line — carries escape sequences into
// whatever renders them next, which for an embedder is a log line or a toast.
// A flag rather than NO_COLOR in the environment, because Run execs the
// caller's own program through `uv run --` and that program's color is not
// this package's business.
func (c *Client) globalArgs(args []string) []string {
	prefix := []string{"--color", "never"}
	if c.opts.NoConfig {
		prefix = append(prefix, "--no-config")
	}
	return append(prefix, args...)
}

// fetch runs a uv operation whose only effects are the cache, the lockfile and
// the venv, so running it twice reaches the same state — which is what lets it
// retry a rejected certificate chain against the platform trust store.
//
// That retry exists because it is the one uv failure a different environment
// reliably fixes. uv validates against its own bundled Mozilla roots, so a
// machine whose only anchor lives in the OS store — a corporate proxy CA pushed
// by MDM, overwhelmingly the common case — fails while Go's own HTTPS calls to
// the same index succeed, since Go consults the OS store on macOS and Windows
// already. Nothing in the project explains it and it arrives as a TLS error
// rather than a refusal, so it reads as "the index is down".
//
// Selecting the platform store REPLACES the bundled roots rather than adding to
// them, which is why it is a retry and not the default: as a default it would
// move every user onto a path that is only better on the machines that need it,
// and worse on any box whose OS store lacks a root the Mozilla bundle carries.
//
// An environment that already states a certificate preference is left alone,
// including when it deliberately chose the bundled roots.
func (c *Client) fetch(ctx context.Context, project string, stdio Stdio, args ...string) error {
	args = c.globalArgs(args)
	// Buffered so a second attempt gets the same input the first one consumed.
	// Every operation routed here takes at most a requirements list on stdin,
	// which is small; Run, whose stdin can be a terminal or a stream, does not
	// come through here. attempt hands each invocation its own reader over the
	// same bytes — one shared reader would arrive at the retry already drained,
	// which is the whole failure being avoided.
	in, err := bufferStdin(stdio.In)
	if err != nil {
		return err
	}
	attempt := func(env []string) (string, error) {
		perRun := stdio
		if in != nil {
			perRun.In = bytes.NewReader(in)
		}
		return c.run(ctx, project, env, perRun, args)
	}

	env := c.childEnv()
	stderr, runErr := attempt(env)
	if runErr != nil && ctx.Err() == nil && isTLSTrustFailure(stderr) && !systemCertsChosen(env) {
		retryEnv := append(append([]string(nil), env...), systemCertsEnv+"=1")
		retryStderr, retryErr := attempt(retryEnv)
		switch {
		case retryErr == nil:
			if c.opts.OnCertFallback != nil {
				c.opts.OnCertFallback()
			}
			return nil
		case isTLSTrustFailure(retryStderr):
			// The platform store was refused too. Report that attempt, since
			// it describes the environment the caller ended up in.
			stderr, runErr = retryStderr, retryErr
		default:
			// The retry failed for some unrelated reason, or produced no
			// diagnosis at all. Keeping the first attempt's capture is what
			// preserves the word "certificate" in the reported error — without
			// it the report says "the index is down", which is the
			// misdiagnosis this retry exists to prevent, and anything
			// bucketing on that text files it under the wrong cause.
			runErr = errors.Join(runErr, retryErr)
		}
	}
	return commandError(args, stderr, runErr)
}

// stderrOf returns the captured stderr from err, or "" when err carries none.
func stderrOf(err error) string {
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Stderr
	}
	return ""
}

// commandError wraps a failed invocation, or returns nil for a successful one.
func commandError(args []string, stderr string, err error) error {
	if err == nil {
		return nil
	}
	cmdErr := &CommandError{Args: args, ExitCode: -1, Stderr: stderr, Err: err}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		cmdErr.ExitCode = exitErr.ExitCode()
	}
	return cmdErr
}

// bufferStdin reads in into memory so each attempt can be handed its own
// reader over the same bytes. A nil reader stays nil, which is the common case.
func bufferStdin(in io.Reader) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	buf, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("reading uv stdin: %w", err)
	}
	return buf, nil
}

// run executes uv once with the supplied environment, returning the tail of
// its stderr alongside the outcome.
//
// Because stderr is a capture rather than an *os.File, os/exec creates an OS
// pipe and Wait blocks until every writer to it has closed — not just uv. A
// build backend's grandchild still holding fd 2 after uv exits would otherwise
// wedge Wait indefinitely, and with it whatever the caller does with a start:
// no error, no result, no progress. WaitDelay bounds that, and the clock only
// starts once uv has exited, so the cost of hitting it is a stray
// descendant's trailing output rather than any part of the install.
func (c *Client) run(ctx context.Context, project string, env []string, stdio Stdio, args []string) (string, error) {
	//nolint:gosec // G204: running uv with arguments is what this type is for. c.bin is resolved by the package, and every args slice is built by one of its own methods.
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = project
	cmd.Env = env
	cmd.Stdin = stdio.In
	// stdout is not wrapped. It is uv's real output — the resolved requirement
	// set from pip compile, the program's own output from Run — and nothing
	// captures it internally, so a caller whose writer fails needs to hear
	// about it rather than receive a truncated result and a nil error.
	cmd.Stdout = stdio.Out
	stderrTail := &tailBuffer{max: stderrTailLimit}
	cmd.Stderr = stderrTail
	if stdio.Err != nil {
		cmd.Stderr = io.MultiWriter(stderrTail, quietWriter{stdio.Err})
	}
	cmd.WaitDelay = c.waitDelay
	// Run first, read the tail second: a return statement's operands are
	// evaluated left to right, so the tail has to be read in its own statement
	// to hold anything.
	err := normalizeWaitError(cmd.Run())
	return stderrTail.String(), err
}

// isPoisonedVenv reports whether stderr is uv failing to read back something it
// previously installed. That is the one sync failure deleting the venv fixes:
// uv records an install inside the tree, so a half-written one makes later syncs
// fail on state nothing but a clean tree clears.
//
// Deliberately narrow, and narrower than the hazard's reputation. Against uv
// 0.11 a missing RECORD is a warning it recovers from, a corrupt METADATA is
// ignored, a deleted interpreter makes it rebuild the environment, and a
// garbage pyvenv.cfg is tolerated — the case that actually fails is a dist-info
// it cannot read, reported as "Failed to read metadata from: <path>". Matching
// wider than that spends a working environment to reach the same error twice.
func isPoisonedVenv(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "failed to read metadata")
}

// isTLSTrustFailure reports whether stderr is uv refusing a certificate chain,
// as opposed to any other network failure. Matched on uv's own wording, which
// is not localized.
func isTLSTrustFailure(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "invalid peer certificate") ||
		strings.Contains(s, "unknownissuer") ||
		strings.Contains(s, "certificate verify failed") ||
		strings.Contains(s, "self-signed certificate") ||
		strings.Contains(s, "unable to get local issuer certificate") ||
		strings.Contains(s, "tls handshake")
}

// systemCertsChosen reports whether env already states a certificate-store
// preference, in either spelling. A deliberate choice is never overridden by
// the retry, including a deliberate choice of the bundled roots.
func systemCertsChosen(env []string) bool {
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok &&
			(strings.EqualFold(k, systemCertsEnv) || strings.EqualFold(k, nativeTLSEnv)) {
			return true
		}
	}
	return false
}

// quietWriter forwards writes to w and always reports success. It wraps
// Stdio.Err, which is a place to watch uv scroll by rather than a sink: the
// bytes are already in the capture, so a failure to write there is not a
// failure of the operation, and os/exec would otherwise surface the copy error
// from Wait and report a uv run that exited 0 as failed. It also keeps the
// enclosing io.MultiWriter going, which stops at the first writer that fails —
// hence the capture is ordered first.
//
// Stdio.Out is deliberately not wrapped; see run.
type quietWriter struct{ w io.Writer }

func (q quietWriter) Write(p []byte) (int, error) {
	_, _ = q.w.Write(p) //nolint:errcheck // discarding it is the whole point of this type
	return len(p), nil
}

// normalizeWaitError discards exec.ErrWaitDelay. Wait reports it only when the
// process itself exited successfully but left its pipes open, so the operation
// is done and the lingering writer is somebody else's descendant — returning it
// would fail a uv run that worked. A process that actually failed comes back as
// its own error, which takes priority over the delay, so nothing is masked.
func normalizeWaitError(err error) error {
	if errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return err
}

// childEnv is the parent environment with UV_CACHE_DIR pinned to the shared
// cache and VIRTUAL_ENV dropped — an active parent venv must never capture
// the install (v1 fought this leak with --python on every call; removing
// the variable is simpler).
//
// UV_CACHE_DIR is dropped from the inherited set before being appended, so the
// value here wins outright rather than relying on which duplicate the child's
// libc happens to read.
//
// Options.HermeticEnv additionally strips the inherited UV_* variables that
// steer resolution; see that field for why --no-config does not cover them.
func (c *Client) childEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "UV_CACHE_DIR=") || strings.HasPrefix(kv, "VIRTUAL_ENV=") {
			continue
		}
		if c.opts.HermeticEnv && steersResolution(kv) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "UV_CACHE_DIR="+c.opts.CacheDir)
}

// Certificate-store preference, in uv's two spellings. UV_NATIVE_TLS is the
// older alias and is still honored, so the two travel together — reading one
// and dropping the other would silently override a user who spelled their
// choice the old way.
const (
	systemCertsEnv = "UV_SYSTEM_CERTS"
	nativeTLSEnv   = "UV_NATIVE_TLS"
)

// operationalEnv are the UV_* variables Options.HermeticEnv keeps. They govern
// how uv reaches what it was told to fetch — how patiently, how many at a time,
// how the files land, which trust store — without changing WHICH distribution
// it resolves to, or what it executes. Stripping them breaks the
// constrained-network users hermeticity is meant to help, who are the same
// people the certificate retry is for.
//
// UV_REQUEST_TIMEOUT is uv's older spelling of UV_HTTP_TIMEOUT, and
// UV_CONCURRENT_INSTALLS and UV_CONCURRENT_BUILDS are "how many at a time"
// exactly as UV_CONCURRENT_DOWNLOADS is. Keeping one spelling, or one third of
// a group, is the same silent override the two cert-variable spellings are
// handled together to avoid.
//
// Deliberately excluded, though uv documents them beside the knobs above and a
// restricted network can genuinely need them:
//
//   - UV_INSECURE_HOST turns off certificate verification for a host.
//   - UV_PYTHON_INSTALL_MIRROR, UV_PYPY_INSTALL_MIRROR and
//     UV_PYTHON_INSTALL_REGISTRY choose where a managed interpreter is fetched
//     from; UV_PYTHON_INSTALL_DIR and UV_PYTHON_INSTALL_BIN choose where one is
//     found.
//
// Those decide what binary an embedder downloads and runs, which puts them with
// UV_INDEX rather than with a timeout — and the whole point of HermeticEnv is
// that an embedder passing its own inputs is not overruled by an environment it
// does not control. A user who needs an internal mirror configures it in the
// project, where it is visible, rather than in an ambient variable. (An earlier
// revision of this list kept all six; that was wrong, and Astro Desktop's own
// hermetic filter had it right.)
var operationalEnv = []string{
	systemCertsEnv,
	nativeTLSEnv,
	"UV_HTTP_TIMEOUT",
	"UV_REQUEST_TIMEOUT",
	"UV_HTTP_RETRIES",
	"UV_CONCURRENT_DOWNLOADS",
	"UV_CONCURRENT_INSTALLS",
	"UV_CONCURRENT_BUILDS",
	"UV_LINK_MODE",
	"UV_KEYRING_PROVIDER",
	"UV_NO_PROGRESS",
}

// steersResolution reports whether kv is an inherited UV_* variable outside the
// operational allowlist.
func steersResolution(kv string) bool {
	k, _, ok := strings.Cut(kv, "=")
	if !ok || !strings.HasPrefix(strings.ToUpper(k), "UV_") {
		return false
	}
	return !isOperationalEnv(k)
}

// isOperationalEnv reports whether key is one of the pass-through variables.
// Matched case-insensitively: uv reads its environment that way on Windows,
// where os.Environ can yield any spelling.
func isOperationalEnv(key string) bool {
	for _, allowed := range operationalEnv {
		if strings.EqualFold(key, allowed) {
			return true
		}
	}
	return false
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string { return string(b.buf) }
