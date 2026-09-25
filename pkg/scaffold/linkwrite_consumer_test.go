package scaffold_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// This file calls the link writers from outside the package, the way
// Astro Desktop's deploylinks does: one package-level lock every writer of the
// manifest takes, plus a hold on the dependency watcher for the length of the
// write, passed to each writer as its wrapper. If the exported API stops
// fitting that caller, this stops compiling.

// desktopWriter is the desktop's wrap: its write lock, and the watcher hold,
// which here counts the writes that ran inside it.
type desktopWriter struct {
	mu    sync.Mutex
	held  atomic.Int32
	holds atomic.Int32
}

func (w *desktopWriter) wrap(run func() error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held.Add(1) != 1 {
		return errors.New("two writes held the watcher at once")
	}
	defer w.held.Add(-1)
	w.holds.Add(1)
	return run()
}

// errNoManifest is the desktop's own sentinel, translated from the manifest
// package's the way deploylinks does.
var errNoManifest = errors.New("this project has no pyproject.toml yet")

func desktopSave(w *desktopWriter, dir string, l *scaffold.Link) error {
	err := scaffold.SaveLink(dir, w.wrap, *l)
	if errors.Is(err, manifest.ErrNotFound) {
		return errNoManifest
	}
	return err
}

func TestTheLinkWritersServeTheDesktop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(`[project]
name = 'orders'
dependencies = ['apache-airflow==3.1.*']

# the team's default workspace
[tool.astro]
workspace = 'ws_A'
domain = 'astronomer.io'

[tool.astro.deployments.dev]
deployment = 'dep-dev'
default = true

[tool.astro.deployments.old]
deployment = 'dep-old'
`), 0o644))

	w := &desktopWriter{}
	writes := []func() error{
		func() error {
			return desktopSave(w, dir, &scaffold.Link{Name: "prod", Kind: manifest.KindAstro, Workspace: "ws_A", Deployment: "dep-prod"})
		},
		func() error {
			return desktopSave(w, dir, &scaffold.Link{Name: "aws", Kind: manifest.KindMWAA, Environment: "orders", TargetRegion: "eu-west-1"})
		},
		func() error { return scaffold.SetDefaultLink(dir, w.wrap, "dev") },
		func() error { _, err := scaffold.RemoveLink(dir, w.wrap, "old"); return err },
		func() error {
			_, err := scaffold.SetWorkspaceLink(dir, w.wrap, "ws_B", "https://cloud.astronomer.io")
			return err
		},
		func() error {
			return scaffold.AddEnvDeclaration(dir, w.wrap, envschema.SectionEnvVar, "REGION", nil)
		},
	}
	var wg sync.WaitGroup
	errs := make([]error, len(writes))
	for i, write := range writes {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = write() }()
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "write %d", i)
	}
	assert.EqualValues(t, len(writes), w.holds.Load(), "a write ran outside the wrapper")

	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "ws_B", m.Astro.Workspace)
	assert.Equal(t, "astronomer.io", m.Astro.Domain)
	assert.Equal(t, "ws_A", m.Astro.Deployments["dev"].Workspace, "the switch moved an inheriting link")
	assert.True(t, m.Astro.Deployments["dev"].Default)
	assert.NotContains(t, m.Astro.Deployments, "old")
	assert.Equal(t, manifest.KindMWAA, m.Astro.Deployments["aws"].Kind())
	assert.Equal(t, "eu-west-1", m.Astro.Targets["mwaa"]["region"])
	// prod was saved inheriting ws_A, before or after the switch: either it was
	// pinned by the switch, or it was saved against ws_B and so wrote ws_A.
	assert.Equal(t, "ws_A", m.Astro.Deployments["prod"].Workspace)
	assert.Contains(t, m.Astro.Env, "REGION", "one writer's edit was lost to another")

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "# the team's default workspace")

	require.ErrorIs(t, desktopSave(w, t.TempDir(), &scaffold.Link{Name: "x", Kind: manifest.KindAstro, Workspace: "W", Deployment: "d"}), errNoManifest)
}
