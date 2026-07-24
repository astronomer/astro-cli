package uv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Stdio carries optional live output destinations for a uv invocation.
// Output is always also captured internally (the runner tees, never
// stream-only), so a failure carries uv's stderr on the error value even
// when the caller watched it scroll by.
type Stdio struct {
	// In feeds the process's stdin; nil means no input. Only Run reads it.
	In io.Reader
	// Out and Err receive the process's stdout and stderr as they happen;
	// nil discards the live stream (stderr capture still happens).
	Out, Err io.Writer
}

// Venv creates <project>/.venv without installing anything, tolerating an
// existing one. python selects the interpreter version; "" lets uv pick one
// satisfying the project's requires-python.
func (c *Client) Venv(ctx context.Context, project, python string, stdio Stdio) error {
	args := []string{"venv", "--allow-existing"}
	if python != "" {
		args = append(args, "--python", python)
	}
	return c.command(ctx, project, stdio, args...)
}

// Lock resolves the project's dependencies and writes <project>/uv.lock.
// A solver failure surfaces as *ResolutionError.
func (c *Client) Lock(ctx context.Context, project string, stdio Stdio) error {
	return asResolution("lock", c.command(ctx, project, stdio, "lock"))
}

// VenvAt creates a standalone venv at dir — not the <project>/.venv Venv
// makes — for a scratch environment that lives outside any project (a
// pre-flight check against a platform's Airflow version). python selects the
// interpreter version, e.g. "3.12"; uv provisions a managed CPython when the
// host has none. "" lets uv choose.
func (c *Client) VenvAt(ctx context.Context, dir, python string, stdio Stdio) error {
	args := []string{"venv", dir, "--allow-existing"}
	if python != "" {
		args = append(args, "--python", python)
	}
	return c.command(ctx, "", stdio, args...)
}

// PipInstall installs reqs into the venv whose interpreter is pythonBin, using
// `uv pip install`. constraint, when set, is passed as --constraint (a path or
// URL). A solver failure surfaces as *ResolutionError. It backs a scratch
// venv that is not driven from a pyproject.toml, so it takes an explicit
// requirement list rather than a project directory.
func (c *Client) PipInstall(ctx context.Context, pythonBin string, reqs []string, constraint string, stdio Stdio) error {
	args := []string{"pip", "install", "--python", pythonBin}
	if constraint != "" {
		args = append(args, "--constraint", constraint)
	}
	args = append(args, reqs...)
	return asResolution("pip install", c.command(ctx, "", stdio, args...))
}

// PipCompile resolves reqs (read from stdio.In as a requirements list on
// stdin) against an optional constraint file, without installing anything —
// the resolver runs, the output is discarded. constraint may be a path or a
// URL; pythonVersion, when set, targets the resolve at that interpreter's
// markers (e.g. "3.12") so a platform's Python-specific constraints resolve
// faithfully. A conflict surfaces as *ResolutionError. It is how a caller asks
// "does this dependency set solve under these constraints?" without an install.
func (c *Client) PipCompile(ctx context.Context, constraint, pythonVersion string, stdio Stdio) error {
	args := []string{"pip", "compile", "-", "--no-header", "--no-annotate"}
	if constraint != "" {
		args = append(args, "--constraint", constraint)
	}
	if pythonVersion != "" {
		args = append(args, "--python-version", pythonVersion)
	}
	return asResolution("pip compile", c.command(ctx, "", stdio, args...))
}

// Sync makes <project>/.venv match the project's lockfile, locking first
// when the lockfile is missing or stale — so it too can fail with
// *ResolutionError. python is as for Venv.
func (c *Client) Sync(ctx context.Context, project, python string, stdio Stdio) error {
	args := []string{"sync"}
	if python != "" {
		args = append(args, "--python", python)
	}
	return asResolution("sync", c.command(ctx, project, stdio, args...))
}

// Run executes argv inside the project environment via `uv run`, which
// first brings the environment up to date (and so can also fail with
// *ResolutionError).
func (c *Client) Run(ctx context.Context, project string, argv []string, stdio Stdio) error {
	args := append([]string{"run", "--"}, argv...)
	return asResolution("run", c.command(ctx, project, stdio, args...))
}

// markerName marks a fully synced venv. uv tracks installed packages inside
// the venv (dist-info), so partial state from an interrupted install poisons
// later syncs with metadata errors; the marker's absence is how we tell.
const markerName = ".install-complete"

// EnsureSynced provisions <project>/.venv from the project's pyproject and
// lockfile, recovering from poisoned venvs: a venv without the completion
// marker is wiped up front, and a failed sync gets one wipe-and-retry before
// the error stands. The marker is dropped while syncing so an interruption
// mid-flight leaves the venv marked incomplete.
func (c *Client) EnsureSynced(ctx context.Context, project, python string, stdio Stdio) error {
	venv := filepath.Join(project, ".venv")
	marker := filepath.Join(venv, markerName)

	if _, err := os.Stat(venv); err == nil {
		if _, err := os.Stat(marker); err != nil {
			if rmErr := os.RemoveAll(venv); rmErr != nil {
				return fmt.Errorf("removing half-installed venv: %w", rmErr)
			}
		}
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing install marker: %w", err)
	}

	if err := c.Sync(ctx, project, python, stdio); err != nil {
		var re *ResolutionError
		if errors.As(err, &re) || ctx.Err() != nil {
			// The solver is deterministic — a fresh venv cannot change
			// its answer — and a canceled context is not a poisoned venv.
			return err
		}
		if rmErr := os.RemoveAll(venv); rmErr != nil {
			return errors.Join(err, fmt.Errorf("removing venv for retry: %w", rmErr))
		}
		if err := c.Sync(ctx, project, python, stdio); err != nil {
			return err
		}
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		return fmt.Errorf("writing install marker: %w", err)
	}
	return nil
}

// stderrTailLimit bounds captured stderr so a chatty command cannot grow an
// error value without limit; uv's diagnostics fit comfortably.
const stderrTailLimit = 64 << 10

// command runs uv with the shared environment, project as the working
// directory, and stderr teed into a bounded capture. A non-zero exit comes
// back as *CommandError carrying that capture.
func (c *Client) command(ctx context.Context, project string, stdio Stdio, args ...string) error {
	if c.opts.NoConfig {
		args = append([]string{"--no-config"}, args...)
	}
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = project
	cmd.Env = c.childEnv()
	cmd.Stdin = stdio.In
	cmd.Stdout = stdio.Out
	stderrTail := &tailBuffer{max: stderrTailLimit}
	cmd.Stderr = stderrTail
	if stdio.Err != nil {
		cmd.Stderr = io.MultiWriter(stderrTail, stdio.Err)
	}
	if err := cmd.Run(); err != nil {
		cmdErr := &CommandError{Args: args, ExitCode: -1, Stderr: stderrTail.String(), Err: err}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			cmdErr.ExitCode = exitErr.ExitCode()
		}
		return cmdErr
	}
	return nil
}

// childEnv is the parent environment with UV_CACHE_DIR pinned to the shared
// cache and VIRTUAL_ENV dropped — an active parent venv must never capture
// the install (v1 fought this leak with --python on every call; removing
// the variable is simpler).
func (c *Client) childEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, "UV_CACHE_DIR=") || strings.HasPrefix(kv, "VIRTUAL_ENV=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "UV_CACHE_DIR="+c.opts.CacheDir)
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string { return string(b.buf) }
