package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// syncQuietFor is how long a sync of a complete venv may run before it is
// narrated. uv sync always prints a "Resolved/Audited" summary, and on the
// common case, a venv that is already current, that is two lines of noise on
// every `astro local run`. A sync that takes longer than this is installing
// something, and is worth saying so.
const syncQuietFor = 2 * time.Second

// syncLockWait bounds how long a run waits for another astro command that is
// preparing the same project's environment, and syncLockPoll is how often it
// asks again.
//
// Waiting rather than failing, because the other command is building exactly
// what this one needs and the retry is then a quick no-op. Not waiting forever,
// because a start that never finishes should not hang a CI job silently.
const (
	syncLockWait = 5 * time.Minute
	syncLockPoll = time.Second
)

// uvNoSyncSet reports whether UV_NO_SYNC asks for no sync. It is uv's own
// variable for `uv run --no-sync`, read here with uv's boolean spellings, so a
// CI image that already sets it keeps the behavior it asked uv for.
// UV_FROZEN is not read: it changes how uv sync treats the lockfile, which the
// sync below inherits through the environment, and does not skip the sync.
func uvNoSyncSet() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("UV_NO_SYNC")))
	return slices.Contains(strings.Fields("1 true yes on y t"), v)
}

// venvState is what the project's .venv is, as the sync will find it.
type venvState int

const (
	venvNone     venvState = iota // no .venv
	venvUnmarked                  // a .venv astro did not finish building
	venvComplete                  // a .venv carrying astro's install marker
)

// venvStateOf reads the state from the marker uv.EnsureSynced writes, which is
// also what it decides on: a venv without it is wiped and rebuilt.
func venvStateOf(project string) venvState {
	venv := filepath.Join(project, ".venv")
	if _, err := os.Stat(venv); err != nil {
		return venvNone
	}
	if _, err := os.Stat(filepath.Join(venv, uv.MarkerName)); err != nil {
		return venvUnmarked
	}
	return venvComplete
}

// syncer carries the seams a test replaces: the clock-driven pieces.
type syncer struct {
	c     *cli
	after func(d time.Duration, f func()) (stop func())
	sleep func(ctx context.Context, d time.Duration) error
	wait  time.Duration
}

func realAfter(d time.Duration, f func()) func() {
	t := time.AfterFunc(d, f)
	return func() { t.Stop() }
}

func realSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// syncEnvironment builds or updates the project's venv before a command runs
// in it, the way `uv run` does. Everything it reports goes to stderr: stdout
// belongs to the wrapped command.
func (c *cli) syncEnvironment(ctx context.Context, p localrt.Plan) error {
	s := syncSeams
	s.c = c
	return s.run(ctx, p)
}

// syncSeams is the production wiring of the syncer; a test replaces it.
var syncSeams = syncer{after: realAfter, sleep: realSleep, wait: syncLockWait}

func (s *syncer) run(ctx context.Context, p localrt.Plan) error {
	stderr := s.c.d.Stderr
	rep := &syncReporter{out: stderr}
	switch venvStateOf(p.ProjectPath) {
	case venvNone:
		fmt.Fprintln(stderr, "Building this project's environment (the first run can take a few minutes)...")
		rep.show = true
	case venvUnmarked:
		// EnsureSynced wipes a venv astro did not mark complete, as a start
		// does. Say so rather than do it quietly.
		fmt.Fprintln(stderr, "Rebuilding this project's environment (it was not built by astro)...")
		rep.show = true
	case venvComplete:
		rep.note = "Updating this project's environment..."
		stop := s.after(syncQuietFor, rep.slow)
		defer stop()
	}

	deadline := time.Now().Add(s.wait)
	waited := false
	for {
		err := s.c.d.Runtime.Sync(ctx, p, localrt.Callbacks{OnLine: rep.line})
		if !errors.Is(err, localrt.ErrStartInProgress) {
			if err != nil {
				rep.flush()
			}
			return err
		}
		// Another astro command holds the project's lock. A complete venv is
		// usable as it is: nothing is writing to it unless that command is
		// mid-sync, which drops the marker, and then waiting is the right call.
		if venvStateOf(p.ProjectPath) == venvComplete {
			fmt.Fprintln(stderr, "Another astro command is preparing this project's environment; using it as it is.")
			return nil
		}
		if !waited {
			fmt.Fprintln(stderr, "Waiting for another astro command to finish preparing this project's environment...")
			waited = true
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: waited %s for another astro command to finish preparing this project's environment; try again once it ends", err, s.wait)
		}
		if serr := s.sleep(ctx, syncLockPoll); serr != nil {
			return serr
		}
	}
}

// syncReporter writes a sync's output lines to out. Held back until slow is
// called (by a timer, while the sync is still going) or the sync fails, unless
// show is set up front.
type syncReporter struct {
	out  io.Writer
	note string // said once when slow fires with lines still held back

	mu   sync.Mutex
	show bool
	held []string
}

func (r *syncReporter) line(l localrt.LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.show {
		fmt.Fprintln(r.out, l.Text)
		return
	}
	r.held = append(r.held, l.Text)
}

// slow is the timer's call: the sync is taking long enough that silence would
// read as a hang, so say what it is doing and show what uv has said so far.
func (r *syncReporter) slow() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.show {
		return
	}
	if r.note != "" {
		fmt.Fprintln(r.out, r.note)
	}
	r.flushLocked()
}

// flush shows what was held back; a failed sync needs all of it.
func (r *syncReporter) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushLocked()
}

func (r *syncReporter) flushLocked() {
	r.show = true
	for _, t := range r.held {
		fmt.Fprintln(r.out, t)
	}
	r.held = nil
}
