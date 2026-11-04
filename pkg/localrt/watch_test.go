package localrt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// loopHarness runs a watchLoop over names a test writes, with a manifest the
// test swaps, and records what the loop did.
type loopHarness struct {
	t     *testing.T
	names chan string
	done  chan struct{}

	mu        sync.Mutex
	current   *manifest.Manifest
	loadErr   error
	installs  [][]string
	installFn func([]string) error
	restarts  []Change
	// restartFn answers a restart; nil succeeds, returning no manifest.
	restartFn func(context.Context, Change) (*manifest.Manifest, error)
	watched   []string

	events chan WatchEvent
	cancel context.CancelFunc
}

func newLoopHarness(t *testing.T, mode Mode, running *manifest.Manifest) *loopHarness {
	t.Helper()
	return newLoopHarnessOnDisk(t, mode, running, running)
}

// newLoopHarnessOnDisk is newLoopHarness for a project whose manifest on disk
// is already onDisk when the watch begins, edited while the start ran.
func newLoopHarnessOnDisk(t *testing.T, mode Mode, running, onDisk *manifest.Manifest) *loopHarness {
	t.Helper()
	return newLoopHarnessWith(t, mode, running, onDisk, nil)
}

// newLoopHarnessWithFiles is newLoopHarness watching extra beside the
// built-in files, as WatchOptions.Files does.
func newLoopHarnessWithFiles(t *testing.T, mode Mode, running *manifest.Manifest, extra ...WatchedFile) *loopHarness {
	t.Helper()
	return newLoopHarnessWith(t, mode, running, running, append(slices.Clone(builtinWatchedFiles), extra...))
}

func newLoopHarnessWith(t *testing.T, mode Mode, running, onDisk *manifest.Manifest, files []WatchedFile) *loopHarness {
	t.Helper()
	h := &loopHarness{
		t:       t,
		names:   make(chan string),
		done:    make(chan struct{}),
		current: onDisk,
		events:  make(chan WatchEvent, 64),
	}
	w := watchLoop{
		mode:     mode,
		debounce: 20 * time.Millisecond,
		files:    files,
		load: func() (*manifest.Manifest, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.current, h.loadErr
		},
		hotInstall: func(_ context.Context, deps []string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.installs = append(h.installs, deps)
			if h.installFn != nil {
				return h.installFn(deps)
			}
			return nil
		},
		restart: func(ctx context.Context, c Change) (*manifest.Manifest, error) {
			h.mu.Lock()
			h.restarts = append(h.restarts, c)
			fn := h.restartFn
			h.mu.Unlock()
			if fn != nil {
				return fn(ctx, c)
			}
			return nil, nil
		},
		emit: func(e WatchEvent) { h.events <- e },
		watchDir: func(rel string) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.watched = append(h.watched, rel)
		},
	}
	// Edits a test makes must come after the check of what changed during
	// the start, or that check would apply them instead of their events.
	ready := make(chan struct{})
	w.onReady = func() { close(ready) }
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		defer close(h.done)
		w.run(ctx, h.names, running)
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the watch loop never became ready")
	}
	t.Cleanup(func() {
		cancel()
		<-h.done
	})
	return h
}

// save makes m the manifest on disk (nil with err for one that does not load)
// and reports edits to names.
func (h *loopHarness) save(m *manifest.Manifest, err error, names ...string) {
	h.mu.Lock()
	h.current, h.loadErr = m, err
	h.mu.Unlock()
	for _, n := range names {
		h.names <- n
	}
}

// next waits for the loop's next event.
func (h *loopHarness) next() WatchEvent {
	h.t.Helper()
	select {
	case e := <-h.events:
		return e
	case <-time.After(5 * time.Second):
		h.t.Fatal("no watch event")
		return WatchEvent{}
	}
}

// quiet asserts the loop reports nothing for a while.
func (h *loopHarness) quiet() {
	h.t.Helper()
	select {
	case e := <-h.events:
		h.t.Fatalf("unexpected watch event %+v", e)
	case <-time.After(100 * time.Millisecond):
	}
}

func (h *loopHarness) kinds(n int) []WatchEventKind { //nolint:unparam // a count, not a constant: every case so far happens to wait for two
	h.t.Helper()
	out := make([]WatchEventKind, n)
	for i := range out {
		out[i] = h.next().Kind
	}
	return out
}

func TestWatchHotInstallsAnAddedPackage(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(pyproject(t, baseDeps+`, "requests"`, ""), nil, manifest.Marker)

	start := h.next()
	require.Equal(t, WatchInstalling, start.Kind)
	require.Equal(t, []string{manifest.Marker}, start.Files)
	require.Equal(t, WatchInstalled, h.next().Kind)
	require.Equal(t, [][]string{{"requests"}}, h.installs)
}

// One save is several events. Acting on each would install the same package
// several times, or read the file halfway through its replacement.
func TestWatchActsOnceForABurst(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(pyproject(t, baseDeps+`, "requests"`, ""), nil, manifest.Marker, manifest.Marker, manifest.Marker)

	require.Equal(t, []WatchEventKind{WatchInstalling, WatchInstalled}, h.kinds(2))
	h.quiet()
	require.Len(t, h.installs, 1)
}

// The next edit is compared with what was applied, so a package installed
// once is not installed again by an unrelated save.
func TestWatchComparesWithWhatWasApplied(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	withRequests := pyproject(t, baseDeps+`, "requests"`, "")
	h.save(withRequests, nil, manifest.Marker)
	require.Equal(t, []WatchEventKind{WatchInstalling, WatchInstalled}, h.kinds(2))

	h.save(pyproject(t, baseDeps+`, "requests", "httpx"`, ""), nil, manifest.Marker)
	require.Equal(t, []WatchEventKind{WatchInstalling, WatchInstalled}, h.kinds(2))
	require.Equal(t, [][]string{{"requests"}, {"httpx"}}, h.installs)
}

func TestWatchIgnoresFilesAirflowDoesNotReadAtStart(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(pyproject(t, baseDeps, ""), nil, "dags/etl.py", "README.md", "Dockerfile")
	h.quiet()
}

// A save made mid-thought must not take Airflow down, and must not be
// forgotten either: the save that fixes it is compared with the manifest
// running, not with the broken one.
func TestWatchLeavesAirflowAloneWhileTheManifestDoesNotLoad(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(nil, errors.New("toml: expected ]"), manifest.Marker)
	e := h.next()
	require.Equal(t, WatchUnreadable, e.Kind)
	require.ErrorContains(t, e.Err, "expected ]")
	require.Empty(t, h.installs)
	require.Empty(t, h.restarts)

	h.save(pyproject(t, baseDeps+`, "requests"`, ""), nil, manifest.Marker)
	require.Equal(t, []WatchEventKind{WatchInstalling, WatchInstalled}, h.kinds(2))
	require.Equal(t, [][]string{{"requests"}}, h.installs)
}

func TestWatchRestartsForADotenvEdit(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(pyproject(t, baseDeps, ""), nil, dotenvFile)

	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: ReasonDotenv, Path: dotenvFile}}, e.Change.Reasons)
	require.Equal(t, WatchRestarted, h.next().Kind)
	require.Len(t, h.restarts, 1)
}

// A .env edit made while the manifest is broken is not lost: a restart then
// would refuse the manifest, so it waits for the save that fixes it.
func TestWatchCarriesADotenvEditPastAnUnreadableManifest(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(nil, errors.New("toml: bad"), dotenvFile)
	require.Equal(t, WatchUnreadable, h.next().Kind)

	h.save(pyproject(t, baseDeps, ""), nil, manifest.Marker)
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []string{dotenvFile, manifest.Marker}, e.Files)
	require.Equal(t, []Reason{{Code: ReasonDotenv, Path: dotenvFile}}, e.Change.Reasons)
}

// A restart reason in the same burst overrides the install: installing into
// the environment the restart is about to rebuild is wasted work at best.
func TestWatchRestartOverridesAnInstallInTheSameBurst(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.save(pyproject(t, baseDeps+`, "requests"`, ""), nil, manifest.Marker, dotenvFile)

	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Empty(t, e.Change.Install)
	require.Equal(t, WatchRestarted, h.next().Kind)
	require.Empty(t, h.installs)
}

// A failed install (a typo in a package name) leaves Airflow as it was, and
// the next save tries again rather than assuming it happened.
func TestWatchRetriesAFailedInstall(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.installFn = func([]string) error { return errors.New("no solution found") }
	withTypo := pyproject(t, baseDeps+`, "requestz"`, "")
	h.save(withTypo, nil, manifest.Marker)
	require.Equal(t, WatchInstalling, h.next().Kind)
	failed := h.next()
	require.Equal(t, WatchInstallFailed, failed.Kind)
	require.ErrorContains(t, failed.Err, "no solution")

	h.mu.Lock()
	h.installFn = nil
	h.mu.Unlock()
	h.save(withTypo, nil, manifest.Marker)
	require.Equal(t, []WatchEventKind{WatchInstalling, WatchInstalled}, h.kinds(2))
	require.Len(t, h.installs, 2)
	require.Empty(t, h.restarts)
}

// A project that stopped under the watch cannot take a hot install; a start
// applies the change instead.
func TestWatchRestartsWhenAHotInstallIsUnavailable(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.installFn = func([]string) error { return ErrNotImplemented }
	h.save(pyproject(t, baseDeps+`, "requests"`, ""), nil, manifest.Marker)

	require.Equal(t, WatchInstalling, h.next().Kind)
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: ReasonHotInstallUnavailable}}, e.Change.Reasons)
	require.Equal(t, WatchRestarted, h.next().Kind)
}

// After a failed restart nobody knows what is running, so the next save is
// compared with the manifest from before the failure, and applied in full.
func TestWatchReappliesAfterAFailedRestart(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.restartFn = func(context.Context, Change) (*manifest.Manifest, error) { return nil, errors.New("port in use") }
	newAirflow := pyproject(t, `"apache-airflow==3.2.*", "pandas>=2.1"`, "")
	h.save(newAirflow, nil, manifest.Marker)
	require.Equal(t, WatchRestarting, h.next().Kind)
	require.Equal(t, WatchRestartFailed, h.next().Kind)

	h.mu.Lock()
	h.restartFn = nil
	h.mu.Unlock()
	h.save(newAirflow, nil, manifest.Marker)
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: ReasonAirflow}}, e.Change.Reasons)
	require.Equal(t, WatchRestarted, h.next().Kind)
}

func TestWatchReportsARemovalWithoutActing(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps+`, "requests"`, ""))
	h.save(pyproject(t, baseDeps, ""), nil, manifest.Marker)

	e := h.next()
	require.Equal(t, WatchUnchanged, e.Kind)
	require.Equal(t, []string{"requests"}, e.Change.Removed)
	require.Empty(t, h.installs)
	require.Empty(t, h.restarts)
}

func TestWatchRestartsForAnEditedDockerfileInDockerMode(t *testing.T) {
	m := pyproject(t, baseDeps, "dockerfile = \"docker/Dockerfile\"\n")
	h := newLoopHarness(t, ModeDocker, m)
	h.save(m, nil, "docker/Dockerfile")

	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: ReasonDockerfileEdited, Path: "docker/Dockerfile"}}, e.Change.Reasons)
}

// Standalone builds no image, so its Dockerfile is not something to restart for.
func TestWatchIgnoresTheDockerfileInStandalone(t *testing.T) {
	m := pyproject(t, baseDeps, "dockerfile = \"Dockerfile\"\n")
	h := newLoopHarness(t, ModeStandalone, m)
	h.save(m, nil, "Dockerfile")
	h.quiet()
}

// The real filesystem, through fsnotify: a .env written in place, then a
// manifest saved the way editors save, by renaming a temp file over it. A
// watch on the file rather than the directory would miss the second.
func TestWatchProjectSeesEditsOnDisk(t *testing.T) {
	dir := t.TempDir()
	const head = "[project]\nname = \"p\"\nrequires-python = \">=3.11\"\ndependencies = [" + baseDeps + "]\n\n[tool.astro]\n"
	pyprojectPath := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(pyprojectPath, []byte(head), 0o600))

	restarts := make(chan Change, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var r Runtime
	go func() {
		done <- r.WatchProject(ctx, WatchOptions{
			ProjectPath: dir,
			Mode:        ModeStandalone,
			Debounce:    50 * time.Millisecond,
			Restart: func(_ context.Context, c Change) (*manifest.Manifest, error) {
				restarts <- c
				return nil, nil
			},
		})
	}()
	// fsnotify registers the directory before WatchProject reads any event,
	// but the goroutine may not have got there yet.
	time.Sleep(200 * time.Millisecond)

	waitRestart := func() Change {
		t.Helper()
		select {
		case c := <-restarts:
			return c
		case <-time.After(5 * time.Second):
			t.Fatal("no restart")
			return Change{}
		}
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, dotenvFile), []byte("FOO=bar\n"), 0o600))
	require.Equal(t, []Reason{{Code: ReasonDotenv, Path: dotenvFile}}, waitRestart().Reasons)

	tmp := filepath.Join(dir, ".pyproject.toml.swp")
	require.NoError(t, os.WriteFile(tmp, []byte(head+"[tool.astro.pools.etl]\nslots = 4\n"), 0o600))
	require.NoError(t, os.Rename(tmp, pyprojectPath))
	require.Equal(t, []Reason{{Code: ReasonPools}}, waitRestart().Reasons)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("WatchProject did not return after its context ended")
	}
}

func TestWatchProjectRefusesIncompleteOptions(t *testing.T) {
	var r Runtime
	require.ErrorContains(t, r.WatchProject(context.Background(), WatchOptions{}), "project path")
	require.ErrorContains(t, r.WatchProject(context.Background(), WatchOptions{ProjectPath: t.TempDir()}), "Restart")
}

// A restart that rewrites the manifest itself (the CLI's moves the Astronomer
// build pin) returns what it started from, so its own write is not an edit to
// restart for again.
func TestWatchComparesWithWhatTheRestartStarted(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	rewritten := pyproject(t, `"apache-airflow==3.2.*", "pandas>=2.1"`, "\n[tool.uv]\nconstraint-dependencies = [\"apache-airflow==3.2.1+astro.1\"]\n")
	h.restartFn = func(context.Context, Change) (*manifest.Manifest, error) {
		h.mu.Lock()
		h.current = rewritten // the restart's own write, before it starts
		h.mu.Unlock()
		return rewritten, nil
	}
	h.save(pyproject(t, `"apache-airflow==3.2.*", "pandas>=2.1"`, ""), nil, manifest.Marker)
	require.Equal(t, []WatchEventKind{WatchRestarting, WatchRestarted}, h.kinds(2))

	// The event for the restart's write arrives after it.
	h.save(rewritten, nil, manifest.Marker)
	require.Equal(t, WatchUnchanged, h.next().Kind)
	require.Len(t, h.restarts, 1)
}

// A restart that failed for a reason outside the watched files (a busy port,
// a missing vault value) is retried by the next save, even one that changes
// nothing, rather than leaving Airflow down.
func TestWatchRetriesAnOwedRestart(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	h := newLoopHarness(t, ModeStandalone, m)
	h.restartFn = func(context.Context, Change) (*manifest.Manifest, error) { return nil, errors.New("port in use") }
	h.save(m, nil, dotenvFile)
	require.Equal(t, []WatchEventKind{WatchRestarting, WatchRestartFailed}, h.kinds(2))

	h.mu.Lock()
	h.restartFn = nil
	h.mu.Unlock()
	h.save(m, nil, manifest.Marker)
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: ReasonDotenv, Path: dotenvFile}}, e.Change.Reasons)
	require.Equal(t, WatchRestarted, h.next().Kind)

	// Paid: the next unchanged save is nothing again.
	h.save(m, nil, manifest.Marker)
	require.Equal(t, WatchUnchanged, h.next().Kind)
}

// An edit made while the start ran happened before the watch began, so no
// event reports it: the watch compares the disk with what was started.
func TestWatchAppliesAnEditMadeDuringTheStart(t *testing.T) {
	h := newLoopHarnessOnDisk(t, ModeStandalone, pyproject(t, baseDeps, ""), pyproject(t, baseDeps+`, "requests"`, ""))
	require.Equal(t, []WatchEventKind{WatchInstalling, WatchInstalled}, h.kinds(2))
	require.Equal(t, [][]string{{"requests"}}, h.installs)
}

// Beginning a watch on a project that matches what was started says nothing.
func TestWatchBeginsQuietly(t *testing.T) {
	h := newLoopHarness(t, ModeStandalone, pyproject(t, baseDeps, ""))
	h.quiet()
}

// Ctrl-C during a restart ends the watch, not the restart's prospects: no
// "restart failed, save again" for an interrupt the user made.
func TestWatchSaysNothingWhenInterruptedMidRestart(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	h := newLoopHarness(t, ModeStandalone, m)
	h.restartFn = func(ctx context.Context, _ Change) (*manifest.Manifest, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	h.save(m, nil, dotenvFile)
	require.Equal(t, WatchRestarting, h.next().Kind)
	h.cancel()
	<-h.done
	select {
	case e := <-h.events:
		t.Fatalf("unexpected watch event after the interrupt: %+v", e)
	default:
	}
}

// A Dockerfile moved to another directory is watched there too.
func TestWatchFollowsAMovedDockerfile(t *testing.T) {
	m := pyproject(t, baseDeps, "dockerfile = \"docker/Dockerfile\"\n")
	h := newLoopHarness(t, ModeDocker, m)
	moved := pyproject(t, baseDeps, "dockerfile = \"build/Dockerfile\"\n")
	h.save(moved, nil, manifest.Marker)
	require.Equal(t, []WatchEventKind{WatchRestarting, WatchRestarted}, h.kinds(2))
	h.mu.Lock()
	require.Contains(t, h.watched, "docker")
	require.Contains(t, h.watched, "build")
	h.mu.Unlock()

	h.save(moved, nil, "build/Dockerfile")
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: ReasonDockerfileEdited, Path: "build/Dockerfile"}}, e.Change.Reasons)
}

// An overflowed event queue lost edits to files nobody can name, so every
// file the watch acts on is checked.
func TestWatchRescansAfterAnOverflow(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	h := newLoopHarness(t, ModeStandalone, m)
	h.save(pyproject(t, baseDeps+`, "requests"`, ""), nil, rescanAll)
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind, "the .env is among the files an overflow stands for")
	require.Equal(t, []string{dotenvFile, manifest.Marker}, e.Files)
}

// forward never makes fsnotify wait on a reader busy with a restart: events
// queue, each name once, and an overflow becomes a rescan.
func TestForwardQueuesWhileTheReaderIsBusy(t *testing.T) {
	dir := t.TempDir()
	events := make(chan fsnotify.Event)
	errs := make(chan error)
	out := make(chan string)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go forward(ctx, dir, events, errs, out)

	// Nobody reads out yet: every send must still be taken at once.
	for range 100 {
		events <- fsnotify.Event{Name: filepath.Join(dir, manifest.Marker), Op: fsnotify.Write}
	}
	events <- fsnotify.Event{Name: filepath.Join(dir, dotenvFile), Op: fsnotify.Chmod}
	events <- fsnotify.Event{Name: filepath.Join(dir, dotenvFile), Op: fsnotify.Create}
	errs <- fsnotify.ErrEventOverflow

	require.Equal(t, manifest.Marker, <-out)
	require.Equal(t, dotenvFile, <-out)
	require.Equal(t, rescanAll, <-out)
	select {
	case n := <-out:
		t.Fatalf("each name is queued once, got another: %q", n)
	case <-time.After(50 * time.Millisecond):
	}
}

// settingsFile is a caller's own watched file: one its start reads, which the
// built-in table knows nothing about.
var settingsFile = WatchedFile{
	Path:   fixedPath("config/settings.yaml"),
	Reason: Reason{Code: "settings"},
}

// A caller's file restarts Airflow for the caller's own code, carrying the
// path so even a frontend with no words for the code can say what changed.
func TestWatchRestartsForACallersFile(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	h := newLoopHarnessWithFiles(t, ModeStandalone, m, settingsFile)
	h.mu.Lock()
	require.Equal(t, []string{"config"}, h.watched, "a file below the root has its directory watched")
	h.mu.Unlock()

	h.save(m, nil, "config/settings.yaml")
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: "settings", Path: "config/settings.yaml"}}, e.Change.Reasons)
	require.Equal(t, "config/settings.yaml changed", e.Change.Reasons[0].String())
}

// A file listed for some modes is not watched in the others.
func TestWatchIgnoresACallersFileOutsideItsModes(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	dockerOnly := settingsFile
	dockerOnly.Modes = []Mode{ModeDocker}
	h := newLoopHarnessWithFiles(t, ModeStandalone, m, dockerOnly)
	h.save(m, nil, "config/settings.yaml")
	h.quiet()
}

// ...and is watched in the modes it lists.
func TestWatchRestartsForACallersFileInItsMode(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	dockerOnly := settingsFile
	dockerOnly.Modes = []Mode{ModeDocker}
	h := newLoopHarnessWithFiles(t, ModeDocker, m, dockerOnly)
	h.save(m, nil, "config/settings.yaml")
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []Reason{{Code: "settings", Path: "config/settings.yaml"}}, e.Change.Reasons)
}

// An overflow stands for a caller's files too.
func TestWatchRescansACallersFileAfterAnOverflow(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	h := newLoopHarnessWithFiles(t, ModeStandalone, m, settingsFile)
	h.save(m, nil, rescanAll)
	e := h.next()
	require.Equal(t, WatchRestarting, e.Kind)
	require.Equal(t, []string{dotenvFile, "config/settings.yaml", manifest.Marker}, e.Files)
	require.Equal(t, []Reason{
		{Code: ReasonDotenv, Path: dotenvFile},
		{Code: "settings", Path: "config/settings.yaml"},
	}, e.Change.Reasons)
}

// A path is compared as the event reports it: cleaned and slash-separated,
// however the caller spelled it.
func TestWatchCleansACallersPath(t *testing.T) {
	m := pyproject(t, baseDeps, "")
	h := newLoopHarnessWithFiles(t, ModeStandalone, m, WatchedFile{Path: fixedPath("./config//settings.yaml"), Reason: Reason{Code: "settings"}})
	h.save(m, nil, "config/settings.yaml")
	require.Equal(t, WatchRestarting, h.next().Kind)
}

func TestWatchProjectRefusesAWatchedFileWithoutAPath(t *testing.T) {
	var r Runtime
	err := r.WatchProject(context.Background(), WatchOptions{
		ProjectPath: t.TempDir(),
		Restart:     func(context.Context, Change) (*manifest.Manifest, error) { return nil, nil },
		Files:       []WatchedFile{{Reason: Reason{Code: "settings"}}},
	})
	require.ErrorContains(t, err, "Files[0] needs a Path")
}
