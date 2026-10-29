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
	if c.d.Lock == nil {
		return
	}
	before, hadLock := readLock(dir)
	err := c.d.Lock(ctx, dir)
	switch {
	case errors.Is(err, uv.ErrNotFound):
		if hadLock {
			fmt.Fprintf(c.d.Stderr, "uv.lock: run uv lock so it follows %s\n", manifest.Marker)
		}
		return
	case err != nil:
		// The CLI's stderr Warning. Not emitWarning: in json mode that would put
		// a warning event on stdout ahead of init's one result.
		cliout.Warn(c.d.Stderr, fmt.Sprintf("uv.lock: uv lock failed: %s; run uv lock so it follows %s", firstLine(err.Error()), manifest.Marker))
		return
	}
	after, _ := readLock(dir)
	switch {
	case !hadLock:
		fmt.Fprintln(c.d.Stderr, "uv.lock: locked")
	case before != after:
		switch n := lockChanges(before, after); n {
		case 0:
			fmt.Fprintln(c.d.Stderr, "uv.lock: updated")
		case 1:
			fmt.Fprintln(c.d.Stderr, "uv.lock: updated (1 package changed)")
		default:
			fmt.Fprintf(c.d.Stderr, "uv.lock: updated (%d packages changed)\n", n)
		}
	}
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
