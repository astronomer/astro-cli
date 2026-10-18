package localdocker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func TestParsePGVersion(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"12\n", "12"},
		{"15", "15"},
		{" 16 \n", "16"},
		{"9.6\n", "9.6"},
	} {
		got, err := parsePGVersion(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got)
	}
	// The answer becomes an image tag, so anything that is not a version is
	// refused rather than passed through.
	for _, in := range []string{"", "\n", "abc", "15beta", "0", "12:latest", "12\n13", "../15"} {
		_, err := parsePGVersion(in)
		assert.Error(t, err, "%q", in)
	}
}

func TestImageForPGVersion(t *testing.T) {
	assert.Equal(t, postgresImage, imageForPGVersion(postgresMajor))
	assert.Equal(t, "docker.io/postgres:15", postgresImage, "new projects get Postgres 15")
	assert.Equal(t, legacyPostgresImage, imageForPGVersion("12"),
		"a project on 12 keeps the exact image it was created with, so the upgrade costs it no pull")
	assert.Equal(t, "docker.io/postgres:12.6", legacyPostgresImage)
	assert.Equal(t, "docker.io/postgres:16", imageForPGVersion("16"))
	assert.Equal(t, "docker.io/postgres:9.6", imageForPGVersion("9.6"))
}

// tarOf is a copy stream the way `cp <container>:<file> -` writes one, with
// a directory entry ahead of the file to show those are skipped.
func tarOf(t *testing.T, name, contents string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "data/", Typeflag: tar.TypeDir, Mode: 0o755}))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(contents))}))
	_, err := tw.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func TestFirstFileInTar(t *testing.T) {
	got, err := firstFileInTar(tarOf(t, pgVersionFile, "12\n"))
	require.NoError(t, err)
	assert.Equal(t, "12\n", got)

	var empty bytes.Buffer
	require.NoError(t, tar.NewWriter(&empty).Close())
	_, err = firstFileInTar(empty.Bytes())
	require.ErrorIs(t, err, errNoFileInArchive)

	_, err = firstFileInTar([]byte("not a tar stream at all, and long enough to fill a header block"))
	assert.Error(t, err)
}

// pgEngine scripts a fake engine whose metadata volume for any project exists
// and holds a database PG_VERSION says version created.
type pgEngine struct {
	t       *testing.T
	version string
	// createdAt is what `volume inspect` reports; "" makes it fail.
	createdAt string
	// cpErr makes the copy fail.
	cpErr error
	// imageMissing makes `image inspect` fail, and pullErr the pull after it.
	imageMissing bool
	pullErr      error
}

func (s *pgEngine) output(call string) ([]byte, error) {
	switch {
	case strings.Contains(call, " volume ls ") && strings.HasSuffix(call, "_"+metadataVolumeKey):
		_, vol, _ := strings.Cut(call, "name=")
		return []byte(vol + "\n"), nil
	case strings.Contains(call, " volume inspect "):
		if s.createdAt == "" {
			return nil, errors.New("exit status 1")
		}
		return []byte(s.createdAt + "\n"), nil
	case strings.Contains(call, " image inspect "):
		if s.imageMissing {
			return nil, errors.New("exit status 1")
		}
		return []byte("sha256:abc\n"), nil
	case strings.Contains(call, " container cp "):
		if s.cpErr != nil {
			return nil, s.cpErr
		}
		return tarOf(s.t, pgVersionFile, s.version+"\n"), nil
	}
	return nil, nil
}

func (s *pgEngine) run(call string, _ rt.Stdio) error {
	if strings.HasPrefix(call, "docker pull ") {
		return s.pullErr
	}
	return nil
}

// pgStart is one start against a pgEngine, and what came of it.
type pgStart struct {
	name, compose string
	calls, lines  []string
}

// startOn starts the project at projectPath on e, and returns the compose
// file it wrote, the calls it made and the lines it said.
func startOn(t *testing.T, e *Engine, cmd *fakeCmd, projectPath string) pgStart {
	t.Helper()
	cmd.calls = nil
	p := testPlan(t)
	p.ProjectPath = projectPath
	var out pgStart
	_, err := e.Start(context.Background(), p, rt.Callbacks{OnLine: func(l rt.LogLine) { out.lines = append(out.lines, l.Text) }})
	require.NoError(t, err)

	out.name, err = composeProjectName(projectPath)
	require.NoError(t, err)
	dir, err := rt.StateDir(projectPath)
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, composeFileName))
	require.NoError(t, err)
	out.compose = string(b)
	out.calls = cmd.calls
	return out
}

// newPGEngine is an engine over s, and a project directory to start.
func newPGEngine(t *testing.T, s *pgEngine) (*Engine, *fakeCmd, string) {
	t.Helper()
	s.t = t
	cmd := &fakeCmd{output: s.output, run: s.run}
	e := testEngine(t, cmd)
	return e, cmd, testPlan(t).ProjectPath
}

// startWithVolume starts a project whose metadata volume already exists.
func startWithVolume(t *testing.T, s *pgEngine) pgStart {
	t.Helper()
	e, cmd, project := newPGEngine(t, s)
	return startOn(t, e, cmd, project)
}

func callIndex(calls []string, substr string) int {
	return slices.IndexFunc(calls, func(c string) bool { return strings.Contains(c, substr) })
}

func linesWith(lines []string, substr string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, substr) {
			out = append(out, l)
		}
	}
	return out
}

// The case the whole change is for: a project whose database Postgres 12
// created keeps running on 12, because 15 refuses to start on it.
func TestAnExistingPostgres12DatabaseKeepsRunningOn12(t *testing.T) {
	got := startWithVolume(t, &pgEngine{version: "12", createdAt: "2026-01-02T03:04:05Z"})

	assert.Contains(t, got.compose, "image: "+legacyPostgresImage+"\n")
	assert.NotContains(t, got.compose, "image: "+postgresImage+"\n")

	said := linesWith(got.lines, "created by PostgreSQL 12")
	require.Len(t, said, 1, "a project pinned to an old Postgres is told so; lines were %v", got.lines)
	assert.Contains(t, said[0], legacyPostgresImage)
	assert.Contains(t, said[0], "astro local reset", "and how to move it to the new default")

	// Read through a container that is created and never started, with the
	// volume read-only, from the image the project runs, so a project on 12
	// pulls nothing it would not otherwise need.
	create := callIndex(got.calls, " container create ")
	require.GreaterOrEqual(t, create, 0, "calls were %v", got.calls)
	assert.Contains(t, got.calls[create], "--volume "+metadataVolume(got.name)+":"+pgDataDir+":ro")
	assert.Contains(t, got.calls[create], "--name "+pgProbeName(got.name))
	// Labeled as the project's, so whatever counts a project's containers
	// counts this one.
	assert.Contains(t, got.calls[create], "--label "+composeProjectLabel+"="+got.name)
	// testPlan adds no dependencies, so the image the project runs is the
	// stub's runtime base as-is.
	assert.True(t, strings.HasSuffix(got.calls[create], " "+newStubImages().base),
		"the probe is created from the project's own image, got %s", got.calls[create])
	assert.Equal(t, -1, callIndex(got.calls, " pull "), "an image the engine has is not pulled")

	// And taken back out, before the up, so it never holds the volume compose
	// is about to mount.
	up := callIndex(got.calls, " up ")
	rms := 0
	for i, c := range got.calls {
		if c == "docker container rm --force "+pgProbeName(got.name) {
			rms++
			assert.Less(t, i, up, "the probe is removed before the up")
		}
	}
	assert.Equal(t, 2, rms, "one leftover cleared before the create, and the probe itself after; calls were %v", got.calls)
}

// What was read is remembered for the volume, so the next start neither probes
// nor repeats the note: the user decided the note is said once per project.
func TestTheVersionIsReadAndNotedOncePerVolume(t *testing.T) {
	s := &pgEngine{version: "12", createdAt: "2026-01-02T03:04:05Z"}
	e, cmd, project := newPGEngine(t, s)
	first := startOn(t, e, cmd, project)
	require.Len(t, linesWith(first.lines, "created by PostgreSQL 12"), 1)

	second := startOn(t, e, cmd, project)
	assert.Contains(t, second.compose, "image: "+legacyPostgresImage+"\n", "the remembered version still decides the image")
	assert.Empty(t, linesWith(second.lines, "PostgreSQL"), "said once, not every start")
	assert.False(t, hasCall(second.calls, " container "), "a remembered volume is not probed; calls were %v", second.calls)
}

// A reset deletes the volume and a start makes a new one under the same name.
// The record must not hand the new database the old one's version.
func TestARecreatedVolumeIsReadAgain(t *testing.T) {
	s := &pgEngine{version: "12", createdAt: "2026-01-02T03:04:05Z"}
	e, cmd, project := newPGEngine(t, s)
	startOn(t, e, cmd, project)

	s.version, s.createdAt = "15", "2026-02-03T04:05:06Z"
	again := startOn(t, e, cmd, project)
	assert.True(t, hasCall(again.calls, " container create "), "calls were %v", again.calls)
	assert.Contains(t, again.compose, "image: "+postgresImage+"\n")
}

// Without a creation time there is nothing to key a record by, so every start
// reads the volume.
func TestAVolumeWithoutACreationTimeIsReadEveryStart(t *testing.T) {
	s := &pgEngine{version: "12"}
	e, cmd, project := newPGEngine(t, s)
	startOn(t, e, cmd, project)
	again := startOn(t, e, cmd, project)
	assert.True(t, hasCall(again.calls, " container create "), "calls were %v", again.calls)
	assert.Contains(t, again.compose, "image: "+legacyPostgresImage+"\n")
}

func TestAnExistingPostgres15DatabaseRunsTheDefaultQuietly(t *testing.T) {
	got := startWithVolume(t, &pgEngine{version: "15", createdAt: "2026-01-02T03:04:05Z"})

	assert.Contains(t, got.compose, "image: "+postgresImage+"\n")
	assert.Empty(t, linesWith(got.lines, "PostgreSQL"), "a database on the default has nothing to be told")
}

// An existing volume whose version cannot be read runs 12.6, not the default:
// every volume made before 15 became the default holds a 12 database, and 15
// on one of those never comes up. It is said, because when the guess is wrong
// nothing else explains why.
func TestAnUnreadableVersionStartsOnTheLegacyImageAndSaysSo(t *testing.T) {
	got := startWithVolume(t, &pgEngine{cpErr: errors.New("exit status 1"), createdAt: "2026-01-02T03:04:05Z"})

	assert.Contains(t, got.compose, "image: "+legacyPostgresImage+"\n")
	warned := linesWith(got.lines, "could not read which PostgreSQL version")
	require.Len(t, warned, 1, "lines were %v", got.lines)
	assert.Contains(t, warned[0], legacyPostgresImage)
	assert.Contains(t, warned[0], "astro local reset")
	assert.Equal(t, "docker container rm --force "+pgProbeName(got.name), got.calls[callIndex(got.calls, " container cp ")+1],
		"a failed read still removes its probe")
}

// A probe image that is not there and cannot be pulled is a network problem,
// and says so, rather than sending someone to look at their database.
func TestAPullThatFailsIsReportedAsOne(t *testing.T) {
	got := startWithVolume(t, &pgEngine{imageMissing: true, pullErr: errors.New("no route to host"), createdAt: "x"})

	assert.Contains(t, got.compose, "image: "+legacyPostgresImage+"\n")
	warned := linesWith(got.lines, "could not pull "+newStubImages().base)
	require.Len(t, warned, 1, "lines were %v", got.lines)
	assert.Contains(t, warned[0], "no route to host")
	assert.Empty(t, linesWith(got.lines, "could not read which"))
	assert.False(t, hasCall(got.calls, " container create "), "nothing to create a probe from; calls were %v", got.calls)
}

// A missing image that does pull goes on to be read.
func TestAMissingProbeImageIsPulledFirst(t *testing.T) {
	got := startWithVolume(t, &pgEngine{version: "12", imageMissing: true, createdAt: "x"})

	pull := callIndex(got.calls, " pull ")
	create := callIndex(got.calls, " container create ")
	require.GreaterOrEqual(t, pull, 0, "calls were %v", got.calls)
	assert.Less(t, pull, create)
	assert.Contains(t, got.compose, "image: "+legacyPostgresImage+"\n")
}

// When the project's compose override gives postgres its own image, that image
// is what runs: nothing is probed, and nothing is promised about a reset.
func TestAnOverridePostgresImageIsLeftToTheOverride(t *testing.T) {
	s := &pgEngine{version: "12", createdAt: "x"}
	e, cmd, project := newPGEngine(t, s)
	require.NoError(t, os.WriteFile(filepath.Join(project, rt.ComposeOverrideFile),
		[]byte("services:\n  postgres:\n    image: docker.io/postgres:16\n"), 0o600))

	got := startOn(t, e, cmd, project)
	assert.False(t, hasCall(got.calls, " container "), "calls were %v", got.calls)
	assert.Empty(t, linesWith(got.lines, "PostgreSQL"), "lines were %v", got.lines)
}

// An override that leaves postgres alone changes nothing about the choice.
func TestAnOverrideOfOtherServicesStillPins(t *testing.T) {
	s := &pgEngine{version: "12", createdAt: "x"}
	e, cmd, project := newPGEngine(t, s)
	require.NoError(t, os.WriteFile(filepath.Join(project, rt.ComposeOverrideFile),
		[]byte("services:\n  scheduler:\n    environment:\n      A: b\n"), 0o600))

	got := startOn(t, e, cmd, project)
	assert.Contains(t, got.compose, "image: "+legacyPostgresImage+"\n")
}

// A new project has no volume, and creating a container that mounts one would
// create it: nothing is probed.
func TestANewProjectGetsTheDefaultWithoutAProbe(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	got := startOn(t, e, cmd, testPlan(t).ProjectPath)

	assert.False(t, hasCall(got.calls, " container "), "calls were %v", got.calls)
	assert.False(t, hasCall(got.calls, " volume inspect "), "calls were %v", got.calls)
	assert.Contains(t, got.compose, "image: "+postgresImage+"\n")
}

// An engine that cannot say whether the volume is there is not asked to mount
// it — the probe could create it — and is not given 15 either, which an
// existing volume would refuse.
func TestAnUnknownVolumeIsNotProbedAndRunsTheLegacyImage(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	var lines []string
	got := e.metadataPostgresImage(context.Background(), pgChoice{
		conn: engineConn{bin: "docker"}, stateDir: t.TempDir(), projectPath: t.TempDir(),
		composeProject: testComposeProject, probeImage: "img", own: volumeUnknown,
	}, rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }})
	assert.Equal(t, legacyPostgresImage, got)
	assert.Empty(t, cmd.calls)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "could not tell whether this project already has a database")
}

// A start canceled mid-probe still takes the probe back out: left behind, it
// would hold the volume and keep a reset from deleting it.
func TestACanceledProbeIsStillRemoved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.Contains(call, " container cp ") {
			cancel()
			return nil, context.Canceled
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)

	_, err := e.readPGVersion(ctx, engineConn{bin: "docker"}, testComposeProject, "img")
	require.Error(t, err)
	assert.Equal(t, "docker container rm --force "+pgProbeName(testComposeProject), cmd.calls[len(cmd.calls)-1])
}

// A probe an interrupted start left behind refers to the volume, and the engine
// will not delete a volume in use. Compose does not see the probe — it lists
// only containers it created — so a reset removes it by name before the down.
func TestCleanRemovesALeftoverProbeBeforeTheVolume(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	project := t.TempDir()
	composeMarker(t, project)
	name, err := composeProjectName(project)
	require.NoError(t, err)

	_, _, err = e.Clean(context.Background(), project)
	require.NoError(t, err)

	rm := callIndex(cmd.calls, "container rm --force "+pgProbeName(name))
	down := callIndex(cmd.calls, " down ")
	require.GreaterOrEqual(t, rm, 0, "calls were %v", cmd.calls)
	assert.Less(t, rm, down)
}

// The state directory is a cache and the volume outlives it: a reset whose
// compose file is gone still removes a volume that is there, and a probe with
// it.
func TestCleanTakesDownAVolumeThatOutlivedTheStateDirectory(t *testing.T) {
	project := t.TempDir()
	name, err := composeProjectName(project)
	require.NoError(t, err)
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if call == "docker volume ls --quiet --filter name="+metadataVolume(name) {
			return []byte(metadataVolume(name) + "\n"), nil
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)

	got, reached, err := e.Clean(context.Background(), project)
	require.NoError(t, err)
	assert.True(t, reached)
	assert.Equal(t, name, got)
	rm := callIndex(cmd.calls, "container rm --force "+pgProbeName(name))
	down := callIndex(cmd.calls, " down --timeout 10 --volumes --remove-orphans")
	require.GreaterOrEqual(t, rm, 0, "calls were %v", cmd.calls)
	require.GreaterOrEqual(t, down, 0, "calls were %v", cmd.calls)
	assert.Less(t, rm, down)
}

func TestACleanStopRemovesALeftoverProbeBeforeTheVolume(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	af, _ := startProject(t, e, p)
	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	cmd.calls = nil

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{Clean: true}))

	rm := callIndex(cmd.calls, "container rm --force "+pgProbeName(name))
	down := callIndex(cmd.calls, " down ")
	require.GreaterOrEqual(t, rm, 0, "calls were %v", cmd.calls)
	assert.Less(t, rm, down)
}
