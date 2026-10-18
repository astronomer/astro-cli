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
// and holds a database PG_VERSION says version created. cpErr makes the copy
// fail instead.
type pgEngine struct {
	version string
	cpErr   error
	t       *testing.T
}

func (s pgEngine) output(call string) ([]byte, error) {
	switch {
	case strings.Contains(call, " volume ls ") && strings.HasSuffix(call, "_"+metadataVolumeKey):
		_, vol, _ := strings.Cut(call, "name=")
		return []byte(vol + "\n"), nil
	case strings.Contains(call, " container cp "):
		if s.cpErr != nil {
			return nil, s.cpErr
		}
		return tarOf(s.t, pgVersionFile, s.version+"\n"), nil
	}
	return nil, nil
}

// startWithVolume starts a project whose metadata volume already exists and
// returns the compose file it wrote, the calls it made and the lines it said.
func startWithVolume(t *testing.T, s pgEngine) (name, compose string, calls, lines []string) {
	t.Helper()
	s.t = t
	cmd := &fakeCmd{output: s.output}
	e := testEngine(t, cmd)
	p := testPlan(t)
	_, err := e.Start(context.Background(), p, rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }})
	require.NoError(t, err)

	name, err = composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	dir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, composeFileName))
	require.NoError(t, err)
	return name, string(b), cmd.calls, lines
}

func callIndex(calls []string, substr string) int {
	return slices.IndexFunc(calls, func(c string) bool { return strings.Contains(c, substr) })
}

// The case the whole change is for: a project whose database Postgres 12
// created keeps running on 12, because 15 refuses to start on it.
func TestAnExistingPostgres12DatabaseKeepsRunningOn12(t *testing.T) {
	name, compose, calls, lines := startWithVolume(t, pgEngine{version: "12"})

	assert.Contains(t, compose, "image: "+legacyPostgresImage+"\n")
	assert.NotContains(t, compose, "image: "+postgresImage+"\n")

	var said string
	for _, l := range lines {
		if strings.Contains(l, "PostgreSQL 12") {
			said = l
		}
	}
	require.NotEmpty(t, said, "a project pinned to an old Postgres is told so; lines were %v", lines)
	assert.Contains(t, said, legacyPostgresImage)
	assert.Contains(t, said, "astro local reset", "and how to move it to the new default")

	// Read through a container that is created and never started, with the
	// volume read-only, from the image the project runs, so a project on 12
	// pulls nothing it would not otherwise need.
	create := callIndex(calls, " container create ")
	require.GreaterOrEqual(t, create, 0, "calls were %v", calls)
	assert.Contains(t, calls[create], "--volume "+metadataVolume(name)+":"+pgDataDir+":ro")
	assert.Contains(t, calls[create], "--name "+pgProbeName(name))
	// testPlan adds no dependencies, so the image the project runs is the
	// stub's runtime base as-is.
	assert.True(t, strings.HasSuffix(calls[create], " "+newStubImages().base),
		"the probe is created from the project's own image, got %s", calls[create])
	assert.False(t, hasCall(calls, " start "+pgProbeName(name)), "the probe must never run")

	// And taken back out, before the up, so it never holds the volume compose
	// is about to mount.
	up := callIndex(calls, " up ")
	rms := 0
	for i, c := range calls {
		if c == "docker container rm --force "+pgProbeName(name) {
			rms++
			assert.Less(t, i, up, "the probe is removed before the up")
		}
	}
	assert.Equal(t, 2, rms, "one leftover cleared before the create, and the probe itself after; calls were %v", calls)
}

func TestAnExistingPostgres15DatabaseRunsTheDefaultQuietly(t *testing.T) {
	_, compose, _, lines := startWithVolume(t, pgEngine{version: "15"})

	assert.Contains(t, compose, "image: "+postgresImage+"\n")
	for _, l := range lines {
		assert.NotContains(t, l, "PostgreSQL", "a database on the default has nothing to be told")
	}
}

// A version that cannot be read is not a reason to refuse the start, but it is
// said, because when the guess is wrong the database will not start and nothing
// else explains why.
func TestAnUnreadableVersionStartsOnTheDefaultAndSaysSo(t *testing.T) {
	name, compose, calls, lines := startWithVolume(t, pgEngine{cpErr: errors.New("exit status 1")})

	assert.Contains(t, compose, "image: "+postgresImage+"\n")
	var warned bool
	for _, l := range lines {
		if strings.Contains(l, "could not read which PostgreSQL version") {
			warned = true
			assert.Contains(t, l, "astro local reset")
		}
	}
	assert.True(t, warned, "lines were %v", lines)
	assert.Equal(t, "docker container rm --force "+pgProbeName(name), calls[callIndex(calls, " container cp ")+1],
		"a failed read still removes its probe")
}

// A new project has no volume, and creating a container that mounts one would
// create it: nothing is probed.
func TestANewProjectGetsTheDefaultWithoutAProbe(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	assert.False(t, hasCall(cmd.calls, " container "), "calls were %v", cmd.calls)
	dir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, composeFileName))
	require.NoError(t, err)
	assert.Contains(t, string(b), "image: "+postgresImage+"\n")
}

// An engine that cannot say whether the volume is there is not asked to mount
// it either: the probe could create it, and the up is about to fail anyway.
func TestAnUnknownVolumeIsNotProbed(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.Contains(call, " volume ls ") {
			return nil, errors.New("cannot connect to the daemon")
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)
	got := e.metadataPostgresImage(context.Background(), engineConn{bin: "docker"}, testComposeProject, "img",
		e.lookupVolume(context.Background(), engineConn{bin: "docker"}, metadataVolume(testComposeProject)), rt.Callbacks{})
	assert.Equal(t, postgresImage, got)
	assert.False(t, hasCall(cmd.calls, " container "), "calls were %v", cmd.calls)
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
// will not delete a volume in use: a reset clears it before the down.
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
