package uv

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestClient builds a Client over a fake uv whose non---version behavior
// is body.
func newTestClient(t *testing.T, opts Options, body string) *Client {
	t.Helper()
	skipOnWindows(t)
	bin := writeFakeUv(t, t.TempDir(), "9.9.9", body)
	t.Setenv(EnvBin, bin)
	if opts.CacheDir == "" {
		opts.CacheDir = t.TempDir()
	}
	c, err := New(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func fixturePath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "no-solution.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func readCount(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestLockFailureTeesAndCarriesStderr(t *testing.T) {
	c := newTestClient(t, Options{}, "echo \"resolving things\"\necho \"error: something broke\" >&2\nexit 3")
	var out, errBuf bytes.Buffer

	err := c.Lock(t.Context(), t.TempDir(), Stdio{Out: &out, Err: &errBuf})

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Lock() error = %v, want *CommandError", err)
	}
	if cmdErr.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", cmdErr.ExitCode)
	}
	// The error must carry stderr even though a live writer also got it.
	if !strings.Contains(cmdErr.Stderr, "error: something broke") {
		t.Errorf("CommandError.Stderr = %q, want the stderr line", cmdErr.Stderr)
	}
	if !strings.Contains(errBuf.String(), "error: something broke") {
		t.Errorf("live stderr = %q, want the stderr line", errBuf.String())
	}
	if !strings.Contains(out.String(), "resolving things") {
		t.Errorf("live stdout = %q, want the stdout line", out.String())
	}
	want := "uv lock failed (exit 3): error: something broke"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestLockResolutionErrorFromRealFixture(t *testing.T) {
	c := newTestClient(t, Options{}, "cat \""+fixturePath(t)+"\" >&2\nexit 1")

	err := c.Lock(t.Context(), t.TempDir(), Stdio{})

	var resErr *ResolutionError
	if !errors.As(err, &resErr) {
		t.Fatalf("Lock() error = %v, want *ResolutionError", err)
	}
	if resErr.Op != "lock" {
		t.Errorf("Op = %q, want lock", resErr.Op)
	}
	wantPackages := []string{"apache-airflow", "flask"}
	if len(resErr.Packages) != len(wantPackages) {
		t.Fatalf("Packages = %v, want %v", resErr.Packages, wantPackages)
	}
	for i, p := range wantPackages {
		if resErr.Packages[i] != p {
			t.Errorf("Packages = %v, want %v", resErr.Packages, wantPackages)
		}
	}
	for _, want := range []string{"apache-airflow==2.10.4", "flask>=2.2.1,<2.3", "flask>=3.1"} {
		found := false
		for _, got := range resErr.Constraints {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Constraints = %v, missing %q", resErr.Constraints, want)
		}
	}
	if !strings.Contains(resErr.Summary, "unsatisfiable") {
		t.Errorf("Summary = %q, want the solver conclusion", resErr.Summary)
	}
	if strings.Contains(resErr.Summary, "╰─▶") || strings.Contains(resErr.Summary, "\n") {
		t.Errorf("Summary = %q, want one line without box drawing", resErr.Summary)
	}
	if !strings.Contains(resErr.Stderr, noSolutionMarker) {
		t.Errorf("Stderr = %q, want the raw uv output", resErr.Stderr)
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Error("ResolutionError does not unwrap to *CommandError")
	}
}

func TestNoConfigAndArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{NoConfig: true}, "echo \"$@\" > \""+argsFile+"\"")

	if err := c.Sync(t.Context(), t.TempDir(), "3.12", Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCount(t, argsFile), "--color never --no-config sync --python 3.12"; got != want {
		t.Errorf("uv args = %q, want %q", got, want)
	}
}

func TestRunPassesArgv(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"")

	if err := c.Run(t.Context(), t.TempDir(), []string{"airflow", "version"}, Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCount(t, argsFile), "--color never run -- airflow version"; got != want {
		t.Errorf("uv args = %q, want %q", got, want)
	}
}

func TestVenvArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"")

	if err := c.Venv(t.Context(), t.TempDir(), "3.12", Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCount(t, argsFile), "--color never venv --allow-existing --python 3.12"; got != want {
		t.Errorf("uv args = %q, want %q", got, want)
	}
}

func TestChildEnvPinsCacheAndDropsVirtualEnv(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "env")
	cacheDir := t.TempDir()
	t.Setenv("VIRTUAL_ENV", "/somewhere/else")
	c := newTestClient(t, Options{CacheDir: cacheDir},
		"echo \"cache=$UV_CACHE_DIR venv=${VIRTUAL_ENV:-unset}\" > \""+envFile+"\"")

	if err := c.Lock(t.Context(), t.TempDir(), Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCount(t, envFile), "cache="+cacheDir+" venv=unset"; got != want {
		t.Errorf("child env = %q, want %q", got, want)
	}
}

func TestEnsureSyncedWipesUnmarkedVenv(t *testing.T) {
	project := t.TempDir()
	sentinel := filepath.Join(project, ".venv", "leftover")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, Options{}, "mkdir -p .venv")

	if err := c.EnsureSynced(t.Context(), project, "", Stdio{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Error("venv without marker survived EnsureSynced; want it wiped")
	}
	if _, err := os.Stat(filepath.Join(project, ".venv", markerName)); err != nil {
		t.Errorf("marker not written after successful sync: %v", err)
	}
}

func TestEnsureSyncedKeepsMarkedVenvAndDropsMarkerDuringSync(t *testing.T) {
	project := t.TempDir()
	venv := filepath.Join(project, ".venv")
	if err := os.MkdirAll(venv, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{markerName, "keep-me"} {
		if err := os.WriteFile(filepath.Join(venv, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The fake sync fails if the completion marker is visible while it
	// runs: an interruption then must leave the venv marked incomplete.
	c := newTestClient(t, Options{},
		"if [ -f .venv/"+markerName+" ]; then echo \"marker present during sync\" >&2; exit 1; fi\nexit 0")

	if err := c.EnsureSynced(t.Context(), project, "", Stdio{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(venv, "keep-me")); err != nil {
		t.Error("marked venv was wiped; want it kept")
	}
	if _, err := os.Stat(filepath.Join(venv, markerName)); err != nil {
		t.Errorf("marker not restored after sync: %v", err)
	}
}

func TestEnsureSyncedWipesAndRetriesOnce(t *testing.T) {
	project := t.TempDir()
	countFile := filepath.Join(t.TempDir(), "count")
	// First sync poisons the venv and fails; the retry must see it wiped.
	body := `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
if [ "$c" -eq 0 ]; then
  mkdir -p .venv
  touch .venv/junk
  echo "Failed to read metadata" >&2
  exit 1
fi
mkdir -p .venv
exit 0`
	c := newTestClient(t, Options{}, body)

	if err := c.EnsureSynced(t.Context(), project, "", Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got := readCount(t, countFile); got != "2" {
		t.Errorf("sync ran %s times, want 2 (fail, wipe, retry)", got)
	}
	if _, err := os.Stat(filepath.Join(project, ".venv", "junk")); !errors.Is(err, os.ErrNotExist) {
		t.Error("poisoned venv contents survived the retry wipe")
	}
	if _, err := os.Stat(filepath.Join(project, ".venv", markerName)); err != nil {
		t.Errorf("marker not written after retry: %v", err)
	}
}

func TestEnsureSyncedDoesNotRetryResolutionErrors(t *testing.T) {
	project := t.TempDir()
	countFile := filepath.Join(t.TempDir(), "count")
	body := `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
cat "` + fixturePath(t) + `" >&2
exit 1`
	c := newTestClient(t, Options{}, body)

	err := c.EnsureSynced(t.Context(), project, "", Stdio{})

	var resErr *ResolutionError
	if !errors.As(err, &resErr) {
		t.Fatalf("EnsureSynced() error = %v, want *ResolutionError", err)
	}
	if got := readCount(t, countFile); got != "1" {
		t.Errorf("sync ran %s times, want 1: the solver is deterministic, wiping cannot help", got)
	}
}

func TestTailBufferKeepsTail(t *testing.T) {
	b := &tailBuffer{max: 8}
	for _, chunk := range []string{"0123", "4567", "89ab"} {
		if _, err := b.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.String(); got != "456789ab" {
		t.Errorf("tailBuffer = %q, want the last 8 bytes %q", got, "456789ab")
	}
}

func TestVenvAtPassesDirAndPython(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"\nexit 0")

	dir := filepath.Join(t.TempDir(), "scratch")
	if err := c.VenvAt(t.Context(), dir, "3.12", Stdio{}); err != nil {
		t.Fatalf("VenvAt() error = %v", err)
	}
	got := readCount(t, argsFile)
	for _, want := range []string{"venv", dir, "--allow-existing", "--python 3.12"} {
		if !strings.Contains(got, want) {
			t.Errorf("VenvAt args = %q, missing %q", got, want)
		}
	}
}

func TestPipInstallPassesInterpreterConstraintAndReqs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"\nexit 0")

	err := c.PipInstall(t.Context(), "/scratch/bin/python", []string{"apache-airflow==3.0.6", "pandas"}, "https://c/constraints.txt", Stdio{})
	if err != nil {
		t.Fatalf("PipInstall() error = %v", err)
	}
	got := readCount(t, argsFile)
	for _, want := range []string{"pip install", "--python /scratch/bin/python", "--constraint https://c/constraints.txt", "apache-airflow==3.0.6", "pandas"} {
		if !strings.Contains(got, want) {
			t.Errorf("PipInstall args = %q, missing %q", got, want)
		}
	}
}

func TestPipInstallResolutionError(t *testing.T) {
	c := newTestClient(t, Options{}, "cat \""+fixturePath(t)+"\" >&2\nexit 1")
	err := c.PipInstall(t.Context(), "/p", []string{"pandas"}, "", Stdio{})
	var re *ResolutionError
	if !errors.As(err, &re) {
		t.Fatalf("PipInstall() error = %v, want *ResolutionError", err)
	}
}

func TestPipCompileReadsStdinAndTargetsPython(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	stdinFile := filepath.Join(t.TempDir(), "stdin")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"\ncat > \""+stdinFile+"\"\nexit 0")

	err := c.PipCompile(t.Context(), "/tmp/constraints.txt", "3.12", Stdio{In: strings.NewReader("pandas\nnumpy\n")})
	if err != nil {
		t.Fatalf("PipCompile() error = %v", err)
	}
	got := readCount(t, argsFile)
	for _, want := range []string{"pip compile", "-", "--constraint /tmp/constraints.txt", "--python-version 3.12"} {
		if !strings.Contains(got, want) {
			t.Errorf("PipCompile args = %q, missing %q", got, want)
		}
	}
	if in := readCount(t, stdinFile); !strings.Contains(in, "pandas") || !strings.Contains(in, "numpy") {
		t.Errorf("PipCompile stdin = %q, want the requirements", in)
	}
}

func TestPipCompileConflictIsResolutionError(t *testing.T) {
	c := newTestClient(t, Options{}, "cat \""+fixturePath(t)+"\" >&2\nexit 1")
	err := c.PipCompile(t.Context(), "", "", Stdio{In: strings.NewReader("pandas\n")})
	var re *ResolutionError
	if !errors.As(err, &re) {
		t.Fatalf("PipCompile() error = %v, want *ResolutionError", err)
	}
}

func TestColorIsSuppressedByFlagAndNotByTheChildEnvironment(t *testing.T) {
	// Color is suppressed through uv's own flag, so the child's environment is
	// left as the caller had it — Run execs the caller's program through
	// `uv run --`, and that program's color is its own business.
	t.Setenv("NO_COLOR", "0")
	seen := filepath.Join(t.TempDir(), "seen")
	c := newTestClient(t, Options{}, "echo \"nocolor=${NO_COLOR:-unset} args=$*\" > \""+seen+"\"\nexit 0")

	if err := c.Lock(t.Context(), t.TempDir(), Stdio{}); err != nil {
		t.Fatal(err)
	}

	if got, want := readCount(t, seen), "nocolor=0 args=--color never lock"; got != want {
		t.Errorf("child saw %q, want %q", got, want)
	}
}

func TestVerbNamesTheSubcommandPastTheGlobalFlags(t *testing.T) {
	// --color takes a separate value, so a verb() that only skips arguments
	// starting with "-" would report the operation as "never" and every error
	// message would read "uv never failed".
	c := newTestClient(t, Options{NoConfig: true}, "echo \"error: boom\" >&2\nexit 2")

	err := c.Sync(t.Context(), t.TempDir(), "", Stdio{})

	if err == nil {
		t.Fatal("Sync() error = nil, want the failure")
	}
	if want := "uv sync failed (exit 2): error: boom"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestCommandReturnsWhenADescendantOutlivesUv(t *testing.T) {
	// uv exits 0 but leaves a child holding the stderr pipe — a build backend
	// that outlives the install, in the real case. Wait blocks on every writer
	// to that pipe rather than on uv, so without a delay this call does not
	// return until the straggler does, and the caller sees neither a result
	// nor an error in the meantime.
	c := newTestClient(t, Options{}, "sleep 20 &\nexit 0")
	c.waitDelay = 200 * time.Millisecond

	start := time.Now()
	err := c.Lock(t.Context(), t.TempDir(), Stdio{})
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Lock() error = %v, want nil — uv exited 0, so the straggler is not its failure", err)
	}
	// Generous against a loaded CI box, and still nowhere near the 20s the
	// child holds the pipe for.
	if elapsed > 5*time.Second {
		t.Errorf("Lock() took %s, want it bounded by the wait delay", elapsed)
	}
}

// failingWriter errors on every write, standing in for a caller's live stream
// whose destination has gone away: a closed descriptor, a full disk, a log
// file on an unmounted volume.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stream gone") }

func TestABrokenStderrViewDoesNotFailASuccessfulRun(t *testing.T) {
	// Stdio.Err is a place to watch uv scroll by, teed off a capture that
	// already has the bytes. Failing the operation over it would report a
	// successful lock as "uv lock failed (exit -1)" on the strength of one
	// cosmetic warning.
	c := newTestClient(t, Options{}, "echo \"resolved 12 packages\"\necho \"warning: something cosmetic\" >&2\nexit 0")

	err := c.Lock(t.Context(), t.TempDir(), Stdio{Err: failingWriter{}})
	if err != nil {
		t.Errorf("Lock() error = %v, want nil — only the caller's stderr view broke", err)
	}
}

func TestABrokenStdoutSinkFailsTheRun(t *testing.T) {
	// Stdout is the opposite case and must not be swallowed: it carries pip
	// compile's resolved requirement set and Run's program output, and nothing
	// captures it internally. A caller whose file went away needs the error
	// rather than a truncated result reported as success.
	c := newTestClient(t, Options{}, "echo \"annotated-types==0.7.0\"\nexit 0")

	err := c.PipCompile(t.Context(), "", "", Stdio{In: strings.NewReader("annotated-types\n"), Out: failingWriter{}})
	if err == nil {
		t.Error("PipCompile() error = nil, want the lost output reported")
	}
}

func TestABrokenLiveStreamStillLeavesTheCapturedStderr(t *testing.T) {
	// Two writes with a gap between them, so they arrive as separate reads.
	// The capture and the live stream share one io.MultiWriter, which returns
	// at the first writer that fails — unguarded, that aborts the copy and the
	// second line, the one carrying uv's actual diagnosis, never lands.
	c := newTestClient(t, Options{}, "echo \"resolving dependencies\" >&2\nsleep 0.3\necho \"error: something broke\" >&2\nexit 3")

	err := c.Lock(t.Context(), t.TempDir(), Stdio{Err: failingWriter{}})

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Lock() error = %v, want *CommandError", err)
	}
	if !strings.Contains(cmdErr.Stderr, "something broke") {
		t.Errorf("Stderr = %q, want uv's own diagnosis despite the broken live stream", cmdErr.Stderr)
	}
}

// envProbe builds a client whose fake uv records the three variables the
// hermeticity rule has to treat differently: one that steers resolution, one
// on the operational allowlist, and one outside the UV_ prefix entirely.
func envProbe(t *testing.T, opts Options) (client *Client, seenFile string) {
	t.Helper()
	t.Setenv("UV_EXCLUDE_NEWER", "2020-01-01")
	t.Setenv("UV_HTTP_TIMEOUT", "300")
	t.Setenv("HTTPS_PROXY", "http://proxy.internal:3128")
	seen := filepath.Join(t.TempDir(), "env")
	body := "echo \"newer=${UV_EXCLUDE_NEWER:-unset} timeout=${UV_HTTP_TIMEOUT:-unset} proxy=${HTTPS_PROXY:-unset}\" > \"" + seen + "\""
	return newTestClient(t, opts, body), seen
}

func TestHermeticEnvStripsOnlyTheResolutionSteeringVariables(t *testing.T) {
	// UV_EXCLUDE_NEWER is the concrete hazard: it filters out freshly
	// published builds, so an inherited one makes a managed install fail as
	// "unsatisfiable" for reasons nothing in the project says. --no-config
	// does not reach it, because it is not configuration in a file.
	c, seen := envProbe(t, Options{HermeticEnv: true})

	if err := c.Lock(t.Context(), t.TempDir(), Stdio{}); err != nil {
		t.Fatal(err)
	}

	want := "newer=unset timeout=300 proxy=http://proxy.internal:3128"
	if got := readCount(t, seen); got != want {
		t.Errorf("child env = %q, want %q", got, want)
	}
}

func TestWithoutHermeticEnvTheEnvironmentIsUntouched(t *testing.T) {
	// The CLI leaves this off for the same reason it leaves NoConfig off: a
	// user's uv settings are theirs. Opting in is what changes behavior.
	c, seen := envProbe(t, Options{})

	if err := c.Lock(t.Context(), t.TempDir(), Stdio{}); err != nil {
		t.Fatal(err)
	}

	want := "newer=2020-01-01 timeout=300 proxy=http://proxy.internal:3128"
	if got := readCount(t, seen); got != want {
		t.Errorf("child env = %q, want %q", got, want)
	}
}

// withoutAmbientCertChoice clears both spellings of the certificate-store
// preference for the duration of a test. Without it these tests read the
// developer's own machine: on the corporate-proxy box this feature exists for,
// an exported UV_SYSTEM_CERTS=1 makes the fake uv succeed on the first attempt
// and the retry assertions fail. t.Setenv cannot do it — an empty value is
// still a value, and systemCertsChosen is about presence.
func withoutAmbientCertChoice(t *testing.T) {
	t.Helper()
	for _, key := range []string{systemCertsEnv, nativeTLSEnv} {
		if old, ok := os.LookupEnv(key); ok {
			t.Cleanup(func() { _ = os.Setenv(key, old) })
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// certFakeUv refuses a certificate chain in uv's own wording unless the
// platform store has been selected, and counts its attempts so a test can tell
// "not retried" from "retried and still failed".
func certFakeUv(countFile string) string {
	return `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
if [ -n "${UV_SYSTEM_CERTS:-}" ]; then exit 0; fi
echo "error: Failed to fetch: https://pypi.org/simple/apache-airflow/" >&2
echo "  Caused by: invalid peer certificate: UnknownIssuer" >&2
exit 2`
}

func TestATrustFailureIsRetriedAgainstThePlatformStore(t *testing.T) {
	withoutAmbientCertChoice(t)
	countFile := filepath.Join(t.TempDir(), "count")
	fallbacks := 0
	c := newTestClient(t, Options{OnCertFallback: func() { fallbacks++ }}, certFakeUv(countFile))

	if err := c.Sync(t.Context(), t.TempDir(), "", Stdio{}); err != nil {
		t.Fatalf("Sync() error = %v, want nil after the platform-store retry", err)
	}

	if got := readCount(t, countFile); got != "2" {
		t.Errorf("uv invocations = %s, want 2 (the failure and the retry)", got)
	}
	if fallbacks != 1 {
		t.Errorf("OnCertFallback called %d times, want 1", fallbacks)
	}
}

func TestADeliberateCertificateChoiceIsNotOverridden(t *testing.T) {
	// A user who turned the platform store off has decided; the retry must not
	// decide again for them. UV_NATIVE_TLS is the older spelling, and honoring
	// only the new one would silently override exactly the people who set this
	// years ago.
	t.Setenv(nativeTLSEnv, "0")
	countFile := filepath.Join(t.TempDir(), "count")
	fallbacks := 0
	c := newTestClient(t, Options{OnCertFallback: func() { fallbacks++ }}, certFakeUv(countFile))

	err := c.Sync(t.Context(), t.TempDir(), "", Stdio{})

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Sync() error = %v, want *CommandError", err)
	}
	if got := readCount(t, countFile); got != "1" {
		t.Errorf("uv invocations = %s, want 1 — the choice was already stated", got)
	}
	if fallbacks != 0 {
		t.Errorf("OnCertFallback called %d times, want 0", fallbacks)
	}
}

func TestAFailureThatIsNotAboutTrustIsNotRetried(t *testing.T) {
	countFile := filepath.Join(t.TempDir(), "count")
	body := `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
cat "` + fixturePath(t) + `" >&2
exit 1`
	c := newTestClient(t, Options{}, body)

	var resErr *ResolutionError
	if err := c.Sync(t.Context(), t.TempDir(), "", Stdio{}); !errors.As(err, &resErr) {
		t.Fatalf("Sync() error = %v, want *ResolutionError", err)
	}
	if got := readCount(t, countFile); got != "1" {
		t.Errorf("uv invocations = %s, want 1 — a different trust store cannot solve a conflict", got)
	}
}

func TestTheRetryReplaysTheCallersStdin(t *testing.T) {
	withoutAmbientCertChoice(t)
	// pip compile is a pure network resolve and so the operation most likely to
	// meet the MDM-CA failure, and it feeds its requirements on stdin. Each
	// attempt gets its own reader over the buffered bytes; one shared reader
	// would reach the retry drained and resolve an empty requirement set into a
	// cheerful success.
	stdinFile := filepath.Join(t.TempDir(), "stdin")
	countFile := filepath.Join(t.TempDir(), "count")
	body := `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
cat >> "` + stdinFile + `"
if [ -n "${UV_SYSTEM_CERTS:-}" ]; then exit 0; fi
echo "error: Failed to fetch: https://pypi.org/simple/pandas/" >&2
echo "  Caused by: invalid peer certificate: UnknownIssuer" >&2
exit 2`
	c := newTestClient(t, Options{}, body)

	if err := c.PipCompile(t.Context(), "", "", Stdio{In: strings.NewReader("pandas\n")}); err != nil {
		t.Fatalf("PipCompile() error = %v, want nil after the platform-store retry", err)
	}

	if got := readCount(t, countFile); got != "2" {
		t.Errorf("uv invocations = %s, want 2", got)
	}
	// Both attempts must have been fed, not just the first.
	if got, want := readCount(t, stdinFile), "pandas\npandas"; got != want {
		t.Errorf("stdin across attempts = %q, want %q", got, want)
	}
}

func TestRunNeverRetriesTheCallersProgram(t *testing.T) {
	withoutAmbientCertChoice(t)
	// Run's argv is the caller's own program, so a second execution is a side
	// effect rather than a retry — `uv run -- airflow db migrate` would migrate
	// twice. The classifier reads the child's stderr, which here is that
	// program's output, and an application logging its own TLS trouble must not
	// be mistaken for uv refusing an index.
	countFile := filepath.Join(t.TempDir(), "count")
	body := `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
echo "error: could not reach the warehouse: tls handshake eof" >&2
exit 1`
	c := newTestClient(t, Options{}, body)

	if err := c.Run(t.Context(), t.TempDir(), []string{"airflow", "db", "migrate"}, Stdio{}); err == nil {
		t.Fatal("Run() error = nil, want the failure reported")
	}

	if got := readCount(t, countFile); got != "1" {
		t.Errorf("program executions = %s, want 1 — a retry here runs the caller's command again", got)
	}
}

func TestAFailedRetryKeepsTheCertificateDiagnosis(t *testing.T) {
	// The platform store is refused too, and the second attempt says something
	// unrelated. Reporting only that loses the word "certificate" and the user
	// is told the index is down — the misdiagnosis the retry exists to prevent,
	// and what anything bucketing on the text would file it under.
	withoutAmbientCertChoice(t)
	body := `if [ -n "${UV_SYSTEM_CERTS:-}" ]; then
  echo "error: Failed to fetch: connection closed before message completed" >&2
  exit 2
fi
echo "error: Failed to fetch: https://pypi.org/simple/apache-airflow/" >&2
echo "  Caused by: invalid peer certificate: UnknownIssuer" >&2
exit 2`
	c := newTestClient(t, Options{}, body)

	err := c.Sync(t.Context(), t.TempDir(), "", Stdio{})

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("Sync() error = %v, want *CommandError", err)
	}
	if !strings.Contains(cmdErr.Stderr, "invalid peer certificate") {
		t.Errorf("Stderr = %q, want the certificate diagnosis from the first attempt", cmdErr.Stderr)
	}
}

func TestEnsureSyncedDoesNotWipeAndReResolveAfterATrustFailure(t *testing.T) {
	// The cert retry lives inside Sync, so without a guard the two retries
	// compose: sync, cert retry, wipe, sync, cert retry. Four full resolves and
	// downloads to reach a refusal a wipe cannot affect, on exactly the link
	// least able to afford them.
	withoutAmbientCertChoice(t)
	countFile := filepath.Join(t.TempDir(), "count")
	// Refused whichever trust store is selected — the machine whose OS store
	// lacks the root too. A fake that succeeded on the retry would let Sync
	// return nil and never reach the branch under test.
	body := `c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
mkdir -p .venv
echo "error: Failed to fetch: https://pypi.org/simple/apache-airflow/" >&2
echo "  Caused by: invalid peer certificate: UnknownIssuer" >&2
exit 2`
	c := newTestClient(t, Options{}, body)

	err := c.EnsureSynced(t.Context(), t.TempDir(), "", Stdio{})

	if err == nil {
		t.Fatal("EnsureSynced() error = nil, want the trust failure")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("EnsureSynced() error = %v, want the certificate diagnosis", err)
	}
	if got := readCount(t, countFile); got != "2" {
		t.Errorf("uv invocations = %s, want 2 (the sync and its one cert retry)", got)
	}
}

func TestHermeticEnvKeepsTheVariablesConstrainedNetworksNeed(t *testing.T) {
	// Deny-by-default makes a missing allowlist entry invisible until someone
	// on a restricted network hits it, so the awkward ones are pinned here:
	// the self-signed-index escape hatch, an internal interpreter mirror, and
	// the older spelling of a timeout whose current spelling is already kept.
	t.Setenv("UV_INSECURE_HOST", "index.corp.internal")
	t.Setenv("UV_PYTHON_INSTALL_MIRROR", "https://mirror.corp.internal/python")
	t.Setenv("UV_REQUEST_TIMEOUT", "300")
	t.Setenv("UV_EXCLUDE_NEWER", "2020-01-01")
	seen := filepath.Join(t.TempDir(), "env")
	body := "echo \"host=${UV_INSECURE_HOST:-unset} mirror=${UV_PYTHON_INSTALL_MIRROR:-unset} timeout=${UV_REQUEST_TIMEOUT:-unset} newer=${UV_EXCLUDE_NEWER:-unset}\" > \"" + seen + "\""
	c := newTestClient(t, Options{HermeticEnv: true}, body)

	if err := c.Lock(t.Context(), t.TempDir(), Stdio{}); err != nil {
		t.Fatal(err)
	}

	want := "host=index.corp.internal mirror=https://mirror.corp.internal/python timeout=300 newer=unset"
	if got := readCount(t, seen); got != want {
		t.Errorf("child env = %q, want %q", got, want)
	}
}
