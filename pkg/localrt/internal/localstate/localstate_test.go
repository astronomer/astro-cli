package localstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
)

// setCache points the cache root at a fresh temp dir.
func setCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func sampleRecord(t *testing.T) Record {
	t.Helper()
	return Record{
		ProjectPath:     t.TempDir(),
		Mode:            rt.ModeDocker,
		ComposeProject:  "astro-demo-abc123",
		Port:            8081,
		Hostname:        "demo.localhost",
		StartedAt:       time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		StopWithSession: true,
		PID:             4242,
	}
}

func TestRecordRoundTrip(t *testing.T) {
	setCache(t)
	rec := sampleRecord(t)
	require.NoError(t, Save(rec))

	got, err := Load(rec.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, rec, got)
}

func TestLoadMissingIsErrNotRunning(t *testing.T) {
	setCache(t)
	_, err := Load(t.TempDir())
	assert.ErrorIs(t, err, ErrNotRunning)
}

func TestSaveDoesNotTouchUserstateFile(t *testing.T) {
	setCache(t)
	rec := sampleRecord(t)
	dir, err := rt.StateDir(rec.ProjectPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	prefs := []byte(`{"port": 9090}` + "\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), prefs, 0o600))

	require.NoError(t, Save(rec))

	got, err := os.ReadFile(filepath.Join(dir, "state.json"))
	require.NoError(t, err)
	assert.Equal(t, prefs, got, "the runtime record must live beside, not inside, userstate's file")
}

func TestRemoveIsIdempotent(t *testing.T) {
	setCache(t)
	rec := sampleRecord(t)
	require.NoError(t, Save(rec))
	require.NoError(t, Remove(rec.ProjectPath))
	require.NoError(t, Remove(rec.ProjectPath))
	_, err := Load(rec.ProjectPath)
	assert.ErrorIs(t, err, ErrNotRunning)
}

func TestListScansAllProjects(t *testing.T) {
	setCache(t)
	a, b := sampleRecord(t), sampleRecord(t)
	b.Mode = rt.ModeStandalone
	b.ComposeProject = ""
	require.NoError(t, Save(a))
	require.NoError(t, Save(b))

	// A directory without a record (e.g. only userstate prefs) is skipped.
	dir, err := rt.StateDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))

	recs, err := List()
	require.NoError(t, err)
	require.Len(t, recs, 2)
	paths := []string{recs[0].ProjectPath, recs[1].ProjectPath}
	assert.ElementsMatch(t, []string{a.ProjectPath, b.ProjectPath}, paths)
}

func TestListEmptyCache(t *testing.T) {
	setCache(t)
	recs, err := List()
	require.NoError(t, err)
	assert.Empty(t, recs)
}

func TestStatusFromRecord(t *testing.T) {
	rec := sampleRecord(t)

	st := rec.Status(true)
	assert.Equal(t, rt.StateRunning, st.State)
	assert.Equal(t, rec.ProjectPath, st.ProjectPath)
	assert.Equal(t, rec.Port, st.Port)
	assert.Equal(t, rec.Hostname, st.Hostname)
	assert.Equal(t, rec.StartedAt, st.StartedAt)
	assert.True(t, st.StopWithSession)

	// A stopped record's pid and port are stale, so Status omits them: the
	// process is gone and the port is reassignable.
	stopped := rec.Status(false)
	assert.Equal(t, rt.StateStopped, stopped.State)
	assert.Zero(t, stopped.PID)
	assert.Zero(t, stopped.Port)
	// Running keeps them.
	assert.Equal(t, rec.PID, rec.Status(true).PID)
}
