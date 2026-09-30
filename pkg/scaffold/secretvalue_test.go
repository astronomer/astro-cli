package scaffold

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// comparingWriter is a recordingWriter that can compare values, and can say
// its copies live somewhere other than the vault, as Astro Desktop's .env.
type comparingWriter struct {
	*recordingWriter
	where string
	// notCompared names what the writer declines to compare, as a writer
	// that will not read the keyring at plan time answers for a vault copy.
	notCompared map[string]bool
}

func (w *comparingWriter) HasSecretValue(_ secrets.Kind, name, value string) (SecretHeld, error) {
	v, ok := w.held[name]
	if w.notCompared[name] {
		// Equal without Compared means nothing, and must be ignored.
		return SecretHeld{Held: ok, Equal: true, Where: w.where}, w.hasErr
	}
	return SecretHeld{Held: ok, Compared: ok, Equal: ok && v == value, Where: w.where}, w.hasErr
}

// An equal value already held is not a conflict: it is carried, and the file
// retires as though nothing were held.
func TestAnEqualHeldValueIsCarried(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := &comparingWriter{recordingWriter: newRecordingWriter(), where: ".env"}
	writer.held["API_TOKEN"] = "tok-123"
	writer.held["warehouse"] = `{"conn_type": "snowflake", "password": "hunter2"}`

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Equal(t, "tok-123", writer.stored["API_TOKEN"], "the equal value is stored, so a writer can move it")
	require.False(t, anyContains(res.Advisories, "API_TOKEN: kept"), "%v", res.Advisories)
	// warehouse's held value is not the carried one, so it is kept, and the
	// advisory says where it is.
	require.NotContains(t, writer.stored, "warehouse")
	require.Equal(t, "warehouse: kept the value already in .env. "+SettingsRelPath+
		" holds a different one, which was not carried over it", findAdvisory(t, res.Advisories, "kept the value"))
	require.Equal(t,
		SettingsRelPath+" still contains API_TOKEN and warehouse in plaintext, and is kept because .env "+
			"already held warehouse, so the file's value was not carried over it",
		findAdvisory(t, res.Advisories, "plaintext"))
}

func TestEveryValueEqualRetiresTheFile(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := &comparingWriter{recordingWriter: newRecordingWriter()}
	writer.held["API_TOKEN"] = "tok-123"

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	require.True(t, anyContains(cs.Deleted, SettingsRelPath), "%v", cs.Deleted)
	_, err = cs.Apply()
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Equal(t, "tok-123", writer.stored["API_TOKEN"])
}

// A writer with only HasSecret cannot compare, so the advisory does not claim
// the values differ.
func TestAWriterThatCannotCompareSaysOnlyThatOneIsHeld(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := newRecordingWriter()
	writer.held["API_TOKEN"] = "tok-123"

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)
	require.NotContains(t, writer.stored, "API_TOKEN")
	require.Equal(t, "API_TOKEN: kept the value already in your vault. "+SettingsRelPath+
		" holds one too, which was not carried over it", findAdvisory(t, res.Advisories, "kept the value"))
}

// "Not compared" keeps the conservative behavior, even for a value that is
// in fact equal: kept, the file stays, and the advisory claims no difference.
func TestANotComparedHeldValueIsKept(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := &comparingWriter{recordingWriter: newRecordingWriter(), notCompared: map[string]bool{"API_TOKEN": true}}
	writer.held["API_TOKEN"] = "tok-123"

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)
	require.NotContains(t, writer.stored, "API_TOKEN")
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Equal(t, "API_TOKEN: kept the value already in your vault. "+SettingsRelPath+
		" holds one too, which was not carried over it", findAdvisory(t, res.Advisories, "kept the value"))
}
