package local

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// A seed writes pyproject.toml as every other write of it does: a manifest
// that is a symlink is written through it, and stays a link.
func TestASeedKeepsASymlinkedManifestALink(t *testing.T) {
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, init: true, symlink: true}.start(t)
	if !r.manifestIsLink {
		t.Fatalf("the seed replaced the symlinked pyproject.toml with a file:\n%s", r.manifest)
	}
	if !strings.Contains(r.manifest, seedRecord339) || strings.Contains(r.manifest, "python_version") {
		t.Errorf("the seed did not record itself through the link, or left its constraints:\n%s", r.manifest)
	}
	if r.uv.seedLocks != 1 {
		t.Errorf("seed locks = %d, want 1", r.uv.seedLocks)
	}
}

// A process killed after the seed wrote its constraints, before it put the
// manifest back, leaves them between astro's comments: they are never read as
// the project's own, and the next write of the manifest takes them out.
func TestTheConstraintsOfAKilledSeedAreAstrosAndComeOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(buildManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	// The killed seed held pandas to another version than the list's.
	stale := &scaffold.RuntimePins{Build: "3.3-9", Python: "3.12", Pins: []string{"pandas==1.0.0"}}
	if n, err := scaffold.AddSeedConstraints(dir, nil, stale); err != nil || n != 1 {
		t.Fatalf("AddSeedConstraints = %d, %v", n, err)
	}
	left, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	pins := &scaffold.RuntimePins{Build: "3.3-9", Python: "3.12", Pins: freeze339}
	if seeded := scaffold.SeededPins(dir, pins); !slices.Contains(seeded, "pandas==2.2.3") {
		t.Errorf("the leftover pandas==1.0.0 counted as the project's own and dropped the list's pin: %v", seeded)
	}

	r := pinsRun{src: string(left), files: map[string]string{"uv.lock": "version = 1\n"}, build: build332, freeze: freeze339}.start(t)
	if strings.Contains(r.manifest, "python_version") || strings.Contains(r.manifest, "ships these versions") {
		t.Errorf("the next start left the killed seed's constraints:\n%s", r.manifest)
	}
}

// A member of a uv workspace has no uv.lock of its own (uv locks the
// workspace at its root), so it is not seeded, and the run says why.
func TestAWorkspaceMemberIsNotSeeded(t *testing.T) {
	const root = "[project]\nname = 'root'\n\n[tool.uv.workspace]\nmembers = ['proj']\n"
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, init: true, workspace: root}.start(t)
	if r.uv.seedLocks+r.uv.projectLocks != 0 || r.lock != "" {
		t.Errorf("a workspace member was locked (%d seed, %d project)", r.uv.seedLocks, r.uv.projectLocks)
	}
	if strings.Contains(r.manifest, "astro seeded") {
		t.Errorf("a workspace member recorded a seed:\n%s", r.manifest)
	}
	if r.seedWarn == nil || !strings.Contains(r.seedWarn.Error(), "member of the uv workspace") {
		t.Errorf("no word of the workspace: %v", r.seedWarn)
	}

	// A workspace that excludes the project, or whose members name another
	// directory, leaves it its own project, which is seeded.
	const others = "[project]\nname = 'root'\n\n[tool.uv.workspace]\nmembers = ['packages/*']\n"
	if r := (pinsRun{src: buildManifest, build: build332, freeze: freeze339, init: true, workspace: others}).start(t); r.uv.seedLocks != 1 {
		t.Errorf("a project outside the workspace's members was not seeded (seed locks %d)", r.uv.seedLocks)
	}
	const excluded = "[project]\nname = 'root'\n\n[tool.uv.workspace]\nmembers = ['*']\nexclude = ['proj']\n"
	if r := (pinsRun{src: buildManifest, build: build332, freeze: freeze339, init: true, workspace: excluded}).start(t); r.uv.seedLocks != 1 {
		t.Errorf("a project the workspace excludes was not seeded (seed locks %d)", r.uv.seedLocks)
	}
}

// exclude-newer in the user's own uv.toml holds uv to a cutoff on this
// machine: a lock is not seeded under it, and a seed already recorded stays.
func TestTheUsersUVConfigCutoffIsTheEnvironments(t *testing.T) {
	r := seeded(t)
	config := t.TempDir()
	if err := os.MkdirAll(filepath.Join(config, "uv"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "uv", "uv.toml"), []byte("exclude-newer = \"2026-01-01\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UV_EXCLUDE_NEWER", "")
	t.Setenv("UV_NO_CONFIG", "")
	t.Setenv("UV_CONFIG_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", config)

	kept := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freeze: freeze339, init: true}.start(t)
	if kept.manifest != r.manifest || kept.lock != r.lock || kept.uv.seedLocks != 0 {
		t.Errorf("a cutoff in the user's uv.toml changed the seed (seed locks %d):\n%s", kept.uv.seedLocks, kept.manifest)
	}
	fresh := pinsRun{src: buildManifest, build: build332, freeze: freeze339, init: true}.start(t)
	if fresh.uv.seedLocks != 0 || strings.Contains(fresh.manifest, "astro seeded") {
		t.Errorf("a lock was seeded under the user's uv.toml cutoff:\n%s", fresh.manifest)
	}

	t.Setenv("UV_NO_CONFIG", "1")
	if r := (pinsRun{src: buildManifest, build: build332, freeze: freeze339, init: true}).start(t); r.uv.seedLocks != 1 {
		t.Errorf("under UV_NO_CONFIG, which reads no uv.toml, the lock was not seeded (seed locks %d)", r.uv.seedLocks)
	}
}

// The versions a lock holds are compared with the list's as PEP 440 orders
// them: pip freeze and uv.lock can spell one version two ways.
func TestALockHoldsAVersionSpelledAnotherWay(t *testing.T) {
	if !holds([]string{"1.0.0.post0"}, "1.0.0-post0") || !holds([]string{"2.0.0"}, "2.0") {
		t.Error("one version spelled two ways did not count as held")
	}
	if holds([]string{"2.0.1"}, "2.0") {
		t.Error("another version counted as held")
	}
}
