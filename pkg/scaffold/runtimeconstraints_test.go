package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

var rc339 = RuntimeConstraints{
	Build: "3.3-9", Python: "3.14",
	Lines: []string{"apache-airflow==3.3.2+astro.2", "apache-airflow-task-sdk==1.3.2+astro.1", "SQLAlchemy==2.0.50"},
}

func withConstraints(t *testing.T, src string, rc *RuntimeConstraints) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(src), 0o600))
	require.NoError(t, SetAstroBuildWith(dir, nil, build332, rc))
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	return string(out)
}

func TestSetAstroBuildWithWritesTheRuntimesLinesUnderTheRecord(t *testing.T) {
	got := withConstraints(t, astroHead+"\n[tool.uv]\nconstraint-dependencies = [\n    'urllib3<3',\n]\n", &rc339)
	assert.Contains(t, got, "    'urllib3<3',\n    'apache-airflow==3.3.2+astro.1',\n    'apache-airflow-task-sdk==1.3.2+astro.1',\n"+
		"    # Astro Runtime 3.3-9 (Python 3.14) constrains these. astro writes them.\n    'SQLAlchemy==2.0.50',\n")
	m, err := manifest.Parse([]byte(got))
	require.NoError(t, err)
	assert.Equal(t, "3.3-9", m.UV.RuntimeBuild)
	assert.Equal(t, "3.14", m.UV.RuntimePython)

	// A new build replaces the block; nil leaves it; a zero one takes it out.
	moved := withConstraints(t, got, &RuntimeConstraints{Build: "3.3-10", Python: "3.14", Lines: []string{"SQLAlchemy==2.0.51"}})
	assert.Contains(t, moved, "Astro Runtime 3.3-10 (Python 3.14)")
	assert.Contains(t, moved, "'SQLAlchemy==2.0.51'")
	assert.NotContains(t, moved, "2.0.50")
	assert.Equal(t, got, withConstraints(t, got, nil))
	gone := withConstraints(t, got, &RuntimeConstraints{})
	assert.NotContains(t, gone, "constrains these")
	assert.NotContains(t, gone, "SQLAlchemy")
	assert.Contains(t, gone, "'urllib3<3'")
	assert.Contains(t, gone, "apache-airflow==3.3.2+astro.1")
}
