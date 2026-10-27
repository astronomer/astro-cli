package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// A project's uv.lock is seeded with the versions its runtime image ships,
// in the project itself: its pyproject.toml takes the image's versions as
// constraints for one `uv lock`, then goes back to what it was, and a second
// `uv lock` brings the lock's record of the project's settings up to date
// while uv keeps every version the lock already holds (its preferences).
// pyproject.toml carries none of those versions, only the record of which
// build and Python they were (scaffold.Seed).
//
// Locking in place is what makes every project lock as it does on its own: a
// dynamic version, a build backend, relative find-links or index paths, and
// a uv.toml (exclude-newer included) all apply as they always do.
//
// From then on uv keeps the versions on its own: `uv add`, `uv lock` and `uv
// sync` change a seeded version only where a requirement needs another. `uv
// lock --upgrade`, or deleting uv.lock, lets them go, on purpose; `astro
// init` seeds the lock again.

// lockFile is uv's lockfile, beside pyproject.toml.
const lockFile = "uv.lock"

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
// many packages the lock holds at the image's versions once it is. prepare,
// when not nil, writes what the seed is part of (the runtime group and its
// record) into pyproject.toml first, so the lock is seeded with the group's
// packages in it. None, with no error, when there is nothing to seed with.
// On an error, a canceled ctx included, and when there is nothing to seed,
// pyproject.toml and uv.lock are put back as they were before prepare.
//
// Every write of pyproject.toml is the manifest's own
// (scaffold.AddSeedConstraints, scaffold.RestoreManifest): a symlinked
// manifest stays a link, a read-only one is refused. The constraints are
// written between the comments that mark them as astro's, so a process
// killed before it puts the manifest back leaves nothing the next write of
// the manifest does not take out.
func (c *cli) seedLock(ctx context.Context, dir string, pins *scaffold.RuntimePins, prepare func() error) (int, error) {
	saved, err := saveProject(dir)
	if err != nil {
		return 0, err
	}
	if prepare != nil {
		if err := prepare(); err != nil {
			return 0, errors.Join(err, saved.restore(dir))
		}
	}
	prepared, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return 0, errors.Join(err, saved.restore(dir))
	}
	added, err := scaffold.AddSeedConstraints(dir, nil, pins)
	if err != nil || added == 0 {
		return 0, errors.Join(err, saved.restore(dir))
	}
	steps := []func() error{
		func() error {
			if err := c.d.Lock(ctx, dir); err != nil {
				return fmt.Errorf("resolving the image's versions: %w", err)
			}
			return nil
		},
		func() error { return scaffold.RestoreManifest(dir, nil, prepared) },
		ctx.Err,
		func() error {
			if err := c.d.Lock(ctx, dir); err != nil {
				return fmt.Errorf("locking the project with them: %w", err)
			}
			return nil
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			// What the seed left is not kept: the project is as it was before.
			return 0, errors.Join(err, saved.restore(dir))
		}
	}
	// A seed whose lock holds none of the image's versions seeded nothing, and
	// its record and group would say otherwise.
	held := heldPins(dir, scaffold.SeededPins(dir, pins))
	if held == 0 {
		return 0, errors.Join(errSeedHeldNothing, saved.restore(dir))
	}
	return held, nil
}

// errSeedHeldNothing is a seed whose lock, once resolved, holds none of the
// image's versions.
var errSeedHeldNothing = errors.New("uv resolved none of its versions")

// seedSkipsDir is where seeds a start does not try again are remembered,
// under the cache root, and seedSkipTTL how long: long enough that a project
// that does not seed is not locked on every start, short enough that one
// whose failure was the network's is tried again the next day. `astro init`
// always tries.
const (
	seedSkipsDir = "seed-skips"
	seedSkipTTL  = 24 * time.Hour
)

// seedSkipKey names a seed by everything that decides how it goes: the
// project's directory, its pyproject.toml and uv.lock as they are, and the
// build and Python of the list. "" when the project cannot be read.
func seedSkipKey(dir string, pins *scaffold.RuntimePins) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return ""
	}
	lockBytes, _ := os.ReadFile(filepath.Join(dir, lockFile)) //nolint:errcheck // no lock is an input too
	h := sha256.New()
	for _, part := range [][]byte{[]byte(abs), manifestBytes, lockBytes, []byte(pins.Build), []byte(pins.Python)} {
		fmt.Fprintf(h, "%d:", len(part))
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// seedSkipPath is where the seed key names is remembered, or "".
func seedSkipPath(key string) string {
	root, err := localrt.CacheRoot()
	if key == "" || err != nil {
		return ""
	}
	return filepath.Join(root, seedSkipsDir, key)
}

// seedSkipped reports a seed of the project in dir with pins that failed,
// held nothing, or could not write its group, with the same inputs, within
// seedSkipTTL.
func seedSkipped(dir string, pins *scaffold.RuntimePins) bool {
	path := seedSkipPath(seedSkipKey(dir, pins))
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < seedSkipTTL
}

// rememberSeedSkip remembers the seed of the project in dir with pins, as the
// project is now, for seedSkipped. A cache that cannot be written forgets it,
// which costs a seed and nothing else.
func rememberSeedSkip(dir string, pins *scaffold.RuntimePins) {
	path := seedSkipPath(seedSkipKey(dir, pins))
	if path == "" || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	_ = fsatomic.WriteFile(path, nil, 0o644) //nolint:errcheck // best effort, see above
}

// savedProject is a project's pyproject.toml and uv.lock as they were before
// a seed, to put back when it fails.
type savedProject struct {
	lockPath       string
	manifest, lock []byte
	// hadLock says there was a uv.lock to put back; without one, a lock the
	// seed wrote is removed.
	hadLock bool
}

// saveProject reads what restore puts back.
func saveProject(dir string) (*savedProject, error) {
	s := &savedProject{lockPath: filepath.Join(dir, lockFile)}
	var err error
	if s.manifest, err = os.ReadFile(filepath.Join(dir, manifest.Marker)); err != nil {
		return nil, err
	}
	s.lock, err = os.ReadFile(s.lockPath)
	switch {
	case err == nil:
		s.hadLock = true
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	return s, nil
}

// restore puts the project's pyproject.toml and uv.lock in dir back as they
// were, the manifest through its own write (scaffold.RestoreManifest).
func (s *savedProject) restore(dir string) error {
	err := scaffold.RestoreManifest(dir, nil, s.manifest)
	if s.hadLock {
		return errors.Join(err, fsatomic.WriteFile(s.lockPath, s.lock, lockFilePerm))
	}
	if rmErr := os.Remove(s.lockPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		err = errors.Join(err, rmErr)
	}
	return err
}

// workspaceRoot is the root of the uv workspace the project in dir is a member
// of, or "" when it is in none. uv locks a workspace once, at its root, so a
// member has no uv.lock of its own beside pyproject.toml: there is nothing
// there to seed, and a lock its `uv lock` writes would be the root's. A
// member is a project whose directory an ancestor's [tool.uv.workspace]
// members glob names and its exclude does not.
func workspaceRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for parent := filepath.Dir(abs); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		data, err := os.ReadFile(filepath.Join(parent, manifest.Marker))
		if err != nil {
			continue
		}
		var doc struct {
			Tool struct {
				UV struct {
					Workspace *struct {
						Members []string `toml:"members"`
						Exclude []string `toml:"exclude"`
					} `toml:"workspace"`
				} `toml:"uv"`
			} `toml:"tool"`
		}
		if toml.Unmarshal(data, &doc) != nil || doc.Tool.UV.Workspace == nil {
			continue
		}
		rel, err := filepath.Rel(parent, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if globsMatch(doc.Tool.UV.Workspace.Members, rel) && !globsMatch(doc.Tool.UV.Workspace.Exclude, rel) {
			return parent
		}
	}
	return ""
}

// globsMatch reports whether one of globs, as a workspace's members and
// exclude write them ("packages/*"), names rel, a slash-separated path.
func globsMatch(globs []string, rel string) bool {
	for _, g := range globs {
		g = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(g), "./"), "/")
		if ok, err := path.Match(g, rel); err == nil && ok {
			return true
		}
	}
	return false
}

// lockFilePerm is uv.lock's mode, as uv writes it.
const lockFilePerm = 0o644

// hasLock reports a uv.lock in dir.
func hasLock(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, lockFile))
	return err == nil
}

// lockedVersions are the versions the uv.lock in dir holds of each
// distribution, by name (manifest.DistName). nil when there is no lock to
// read.
func lockedVersions(dir string) map[string][]string {
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return nil
	}
	var lock struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil
	}
	locked := map[string][]string{}
	for _, p := range lock.Package {
		name := manifest.DistName(p.Name)
		locked[name] = append(locked[name], p.Version)
	}
	return locked
}

// heldPins counts the seeded pins the uv.lock in dir holds at their version:
// what a seed is reported as.
func heldPins(dir string, seeded []string) int {
	locked := lockedVersions(dir)
	held := 0
	for _, pin := range seeded {
		name, version, _ := strings.Cut(pin, "==")
		if holds(locked[manifest.DistName(name)], version) {
			held++
		}
	}
	return held
}

// lockDrift counts the packages the uv.lock in dir holds at a version other
// than pins', among the ones a seed holds it to (scaffold.SeededPins, which
// leaves out what the project chooses itself): what has drifted from the
// image since the lock was seeded, or what a lock that was never seeded
// holds apart from it. A package the lock does not hold is no drift: a seed
// adds none. 0 when there is no lock to read.
func lockDrift(dir string, pins *scaffold.RuntimePins) int {
	locked := lockedVersions(dir)
	drift := 0
	for _, pin := range scaffold.SeededPins(dir, pins) {
		name, version, _ := strings.Cut(pin, "==")
		versions, held := locked[manifest.DistName(name)]
		if held && !holds(versions, version) {
			drift++
		}
	}
	return drift
}

// holds reports whether one of the versions a lock holds of a package is
// version, as PEP 440 orders them (manifest.SameVersion): pip freeze and
// uv.lock can spell one version two ways.
func holds(locked []string, version string) bool {
	return slices.ContainsFunc(locked, func(v string) bool { return manifest.SameVersion(v, version) })
}
