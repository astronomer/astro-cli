package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// lockFile is the lockfile uv writes beside pyproject.toml.
const lockFile = "uv.lock"

// uvLock is the production Deps.Lock: uv discovered and cached as a
// standalone start discovers it, so the lock resolves from the same cache.
func uvLock(ctx context.Context, dir string) error {
	root, err := localrt.CacheRoot()
	if err != nil {
		return err
	}
	client, err := uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
	if err != nil {
		return err
	}
	return client.Lock(ctx, dir, uv.Stdio{})
}

// lockAfterInit runs `uv lock` once init has changed [tool.uv], so uv.lock
// follows what init wrote: created for a new project, updated for one that
// had a lock. Without uv it asks for the lock to be run; a lock that fails
// warns with uv's reason. Neither fails init, which has done its part. On
// stderr as text in either format, since init's stdout is its one result.
func (c *cli) lockAfterInit(ctx context.Context, dir string) {
	o := c.relock(ctx, dir)
	switch {
	case o.Status == lockFailed:
		// The CLI's stderr Warning. Not emitWarning: in json mode that would put
		// a warning event on stdout ahead of init's one result.
		cliout.Warn(c.d.Stderr, o.text())
	case o.text() != "":
		fmt.Fprintln(c.d.Stderr, o.text())
	}
}

// What a relock did to uv.lock.
const (
	lockLocked    = "locked"    // created it
	lockUpdated   = "updated"   // rewrote it
	lockUnchanged = "unchanged" // left it as it was
	lockNoUV      = "no_uv"     // found no uv to run
	lockFailed    = "failed"    // uv lock failed
)

// lockOutcome is what relock did, for its caller to report.
type lockOutcome struct {
	Status string `json:"status"`
	// Changed counts the packages an update added, removed or moved.
	Changed int `json:"packages_changed,omitempty"`
	// Error is uv's reason, for lockFailed.
	Error string `json:"error,omitempty"`
	// hadLock reports a uv.lock before the run.
	hadLock bool
}

// relock runs `uv lock` in dir, so uv.lock follows a [project] or [tool.uv]
// change. Nothing (a zero outcome) when this build has no way to lock.
func (c *cli) relock(ctx context.Context, dir string) lockOutcome {
	if c.d.Lock == nil {
		return lockOutcome{}
	}
	before, hadLock := readLock(dir)
	err := c.d.Lock(ctx, dir)
	switch {
	case errors.Is(err, uv.ErrNotFound):
		return lockOutcome{Status: lockNoUV, hadLock: hadLock}
	case err != nil:
		return lockOutcome{Status: lockFailed, Error: firstLine(err.Error()), hadLock: hadLock}
	}
	after, _ := readLock(dir)
	switch {
	case !hadLock:
		return lockOutcome{Status: lockLocked}
	case before != after:
		return lockOutcome{Status: lockUpdated, Changed: lockChanges(before, after), hadLock: true}
	}
	return lockOutcome{Status: lockUnchanged, hadLock: true}
}

// text is the uv.lock line for an outcome, "" when there is nothing to say:
// no lock run, one that changed nothing, or no uv and no lock to fall behind.
func (o lockOutcome) text() string {
	switch o.Status {
	case lockNoUV:
		if o.hadLock {
			return fmt.Sprintf("uv.lock: run uv lock so it follows %s", manifest.Marker)
		}
	case lockFailed:
		return fmt.Sprintf("uv.lock: uv lock failed: %s; run uv lock so it follows %s", o.Error, manifest.Marker)
	case lockLocked:
		return "uv.lock: locked"
	case lockUpdated:
		switch o.Changed {
		case 0:
			return "uv.lock: updated"
		case 1:
			return "uv.lock: updated (1 package changed)"
		default:
			return fmt.Sprintf("uv.lock: updated (%d packages changed)", o.Changed)
		}
	}
	return ""
}

// readLock is the project's uv.lock and whether it exists.
func readLock(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// lockChanges counts the packages a lock rewrite added, removed or moved to
// another version, or 0 when either lock does not parse.
func lockChanges(before, after string) int {
	b, okB := lockVersions(before)
	a, okA := lockVersions(after)
	if !okB || !okA {
		return 0
	}
	n := 0
	for name, v := range a {
		if b[name] != v {
			n++
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			n++
		}
	}
	return n
}

// lockVersions maps each locked package to its versions, joined: a lock
// that resolves one package to two versions (one per Python) lists both.
func lockVersions(data string) (map[string]string, bool) {
	var lock struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal([]byte(data), &lock); err != nil {
		return nil, false
	}
	out := make(map[string]string, len(lock.Package))
	for _, p := range lock.Package {
		if out[p.Name] != "" {
			out[p.Name] += "," + p.Version
		} else {
			out[p.Name] = p.Version
		}
	}
	return out, true
}
