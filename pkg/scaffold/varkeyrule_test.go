package scaffold

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A 1.x variable whose key starts with a digit cannot be stored as an Airflow
// Variable, so the conversion refuses to carry the file and names the key.
func TestAVariableKeyWithALeadingDigitIsNotCarried(t *testing.T) {
	dir := project1xWithSettings(t, `airflow:
  variables:
    - variable_name: 1st_run
      variable_value: "yes"
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	require.Contains(t, cs.Notes, SettingsRelPath+`: "1st_run" is not a valid Airflow Variable key `+
		`(letters, digits, _; no leading digit). Rename the variable, and the Dags reading it`)
	_, err = cs.Apply()
	require.NoError(t, err)
	require.Empty(t, writer.stored)
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
}
