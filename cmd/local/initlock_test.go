package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// lockOf is a uv.lock holding the given name/version pairs.
func lockOf(pairs ...string) string {
	var b strings.Builder
	b.WriteString("version = 1\n")
	for i := 0; i+1 < len(pairs); i += 2 {
		fmt.Fprintf(&b, "\n[[package]]\nname = %q\nversion = %q\n", pairs[i], pairs[i+1])
	}
	return b.String()
}

// initWithLock runs init in dir with the build lookup answering build332 and
// Lock as given, and returns init's stderr and how many times Lock ran.
func initWithLock(t *testing.T, dir string, lock func(ctx context.Context, dir string) error) (stderr string, calls int) {
	t.Helper()
	d, _ := testDeps(t)
	errOut := &bytes.Buffer{}
	d.Stderr = errOut
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) { return build332, nil }
	d.Lock = func(ctx context.Context, dir string) error {
		calls++
		return lock(ctx, dir)
	}
	if err := execute(t, d, "init", "--name", "demo", "--airflow-version", "3.3"); err != nil {
		t.Fatalf("init: %v", err)
	}
	return errOut.String(), calls
}

func writeLock(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "uv.lock"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInitLocksANewProject(t *testing.T) {
	dir := t.TempDir()
	out, calls := initWithLock(t, dir, func(_ context.Context, dir string) error {
		writeLock(t, dir, lockOf("sqlalchemy", "2.0.50"))
		return nil
	})
	if calls != 1 || !strings.Contains(out, "uv.lock: locked") {
		t.Errorf("init did not lock the new project once (calls %d):\n%s", calls, out)
	}
	if strings.Contains(out, "run uv lock") {
		t.Errorf("init asked for a lock it ran:\n%s", out)
	}
}

func TestInitUpdatesAnExistingLock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = 'demo'\nversion = '0.1.0'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeLock(t, dir, lockOf("sqlalchemy", "2.0.55", "pandas", "2.3.3"))
	out, calls := initWithLock(t, dir, func(_ context.Context, dir string) error {
		writeLock(t, dir, lockOf("sqlalchemy", "2.0.50", "pandas", "2.3.3"))
		return nil
	})
	if calls != 1 || !strings.Contains(out, "uv.lock: updated (1 package changed)") {
		t.Errorf("init did not report the one package the lock moved (calls %d):\n%s", calls, out)
	}
}

func TestInitWithoutUVAsksForTheLock(t *testing.T) {
	missing := func(context.Context, string) error { return fmt.Errorf("discovering uv: %w", uv.ErrNotFound) }

	dir := t.TempDir()
	out, _ := initWithLock(t, dir, missing)
	if strings.Contains(out, "uv.lock") {
		t.Errorf("a project with no lock was asked to lock without uv:\n%s", out)
	}

	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = 'demo'\nversion = '0.1.0'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeLock(t, dir, lockOf("sqlalchemy", "2.0.55"))
	out, _ = initWithLock(t, dir, missing)
	if !strings.Contains(out, "uv.lock: run uv lock so it follows pyproject.toml") {
		t.Errorf("a lock left behind without uv was not mentioned:\n%s", out)
	}
}

func TestInitWarnsWhenTheLockFails(t *testing.T) {
	dir := t.TempDir()
	out, _ := initWithLock(t, dir, func(context.Context, string) error {
		return errors.New("uv lock: no solution: sqlalchemy==2.0.50 conflicts\nmore detail")
	})
	if !strings.Contains(out, "Warning: uv.lock: uv lock failed: uv lock: no solution: sqlalchemy==2.0.50 conflicts; run uv lock so it follows pyproject.toml") {
		t.Errorf("a failed lock did not warn on one line:\n%s", out)
	}
}

func TestInitLocksNothingWhenToolUVIsUnchanged(t *testing.T) {
	d, _ := testDeps(t)
	dir := t.TempDir()
	d.WorkingDir = func() (string, error) { return dir, nil }
	calls := 0
	d.Lock = func(context.Context, string) error {
		calls++
		return nil
	}
	// No build lookup: init writes nothing into [tool.uv].
	if err := execute(t, d, "init", "--name", "demo", "--airflow-version", "3.3"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if calls != 0 {
		t.Errorf("init locked though it changed nothing in [tool.uv] (%d calls)", calls)
	}
}
