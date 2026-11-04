package localrt

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// DefaultWatchDebounce is how long WatchProject waits after the last write
// before it acts. One save is several events (an editor writes a temp file and
// renames it over the original, a formatter rewrites what was just saved), and
// acting on the first would read a file halfway through being replaced.
const DefaultWatchDebounce = 500 * time.Millisecond

// dotenvFile is the env file both engines read at start.
const dotenvFile = ".env"

// rescanAll stands in for every file a watch acts on, when the event queue
// overflowed and which of them changed is unknown. Not a path: it contains a
// NUL, which no file name can.
const rescanAll = "\x00rescan"

// WatchEventKind is what WatchProject did about one burst of edits.
type WatchEventKind string

const (
	// WatchUnreadable: the manifest no longer loads (a half-finished edit, a
	// typo). Nothing was applied, and the running Airflow is untouched; the
	// next save that loads is compared with the last one that did.
	WatchUnreadable WatchEventKind = "unreadable"
	// WatchUnchanged: nothing the running Airflow reads changed. Change may
	// still list Removed requirements, which stay installed until a restart.
	WatchUnchanged WatchEventKind = "unchanged"
	// WatchInstalling and WatchInstalled bracket a hot install of
	// Change.Install; WatchInstallFailed replaces WatchInstalled when it fails.
	// A failed install leaves Airflow running on what it had, and the next
	// save tries again.
	WatchInstalling    WatchEventKind = "installing"
	WatchInstalled     WatchEventKind = "installed"
	WatchInstallFailed WatchEventKind = "install-failed"
	// WatchRestarting and WatchRestarted bracket WatchOptions.Restart, for
	// Change.Reasons; WatchRestartFailed replaces WatchRestarted when it fails.
	// A failed restart is owed: the next burst restarts again, for its own
	// reasons and the failed one's.
	WatchRestarting    WatchEventKind = "restarting"
	WatchRestarted     WatchEventKind = "restarted"
	WatchRestartFailed WatchEventKind = "restart-failed"
)

// WatchEvent is one report from WatchProject.
type WatchEvent struct {
	Kind WatchEventKind
	// Files are the project-relative paths whose edits this burst covers.
	Files []string
	// Change is what the burst amounts to. Empty for WatchUnreadable.
	Change Change
	// Err is why the manifest did not load, or why the install or restart
	// failed. Nil otherwise.
	Err error
}

// WatchOptions configure WatchProject.
type WatchOptions struct {
	// ProjectPath is the project to watch. Required.
	ProjectPath string
	// Mode is the mode the project is running in. Empty reads it from the
	// project's record.
	Mode Mode
	// Manifest is the manifest the running Airflow was started from. The
	// watch compares the project with it as soon as it begins, so an edit made
	// while the start ran is applied rather than missed. Nil loads it when the
	// watch begins, which cannot tell such an edit from what was started.
	Manifest *manifest.Manifest
	// Debounce overrides DefaultWatchDebounce.
	Debounce time.Duration
	// Restart applies a change that needs a start: it stops the running
	// Airflow and starts it again from the project as it now is, which is the
	// caller's to define (the CLI rebuilds its plan and warnings; Astro
	// Desktop has its own). Required.
	//
	// It returns the manifest the new start was built from, which later edits
	// are compared with. That matters when the restart writes the manifest
	// itself, as the CLI's does when it moves the Astronomer build pin: the
	// write is then already in what it returns, and not an edit to restart
	// for again. Nil has the watch reload the manifest after the restart,
	// which is right unless the manifest was edited while the restart ran.
	Restart func(ctx context.Context, c Change) (*manifest.Manifest, error)
	// OnEvent receives every WatchEvent, synchronously and in order. Optional.
	OnEvent func(WatchEvent)
	// Callbacks carry a hot install's output.
	Callbacks Callbacks
}

// WatchProject keeps a running project's Airflow in step with edits to its
// manifest, its .env and, in docker mode, a Dockerfile the manifest declares,
// until ctx is done. It returns nil then, or an error only if it could not
// begin watching.
//
// Each burst of edits is classified by ClassifyChange against the manifest
// last applied, and acted on: new requirements are hot installed, anything a
// hot install cannot apply is handed to Restart, and an edit that leaves the
// manifest unloadable is reported and otherwise ignored, so a save made
// mid-thought never takes Airflow down.
//
// It is here rather than in a frontend for the reason ClassifyChange is: the
// CLI's --watch and Astro Desktop both keep a running Airflow in step with
// its project, and two loops would drift in what they watch, how they wait,
// and which manifest they compare against.
//
// Edits are applied one burst at a time. Edits made while an install or a
// restart runs are queued and applied, against what it left running, once it
// finishes. An install or restart that ctx ends partway reports nothing: the
// caller ended the watch, and is about to say so itself.
func (r *Runtime) WatchProject(ctx context.Context, o WatchOptions) error {
	if o.ProjectPath == "" {
		return errors.New("WatchProject needs a project path")
	}
	if o.Restart == nil {
		return errors.New("WatchProject needs a Restart")
	}
	dir, err := filepath.Abs(o.ProjectPath)
	if err != nil {
		return err
	}
	mode := o.Mode
	if mode == "" {
		st, err := r.ReadStatus(dir)
		if err != nil {
			return err
		}
		mode = st.Mode
	}
	load := func() (*manifest.Manifest, error) { return manifest.Load(filepath.Join(dir, manifest.Marker)) }
	running := o.Manifest
	if running == nil {
		if running, err = load(); err != nil {
			return fmt.Errorf("reading the manifest the running Airflow was started from: %w", err)
		}
	}

	names, addDir, closeWatch, err := watchDirs(ctx, dir)
	if err != nil {
		return err
	}
	defer closeWatch()

	w := watchLoop{
		mode:     mode,
		debounce: o.Debounce,
		load:     load,
		hotInstall: func(ctx context.Context, deps []string) error {
			return r.HotInstall(ctx, dir, deps, o.Callbacks)
		},
		restart: o.Restart,
		emit:    o.OnEvent,
		watchDir: func(rel string) {
			//nolint:errcheck // best effort: a Dockerfile in a directory that does not exist yet is a start failure the restart reports
			addDir(filepath.Join(dir, rel))
		},
	}
	w.run(ctx, names, running)
	return nil
}

// watchDirs reports the path, relative to dir and slash-separated, of every
// file created, written, renamed or removed in dir, or in a directory addDir
// adds, until ctx is done. closeWatch releases the watch.
//
// Directories, not files: an editor that saves by renaming a temp file over
// the original replaces the inode a file watch is on, and every edit after the
// first would go unseen.
func watchDirs(ctx context.Context, dir string) (names <-chan string, addDir func(string) error, closeWatch func(), err error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, nil, err
	}
	closeWatch = func() {
		//nolint:errcheck // closing a watch has nothing left to report to
		fsw.Close()
	}
	if err := fsw.Add(dir); err != nil {
		closeWatch()
		return nil, nil, nil, err
	}
	out := make(chan string)
	go forward(ctx, dir, fsw.Events, fsw.Errors, out)
	return out, fsw.Add, closeWatch, nil
}

// forward turns fsnotify's events into project-relative names on out, until
// ctx is done or fsnotify closes.
//
// It never blocks fsnotify on the reader: names wait in a queue, each once,
// while the reader is busy with a restart that takes minutes, rather than
// leaving the events in the operating system's queue, which overflows. An
// overflow, if one happens anyway, is reported as rescanAll, since which
// files it lost edits to is not known.
func forward(ctx context.Context, dir string, events <-chan fsnotify.Event, errs <-chan error, out chan<- string) {
	defer close(out)
	var queue []string
	queued := map[string]bool{}
	push := func(name string) {
		if !queued[name] {
			queued[name] = true
			queue = append(queue, name)
		}
	}
	for {
		// A nil channel is never ready, so nothing is sent while the queue
		// is empty.
		var send chan<- string
		var head string
		if len(queue) > 0 {
			send, head = out, queue[0]
		}
		select {
		case <-ctx.Done():
			return
		case send <- head:
			queue = queue[1:]
			delete(queued, head)
		case ev, ok := <-events:
			if !ok {
				return
			}
			rel, err := filepath.Rel(dir, ev.Name)
			if ev.Op != fsnotify.Chmod && err == nil {
				push(filepath.ToSlash(rel))
			}
		case err, ok := <-errs:
			if !ok {
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				push(rescanAll)
			}
		}
	}
}

// declaredDockerfile is the project-relative path of the Dockerfile the
// manifest declares, "" when it declares none.
func declaredDockerfile(m *manifest.Manifest) string {
	if m == nil || m.Astro.Dockerfile == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(m.Astro.Dockerfile))
}

// watchLoop is WatchProject without the filesystem, so its decisions can be
// tested against events a test writes.
type watchLoop struct {
	mode       Mode
	debounce   time.Duration
	load       func() (*manifest.Manifest, error)
	hotInstall func(ctx context.Context, deps []string) error
	restart    func(ctx context.Context, c Change) (*manifest.Manifest, error)
	emit       func(WatchEvent)
	// watchDir adds a project-relative directory to the watch. nil in tests
	// that do not look.
	watchDir func(rel string)
	// onReady is called once the check of what changed while the start ran
	// is done, before the first event is read. For tests, which must not
	// edit the project before that check reads it. Optional.
	onReady func()

	// owed are the reasons of a restart that failed: whatever left Airflow
	// down may be fixed in a file the watch does not read, so the next burst
	// restarts for them even if it changes nothing itself.
	owed []Reason
}

// run applies each debounced burst of names to running, until names closes or
// ctx is done.
func (w *watchLoop) run(ctx context.Context, names <-chan string, running *manifest.Manifest) {
	debounce := w.debounce
	if debounce <= 0 {
		debounce = DefaultWatchDebounce
	}
	pending := map[string]bool{}
	w.watchDockerfileDir(running)
	// An edit made while the start ran happened before the watch began, so no
	// event will report it: compare what is on disk with what was started.
	running, carry := w.apply(ctx, []string{manifest.Marker}, running, true)
	w.watchDockerfileDir(running)
	for _, f := range carry {
		pending[f] = true
	}
	if w.onReady != nil {
		w.onReady()
	}

	var timer *time.Timer
	var fire <-chan time.Time
	stopTimer := func() {
		if timer != nil {
			timer.Stop()
		}
	}
	defer stopTimer()
	for {
		select {
		case <-ctx.Done():
			return
		case name, ok := <-names:
			if !ok {
				return
			}
			added := false
			for _, n := range w.expand(name, running) {
				pending[n] = true
				added = true
			}
			if !added {
				continue
			}
			stopTimer()
			timer = time.NewTimer(debounce)
			fire = timer.C
		case <-fire:
			fire = nil
			files := slices.Sorted(maps.Keys(pending))
			clear(pending)
			running, carry = w.apply(ctx, files, running, false)
			w.watchDockerfileDir(running)
			// Files whose edit could not be acted on yet, because the
			// manifest did not load, are acted on with the next burst.
			for _, f := range carry {
				pending[f] = true
			}
		}
	}
}

// expand is the files an event for name stands for that the running Airflow
// could see: the manifest, the .env, or in docker mode the declared
// Dockerfile. An overflow stands for all of them.
func (w *watchLoop) expand(name string, running *manifest.Manifest) []string {
	dockerfile := ""
	if w.mode == ModeDocker {
		dockerfile = declaredDockerfile(running)
	}
	switch {
	case name == rescanAll:
		out := []string{manifest.Marker, dotenvFile}
		if dockerfile != "" {
			out = append(out, dockerfile)
		}
		return out
	case name == manifest.Marker, name == dotenvFile, dockerfile != "" && name == dockerfile:
		return []string{name}
	}
	return nil
}

// watchDockerfileDir makes sure the directory of the Dockerfile running
// declares is watched, in docker mode: a manifest that moves it to a new
// directory would otherwise leave its edits unseen.
func (w *watchLoop) watchDockerfileDir(running *manifest.Manifest) {
	if w.mode != ModeDocker || w.watchDir == nil {
		return
	}
	if d := declaredDockerfile(running); d != "" {
		if sub := filepath.Dir(d); sub != "." {
			w.watchDir(sub)
		}
	}
}

// apply acts on one burst and returns the manifest the running Airflow now
// reflects, with the files to fold into the next burst. quiet leaves out the
// report of a burst that changed nothing, for the check run makes when the
// watch begins.
func (w *watchLoop) apply(ctx context.Context, files []string, running *manifest.Manifest, quiet bool) (applied *manifest.Manifest, carry []string) {
	next, err := w.load()
	if err != nil {
		// Nothing is applied while the manifest does not load, not even a
		// .env edit: a restart would refuse the same manifest. The edits are
		// kept, so the save that fixes the manifest applies them too.
		w.send(WatchEvent{Kind: WatchUnreadable, Files: files, Err: err})
		return running, files
	}

	c := ClassifyChange(running, next, w.mode)
	if slices.Contains(files, dotenvFile) {
		c = needsRestart(c, Reason{Code: ReasonDotenv})
	}
	if w.mode == ModeDocker {
		if d := declaredDockerfile(next); d != "" && slices.Contains(files, d) {
			c = needsRestart(c, Reason{Code: ReasonDockerfileEdited})
		}
	}
	for _, why := range w.owed {
		if !slices.Contains(c.Reasons, why) {
			c = needsRestart(c, why)
		}
	}

	switch c.Action {
	case ChangeNone:
		if !quiet || len(c.Removed) > 0 {
			w.send(WatchEvent{Kind: WatchUnchanged, Files: files, Change: c})
		}
		return next, nil
	case ChangeHotInstall:
		w.send(WatchEvent{Kind: WatchInstalling, Files: files, Change: c})
		err := w.hotInstall(ctx, c.Install)
		switch {
		case err == nil:
			w.send(WatchEvent{Kind: WatchInstalled, Files: files, Change: c})
			return next, nil
		case ctx.Err() != nil:
			// The watch is ending, and the install with it: not a failure
			// to report, and not one to retry.
			return running, nil
		case errors.Is(err, ErrNotImplemented):
			// The project stopped under the watch, or its engine takes no
			// hot install: a start applies the change instead.
			c = needsRestart(c, Reason{Code: ReasonHotInstallUnavailable})
		default:
			// Keep the manifest from before, so the next save tries this
			// install again rather than deciding it already happened.
			w.send(WatchEvent{Kind: WatchInstallFailed, Files: files, Change: c, Err: err})
			return running, nil
		}
	case ChangeRestart:
		// Below, with the hot install that fell back to one.
	}

	w.send(WatchEvent{Kind: WatchRestarting, Files: files, Change: c})
	started, err := w.restart(ctx, c)
	if ctx.Err() != nil {
		return running, nil
	}
	if err != nil {
		// What is running now is unknown: the restart may have stopped
		// Airflow before failing. The next save is compared with the
		// manifest from before, so it is applied in full, and the reasons are
		// owed, so it restarts even if what failed was outside the files
		// this watches.
		w.owed = c.Reasons
		w.send(WatchEvent{Kind: WatchRestartFailed, Files: files, Change: c, Err: err})
		return running, nil
	}
	w.owed = nil
	w.send(WatchEvent{Kind: WatchRestarted, Files: files, Change: c})
	if started == nil {
		if started, err = w.load(); err != nil {
			// Edited into an unreadable state while the restart ran: the
			// queued event for that edit reports it, against what was
			// classified.
			started = next
		}
	}
	return started, nil
}

// needsRestart is c with one more reason to restart, which overrides any hot
// install it carried, as ClassifyChange's own reasons do.
func needsRestart(c Change, why Reason) Change {
	c.Action = ChangeRestart
	c.Install = nil
	c.Reasons = append(slices.Clone(c.Reasons), why)
	return c
}

func (w *watchLoop) send(e WatchEvent) {
	if w.emit != nil {
		w.emit(e)
	}
}
