package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// A project's uv.lock is seeded with the versions its runtime image ships:
// resolved once with them as constraints, in a scratch copy of the project,
// then copied back and locked once more in the project itself, where uv keeps
// every version the lock already holds (its preferences) and only brings the
// lock's record of the project's settings up to date. pyproject.toml carries
// none of those versions, only the record of which build and Python they were
// (scaffold.Seed).
//
// From then on uv keeps them on its own: `uv add`, `uv lock` and `uv sync`
// change a seeded version only where a requirement needs another. `uv lock
// --upgrade`, or deleting uv.lock, lets them go, on purpose; `astro init`
// seeds the lock again.

// lockFile is uv's lockfile, beside pyproject.toml.
const lockFile = "uv.lock"

// seedLockCopied are the files beside pyproject.toml the scratch copy takes:
// the lock, whose versions the seed starts from and keeps where it can, and
// what else uv reads there to lock (its own settings, the Python pin).
var seedLockCopied = []string{lockFile, "uv.toml", ".python-version"}

// lockProject is the production Deps.Lock: `uv lock` in dir, through the uv
// the CLI finds, sharing the CLI's uv cache.
func lockProject(ctx context.Context, dir string) error {
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

// seedLock seeds the uv.lock of the project in dir with pins, and reports how
// many of the image's versions it held the lock to. None, with no error, when
// there is nothing to seed with. On an error the project's lock is as it was.
func (c *cli) seedLock(ctx context.Context, dir string, pins *scaffold.RuntimePins) (int, error) {
	src, n, err := scaffold.SeedManifest(dir, pins)
	if err != nil || n == 0 {
		return 0, err
	}
	scratch, err := os.MkdirTemp("", "astro-seed-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(scratch) //nolint:errcheck // a scratch copy left in the temp dir costs nothing
	if err := os.WriteFile(filepath.Join(scratch, manifest.Marker), src, scratchFilePerm); err != nil {
		return 0, err
	}
	for _, name := range seedLockCopied {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(scratch, name), data, scratchFilePerm); err != nil { //nolint:gosec // name is one of seedLockCopied
			return 0, err
		}
	}
	if err := c.d.Lock(ctx, scratch); err != nil {
		return 0, fmt.Errorf("resolving the image's versions: %w", err)
	}
	seeded, err := os.ReadFile(filepath.Join(scratch, lockFile))
	if err != nil {
		return 0, err
	}
	path := filepath.Join(dir, lockFile)
	original, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return 0, readErr
	}
	if err := fsatomic.WriteFile(path, seeded, lockFilePerm); err != nil {
		return 0, err
	}
	if err := c.d.Lock(ctx, dir); err != nil {
		// The project does not lock with the seeded versions; leave it as it
		// was, which is what it locks to without them.
		var restoreErr error
		if readErr == nil {
			restoreErr = fsatomic.WriteFile(path, original, lockFilePerm)
		} else {
			restoreErr = os.Remove(path)
		}
		return 0, errors.Join(fmt.Errorf("locking the project with them: %w", err), restoreErr)
	}
	return n, nil
}

const (
	// scratchFilePerm is the mode of the scratch copy's files, read by this
	// user's uv alone.
	scratchFilePerm = 0o600
	// lockFilePerm is uv.lock's mode, as uv writes it.
	lockFilePerm = 0o644
)

// hasLock reports a uv.lock in dir.
func hasLock(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, lockFile))
	return err == nil
}

// lockDrift counts the packages the uv.lock in dir holds at a version other
// than pins', where the project does not choose another itself
// (scaffold.Excludes): what has drifted from the image since the lock was
// seeded, or what a lock that was never seeded holds apart from it. A
// package the lock does not hold is no drift: a seed adds none. 0 when there
// is no lock to read.
func lockDrift(dir string, pins *scaffold.RuntimePins) int {
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return 0
	}
	var lock struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return 0
	}
	locked := map[string][]string{}
	for _, p := range lock.Package {
		name := manifest.DistName(p.Name)
		locked[name] = append(locked[name], p.Version)
	}
	own := scaffold.Excludes(dir, pins.Pins)
	drift := 0
	for _, pin := range pins.Pins {
		name, version, ok := strings.Cut(pin, "==")
		name = manifest.DistName(name)
		if !ok || own[name] || strings.Contains(version, "+astro.") {
			continue
		}
		versions, held := locked[name]
		if held && !slices.Contains(versions, strings.TrimSpace(version)) {
			drift++
		}
	}
	return drift
}
