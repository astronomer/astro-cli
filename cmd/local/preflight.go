package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// uvProvisioner is the production checks.Provisioner: it builds scratch check
// venvs with uv, caches them under the astro cache dir, and resolves MWAA
// constraints with uv's resolver. It holds no per-run state, so one instance
// serves every target in a run.
type uvProvisioner struct {
	client   *uv.Client
	cacheDir string // <astro cache>/check-venvs
	// fetch retrieves a constraints file; a field so tests need no network.
	fetch func(ctx context.Context, url string) ([]byte, error)
}

// newUVProvisioner wires uv against the shared astro cache. It is a package var
// so a test can supply a fake without a real uv binary or network.
var newUVProvisioner = func(ctx context.Context) (checks.Provisioner, error) {
	root, err := localrt.CacheRoot()
	if err != nil {
		return nil, err
	}
	client, err := uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
	if err != nil {
		return nil, err
	}
	return &uvProvisioner{
		client:   client,
		cacheDir: filepath.Join(root, "check-venvs"),
		fetch:    httpFetch,
	}, nil
}

// venvMarker names a fully provisioned check venv. Its absence means a partial
// or missing install, so a run that finds no marker rebuilds from scratch.
//
// Its mtime is also the venv's last use, stamped on every cache hit, which is
// what sweepCheckVenvs ages by.
const venvMarker = ".check-complete"

// venvConstraints names the constraints file written into a check venv whose
// project declares [tool.uv] constraint-dependencies, for uv pip install to read.
const venvConstraints = ".check-constraints.txt"

// venvUnusedFor is how long a cached check environment survives without being
// used.
//
// It needs a policy because nothing else collects one. The key covers the whole
// requirement set, so every Airflow bump and every added dependency lands in a
// fresh directory — measured at 201 MB each — and the old one would otherwise
// stay for good. `astro local list --clean` is not the answer: it prunes
// runtime records, and a check environment is not one.
//
// Two weeks, because the costs are lopsided. Keeping a venv nobody wants costs
// 200 MB until somebody goes looking for it; dropping one that is wanted costs
// the rebuild below, measured at ~5s against a warm uv download cache.
const (
	venvUnusedDays = 14
	venvUnusedFor  = venvUnusedDays * 24 * time.Hour
)

const (
	// cacheDirPerms and markerPerms mirror the modes pkg/uv and the rest of the
	// cache tree use: a normal directory, a private marker file.
	cacheDirPerms = 0o755
	markerPerms   = 0o600
	// constraintsFetchTimeout bounds the constraints-file download, so an
	// offline or slow network degrades to a skip rather than hanging the check.
	constraintsFetchTimeout = 15 * time.Second
	// constraintsSizeLimit caps the constraints-file read; Airflow's own files
	// are well under a megabyte, so 8 MiB is generous headroom.
	constraintsSizeLimit = 8 << 20
)

// EnsureVenv returns the interpreter for a check venv matching spec, building
// it on a cache miss and reusing it on a hit. The cache key covers the Python
// version and the full requirement set (the Airflow pin among them), so a
// changed dependency or version lands in a fresh directory.
func (p *uvProvisioner) EnsureVenv(ctx context.Context, spec checks.VenvSpec, progress func(string)) (string, error) {
	dir := filepath.Join(p.cacheDir, p.key(spec))
	python := checks.VenvInterpreter(dir)
	marker := filepath.Join(dir, venvMarker)

	if _, err := os.Stat(marker); err == nil {
		// Stamp the use, which is what sweepCheckVenvs ages by and what keeps
		// a venv somebody relies on daily from being collected. It also makes
		// a concurrent sweep safe: a venv in use carries a fresh marker.
		now := time.Now()
		if cerr := os.Chtimes(marker, now, now); cerr != nil {
			// Say so rather than swallow it. Unstamped, this venv is deleted
			// venvUnusedFor after it was BUILT however often it is used, and
			// then rebuilt, and then deleted again — a recurring full
			// provision with nothing to explain it.
			progress(fmt.Sprintf("could not record the use of the cached check environment, so it will be rebuilt periodically: %v", cerr))
		}
		progress(fmt.Sprintf("reusing the cached check environment for Airflow %s", spec.Airflow))
		return python, nil
	}
	// A directory without the marker is a half-built venv; clear it so the
	// rebuild starts clean.
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clearing a stale check environment: %w", err)
	}
	if err := os.MkdirAll(p.cacheDir, cacheDirPerms); err != nil {
		return "", fmt.Errorf("preparing the check-environment cache: %w", err)
	}

	progress(fmt.Sprintf("provisioning a check environment for Airflow %s (first run installs packages; later runs reuse the cache)", spec.Airflow))
	if err := p.client.VenvAt(ctx, dir, spec.Python, uv.Stdio{}); err != nil {
		return "", err
	}
	// No project directory, deliberately: this is a scratch environment in the
	// cache, not a project, so uv should not pick up whatever [tool.uv] the
	// caller happens to be sitting in. The project's own constraints arrive in
	// the spec instead, written beside the venv for uv to read.
	constraints := ""
	if len(spec.Constraints) > 0 {
		constraints = filepath.Join(dir, venvConstraints)
		if err := os.WriteFile(constraints, []byte(strings.Join(spec.Constraints, "\n")+"\n"), markerPerms); err != nil {
			return "", fmt.Errorf("writing the check environment's constraints: %w", err)
		}
	}
	if err := p.client.PipInstall(ctx, "", python, spec.Reqs, constraints, uv.Stdio{}); err != nil {
		return "", err
	}
	if err := os.WriteFile(marker, nil, markerPerms); err != nil {
		return "", fmt.Errorf("marking the check environment ready: %w", err)
	}
	return python, nil
}

// sweepCheckVenvs removes cached check environments nothing has used for
// venvUnusedFor, and the wreckage of builds that died. Its progress notes name
// what it removed.
//
// Called once when a run is FINISHED with the cache, never from inside
// EnsureVenv. One `astro local check` can resolve several environments — one
// per --target, which map to different Airflow versions and so to different
// keys — and a sweep between two of them deletes what the next one is about to
// use, then watches it rebuild in the same command.
//
// Which is also why there is no set of live keys to carry around: after the
// run, everything it touched carries a marker stamped seconds ago, so plain
// age is enough.
//
// Best-effort throughout. A sweep is a side effect of the work the caller
// asked for, so an unreadable cache, or one entry that will not go, must not
// fail a check that otherwise passed.
func sweepCheckVenvs(progress func(string)) {
	root, err := localrt.CacheRoot()
	if err != nil {
		return
	}
	cacheDir := filepath.Join(root, "check-venvs")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-venvUnusedFor)
	var unused, wreckage int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), venvRemoving) {
			// Leftovers from a sweep whose delete did not finish. Retried
			// every sweep until they go, and not counted: the entry they came
			// from was reported removed when it was renamed.
			//nolint:errcheck // best-effort, and tried again next time
			os.RemoveAll(filepath.Join(cacheDir, e.Name()))
			continue
		}
		// Only what this code creates: a directory whose name is a key it
		// would produce. Anything else beside the venvs — a lock, an index, a
		// staging directory a later change adds — is not this function's to
		// delete.
		if !e.IsDir() || !isVenvKey(e.Name()) {
			continue
		}
		dir := filepath.Join(cacheDir, e.Name())
		stale, built := venvStale(dir, cutoff)
		if !stale {
			continue
		}
		if !removeVenv(dir) {
			continue
		}
		if built {
			unused++
		} else {
			wreckage++
		}
	}
	switch {
	case unused > 0 && wreckage > 0:
		progress(fmt.Sprintf("removed %d check environment(s) unused for %d days or more, and %d abandoned partial build(s)",
			unused, venvUnusedDays, wreckage))
	case unused > 0:
		progress(fmt.Sprintf("removed %d check environment(s) unused for %d days or more", unused, venvUnusedDays))
	case wreckage > 0:
		progress(fmt.Sprintf("removed %d abandoned partial check-environment build(s)", wreckage))
	}
}

// venvStale reports whether a cache entry can go, and whether it was ever a
// finished environment. Written as a predicate because deleting 200 MB should
// not be what happens when no branch matched.
func venvStale(dir string, cutoff time.Time) (stale, built bool) {
	info, err := os.Stat(filepath.Join(dir, venvMarker))
	switch {
	case err == nil:
		// A finished environment. The marker's mtime is its last use.
		return info.ModTime().Before(cutoff), true

	case errors.Is(err, fs.ErrNotExist):
		// No marker: a build that is still running, or one that died. The
		// directory's own mtime separates them only in the weak sense that it
		// records when the venv was CREATED — uv writes the tree under
		// lib/, which never touches the root again — so this says "created
		// long enough ago that no build is still going", which holds while
		// the window is days and a build is minutes.
		//
		// Worth collecting at all because EnsureVenv only clears a
		// half-built venv when its exact key comes up again, and an abandoned
		// key may never come up.
		di, derr := os.Stat(dir)
		return derr == nil && di.ModTime().Before(cutoff), false

	default:
		// The marker may well be there; something else went wrong reading it
		// (a permission, a symlink loop, a volume that went away). Deleting
		// on that would fall through to the rule above, which for a finished
		// venv compares against its BUILD time and would collect one used
		// this morning.
		return false, false
	}
}

// venvRemoving marks a cache entry that has been renamed out of the way and is
// being deleted. A later sweep finishes the job if this one could not.
const venvRemoving = ".removing"

// venvKeyDigestLen is how much of the requirement-set hash a cache key
// carries. Shared by key() and isVenvKey, which have to agree or the sweep
// stops recognizing the directories it created.
const venvKeyDigestLen = 12

// removeVenv frees a cache entry, renaming it out of the way before deleting.
//
// A bare RemoveAll is the trap: it makes maximal progress, so on a file that
// will not unlink it can take the marker and leave the rest — turning a
// working entry into one EnsureVenv can neither reuse (no marker) nor clear
// (the same file refuses to go), which fails every later check for that spec
// with "clearing a stale check environment". A rename is atomic: the key is
// either untouched or free for a clean rebuild.
//
// Reports whether the KEY is free, which is what a caller and a rebuild care
// about. Bytes left under the renamed path are collected by a later sweep.
func removeVenv(dir string) bool {
	if err := os.Rename(dir, dir+venvRemoving); err != nil {
		// Nothing moved, so the entry is exactly as it was.
		return false
	}
	//nolint:errcheck // leftovers are collected by a later sweep
	os.RemoveAll(dir + venvRemoving)
	return true
}

// isVenvKey reports whether a directory name is one key() produces:
// af<version>-py<version>-<digest>, the shape key() produces.
func isVenvKey(name string) bool {
	if !strings.HasPrefix(name, "af") {
		return false
	}
	digest := name[strings.LastIndexByte(name, '-')+1:]
	if len(digest) != venvKeyDigestLen {
		return false
	}
	for _, r := range digest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return strings.Contains(name, "-py")
}

// key hashes a spec into a cache-directory name: a readable Airflow/Python
// prefix plus a digest of the exact requirement set, so two runs with the same
// dependencies share a venv and a changed dependency does not.
func (p *uvProvisioner) key(spec checks.VenvSpec) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", spec.Airflow, spec.Python)
	for _, r := range spec.Reqs {
		fmt.Fprintf(h, "%s\n", r)
	}
	// Only when there are any, so a spec without constraints keeps the key it
	// always had and its cached venv.
	if len(spec.Constraints) > 0 {
		fmt.Fprintf(h, "\x00constraints\n")
		for _, c := range spec.Constraints {
			fmt.Fprintf(h, "%s\n", c)
		}
	}
	python := spec.Python
	if python == "" {
		python = "default"
	}
	return fmt.Sprintf("af%s-py%s-%s", pathSafe(spec.Airflow), pathSafe(python),
		hex.EncodeToString(h.Sum(nil))[:venvKeyDigestLen])
}

// pathSafe reduces a version or version request to characters a directory name
// can hold on every platform this runs on.
//
// The readable prefix used to be pasted in verbatim, which was safe while every
// caller passed a concrete version like "3.12". A caller now passes the
// manifest's requires-python, so ">=3.10,<3.13" would reach a path — and "<"
// and ">" are reserved on Windows, where the create fails with
// ERROR_INVALID_NAME rather than anything that reads like a version problem.
// The hash above already carries the exact value, so the prefix only has to
// stay recognizable.
func pathSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}

// ResolveConstraints fetches the platform constraints file and runs uv's
// resolver over reqs against it, without installing. Fetching it here — rather
// than letting uv pull the URL — is what tells "offline, could not fetch" from
// a genuine dependency conflict: a fetch failure is ErrConstraintsUnavailable,
// while a uv resolution failure over a fetched file is a *ConstraintConflict.
func (p *uvProvisioner) ResolveConstraints(ctx context.Context, reqs []string, url, pythonVersion string) error {
	body, err := p.fetch(ctx, url)
	if err != nil {
		return errors.Join(checks.ErrConstraintsUnavailable, err)
	}
	file, err := os.CreateTemp("", "mwaa-constraints-*.txt")
	if err != nil {
		return fmt.Errorf("writing the constraints file: %w", err)
	}
	defer os.Remove(file.Name()) //nolint:errcheck // best-effort cleanup of a temp file
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return fmt.Errorf("writing the constraints file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("writing the constraints file: %w", err)
	}

	stdin := strings.NewReader(strings.Join(reqs, "\n") + "\n")
	err = p.client.PipCompile(ctx, file.Name(), pythonVersion, uv.Stdio{In: stdin})
	if err == nil {
		return nil
	}
	var re *uv.ResolutionError
	if errors.As(err, &re) {
		return &checks.ConstraintConflict{Summary: re.Error(), Detail: re.Stderr}
	}
	return err
}

// httpFetch gets a constraints file over HTTP with a short timeout, so an
// offline or slow network degrades to a skip rather than hanging the check.
func httpFetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, constraintsFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, constraintsSizeLimit))
}
