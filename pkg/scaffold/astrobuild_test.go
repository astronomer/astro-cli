package scaffold

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

var build332 = runtimeversions.AstroBuild{
	Index:   "https://pip.astronomer.io/v2/",
	Airflow: "3.3.2+astro.1",
	TaskSDK: "1.3.2+astro.1",
}

// uvView is the part of a manifest this file writes, decoded.
type uvView struct {
	Project struct {
		Dependencies []string `toml:"dependencies"`
	} `toml:"project"`
	Tool struct {
		UV struct {
			Environments           []string                  `toml:"environments"`
			ConstraintDependencies []string                  `toml:"constraint-dependencies"`
			Index                  []map[string]any          `toml:"index"`
			Sources                map[string]map[string]any `toml:"sources"`
			ExcludeNewer           string                    `toml:"exclude-newer"`
			ExcludeNewerPackage    map[string]any            `toml:"exclude-newer-package"`
		} `toml:"uv"`
	} `toml:"tool"`
}

func withBuild(t *testing.T, src string, b runtimeversions.AstroBuild) (string, uvView) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte(src), 0o600))
	require.NoError(t, SetAstroBuild(dir, nil, b))
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	var v uvView
	require.NoError(t, toml.Unmarshal(out, &v), "%s", out)
	return string(out), v
}

const astroHead = "[project]\nname = 'demo'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n"

func TestSetAstroBuildWritesWhatUVNeeds(t *testing.T) {
	_, v := withBuild(t, astroHead, build332)

	assert.Equal(t, []string{"apache-airflow==3.3.*", "apache-airflow-core", "apache-airflow-task-sdk"}, v.Project.Dependencies,
		"sources apply only to direct dependencies, so core and the SDK are listed")
	assert.Equal(t, AstroEnvironments, v.Tool.UV.Environments)
	assert.Equal(t, []string{"apache-airflow==3.3.2+astro.1", "apache-airflow-task-sdk==1.3.2+astro.1"}, v.Tool.UV.ConstraintDependencies)
	assert.Equal(t, []map[string]any{{"name": "astronomer", "url": "https://pip.astronomer.io/v2/", "explicit": true}}, v.Tool.UV.Index)
	for _, d := range []string{"apache-airflow", "apache-airflow-core", "apache-airflow-task-sdk"} {
		assert.Equal(t, map[string]any{"index": "astronomer"}, v.Tool.UV.Sources[d], d)
	}
	assert.Equal(t, map[string]any{"apache-airflow": false, "apache-airflow-core": false, "apache-airflow-task-sdk": false},
		v.Tool.UV.ExcludeNewerPackage, "the index lists no upload times, so any cutoff would hide every build")
}

// The table is the user's too: their keys, their constraints and their
// comments stay, and a second write of the same build changes nothing.
func TestSetAstroBuildKeepsTheUsersTable(t *testing.T) {
	src := astroHead + "\n[tool.uv]\n# a cooldown for everything\nexclude-newer = \"1 week\"\n" +
		"environments = [\"sys_platform == 'linux'\"]\nconstraint-dependencies = ['protobuf<6']\n\n" +
		"[[tool.uv.index]]\nname = 'corp'\nurl = 'https://corp.example/simple'\n\n" +
		"[tool.uv.sources]\nmylib = { path = '../mylib' }\n"

	out, v := withBuild(t, src, build332)

	assert.Contains(t, out, "# a cooldown for everything")
	assert.Equal(t, []string{"sys_platform == 'linux'"}, v.Tool.UV.Environments, "a project's own environments stand")
	assert.Equal(t, []string{"protobuf<6", "apache-airflow==3.3.2+astro.1", "apache-airflow-task-sdk==1.3.2+astro.1"}, v.Tool.UV.ConstraintDependencies)
	require.Len(t, v.Tool.UV.Index, 2)
	assert.Equal(t, "corp", v.Tool.UV.Index[0]["name"])
	assert.Equal(t, map[string]any{"path": "../mylib"}, v.Tool.UV.Sources["mylib"])
	assert.Equal(t, map[string]any{"apache-airflow": false, "apache-airflow-core": false, "apache-airflow-task-sdk": false},
		v.Tool.UV.ExcludeNewerPackage, "the project's cutoff would hide every build")

	dir := t.TempDir()
	path := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o600))
	require.NoError(t, SetAstroBuild(dir, nil, build332))
	again, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, out, string(again), "writing the same build twice changed the file")
}

func TestSetAstroBuildMovesToANewBuild(t *testing.T) {
	older := runtimeversions.AstroBuild{Index: build332.Index, Airflow: "3.3.1+astro.4", TaskSDK: "1.3.1+astro.2"}
	out, _ := withBuild(t, astroHead, older)
	dir := t.TempDir()
	path := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o600))

	require.NoError(t, SetAstroBuild(dir, nil, build332))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var v uvView
	require.NoError(t, toml.Unmarshal(data, &v))
	assert.Equal(t, []string{"apache-airflow==3.3.2+astro.1", "apache-airflow-task-sdk==1.3.2+astro.1"}, v.Tool.UV.ConstraintDependencies)
	assert.Len(t, v.Tool.UV.Index, 1)
	assert.Len(t, v.Project.Dependencies, 3)
}

// An Airflow with no Astronomer build comes from PyPI, so what points uv at
// the index goes, and what the project wrote itself stays.
func TestAZeroBuildTakesOutWhatABuildWrote(t *testing.T) {
	src := astroHead + "\n[tool.uv]\nconstraint-dependencies = ['protobuf<6']\n"
	out, _ := withBuild(t, src, build332)
	dir := t.TempDir()
	path := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o600))

	require.NoError(t, SetAstroBuild(dir, nil, runtimeversions.AstroBuild{}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var v uvView
	require.NoError(t, toml.Unmarshal(data, &v), "%s", data)
	assert.Equal(t, []string{"protobuf<6"}, v.Tool.UV.ConstraintDependencies)
	assert.Empty(t, v.Tool.UV.Index)
	assert.Empty(t, v.Tool.UV.Sources)
	assert.Empty(t, v.Tool.UV.ExcludeNewerPackage)
}

func TestSetAstroBuildForAirflow2AndForCore(t *testing.T) {
	af2 := runtimeversions.AstroBuild{Index: build332.Index, Airflow: "2.11.2+astro.7"}
	_, v := withBuild(t, "[project]\nname = 'old'\ndependencies = ['apache-airflow==2.11.*']\n\n[tool.astro]\n", af2)
	assert.Equal(t, []string{"apache-airflow==2.11.*"}, v.Project.Dependencies)
	assert.Equal(t, []string{"apache-airflow==2.11.2+astro.7"}, v.Tool.UV.ConstraintDependencies)
	assert.Equal(t, []string{"apache-airflow"}, keys(v.Tool.UV.Sources))

	moved, _ := withBuild(t, astroHead, build332)
	moved = strings.Replace(moved, "apache-airflow==3.3.*", "apache-airflow==2.11.*", 1)
	_, v = withBuild(t, moved, af2)
	assert.Equal(t, []string{"apache-airflow==2.11.*"}, v.Project.Dependencies, "Airflow 2 publishes no core or Task SDK")
	assert.Equal(t, []string{"apache-airflow==2.11.2+astro.7"}, v.Tool.UV.ConstraintDependencies)
	assert.Equal(t, []string{"apache-airflow"}, keys(v.Tool.UV.Sources))

	_, v = withBuild(t, "[project]\nname = 'slim'\ndependencies = ['apache-airflow-core==3.3.*']\n\n[tool.astro]\n", build332)
	assert.Equal(t, []string{"apache-airflow-core==3.3.*", "apache-airflow-task-sdk"}, v.Project.Dependencies,
		"a project on the distribution without providers does not gain it")
}

func keys(m map[string]map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

// With no build, a project moved to Airflow 2 loses the bare Airflow 3
// entries, and the platforms narrowed for the build's lockfile go too.
func TestAZeroBuildOnAirflow2TakesOutTheAirflow3Entries(t *testing.T) {
	written, _ := withBuild(t, astroHead, build332)
	moved := strings.Replace(written, "apache-airflow==3.3.*", "apache-airflow==2.11.*", 1)

	_, v := withBuild(t, moved, runtimeversions.AstroBuild{})

	assert.Equal(t, []string{"apache-airflow==2.11.*"}, v.Project.Dependencies)
	assert.Empty(t, v.Tool.UV.Environments)
	assert.Empty(t, v.Tool.UV.ConstraintDependencies)
}

// Settings the project wrote itself are not taken over: nothing is written,
// and the error says what to change.
func TestSetAstroBuildLeavesTheUsersOwnIndexAndSources(t *testing.T) {
	for name, uv := range map[string]string{
		"an index of that name":    "[[tool.uv.index]]\nname = 'astronomer'\nurl = 'https://mirror.example/simple'\n",
		"a source of its own kind": "[tool.uv.sources]\napache-airflow = { index = 'corp' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, manifest.Marker)
			src := astroHead + "\n" + uv
			require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

			err := SetAstroBuild(dir, nil, build332)

			require.ErrorIs(t, err, ErrNotAstrosToWrite)
			data, rerr := os.ReadFile(path)
			require.NoError(t, rerr)
			assert.Equal(t, src, string(data))
		})
	}
}

// An inline index array takes the entry whole, since an array of tables
// cannot be started inside it.
func TestSetAstroBuildAppendsToAnInlineIndexArray(t *testing.T) {
	src := astroHead + "\n[tool.uv]\nindex = [{ name = 'corp', url = 'https://corp.example/simple' }]\n"

	_, v := withBuild(t, src, build332)

	require.Len(t, v.Tool.UV.Index, 2)
	assert.Equal(t, map[string]any{"name": "astronomer", "url": "https://pip.astronomer.io/v2/", "explicit": true}, v.Tool.UV.Index[1])
}
