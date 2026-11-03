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
	// Manifest is the manifest the running Airflow was started from, the one
	// the first edit is compared with. Nil loads it when the watch begins,
	// which misses an edit made between the start and the watch.
	Manifest *manifest.Manifest
	// Debounce overrides DefaultWatchDebounce.
	Debounce time.Duration
	// Restart applies a change that needs a start: it stops the running
	// Airflow and starts it again from the project as it now is, which is the
	// caller's to define (the CLI rebuilds its plan and warnings; Astro
	// Desktop has its own). Required.
	Restart func(ctx context.Context, c Change) error
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
// Edits are applied one burst at a time. Edits made while a restart runs are
// queued and applied, against what the restart started, once it finishes.
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
	running := o.Manifest
	if running == nil {
		if running, err = manifest.Load(filepath.Join(dir, manifest.Marker)); err != nil {
			return fmt.Errorf("reading the manifest the running Airflow was started from: %w", err)
		}
	}

	var extra []string
	if d := declaredDockerfile(running); d != "" && mode == ModeDocker {
		extra = append(extra, filepath.Dir(filepath.Join(dir, d)))
	}
	names, closeWatch, err := watchDirs(ctx, dir, extra)
	if err != nil {
		return err
	}
	defer closeWatch()

	w := watchLoop{
		mode:     mode,
		debounce: o.Debounce,
		load:     func() (*manifest.Manifest, error) { return manifest.Load(filepath.Join(dir, manifest.Marker)) },
		hotInstall: func(ctx context.Context, deps []string) error {
			return r.HotInstall(ctx, dir, deps, o.Callbacks)
		},
		restart: o.Restart,
		emit:    o.OnEvent,
	}
	w.run(ctx, names, running)
	return nil
}

// watchDirs reports the path, relative to dir and slash-separated, of every
// file created, written, renamed or removed in dir or in one of extra, until
// ctx is done. close releases the watch.
//
// Directories, not files: an editor that saves by renaming a temp file over
// the original replaces the inode a file watch is on, and every edit after the
// first would go unseen.
func watchDirs(ctx context.Context, dir string, extra []string) (names <-chan string, closeWatch func(), err error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	closeWatch = func() {
		//nolint:errcheck // closing a watch has nothing left to report to
		fsw.Close()
	}
	if err := fsw.Add(dir); err != nil {
		closeWatch()
		return nil, nil, err
	}
	for _, d := range extra {
		if d != dir {
			//nolint:errcheck // best effort: a Dockerfile in a directory that does not exist yet is a start failure the restart reports
			fsw.Add(d)
		}
	}

	out := make(chan string)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-fsw.Events:
				if !ok {
					return
				}
				rel, err := filepath.Rel(dir, ev.Name)
				if ev.Op == fsnotify.Chmod || err != nil {
					continue
				}
				select {
				case out <- filepath.ToSlash(rel):
				case <-ctx.Done():
					return
				}
			case _, ok := <-fsw.Errors:
				// An overflowed queue or a vanished directory. Neither is
				// actionable here, and the next event still arrives.
				if !ok {
					return
				}
			}
		}
	}()
	return out, closeWatch, nil
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
	restart    func(ctx context.Context, c Change) error
	emit       func(WatchEvent)
}

// run applies each debounced burst of names to running, until names closes or
// ctx is done.
func (w *watchLoop) run(ctx context.Context, names <-chan string, running *manifest.Manifest) {
	debounce := w.debounce
	if debounce <= 0 {
		debounce = DefaultWatchDebounce
	}
	pending := map[string]bool{}
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
			if !w.relevant(name, running) {
				continue
			}
			pending[name] = true
			stopTimer()
			timer = time.NewTimer(debounce)
			fire = timer.C
		case <-fire:
			fire = nil
			files := slices.Sorted(maps.Keys(pending))
			clear(pending)
			var carry []string
			running, carry = w.apply(ctx, files, running)
			// Files whose edit could not be acted on yet, because the
			// manifest did not load, are acted on with the next burst.
			for _, f := range carry {
				pending[f] = true
			}
		}
	}
}

// relevant reports whether an edit to name is one the running Airflow could
// see: the manifest, the .env, or in docker mode the declared Dockerfile.
func (w *watchLoop) relevant(name string, running *manifest.Manifest) bool {
	switch name {
	case manifest.Marker, dotenvFile:
		return true
	}
	return w.mode == ModeDocker && name != "" && name == declaredDockerfile(running)
}

// apply acts on one burst and returns the manifest the running Airflow now
// reflects, with the files to fold into the next burst.
func (w *watchLoop) apply(ctx context.Context, files []string, running *manifest.Manifest) (applied *manifest.Manifest, carry []string) {
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

	switch c.Action {
	case ChangeNone:
		w.send(WatchEvent{Kind: WatchUnchanged, Files: files, Change: c})
		return next, nil
	case ChangeHotInstall:
		w.send(WatchEvent{Kind: WatchInstalling, Files: files, Change: c})
		err := w.hotInstall(ctx, c.Install)
		switch {
		case err == nil:
			w.send(WatchEvent{Kind: WatchInstalled, Files: files, Change: c})
			return next, nil
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
	if err := w.restart(ctx, c); err != nil {
		// What is running now is unknown: the restart may have stopped
		// Airflow before failing. Comparing the next save with the manifest
		// from before means it is applied in full, and a hot install into a
		// project that is no longer running falls back to a restart.
		w.send(WatchEvent{Kind: WatchRestartFailed, Files: files, Change: c, Err: err})
		return running, nil
	}
	w.send(WatchEvent{Kind: WatchRestarted, Files: files, Change: c})
	return next, nil
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
