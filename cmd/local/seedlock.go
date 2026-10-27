package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
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
// many packages the lock holds at the image's versions once it is. None, with
// no error, when there is nothing to seed with. On an error, a canceled ctx
// included, pyproject.toml and uv.lock are put back as they were.
//
// Every write of pyproject.toml is the manifest's own
// (scaffold.AddSeedConstraints, scaffold.RestoreManifest): a symlinked
// manifest stays a link, a read-only one is refused. The constraints are
// written between the comments that mark them as astro's, so a process
// killed before it puts the manifest back leaves nothing the next write of
// the manifest does not take out.
//
// The image's versions are first held on every Python requires-python
// allows, so the lock resolves once rather than forking per Python, which
// would hold every package the image ships twice. When that leaves uv
// nothing to lock (a version that needs a newer Python than requires-python
// allows), the project is put back and the seed holds them on the image's
// Python alone (scaffold.RuntimePins.OnlyImagePython); narrowed then says
// why, and is nil when the first one held.
func (c *cli) seedLock(ctx context.Context, dir string, pins *scaffold.RuntimePins) (held int, narrowed, err error) {
	everywhere := *pins
	everywhere.OnlyImagePython = false
	held, err = c.seedOnce(ctx, dir, &everywhere)
	var resolution *uv.ResolutionError
	if err == nil || !errors.As(err, &resolution) || ctx.Err() != nil {
		return held, nil, err
	}
	imageOnly := *pins
	imageOnly.OnlyImagePython = true
	held, err = c.seedOnce(ctx, dir, &imageOnly)
	if err != nil {
		return 0, nil, err
	}
	return held, resolution, nil
}

// seedOnce is one try of seedLock: pins written in, the lock resolved, pins
// taken out and the lock resolved again, with the project put back as it was
// on any error.
func (c *cli) seedOnce(ctx context.Context, dir string, pins *scaffold.RuntimePins) (int, error) {
	saved, err := saveProject(dir)
	if err != nil {
		return 0, err
	}
	added, err := scaffold.AddSeedConstraints(dir, nil, pins)
	if err != nil || added == 0 {
		return 0, err
	}
	steps := []func() error{
		func() error {
			if err := c.d.Lock(ctx, dir); err != nil {
				return fmt.Errorf("resolving the image's versions: %w", err)
			}
			return nil
		},
		func() error { return scaffold.RestoreManifest(dir, nil, saved.manifest) },
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
	return heldPins(dir, scaffold.SeededPins(dir, pins)), nil
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
