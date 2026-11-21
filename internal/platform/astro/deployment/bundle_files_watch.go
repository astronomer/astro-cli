package deployment

import (
	httpContext "context"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/usageerr"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

var (
	errSyncWatch       = usageerr.Usage(errors.New("--watch applies only when syncing to a bundle"))
	errSyncWatchDryRun = usageerr.Usage(errors.New("--watch and --dry-run cannot be used together"))
)

var (
	// bundleSyncQuietPeriod is how long a watching sync waits for local files to stop changing before it syncs them.
	bundleSyncQuietPeriod = 2 * time.Second
	// bundleSyncMaxDelay is the longest a watching sync waits to sync a change while local files keep changing.
	bundleSyncMaxDelay = 10 * time.Second
	// bundleSyncRetryDelay is how long a watching sync waits to try a failed write again.
	bundleSyncRetryDelay = 10 * time.Second
)

// maxWatchedDirs is the most local directories a watching sync watches.
const maxWatchedDirs = 1024

// resync writes what changed under src since the last write, if anything.
func (t *bundleSyncTarget) resync(ctx httpContext.Context, src string, filter *localFileFilter, deleteExtras bool, out BundleOutput, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	files, _, err := localFiles(src, filter)
	if err != nil {
		return err
	}
	err = checkBundleUpload(src, t.path.Path, files)
	if err != nil {
		return err
	}
	local, err := localDigests(src, files)
	if err != nil {
		return err
	}
	plan := t.plan(local, deleteExtras)
	if plan.empty() {
		return nil
	}
	version, err := t.write(ctx, src, local, plan, astroV1Alpha1StreamClient)
	if err != nil {
		return err
	}
	result := newSyncResult(src, t.path.String(), syncUpload, plan, 0, false)
	result.Changed, result.Version = true, version
	return out.publishSync(result, plan, t.name, syncedLine(src, t.path, version, plan), true)
}

// watch syncs src into t, then again each time files under it stop changing for
// bundleSyncQuietPeriod, or keep changing for bundleSyncMaxDelay, until ctx is
// done (the run's first interrupt cancels it), or the API refuses a write for
// good. A write in flight when ctx ends is let finish, so the bundle is never
// left with one whose outcome the run does not know; a second interrupt ends
// the process.
func (t *bundleSyncTarget) watch(ctx httpContext.Context, src string, filterOpts BundleFilterOptions, filter *localFileFilter, deleteExtras bool, out BundleOutput, astroV1Alpha1StreamClient astrov1alpha1.ClientInterface) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrapf(err, "failed to watch %s", src)
	}
	defer watcher.Close() //nolint:errcheck // a watcher; nothing is flushed by closing it
	err = watchLocalDirs(watcher, src, src, filter)
	if err != nil {
		return errors.Wrapf(err, "failed to watch %s", src)
	}
	fmt.Fprintf(out.Notes, "Watching %s for changes. Press Ctrl+C to stop\n", src)

	// The first sync runs at once, after the watches are in place, so no change slips between them.
	next := time.NewTimer(0)
	defer next.Stop()
	// pending is when the oldest change not yet synced was seen, or zero.
	var pending time.Time
	// refilter is set when a change may alter what the filter skips, until the filter is built again.
	refilter := false
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case event := <-watcher.Events:
			changed, err := localChange(watcher, src, filter, event)
			if err != nil {
				return err
			}
			if event.Op != fsnotify.Chmod && changesFilter(event) {
				refilter, changed = true, true
			}
			if changed {
				pending = scheduleSync(next, pending)
			}
		case err := <-watcher.Errors:
			if !errors.Is(err, fsnotify.ErrEventOverflow) {
				return errors.Wrapf(err, "failed to watch %s", src)
			}
			pending = scheduleSync(next, pending)
		case <-next.C:
			pending = time.Time{}
			var err error
			if refilter {
				filter, err = t.refilter(watcher, src, filterOpts)
				refilter = err != nil
			}
			if err == nil {
				err = t.resync(httpContext.WithoutCancel(ctx), src, filter, deleteExtras, out, astroV1Alpha1StreamClient)
			}
			if err != nil {
				err = syncFailed(err, src, next, out)
			}
			if err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(out.Notes, "Stopped watching %s\n", src)
	return nil
}

// changesFilter reports whether event may change what the filter skips: a
// .gitignore changed, or a directory that may hold one appeared.
func changesFilter(event fsnotify.Event) bool {
	if filepath.Base(event.Name) == ".gitignore" {
		return true
	}
	info, err := os.Lstat(event.Name)
	return event.Has(fsnotify.Create) && err == nil && info.IsDir()
}

// refilter builds the filter again from the .gitignore files under src as they
// are now, and watches the directories it newly keeps. The files it newly skips
// are dropped from t, so the sync keeps them in the bundle instead of deleting
// them.
func (t *bundleSyncTarget) refilter(watcher *fsnotify.Watcher, src string, opts BundleFilterOptions) (*localFileFilter, error) {
	filter, err := newLocalFileFilter(src, opts)
	if err != nil {
		return nil, err
	}
	err = watchLocalDirs(watcher, src, src, filter)
	if err != nil {
		return nil, err
	}
	maps.DeleteFunc(t.digests, func(relPath, _ string) bool {
		return filter.excluded(relPath)
	})
	return filter, nil
}

// scheduleSync sets next to fire once local files stop changing for
// bundleSyncQuietPeriod, but no later than bundleSyncMaxDelay after pending,
// and returns pending, set to now if it was zero.
func scheduleSync(next *time.Timer, pending time.Time) time.Time {
	if pending.IsZero() {
		pending = time.Now()
	}
	next.Reset(min(bundleSyncQuietPeriod, time.Until(pending.Add(bundleSyncMaxDelay))))
	return pending
}

// syncFailed reports a failed sync, and returns err if it ends the watch: when
// the API refused the write for good, or src is gone. A write that could
// succeed later is tried again after bundleSyncRetryDelay, and a local file
// that cannot be synced waits for the change that fixes it.
func syncFailed(err error, src string, next *time.Timer, out BundleOutput) error {
	var writeErr *bundleWriteError
	if errors.As(err, &writeErr) {
		if !writeErr.retryable() {
			return err
		}
		ansi.Warn(out.Notes, fmt.Sprintf("failed to sync %s (%s); retrying in %s", src, err, bundleSyncRetryDelay))
		next.Reset(bundleSyncRetryDelay)
		return nil
	}
	_, statErr := os.Stat(src)
	if statErr != nil {
		return err
	}
	ansi.Warn(out.Notes, err.Error())
	return nil
}

// localChange reports whether event changed a path under src that a sync reads,
// and watches the directory the event created, if any.
func localChange(watcher *fsnotify.Watcher, src string, filter *localFileFilter, event fsnotify.Event) (bool, error) {
	if event.Op == fsnotify.Chmod {
		return false, nil
	}
	relPath, err := filepath.Rel(src, event.Name)
	if err != nil {
		return false, err
	}
	relPath = filepath.ToSlash(relPath)
	info, err := os.Lstat(event.Name)
	if err != nil || !info.IsDir() {
		return !filter.excluded(relPath), nil
	}
	if filter.skipDir(event.Name, relPath) {
		return false, nil
	}
	if event.Has(fsnotify.Create) {
		err = watchLocalDirs(watcher, src, event.Name, filter)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, errors.Wrapf(err, "failed to watch %s", event.Name)
		}
	}
	return true, nil
}

// watchLocalDirs adds dir, which is src or below it, and each directory below
// dir that filter keeps to watcher, refusing to watch more than maxWatchedDirs.
func watchLocalDirs(watcher *fsnotify.Watcher, src, dir string, filter *localFileFilter) error {
	watched := map[string]bool{}
	for _, dirPath := range watcher.WatchList() {
		watched[dirPath] = true
	}
	return filepath.WalkDir(dir, func(dirPath string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(src, dirPath)
		if err != nil {
			return err
		}
		if dirPath != src && filter.skipDir(dirPath, filepath.ToSlash(relPath)) {
			return fs.SkipDir
		}
		if watched[filepath.Clean(dirPath)] {
			return nil
		}
		if len(watched) == maxWatchedDirs {
			return fmt.Errorf("--watch watches at most %d directories; skip some with --exclude", maxWatchedDirs)
		}
		watched[filepath.Clean(dirPath)] = true
		return watcher.Add(dirPath)
	})
}

// retryable reports whether the same write could succeed later.
func (e *bundleWriteError) retryable() bool {
	return e.status == 0 || e.status == http.StatusRequestTimeout || e.status == http.StatusTooManyRequests || e.status >= http.StatusInternalServerError
}
