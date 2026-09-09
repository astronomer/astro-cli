package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
const venvMarker = ".check-complete"

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
	if err := p.client.PipInstall(ctx, python, spec.Reqs, "", uv.Stdio{}); err != nil {
		return "", err
	}
	if err := os.WriteFile(marker, nil, markerPerms); err != nil {
		return "", fmt.Errorf("marking the check environment ready: %w", err)
	}
	return python, nil
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
	python := spec.Python
	if python == "" {
		python = "default"
	}
	return fmt.Sprintf("af%s-py%s-%s", spec.Airflow, python, hex.EncodeToString(h.Sum(nil))[:12])
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
