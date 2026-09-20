//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// ErrNoEnvironment reports a project that has no venv to install into.
//
// A sentinel because a consumer has to tell it apart from a resolution failure
// to say anything useful: this one means "start it first", the other means
// "your dependencies do not agree". Every other refusal in this package is a
// sentinel for the same reason — the alternative is substring-matching a
// message, which puts the consumer's logic inside someone else's string.
var ErrNoEnvironment = errors.New("this project has no environment to install into yet")

// markerMode is owner-only, matching the mode pkg/uv writes the marker at: it
// says this venv finished installing, and a reader that trusts it skips the
// sync that would rebuild it.
const markerMode = 0o600

// HotInstall installs dependencies into a project's existing environment
// without restarting the Airflow running on it.
//
// Installing rather than re-syncing is the point. EnsureSynced resolves the
// whole declared set and removes what the manifest no longer names, which for a
// live scheduler means deleting a package out from under a Dag mid-parse. This
// asks for the named requirements and leaves the rest of the manifest alone.
//
// It is NOT free of replacement, which matters to anyone reading this to decide
// whether it is safe. `uv pip install` will uninstall a conflicting version of
// something already present if a new requirement demands it, so adding a
// package that pins a different pydantic than the running Airflow has will swap
// that pydantic under the live process. That is inherent to installing into a
// running environment. What this avoids is the much larger blast radius of
// reconciling everything the manifest no longer mentions; a caller that can
// tolerate no replacement at all should restart instead.
//
// Afterwards the scheduler is nudged: Airflow re-parses a Dag when its mtime
// moves, and that is what makes the new package visible without a restart. It
// is also why this belongs here rather than being a bare `uv pip install` at
// the call site.
func (e *Engine) HotInstall(ctx context.Context, projectPath string, deps []string, cb rt.Callbacks) error {
	if len(deps) == 0 {
		// Nothing declared is not a failure: a caller watching a manifest cannot
		// know the project has no dependencies before it asks.
		return nil
	}

	projectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", projectPath, err)
	}

	venvPython := filepath.Join(projectPath, ".venv", "bin", "python")
	if _, err := os.Stat(venvPython); err != nil {
		return fmt.Errorf("%w (%s): start it first", ErrNoEnvironment, venvPython)
	}

	emit, out, errW, flush := e.uvProgress(cb)
	defer flush()

	client, err := e.uv(ctx, emit)
	if err != nil {
		return err
	}

	// The marker is dropped for the reason EnsureSynced drops it around a sync:
	// it is the claim that this venv finished installing, and that claim has to
	// be false while one is in progress. Without this, an interrupted hot
	// install leaves half-written dist-info under an intact marker, and the next
	// start believes the marker and skips the wipe that would have repaired it.
	marker := filepath.Join(projectPath, ".venv", uv.MarkerName)
	hadMarker := removeIfPresent(marker)

	// projectPath, not "": uv discovers [tool.uv] configuration by walking up
	// from its working directory, so running anywhere else finds the caller's
	// and a project resolving from a private index quietly gets the public one.
	if err := client.PipInstall(ctx, projectPath, venvPython, deps, "", uv.Stdio{Out: out, Err: errW}); err != nil {
		return fmt.Errorf("installing into the running environment: %w", err)
	}
	if hadMarker {
		restoreMarker(marker)
	}

	touched, skipped := touchDags(projectPath, e.now())
	switch {
	case touched == 0 && skipped == 0:
		// Worth saying rather than passing in silence: the install worked, and
		// the absence of a nudge is the difference between "this is live now"
		// and "this is live after a restart".
		emit(rt.LogLine{Component: "uv", Text: "installed, but found no Dag files to re-parse"})
	case skipped > 0:
		emit(rt.LogLine{Component: "uv", Text: fmt.Sprintf("installed and re-parsed %d Dag files; %d could not be touched and need a restart to pick this up", touched, skipped)})
	}
	return nil
}

// uvProgress builds the emitter and the writers uv's output flows through.
//
// Out and Err get separate writers deliberately. os/exec copies the two streams
// on their own goroutines and rt.LineWriter buffers without a lock, so handing
// one writer to both is a data race — a corrupted line, or a panic, under an
// install chatty enough to interleave them.
func (e *Engine) uvProgress(cb rt.Callbacks) (emit func(rt.LogLine), out, errW *rt.LineWriter, flush func()) {
	emit = func(l rt.LogLine) {
		if l.Time.IsZero() {
			l.Time = e.now()
		}
		if cb.OnLine != nil {
			cb.OnLine(l)
		}
	}
	line := func(s string) { emit(rt.LogLine{Component: "uv", Text: s}) }
	out = &rt.LineWriter{Emit: line}
	errW = &rt.LineWriter{Emit: line}
	return emit, out, errW, func() { out.Flush(); errW.Flush() }
}

// removeIfPresent deletes path, reporting whether it was there to begin with.
func removeIfPresent(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return os.Remove(path) == nil
}

// restoreMarker puts the completion marker back after a successful install. A
// failure to rewrite it costs a needless resync on the next start, which is the
// safe direction and not worth failing an install that already worked.
func restoreMarker(path string) {
	//nolint:gosec,errcheck // G703: the path is built here from the project root, and a failed rewrite is deliberate, for the reason above
	_ = os.WriteFile(path, nil, markerMode)
}

// touchDags bumps the mtime of every Dag file so the scheduler re-parses them
// against the packages that just arrived, and reports how it went.
//
// Recursive, because dags/<subdir>/*.py is the ordinary layout for anything
// past a scaffold. Walking only the top level nudged none of those and still
// reported success, which is precisely the outcome the nudge exists to prevent.
//
// Never fails. A project with no dags directory is ordinary, and a file that
// cannot be touched is still picked up by the scheduler's next scan; undoing an
// install that already worked would be the worse answer. The counts are what
// let the caller say so out loud instead.
func touchDags(projectPath string, now time.Time) (touched, skipped int) {
	dagsDir := filepath.Join(projectPath, "dags")
	//nolint:errcheck // never fails, for the reason on this function
	_ = filepath.WalkDir(dagsDir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		case d.IsDir() || !strings.HasSuffix(d.Name(), ".py"):
			return nil
		}
		if os.Chtimes(path, now, now) != nil {
			skipped++
			return nil
		}
		touched++
		return nil
	})
	return touched, skipped
}
