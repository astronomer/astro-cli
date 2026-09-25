package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Both arms that read a v1 project follow the rule `astro init` does: the cap
// goes with the pin the run writes, wherever that pin came from.

func loadConstraints(t *testing.T, dir string) []string {
	t.Helper()
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	return m.UV.ConstraintDependencies
}

func capLabel(res *Result) string {
	for _, l := range append(append([]string{}, res.Updated...), res.Created...) {
		if strings.Contains(l, "constraint-dependencies") {
			return l
		}
	}
	return ""
}

func TestConvertCapsSQLAlchemyForTheDockerfilesPin(t *testing.T) {
	for _, tc := range []struct {
		from string
		want []string
	}{
		{from: "astrocrpublic.azurecr.io/runtime:3.1-12", want: []string{"sqlalchemy<2.1"}},
		{from: "astrocrpublic.azurecr.io/runtime:3.3-8", want: nil},
		{from: "astrocrpublic.azurecr.io/runtime:3.2-4", want: nil},
	} {
		t.Run(tc.from, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM "+tc.from+"\n"), 0o600))

			_, err := Run(dir, Options{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, loadConstraints(t, dir))
		})
	}
}

func TestAdoptCapsSQLAlchemyForAThreePointOnePin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		opts     Options
		want     []string
		reported bool
	}{
		{
			name:     "the flag",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\n",
			opts:     Options{AirflowVersion: "3.1"},
			want:     []string{"sqlalchemy<2.1"},
			reported: true,
		},
		{
			name:     "the manifest's own pin",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\ndependencies = ['apache-airflow==3.1.7']\n",
			want:     []string{"sqlalchemy<2.1"},
			reported: true,
		},
		{
			name:     "3.2",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\n",
			opts:     Options{AirflowVersion: "3.2"},
		},
		{
			name:     "the default",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\n",
		},
		{
			name: "appended to the manifest's own constraints",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\n\n" +
				"[tool.uv]\nconstraint-dependencies = ['pandas<3']\n",
			opts:     Options{AirflowVersion: "3.1"},
			want:     []string{"pandas<3", "sqlalchemy<2.1"},
			reported: true,
		},
		{
			name: "into an existing [tool.uv]",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\n\n" +
				"[tool.uv]\npackage = false\n",
			opts:     Options{AirflowVersion: "3.1"},
			want:     []string{"sqlalchemy<2.1"},
			reported: true,
		},
		{
			name: "the manifest's own SQLAlchemy constraint wins",
			manifest: "[project]\nname = 'x'\nversion = '1.0'\n\n" +
				"[tool.uv]\nconstraint-dependencies = ['SQLAlchemy<2.0.40']\n",
			opts: Options{AirflowVersion: "3.1"},
			want: []string{"SQLAlchemy<2.0.40"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(tc.manifest), 0o600))

			res, err := Run(dir, tc.opts)
			require.NoError(t, err)
			require.True(t, res.Adopted)
			assert.Equal(t, tc.want, loadConstraints(t, dir))

			label := capLabel(res)
			if !tc.reported {
				assert.Empty(t, label, "nothing added, nothing reported")
				return
			}
			assert.Contains(t, label, "sqlalchemy<2.1", "a change performed but unreported cannot be reviewed")
		})
	}
}

// The existing table keeps the key it had: the cap is added beside it, not in
// place of it.
func TestAdoptKeepsTheRestOfToolUV(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(
		"[project]\nname = 'x'\nversion = '1.0'\n\n[tool.uv]\npackage = false\n"), 0o600))

	_, err := Run(dir, Options{AirflowVersion: "3.1"})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "package = false")
	assert.Equal(t, 1, strings.Count(string(raw), "[tool.uv]"), "one table, not a second one\n%s", raw)
}
