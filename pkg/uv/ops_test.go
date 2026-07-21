package uv

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if got, want := readCount(t, argsFile), "--no-config sync --python 3.12"; got != want {
		t.Errorf("uv args = %q, want %q", got, want)
	}
}

func TestRunPassesArgv(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"")

	if err := c.Run(t.Context(), t.TempDir(), []string{"airflow", "version"}, Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCount(t, argsFile), "run -- airflow version"; got != want {
		t.Errorf("uv args = %q, want %q", got, want)
	}
}

func TestVenvArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	c := newTestClient(t, Options{}, "echo \"$@\" > \""+argsFile+"\"")

	if err := c.Venv(t.Context(), t.TempDir(), "3.12", Stdio{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCount(t, argsFile), "venv --allow-existing --python 3.12"; got != want {
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
