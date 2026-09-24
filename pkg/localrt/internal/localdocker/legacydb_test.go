package localdocker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// The expected names here are pinned to what `astro dev` actually produced, not
// recomputed from the code under test: the md5 prefixes were taken with the
// md5(1) tool, and the first two cases are real volumes observed on a machine
// that had run both CLIs against those directories.
func TestLegacyMetadataVolumeReproducesV1Names(t *testing.T) {
	for _, tc := range []struct {
		name        string
		projectPath string
		projectName string
		want        string
	}{
		{"ordinary project", "/home/user/projects/workflows", "workflows", "workflows_d88c12_postgres_data"},
		{"directory and project name differ", "/home/user/projects/airflow-playground", "playground", "playground_f016a3_postgres_data"},
		{"name docker will not take is sanitized", "/home/dev/My Project!", "My Project!", "my-project-b48397_postgres_data"},
		{"unset project name leaves the hash alone", "/tmp/x", "", "7ae397_postgres_data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, legacyMetadataVolume(tc.projectPath, tc.projectName))
		})
	}
}

// The note asks about a name built from metadataVolumeKey and compose mounts the
// name built from the template, so a rename on either side would make every
// start look like the first one here, and announce a new database each time.
// Nothing else ties the two together.
func TestMetadataVolumeKeyMatchesComposeTemplate(t *testing.T) {
	out, err := generateCompose(composeInput{
		ProjectName:   "astro-proj-aaaaaa",
		Image:         "img",
		PostgresImage: postgresImage,
		WebPort:       8080,
		PostgresPort:  5432,
		Services:      airflowServices("3"),
		DBCommand:     dbCommand("3"),
	})
	require.NoError(t, err)

	var spec struct {
		Volumes  map[string]any `yaml:"volumes"`
		Services map[string]struct {
			Volumes []string `yaml:"volumes"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &spec))

	require.Contains(t, spec.Volumes, metadataVolumeKey,
		"the compose spec must declare the volume the migration copies into")
	assert.Contains(t, spec.Services["postgres"].Volumes, metadataVolumeKey+":/var/lib/postgresql/data",
		"postgres must mount that volume at the data directory")
}

const (
	testComposeProject = "astro-proj-aaaaaa"
	testNewVolume      = testComposeProject + "_" + metadataVolumeKey
)

// legacyProject writes the .astro/config.yaml a v1 project would have and
// returns the directory and the volume `astro dev` would have made for it.
func legacyProject(t *testing.T, body string) (projectPath, legacyVolume string) {
	t.Helper()
	projectPath = t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectPath, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, ".astro", "config.yaml"), []byte(body), 0o600))
	// Derived rather than pinned because the path is a temp dir; the derivation
	// itself is pinned against real v1 volumes above.
	return projectPath, legacyMetadataVolume(projectPath, "proj")
}

// engineState scripts which volumes a fake engine has, per engine binary.
type engineState struct {
	docker    []string // volumes the docker engine has
	podman    []string // volumes the podman engine has
	lsUnknown []string // volumes whose lookup cannot reach the daemon
}

func (s *engineState) output(call string) ([]byte, error) {
	if !strings.Contains(call, "volume ls") {
		return nil, nil
	}
	for _, v := range s.lsUnknown {
		if strings.HasSuffix(call, "name="+v) {
			return nil, errors.New("cannot connect to the daemon")
		}
	}
	have := s.docker
	if strings.HasPrefix(call, "podman ") {
		have = s.podman
	}
	for _, v := range have {
		if strings.HasSuffix(call, "name="+v) {
			return []byte(v + "\n"), nil
		}
	}
	return nil, nil
}

// noteRun drives one note against a scripted engine and reports what ran and
// what it said.
func noteRun(t *testing.T, projectPath string, s *engineState) (calls, lines []string) {
	t.Helper()
	cmd := &fakeCmd{output: s.output}
	e := testEngine(t, cmd)
	cb := rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }}
	e.noteLegacyDatabase(context.Background(), engineConn{bin: "docker"}, projectPath, testComposeProject, cb)
	return cmd.calls, lines
}

// A first start of a project arriving from `astro dev` says where the old
// database is and that Astro CLI v1 can still start it. Otherwise a fresh database
// reads as the local Airflow having lost everything.
func TestAFirstStartSaysWhereTheAstroDevDatabaseIs(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	_, lines := noteRun(t, projectPath, &engineState{docker: []string{legacy}})

	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "new local Airflow database")
	assert.Contains(t, lines[0], legacy)
	assert.Contains(t, lines[0], "docker volume")
	assert.Contains(t, lines[0], "Astro CLI v1",
		"the data is reachable from v1, and naming the tool is what stays true in both frontends")
	assert.NotContains(t, lines[0], "astro dev start",
		"v2 removed that command, so pointing at it sends the user into `was removed in Astro CLI v2`")
}

// The whole point of the change: the old database is left alone. Every call
// the note makes is a lookup; nothing is created, copied or removed.
func TestTheNoteOnlyLooks(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	calls, _ := noteRun(t, projectPath, &engineState{docker: []string{legacy}})

	require.NotEmpty(t, calls)
	for _, c := range calls {
		assert.Contains(t, c, "volume ls", "the note must only look, got %v", calls)
	}
}

// v1 honored container.binary, so a podman user's volume is on the engine this
// runtime does not prefer, and the note names the engine that has it.
func TestTheNoteNamesTheEngineThatHasTheVolume(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	_, lines := noteRun(t, projectPath, &engineState{podman: []string{legacy}})

	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "podman volume "+legacy)
}

// Only a first start. A project that already has a database here has been told,
// and is not starting on a new one.
func TestNoNoteOnceThisRuntimeHasAVolume(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	calls, lines := noteRun(t, projectPath, &engineState{docker: []string{legacy, testNewVolume}})

	assert.Empty(t, lines)
	require.Len(t, calls, 1, "a project that has started here must cost one probe, got %v", calls)
}

// A daemon hiccup is not "no volume here": read that way, it would announce a
// new database to a project that has one.
func TestNoNoteWhenTheEngineCannotSayIfThisRuntimeHasAVolume(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	_, lines := noteRun(t, projectPath, &engineState{docker: []string{legacy}, lsUnknown: []string{testNewVolume}})

	assert.Empty(t, lines)
}

func TestNoNoteForAProjectThatNeverRanUnderV1(t *testing.T) {
	calls, lines := noteRun(t, t.TempDir(), &engineState{})

	assert.Empty(t, lines)
	require.Len(t, calls, 1, "without a v1 config there is nothing to look for, got %v", calls)
}

func TestNoNoteWhenTheV1ProjectHasNoVolume(t *testing.T) {
	projectPath, _ := legacyProject(t, "project:\n  name: proj\n")
	_, lines := noteRun(t, projectPath, &engineState{})

	assert.Empty(t, lines)
}

// v1 read the project name per key from the home config when the project's own
// file left it out, and the volume name follows whichever it used.
func TestTheNoteReadsTheProjectNameFromTheHomeConfigToo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ASTRO_HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".astro", "config.yaml"), []byte("project:\n  name: proj\n"), 0o600))

	projectPath, legacy := legacyProject(t, "context: astro\n")
	_, lines := noteRun(t, projectPath, &engineState{docker: []string{legacy}})

	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], legacy)
}

// And Start asks, before the up creates this runtime's volume and the question
// stops being answerable. Driven through Start because a unit test of the note
// passes just as well with the call deleted.
func TestAStartTellsAProjectArrivingFromAstroDev(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	require.NoError(t, os.Mkdir(filepath.Join(projectPath, "dags"), 0o755))
	state := &engineState{docker: []string{legacy}}
	cmd := &fakeCmd{output: state.output}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.ProjectPath = projectPath

	var lines []string
	_, err := e.Start(context.Background(), p, rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }})
	require.NoError(t, err)

	var noted bool
	for _, l := range lines {
		if strings.Contains(l, legacy) {
			noted = true
		}
	}
	assert.True(t, noted, "a first start of a v1 project must say where its old database is; lines were %v", lines)

	name, err := composeProjectName(projectPath)
	require.NoError(t, err)
	probe, up := -1, -1
	for i, c := range cmd.calls {
		if c == "docker volume ls --quiet --filter name="+name+"_"+metadataVolumeKey && probe < 0 {
			probe = i
		}
		if strings.Contains(c, " up ") && up < 0 {
			up = i
		}
	}
	require.GreaterOrEqual(t, probe, 0, "the note never asked about this runtime's volume; calls were %v", cmd.calls)
	require.GreaterOrEqual(t, up, 0)
	assert.Less(t, probe, up, "the note must ask before the up creates the volume")
}

// composeProjectName resolves symlinks and v1's hash did not, so a project
// reached through a symlink has its v1 volume under the other spelling. Missing
// it is a silent no-op, the worst outcome this file has.
func TestLegacyPathSpellingsCoversASymlinkedProject(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Symlink(target, link))

	spellings := legacyPathSpellings(link)

	resolved, err := filepath.EvalSymlinks(link)
	require.NoError(t, err)
	assert.Contains(t, spellings, link, "the spelling the caller handed us")
	assert.Contains(t, spellings, resolved, "and the one v1 would have hashed if it ran from the real path")

	// A plain directory yields exactly one, so the common case costs one lookup.
	assert.Len(t, legacyPathSpellings(resolved), 1)
}
