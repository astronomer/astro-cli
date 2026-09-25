package scaffold_test

import (
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

// This file calls the [tool.astro.env] writer the way Astro Desktop's
// AddToProjectEnvSchema and DeclareFromWorkspace do once they move onto it:
// the kind arrives as the desktop's DTO spells it, which is envschema.Section's
// own spelling, the key arrives as a DAG scan or Environment Manager has it, and
// the wrapper holds the write lock every manifest writer shares plus the
// dependency watcher. If the exported API stops fitting that caller, this stops
// compiling.

// watcherHolds counts the desktop's deps-watcher hold, which has to span the
// write so the app's own edit does not read as an outside change.
var watcherHolds atomic.Int32

// holdWhileWriting is the desktop's wrapper: the write lock, and the watcher
// held inside it.
func holdWhileWriting(run func() error) error {
	return withWriteLock(func() error {
		watcherHolds.Add(1)
		defer watcherHolds.Add(-1)
		return run()
	})
}

// addToProjectEnvSchema is the manifest arm of the desktop's
// AddToProjectEnvSchema. connType applies only to connections.
func addToProjectEnvSchema(projectRoot, kind, key, connType string) error {
	spec := &envschema.ValueSpec{}
	if envschema.Section(kind) == envschema.SectionConnection {
		spec.ConnType = connType
	}
	return scaffold.AddEnvDeclaration(projectRoot, holdWhileWriting, envschema.Section(kind), key, spec)
}

// declareFromWorkspace is the manifest arm of the desktop's
// DeclareFromWorkspace.
func declareFromWorkspace(projectRoot, kind, key, connType string) error {
	return scaffold.DeclareEnvFromWorkspace(projectRoot, holdWhileWriting, envschema.Section(kind), key, connType)
}

func TestEnvWriterServesTheDesktopsActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(`[project]
name = 'orders'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.deployments.prod]
url = 'https://airflow.example.com'
default = true
auth = { method = 'none' }

[tool.astro.env]
LOG_LEVEL = 'info' # committed on purpose
API_TOKEN = { sensitive = true }
`), 0o644))

	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i, action := range []func() error{
		func() error { return addToProjectEnvSchema(dir, "connection", "warehouse", "snowflake") },
		func() error { return addToProjectEnvSchema(dir, "airflow_variable", "batch_size", "ignored") },
		func() error { return declareFromWorkspace(dir, "env_var", "API_TOKEN", "") },
		func() error { return declareFromWorkspace(dir, "connection", "AIRFLOW_CONN_DB_MAIN", "postgres") },
		func() error { return saveLink(dir, "prod", "https://moved.example.com") },
	} {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = action() }()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Zero(t, watcherHolds.Load(), "the watcher was left held")

	m, err := manifest.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "https://moved.example.com", m.Astro.Deployments["prod"].URL)
	schema, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	assert.Equal(t, "snowflake", schema.Connections["warehouse"].ConnType)
	assert.Contains(t, schema.AirflowVariables, "batch_size")
	assert.Equal(t, envschema.ValueSpec{Sensitive: true, HasSensitive: true, Source: envschema.SourceWorkspace},
		schema.EnvVars["API_TOKEN"])
	assert.Equal(t, envschema.ValueSpec{Sensitive: true, ConnType: "postgres", Source: envschema.SourceWorkspace},
		schema.Connections["db_main"])

	// Declare on a committed default is refused with a reason the desktop can
	// branch on, and the declaration is left as it was.
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	err = declareFromWorkspace(dir, "env_var", "LOG_LEVEL", "")
	require.ErrorIs(t, err, scaffold.ErrWorkspaceSourceWithDefault)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}
