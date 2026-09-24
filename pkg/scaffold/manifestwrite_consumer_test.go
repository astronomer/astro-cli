package scaffold_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// This file calls EditManifest from outside the package, the way Astro Desktop
// does: a package-level write lock every writer of the manifest shares, exposed
// as a func(func() error) error, and a link save that replaces the link's table
// and keeps the default flag it already had. If the exported API stops fitting
// that caller, this stops compiling.

var writeMu sync.Mutex

// withWriteLock is the shape of the desktop's deploylinks.WithWriteLock, passed
// straight to EditManifest as its wrapper.
func withWriteLock(fn func() error) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return fn()
}

// saveLink is the desktop's link save, on the shared helper.
func saveLink(projectRoot, name, url string) error {
	return scaffold.EditManifest(projectRoot, withWriteLock, func(before *manifest.Manifest, ed tomledit.Editor) error {
		table := map[string]any{"url": url, "target": "astro", "auth": map[string]any{"method": "none"}}
		if existing, ok := before.Astro.Deployments[name]; ok && existing.Default {
			table["default"] = true
		}
		return scaffold.ReplaceTable(ed, []string{"tool", "astro", "deployments", name}, table)
	})
}

// declareEnv is the desktop's declaration write, under the same lock.
func declareEnv(projectRoot, name string) error {
	return scaffold.AddEnvDeclaration(projectRoot, withWriteLock, envschema.SectionEnvVar, name, nil)
}

func TestEditManifestServesTheDesktopsWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(`[project]
name = 'orders'

[tool.astro]
airflow = '3.1'

[tool.astro.deployments.prod]
url = 'https://airflow.example.com'
default = true
auth = { method = 'none' }
`), 0o644))

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = saveLink(dir, "prod", "https://moved.example.com") }()
	go func() { defer wg.Done(); errs[1] = declareEnv(dir, "REGION") }()
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	m, err := manifest.Load(path)
	require.NoError(t, err)
	prod := m.Astro.Deployments["prod"]
	assert.Equal(t, "https://moved.example.com", prod.URL)
	assert.True(t, prod.Default, "the save dropped the default the link already had")
	assert.Contains(t, m.Astro.Env, "REGION", "one writer's edit was lost to the other")
}
