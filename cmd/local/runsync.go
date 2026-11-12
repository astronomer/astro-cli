package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// syncQuietFor is how long a sync of an existing venv may run before its output
// is shown. uv sync always prints a "Resolved/Audited" summary, and on the
// common case, a venv that is already current, that is two lines of noise on
// every `astro local run`. A sync that takes longer than this is installing
// something, and is worth narrating.
const syncQuietFor = 2 * time.Second

// uvNoSyncSet reports whether UV_NO_SYNC asks for no sync. It is uv's own
// variable for `uv run --no-sync`, read here with uv's boolean spellings, so a
// CI image that already sets it keeps the behavior it asked uv for.
// UV_FROZEN is not read: it changes how uv sync treats the lockfile, which the
// sync below inherits through the environment, and does not skip the sync.
func uvNoSyncSet() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("UV_NO_SYNC")))
	return slices.Contains(strings.Fields("1 true yes on y t"), v)
}

// syncEnvironment builds or updates the project's venv before a command runs
// in it, the way `uv run` does. Everything it reports goes to stderr: stdout
// belongs to the wrapped command.
func (c *cli) syncEnvironment(ctx context.Context, p localrt.Plan) error {
	_, statErr := os.Stat(filepath.Join(p.ProjectPath, ".venv", "bin", "python"))
	first := statErr != nil
	rep := &syncReporter{out: c.d.Stderr, quietFor: syncQuietFor, now: time.Now, started: time.Now()}
	if first {
		fmt.Fprintln(c.d.Stderr, "Building this project's environment (the first run can take a few minutes)...")
		rep.show = true
	}
	err := c.d.Runtime.Sync(ctx, p, localrt.Callbacks{OnLine: rep.line})
	if err != nil {
		rep.flush()
	}
	return err
}

// syncReporter writes a sync's output lines to out, holding them back until
// the sync has run for quietFor (or fails, or show is set up front).
type syncReporter struct {
	out      io.Writer
	quietFor time.Duration
	now      func() time.Time

	mu      sync.Mutex
	started time.Time
	show    bool
	held    []string
}

func (r *syncReporter) line(l localrt.LogLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.show && r.now().Sub(r.started) >= r.quietFor {
		r.flushLocked()
	}
	if r.show {
		fmt.Fprintln(r.out, l.Text)
		return
	}
	r.held = append(r.held, l.Text)
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
