package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

var group339 = &RuntimeGroup{Build: "3.3-9", Names: []string{
	"statsd", "apache-airflow-providers-celery", "Psycopg2-Binary", "pandas-market-calendars", "apache-airflow",
}}

// A package limited to the platforms it has a wheel for keeps its marker,
// sorted by its name.
func TestTheRuntimeGroupKeepsAPlatformMarker(t *testing.T) {
	dir := projectWith(t, astroHead)
	_, m := writeGroup(t, dir, nil, &RuntimeGroup{Build: "3.3-9", Names: []string{"statsd", "Linux_Only;sys_platform == 'linux'", "aiofiles"}})
	assert.Equal(t, []string{"aiofiles", "linux-only; sys_platform == 'linux'", "statsd"}, m.RuntimeGroup)
}

// addDependency adds dep to the project's dependencies after the Airflow
// requirement, however a write has laid the list out, as `uv add` would.
func addDependency(t *testing.T, dir, dep string) {
	t.Helper()
	path := filepath.Join(dir, manifest.Marker)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "'apache-airflow==3.3.*'", "'apache-airflow==3.3.*', '"+dep+"'", 1)
	require.NotEqual(t, string(data), out)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o600))
}

// defaultGroups is [tool.uv] default-groups as out decodes it, nil when unset.
func defaultGroups(t *testing.T, out string) any {
	t.Helper()
	var doc struct {
		Tool struct {
			UV map[string]any `toml:"uv"`
		} `toml:"tool"`
	}
	require.NoError(t, toml.Unmarshal([]byte(out), &doc))
	return doc.Tool.UV["default-groups"]
}

func writeGroup(t *testing.T, dir string, pins *RuntimePins, group *RuntimeGroup) (string, *manifest.Manifest) {
	t.Helper()
	refused, err := SetAstroBuild(dir, nil, build332, pins, group)
	require.NoError(t, err)
	require.False(t, refused.Group)
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	return string(out), m
}

// The group is written sorted, normalized and headed by its comments, without
// Airflow (the project lists it) or a package the project lists itself, and
// joins uv's own default group so `uv sync` installs it.
func TestTheRuntimeGroupIsWrittenAndInstalledByDefault(t *testing.T) {
	src := strings.Replace(astroHead, "['apache-airflow==3.3.*']", "['apache-airflow==3.3.*', 'statsd>=4']", 1)
	dir := projectWith(t, src)
	out, m := writeGroup(t, dir, nil, group339)

	// uv refuses a default-groups naming a group the project does not define,
	// so the dev group it installs by default is defined, empty.
	want := `[dependency-groups]
astro-runtime = [
    # Astro Runtime 3.3-9 installs these, so the local environment has what the image has.
    # astro writes them. To choose a version yourself, list the package in [project] dependencies.
    'apache-airflow-providers-celery',
    'pandas-market-calendars',
    'psycopg2-binary',
]
dev = []`
	assert.Contains(t, out, want)
	assert.Equal(t, []string{"apache-airflow-providers-celery", "pandas-market-calendars", "psycopg2-binary"}, m.RuntimeGroup)
	assert.Equal(t, []any{"dev", "astro-runtime"}, defaultGroups(t, out))

	again, _ := writeGroup(t, dir, nil, group339)
	assert.Equal(t, out, again, "a second write of the same group changes nothing")
}

// A project's own default-groups gains the group at its end; "all" installs
// every group already and is left alone.
func TestTheRuntimeGroupJoinsTheProjectsDefaultGroups(t *testing.T) {
	dir := projectWith(t, astroHead+"\n[tool.uv]\ndefault-groups = ['dev', 'lint']\n")
	out, _ := writeGroup(t, dir, nil, group339)
	assert.Equal(t, []any{"dev", "lint", "astro-runtime"}, defaultGroups(t, out))

	dir = projectWith(t, astroHead+"\n[tool.uv]\ndefault-groups = 'all'\n")
	out, _ = writeGroup(t, dir, nil, group339)
	assert.Equal(t, "all", defaultGroups(t, out))
}

// A project that took the group out of default-groups keeps it out: the group
// is refreshed, and not installed, through emptying and filling again.
func TestTheRuntimeGroupStaysOutOfDefaultGroupsWhenTakenOut(t *testing.T) {
	dir := projectWith(t, astroHead)
	writeGroup(t, dir, nil, group339)
	takeOutOfDefaultGroups(t, dir)

	out, m := writeGroup(t, dir, nil, &RuntimeGroup{Build: "3.3-10", Names: []string{"statsd"}})
	assert.Equal(t, []string{"statsd"}, m.RuntimeGroup)
	assert.Contains(t, out, "Astro Runtime 3.3-10 installs these")
	assert.Equal(t, []any{"dev"}, defaultGroups(t, out))

	// Emptied and filled again, it is still the project's choice: the group
	// stayed, empty, so it was never created again.
	writeGroup(t, dir, nil, &RuntimeGroup{})
	out, m = writeGroup(t, dir, nil, &RuntimeGroup{Build: "3.3-11", Names: []string{"statsd"}})
	assert.Equal(t, []string{"statsd"}, m.RuntimeGroup)
	assert.Equal(t, []any{"dev"}, defaultGroups(t, out))
}

// takeOutOfDefaultGroups takes astro-runtime out of default-groups, as a
// project opting out of it would.
func takeOutOfDefaultGroups(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, manifest.Marker)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	taken := strings.Replace(string(data), "    'astro-runtime',\n", "", 1)
	require.NotEqual(t, string(data), taken)
	require.NoError(t, os.WriteFile(path, []byte(taken), 0o600))
}

// A group that is not known stays, less a package the project now lists; one
// with nothing left stays too, empty under comments that say so, and keeps
// its place in default-groups.
func TestTheRuntimeGroupIsPrunedAndEmptied(t *testing.T) {
	dir := projectWith(t, astroHead)
	writeGroup(t, dir, nil, group339)
	addDependency(t, dir, "psycopg2-binary")

	_, m := writeGroup(t, dir, nil, nil)
	assert.Equal(t, []string{"apache-airflow-providers-celery", "pandas-market-calendars", "statsd"}, m.RuntimeGroup)

	out, m := writeGroup(t, dir, nil, &RuntimeGroup{})
	assert.Nil(t, m.RuntimeGroup)
	assert.Contains(t, out, "astro-runtime = [\n    # astro writes here the packages the project's Astro Runtime image installs")
	assert.Contains(t, out, "# None apply to this project now.\n]")
	assert.Equal(t, []any{"dev", "astro-runtime"}, defaultGroups(t, out))
}

// A group with nothing in it is not created: a project that never had one
// (a runtime with no package list) gets no group and no default-groups.
func TestAnEmptyRuntimeGroupIsNotCreated(t *testing.T) {
	dir := projectWith(t, astroHead)
	out, m := writeGroup(t, dir, nil, &RuntimeGroup{})
	assert.Nil(t, m.RuntimeGroup)
	assert.NotContains(t, out, "dependency-groups")
	assert.NotContains(t, out, "default-groups")
}

// Runtime pins that cannot be written leave no group either: its packages
// would lock unpinned.
func TestNoRuntimeGroupBesideRefusedPins(t *testing.T) {
	src := strings.Replace(astroHead, "[tool.astro]\n", "[tool.astro]\n\n[tool]\nuv = { constraint-dependencies = ['numpy<3'] }\n", 1)
	dir := projectWith(t, src)
	refused, err := SetAstroBuild(dir, nil, runtimeversions.AstroBuild{}, freeze339, group339)
	require.NoError(t, err)
	assert.True(t, refused.Pins)
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.NotContains(t, string(out), "astro-runtime")

	refused, err = SetRuntimePins(dir, nil, freeze339, group339)
	require.NoError(t, err)
	assert.True(t, refused.Pins)
	out, err = os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.NotContains(t, string(out), "astro-runtime")
}

// The group is astro's list of what the image has, not packages the project
// chose: the runtime pins of the packages in it stay.
func TestTheRuntimeGroupDoesNotTakeTheirPinsOut(t *testing.T) {
	dir := projectWith(t, astroHead)
	group := &RuntimeGroup{Build: "3.3-9", Names: []string{"pandas", "protobuf"}}
	writeGroup(t, dir, freeze339, group)
	out, m := writeGroup(t, dir, freeze339, group)
	assert.Equal(t, []string{"pandas", "protobuf"}, m.RuntimeGroup)
	assert.Contains(t, out, `"pandas==2.2.3; python_version == '3.12'"`)
	assert.Contains(t, out, `"protobuf==6.33.6; python_version == '3.12'"`)
}

// SetRuntimePins with nothing known needs no lookup, and lets a package the
// project now lists out of the group, as it lets a pin it excludes go.
func TestSetRuntimePinsPrunesTheGroup(t *testing.T) {
	dir := projectWith(t, astroHead)
	writeGroup(t, dir, freeze339, &RuntimeGroup{Build: "3.3-9", Names: []string{"pandas", "statsd"}})
	addDependency(t, dir, "pandas>=3")

	_, err := SetRuntimePins(dir, nil, nil, nil)
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, []string{"statsd"}, m.RuntimeGroup)
	assert.NotContains(t, strings.Join(m.UV.RuntimePins, "\n"), "pandas==")
}

// An inline [dependency-groups] cannot hold the comments that head the group,
// so it is refused and the document left as it was there.
func TestTheRuntimeGroupIsRefusedInAnInlineTable(t *testing.T) {
	src := astroHead + "\n[tool]\nfoo = 1\n"
	src = strings.Replace(src, "[project]\n", "dependency-groups = { lint = ['ruff'] }\n\n[project]\n", 1)
	dir := projectWith(t, src)
	refused, err := SetAstroBuild(dir, nil, runtimeversions.AstroBuild{}, nil, group339)
	require.NoError(t, err)
	assert.True(t, refused.Group)
	out, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	// Nothing the group would have needed is left behind: no default-groups,
	// no empty dev group.
	assert.Equal(t, src, string(out))
}
