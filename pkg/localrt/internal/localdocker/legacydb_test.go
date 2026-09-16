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

func TestPostgresMajor(t *testing.T) {
	assert.Equal(t, "12", postgresMajor("docker.io/postgres:12.6"))
	assert.Equal(t, "13", postgresMajor("postgres:13"))
	assert.Empty(t, postgresMajor("postgres"))
	// A registry host may carry a port, so the tag is after the last colon that
	// follows the last slash — not the first colon in the string.
	assert.Equal(t, "12", postgresMajor("localhost:5000/postgres:12.6"))
}

// The copy writes to a name built from metadataVolumeKey and compose mounts the
// name built from the template, so a rename on either side would leave the
// database in a volume nothing ever mounts — with the user told it was carried
// over. Nothing else ties the two together.
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

// engineState scripts a fake engine: which volumes exist, and optional
// overrides for the other calls adoption makes.
type engineState struct {
	volumes   []string
	running   string // container ids for `ps --quiet --filter volume=`
	names     string // container names for `ps --all --filter label=`
	pgVersion string
	fail      string   // substring of the call that should fail
	lsUnknown []string // volumes whose lookup cannot reach the daemon
}

func (s *engineState) output(call string) ([]byte, error) {
	if s.fail != "" && strings.Contains(call, s.fail) {
		return nil, errors.New("scripted failure")
	}
	switch {
	case strings.Contains(call, "volume ls"):
		for _, v := range s.lsUnknown {
			if strings.HasSuffix(call, "name="+v) {
				return nil, errors.New("cannot connect to the daemon")
			}
		}
		for _, v := range s.volumes {
			if strings.HasSuffix(call, "name="+v) {
				return []byte(v + "\n"), nil
			}
		}
		return nil, nil
	case strings.Contains(call, "ps --quiet"):
		return []byte(s.running), nil
	case strings.Contains(call, "ps --all"):
		return []byte(s.names), nil
	case strings.Contains(call, "PG_VERSION"):
		if s.pgVersion == "" {
			return []byte("12\n"), nil
		}
		return []byte(s.pgVersion), nil
	}
	return nil, nil
}

// adoptRun drives one adoption against a scripted engine and reports what ran.
func adoptRun(t *testing.T, projectPath, major string, s *engineState) (calls, lines []string) {
	t.Helper()
	cmd := &fakeCmd{output: s.output}
	e := testEngine(t, cmd)
	cb := rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }}
	e.adoptLegacyMetadataDB(context.Background(), engineConn{bin: "docker"}, projectPath, testComposeProject, major, cb)
	return cmd.calls, lines
}

func joined(calls []string) string { return strings.Join(calls, " | ") }

func TestAdoptSkipsWhenThisRuntimeAlreadyHasAVolume(t *testing.T) {
	projectPath, _ := legacyProject(t, "project:\n  name: proj\n")
	calls, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{testNewVolume}})

	require.Len(t, calls, 1, "a project that has already started here must cost one probe, got %v", calls)
	assert.Empty(t, lines)
}

// The one that would destroy data: "absent" is the branch that copies INTO this
// name, so an unanswerable probe must never be read as absent.
func TestAdoptStopsWhenTheEngineCannotSayIfThisRuntimeHasAVolume(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	// The legacy volume is perfectly findable; only the question "does this
	// runtime already have one" goes unanswered. Without that isolation the
	// copy would be skipped for the wrong reason and the test would pass with
	// the guard removed.
	calls, lines := adoptRun(t, projectPath, "3", &engineState{
		volumes:   []string{legacy},
		lsUnknown: []string{testNewVolume},
	})

	assert.NotContains(t, joined(calls), "cp -a",
		"a daemon hiccup must not be read as \"no database here\" and overwrite a live one")
	assert.Empty(t, lines)
}

func TestAdoptSkipsProjectThatNeverRanUnderV1(t *testing.T) {
	calls, lines := adoptRun(t, t.TempDir(), "3", &engineState{})

	require.Len(t, calls, 1, "without a v1 config there is nothing to look for, got %v", calls)
	assert.Empty(t, lines)
}

func TestAdoptCopiesTheLegacyDatabase(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	calls, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{legacy}})

	var create, copyCall string
	for _, c := range calls {
		if strings.Contains(c, "volume create") {
			create = c
		}
		if strings.Contains(c, "cp -a") {
			copyCall = c
		}
	}

	require.NotEmpty(t, create, "the destination must be created with compose's labels, got %v", calls)
	assert.Contains(t, create, "--label com.docker.compose.project="+testComposeProject,
		"an unlabeled volume is invisible to the file-less `down --volumes` the teardown runs")
	assert.Contains(t, create, "--label com.docker.compose.volume="+metadataVolumeKey)
	assert.Contains(t, create, " "+testNewVolume)

	require.NotEmpty(t, copyCall, "expected a copy, got %v", calls)
	assert.Contains(t, copyCall, "-v "+legacy+":/src:ro", "the v1 volume must be mounted read-only")
	assert.Contains(t, copyCall, "-v "+testNewVolume+":/dst")
	assert.Contains(t, copyCall, "chmod 700 /dst", "postgres refuses to start on a group-readable data directory")
	assert.Contains(t, copyCall, "rm -f /dst/postmaster.pid", "a stale lock file can stop postgres coming up")
	assert.NotContains(t, joined(calls), "volume rm", "a successful copy must not remove anything")

	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "carried over")
	assert.Contains(t, lines[0], "left on disk untouched")
}

// Every refusal must name a recovery that can actually work. The start
// continues and compose then creates an empty volume, so without removing that
// volume the retry is foreclosed forever.
func TestEveryRefusalOffersAWorkableRecovery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state engineState
		want  string
	}{
		{"v1 stack running", engineState{running: "deadbeef\n"}, "astro dev stop"},
		{"engine cannot say", engineState{fail: "ps --quiet"}, "could not say"},
		{"pg version unreadable", engineState{fail: "PG_VERSION"}, "could not be read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
			tc.state.volumes = []string{legacy}
			calls, lines := adoptRun(t, projectPath, "3", &tc.state)

			assert.NotContains(t, joined(calls), "cp -a")
			require.Len(t, lines, 1)
			assert.Contains(t, lines[0], tc.want)
			assert.Contains(t, lines[0], "astro local reset",
				"a refusal that does not say how to clear the empty volume can never be acted on")
		})
	}
}

// The unreadable-version case must not render as the mismatch case, which
// printed an empty number: "it is Postgres  data and this runtime runs 12".
func TestAdoptDistinguishesAnUnreadableVersionFromAMismatch(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	_, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{legacy}, fail: "PG_VERSION"})
	require.Len(t, lines, 1)
	assert.NotContains(t, lines[0], "Postgres  ", "an empty version number means the wrong branch reported")
}

func TestAdoptRefusesOnAPostgresMajorMismatch(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	calls, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{legacy}, pgVersion: "13\n"})

	assert.NotContains(t, joined(calls), "cp -a")
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "Postgres 13")
	assert.Contains(t, lines[0], "Postgres 12")
}

func TestAdoptRefusesCustomPostgresCredentials(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"superuser", "project:\n  name: proj\npostgres:\n  user: astro\n", `"astro"`},
		{"password", "project:\n  name: proj\npostgres:\n  password: s3cret\n", "postgres.password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectPath, legacy := legacyProject(t, tc.body)
			calls, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{legacy}})

			assert.NotContains(t, joined(calls), "cp -a",
				"the entrypoint only applies POSTGRES_USER/PASSWORD to an empty data directory, so a copy keeps v1's")
			require.Len(t, lines, 1)
			assert.Contains(t, lines[0], tc.want)
		})
	}
}

// v1 resolved these keys project-first then home, so a global setting has to be
// seen or it slips past the guard above.
func TestAdoptReadsCredentialsFromTheHomeConfigToo(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(home, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".astro", "config.yaml"),
		[]byte("postgres:\n  user: astro\n"), 0o600))
	t.Setenv("ASTRO_HOME", home)

	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	calls, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{legacy}})

	assert.NotContains(t, joined(calls), "cp -a")
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], `"astro"`)
}

// An Airflow 2 database under an Airflow 3 image means db-migration rewrites
// the user's schema irreversibly, or aborts and takes every service with it.
func TestAdoptRefusesAcrossAirflowGenerations(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	legacyProj := strings.TrimSuffix(legacy, "_"+metadataVolumeKey)
	calls, lines := adoptRun(t, projectPath, "3", &engineState{
		volumes: []string{legacy},
		names:   legacyProj + "-webserver-1\n" + legacyProj + "-scheduler-1\n",
	})

	assert.NotContains(t, joined(calls), "cp -a")
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "Airflow 2")
	assert.Contains(t, lines[0], "Airflow 3")
}

// The mirror of the case above: an Airflow 2 project moving to an Airflow 2
// runtime is exactly what should be carried over.
func TestAdoptProceedsForAnAirflow2Project(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	legacyProj := strings.TrimSuffix(legacy, "_"+metadataVolumeKey)
	calls, lines := adoptRun(t, projectPath, "2", &engineState{
		volumes: []string{legacy},
		names:   legacyProj + "-webserver-1\n" + legacyProj + "-scheduler-1\n",
	})

	assert.Contains(t, joined(calls), "cp -a")
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "carried over")
}

func TestAdoptProceedsWhenTheGenerationsAgree(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	legacyProj := strings.TrimSuffix(legacy, "_"+metadataVolumeKey)
	calls, _ := adoptRun(t, projectPath, "3", &engineState{
		volumes: []string{legacy},
		names:   legacyProj + "-api-server-1\n" + legacyProj + "-dag-processor-1\n",
	})

	assert.Contains(t, joined(calls), "cp -a")
}

// v1 honored container.binary, so a podman user's volumes are not on the
// engine this runtime now prefers.
func TestAdoptFindsALegacyVolumeOnTheOtherEngine(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		// Only podman knows the legacy volume.
		if strings.HasPrefix(call, "podman") && strings.Contains(call, "volume ls") &&
			strings.HasSuffix(call, "name="+legacy) {
			return []byte(legacy + "\n"), nil
		}
		if strings.Contains(call, "volume ls") {
			return nil, nil
		}
		if strings.Contains(call, "PG_VERSION") {
			return []byte("12\n"), nil
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)
	var lines []string
	cb := rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }}
	e.adoptLegacyMetadataDB(context.Background(), engineConn{bin: "docker"}, projectPath, testComposeProject, "3", cb)

	assert.Contains(t, joined(cmd.calls), "podman run --rm -v "+legacy+":/src:ro",
		"the copy must read from the engine that actually has the volume, got %v", cmd.calls)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "carried over")
}

func TestAdoptDropsAHalfWrittenVolumeWhenTheCopyFails(t *testing.T) {
	projectPath, legacy := legacyProject(t, "project:\n  name: proj\n")
	calls, lines := adoptRun(t, projectPath, "3", &engineState{volumes: []string{legacy}, fail: "cp -a"})

	assert.Contains(t, joined(calls), "volume rm --force "+testNewVolume,
		"a partial data directory would stop postgres coming up at all, got %v", calls)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "could not carry over")
}

// The copy commonly fails BECAUSE the caller's deadline expired, and os/exec
// refuses to spawn on a canceled context — so a cleanup sharing that context is
// a no-op in exactly the case it exists for, leaving a torn data directory that
// every later start reads as "already migrated".
func TestHalfWrittenVolumeIsRemovedEvenWhenTheContextIsDone(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	e.removeHalfWrittenVolume(ctx, engineConn{bin: "docker"}, testNewVolume)

	assert.Contains(t, joined(cmd.calls), "volume rm --force "+testNewVolume,
		"the removal must run on a context detached from the caller's, got %v", cmd.calls)
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
