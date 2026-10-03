package scaffold

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	manifestpkg "github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// recordingWriter is the vault as a test sees it: what was stored, under what
// kind and name, without a keyring.
type recordingWriter struct {
	stored map[string]string
	kinds  map[string]secrets.Kind
	// held is what the vault already contains before the conversion runs.
	held   map[string]string
	err    error
	hasErr error
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{
		stored: map[string]string{},
		kinds:  map[string]secrets.Kind{},
		held:   map[string]string{},
	}
}

func (w *recordingWriter) SetSecret(kind secrets.Kind, name, value string) error {
	if w.err != nil {
		return w.err
	}
	w.stored[name] = value
	w.kinds[name] = kind
	return nil
}

func (w *recordingWriter) HasSecret(_ secrets.Kind, name string) (bool, error) {
	_, ok := w.held[name]
	return ok, w.hasErr
}

// v1WithSettings is a v1 project carrying an airflow_settings.yaml.
func v1WithSettings(t *testing.T, settings string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("pendulum\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, SettingsRelPath), []byte(settings), 0o600))
	return dir
}

const settingsWithEverything = `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
      conn_host: acct.example.com
      conn_login: dbt
      conn_password: hunter2
      conn_port: 443
  variables:
    - variable_name: batch_size
      variable_value: "50"
  pools:
    - pool_name: heavy
      pool_slot: 4
`

// The whole of it: a connection and a Variable go to the vault, the manifest
// declares both without their values and holds the pool, and the file goes.
func TestConversionSplitsSettingsByWhatEachThingIs(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	writer := newRecordingWriter()

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	// The connection's value is in the vault, as a connection, and never in the
	// project.
	require.Equal(t, secrets.KindConn, writer.kinds["warehouse"])
	var conn map[string]any
	require.NoError(t, json.Unmarshal([]byte(writer.stored["warehouse"]), &conn))
	require.Equal(t, "hunter2", conn["password"])

	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "hunter2", "the credential reached the manifest")

	// The connection is DECLARED, with its kind, and required — no `optional`.
	require.Contains(t, string(manifest), "[tool.astro.env.connections]")
	require.Contains(t, string(manifest), "warehouse = {conn_type = 'snowflake'}")
	require.NotContains(t, string(manifest), "optional")

	// The Variable's value is in the vault too, and the manifest declares it
	// sensitive with no default. v1 projects kept this file out of version
	// control, so a default here would commit a token that never was.
	require.Equal(t, secrets.KindVar, writer.kinds["batch_size"])
	require.Equal(t, "50", writer.stored["batch_size"])
	require.Contains(t, string(manifest), "batch_size = {sensitive = true}")
	require.NotContains(t, string(manifest), "'50'")

	require.Contains(t, string(manifest), "[tool.astro.pools]\nheavy = {slots = 4}")
	require.NoFileExists(t, filepath.Join(dir, SettingsRelPath))
}

// A conn_uri is stored as the JSON the codec produces, not as written. The
// vault holds one shape, and a URI sitting in it is a record only one of the
// two tools can read back.
func TestAConnectionURIIsStoredInTheCanonicalShape(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: pg
      conn_uri: postgres://user:pw@db.example.com:5432/analytics
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	stored := writer.stored["pg"]
	require.True(t, strings.HasPrefix(stored, "{"), "stored as written rather than normalized: %q", stored)
	var conn map[string]any
	require.NoError(t, json.Unmarshal([]byte(stored), &conn))
	require.Equal(t, "postgres", conn["conn_type"])
	require.Equal(t, "db.example.com", conn["host"])
	require.Equal(t, "pw", conn["password"])
	require.Equal(t, "analytics", conn["schema"])
}

// An entry with nothing but an id and a type is a declaration, not a value.
// Storing it would satisfy a required connection with a record that configures
// nothing, which is worse than the project saying it is missing.
func TestAnEmptyConnectionIsDeclaredAndNotStored(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored)
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "warehouse = {conn_type = 'snowflake'}")
	require.True(t, anyContains(cs.Advisories, "no value to carry"),
		"nothing told the user the connection is declared but unset: %v", cs.Advisories)
}

// Values before files. A manifest declaring connections whose values did not
// land is a project that will not start, and nothing on screen would say why.
func TestAFailedVaultWriteWritesNoManifest(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	writer := newRecordingWriter()
	writer.err = os.ErrPermission

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoFileExists(t, filepath.Join(dir, "pyproject.toml"))
}

// A caller with no writer is DECLINING to move credentials, not forgetting to.
//
// The declarations still carry — those are not secrets — and the values stay in
// the file, with a note saying where to move them. `astro init` over an
// existing directory is the caller that needs this: making a directory a
// project is not the reviewed, previewed operation that moving someone's
// credentials into a keychain has to be. Failing would make a v1 directory
// unopenable; carrying silently would move credentials nobody was shown.
func TestNoWriterLeavesTheValuesInTheFileAndSaysSo(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, cs.Secrets, "values were kept for a caller that supplied nowhere to put them")

	res, err := cs.Apply()
	require.NoError(t, err)

	// The declarations carried: a connection is still declared and required, so
	// the project says what it needs.
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "warehouse = {conn_type = 'snowflake'}")
	require.NotContains(t, string(manifest), "hunter2")

	// The file keeps its values, and the note says how to move them.
	settings, err := os.ReadFile(filepath.Join(dir, SettingsRelPath))
	require.NoError(t, err)
	require.Contains(t, string(settings), "hunter2")
	require.True(t, anyContains(res.Notes, "stayed in the file"),
		"nothing said the credentials were not moved: %v", res.Notes)
}

// With no writer the file stays: the values in it went nowhere else, so it is
// their only copy. The note names both commands.
func TestNoWriterKeepsTheFile(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
      conn_password: hunter2
  variables:
    - variable_name: API_TOKEN
      variable_value: tok
`)
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.False(t, anyContains(res.Deleted, SettingsRelPath), "%v", res.Deleted)
	require.Contains(t, res.Notes, SettingsRelPath+": the values of its 1 connection and 1 Airflow variable "+
		"stayed in the file. Convert this project in Astro Desktop, or run "+
		"`astro local env connection set <id>` and `astro local env airflow-variable set <key>`, "+
		"to move them into the encrypted vault")
}

// A project with no values needs no writer, so the common case does not have
// to supply one.
func TestNoWriterIsNeededWhenNothingIsCarried(t *testing.T) {
	dir := v1WithSettings(t, "airflow:\n  pools:\n    - pool_name: p\n      pool_slot: 1\n")
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "pyproject.toml"))
}

// Nothing is carried, and the file stays whole and authoritative — the same
// all-or-nothing rule the env schema follows, for the same reason: a partial
// carry creates [tool.astro.env], and whatever stayed behind has silently
// stopped applying.
func TestAFaultCarriesNothingAndKeepsTheFile(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
      conn_password: hunter2
    - conn_id: ""
      conn_type: postgres
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored, "a blocked file still stored a value")
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "warehouse")
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.True(t, anyContains(cs.Notes, "no conn_id"), "the fault was not named: %v", cs.Notes)
}

// Two files declaring one name is not a merge with a winner. They spell
// different things, so picking one would be this package deciding which of the
// user's two answers it prefers, silently, about a connection.
func TestOneNameInBothFilesCarriesNeither(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
      conn_password: hunter2
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "env.schema.yaml"), []byte(
		"connections:\n  - conn_id: warehouse\n"), 0o600))

	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored)
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "[tool.astro.env")
	require.True(t, anyContains(cs.Notes, "both declare"), "the collision was not named: %v", cs.Notes)
}

// Declarations from both files land together when they do not collide, and the
// preview names both sources rather than whichever one this was built for.
func TestBothFilesCarryWhenTheyDoNotCollide(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
      conn_password: hunter2
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "env.schema.yaml"), []byte(
		"env_vars:\n  - key: LOG_LEVEL\n"), 0o600))

	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "LOG_LEVEL")
	require.Contains(t, string(manifest), "warehouse = {conn_type = 'snowflake'}")
	require.Contains(t, writer.stored, "warehouse")

	require.True(t, anyContains(cs.Updated, SettingsRelPath) || anyContains(cs.Created, SettingsRelPath),
		"the preview did not name airflow_settings.yaml as a source: %v %v", cs.Created, cs.Updated)
}

// A preview must be able to say a credential will be stored without being able
// to say what it is.
func TestAPreviewCarriesNoCredential(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	cs, err := Plan(dir, Options{SecretWriter: newRecordingWriter()})
	require.NoError(t, err)

	encoded, err := json.Marshal(cs)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "hunter2")
	require.Contains(t, string(encoded), "warehouse")
}

func anyContains(hay []string, needle string) bool {
	for _, s := range hay {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// A credential already in the vault wins over the one committed to the v1 file.
//
// What is carried is whatever was committed, which may be months stale or a
// placeholder. What is already there was put there deliberately, through
// `astro local env <noun> set` or the app. Overwriting it destroys the good
// credential and leaves the project running against exactly the value this
// transform exists to get out of version control.
func TestAValueAlreadyInTheVaultIsNotOverwritten(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	writer := newRecordingWriter()
	writer.held["warehouse"] = "the-one-the-user-set"

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.NotContains(t, writer.stored, "warehouse", "the committed credential was written over the user's")
	require.True(t, anyContains(res.Advisories, "kept the value already in your vault"),
		"nothing said the committed value was not carried: %v", res.Advisories)
}

// The file's value for a name the vault already held was not carried, so the
// file is its only copy and stays.
func TestAFileWhoseValueTheVaultHeldIsKept(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := newRecordingWriter()
	writer.held["API_TOKEN"] = "the-one-the-user-set"

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Equal(t,
		SettingsRelPath+" still contains API_TOKEN and warehouse in plaintext, and is kept because the vault "+
			"already held API_TOKEN, so the file's value was not carried over it",
		findAdvisory(t, res.Advisories, "plaintext"))
	require.Equal(t, "1 connection from "+SettingsRelPath+": values stored in this machine's encrypted vault, "+
		"scoped to this project. pyproject.toml declares them without their values",
		findAdvisory(t, res.Advisories, "values stored"))
}

// Plan retired the file because the vault held none of its names. If one
// arrives before Apply, the file would be the only copy of its value, so Apply
// refuses before it stores or deletes anything.
func TestAValueThatReachesTheVaultAfterPlanStopsTheRetirement(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	require.True(t, anyContains(cs.Deleted, SettingsRelPath), "%v", cs.Deleted)

	writer.held["API_TOKEN"] = "set-in-the-app-meanwhile"
	_, err = cs.Apply()
	require.ErrorIs(t, err, ErrChangedOnDisk)
	require.Empty(t, writer.stored)
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
}

// A vault that cannot be asked keeps the file. Plan does not fail over it:
// Apply asks the same question and fails there.
func TestAVaultThatCannotBeAskedKeepsTheFile(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := newRecordingWriter()
	writer.hasErr = errors.New("keyring locked")

	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	require.False(t, anyContains(cs.Deleted, SettingsRelPath), "%v", cs.Deleted)
	_, err = cs.Apply()
	require.ErrorContains(t, err, "keyring locked")
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
}

// The declarations and the values are one carry. Anything that stops the
// declarations reaching the manifest has to stop the values reaching the vault
// — otherwise a project's credentials are written to a shared store while the
// manifest declares none of them, which is the inverse of what applySecrets
// guarantees, and silent.
func TestABlockedEnvSchemaCarriesNoConnectionValuesEither(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "env.schema.yaml"), []byte(
		"env_vars:\n  - key: \"not a legal name\"\n"), 0o600))

	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored, "credentials went to the vault with nothing declaring them")
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "[tool.astro.env")
	require.True(t, anyContains(res.Notes, "has to be fixed first"),
		"nothing said why the settings file was not carried: %v", res.Notes)
}

// v1 read this file through viper, which coerces. yaml.v3 into a typed struct
// does not, and it fails the decode of the WHOLE document — so one quoted port
// used to carry nothing from a file `astro dev start` read without complaint.
func TestShapesV1AcceptedAreStillCarried(t *testing.T) {
	for _, tc := range []struct{ name, body, wantExtra string }{
		{
			name:      "extra as JSON text",
			body:      "airflow:\n  connections:\n    - conn_id: pg\n      conn_type: postgres\n      conn_host: h\n      conn_extra: '{\"sslmode\":\"require\"}'\n",
			wantExtra: "require",
		},
		{
			name:      "port as text",
			body:      "airflow:\n  connections:\n    - conn_id: pg\n      conn_type: postgres\n      conn_host: h\n      conn_port: \"5432\"\n",
			wantExtra: "5432",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := v1WithSettings(t, tc.body)
			writer := newRecordingWriter()
			cs, err := Plan(dir, Options{SecretWriter: writer})
			require.NoError(t, err)
			_, err = cs.Apply()
			require.NoError(t, err)
			require.Contains(t, writer.stored, "pg", "the file carried nothing")
			require.Contains(t, writer.stored["pg"], tc.wantExtra)
		})
	}
}

// A connection resolves through AIRFLOW_CONN_<ID>, which uppercases, so two ids
// differing only in case are one variable at start. Carrying both means the
// project runs against whichever won by composition order.
func TestTwoConnectionsThatDifferOnlyInCaseAreRefused(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: Warehouse
      conn_type: snowflake
      conn_host: a
    - conn_id: warehouse
      conn_type: postgres
      conn_host: b
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored, "both were carried, so one silently wins at start")
	require.True(t, anyContains(res.Notes, "AIRFLOW_CONN_WAREHOUSE"),
		"the collision was not named: %v", res.Notes)
}

// settingsValuesOnly is the shape a real project has: connections and Variables
// holding tokens.
const settingsValuesOnly = `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
      conn_password: hunter2
  variables:
    - variable_name: API_TOKEN
      variable_value: tok-123
`

// With every value in the vault, nothing in the file stays behind, so it is
// retired like any other carried v1 file. No note keeps it and no advisory
// says it still holds plaintext.
func TestASettingsFileWhoseValuesReachTheVaultIsRetired(t *testing.T) {
	dir := v1WithSettings(t, settingsValuesOnly)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Contains(t, res.Deleted, SettingsRelPath+" (migrated into the encrypted vault and pyproject.toml, removed)")
	require.Equal(t, "tok-123", writer.stored["API_TOKEN"])
	require.False(t, anyContains(res.Advisories, "plaintext"), "%v", res.Advisories)
	require.False(t, anyContains(res.Notes, SettingsRelPath), "%v", res.Notes)
	require.Equal(t, "1 connection and 1 Airflow variable from "+SettingsRelPath+
		": values stored in this machine's encrypted vault, scoped to this project. pyproject.toml declares them without their values",
		findAdvisory(t, res.Advisories, "encrypted vault"))
}

// No value from the file reaches the manifest, whatever its kind: a Variable
// is declared sensitive with nothing after it, a connection by its conn_type.
func TestNoSettingsValueReachesTheManifest(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: postgres
      conn_host: db.internal.example.com
      conn_login: loader
      conn_password: hunter2
      conn_port: 5433
      conn_extra: '{"sslmode": "require", "token": "extra-secret"}'
    - conn_id: by_uri
      conn_uri: 'mysql://u:uri-secret@mysql.example.com/db'
  variables:
    - variable_name: CHRONOSPHERE_TOKEN
      variable_value: chrono-secret
    - variable_name: SPLUNK_API_KEY
      variable_value: splunk-secret
`)
	cs, err := Plan(dir, Options{SecretWriter: newRecordingWriter()})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	for _, value := range []string{
		"hunter2", "db.internal.example.com", "loader", "5433", "sslmode", "extra-secret",
		"uri-secret", "mysql.example.com", "chrono-secret", "splunk-secret", "default",
	} {
		require.NotContains(t, string(manifest), value)
	}
	require.Contains(t, string(manifest), "CHRONOSPHERE_TOKEN = {sensitive = true}")
	require.Contains(t, string(manifest), "SPLUNK_API_KEY = {sensitive = true}")
}

// An empty Variable has nothing to store, and v1 skipped it rather than create
// it, so a Dag reading it with a fallback ran. It is declared optional, not
// required: the converted project starts with it unset, and the run says how to
// set it.
func TestAnEmptyVariableIsDeclaredOptionalAndNotStored(t *testing.T) {
	dir := v1WithSettings(t, "airflow:\n  variables:\n    - variable_name: region\n      variable_value: \"\"\n")
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored)
	raw, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "region = {optional = true, sensitive = true}")
	require.Contains(t, res.Advisories,
		"region: declared as an optional Airflow variable, since it had no value to carry. Set it with `astro local env airflow-variable set region`")
	for _, a := range res.Advisories {
		require.NotContains(t, a, "required")
	}

	// The gate `astro local start` runs: with nothing set, nothing is missing.
	m, err := manifestpkg.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	require.True(t, s.AirflowVariables["region"].Optional)
	require.True(t, s.AirflowVariables["region"].Sensitive)
	require.Empty(t, envschema.Validate(s, envschema.Values{}),
		"an empty v1 variable must not stop the converted project starting")
}

// A Variable with a value stays required: whoever converts has it in the vault,
// and a teammate without it is told which one is missing.
func TestAStoredVariableStaysRequired(t *testing.T) {
	dir := v1WithSettings(t, "airflow:\n  variables:\n    - variable_name: region\n      variable_value: us-east-1\n")
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	require.Equal(t, "us-east-1", writer.stored["region"])
	m, err := manifestpkg.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	require.False(t, s.AirflowVariables["region"].Optional)
	require.Len(t, envschema.Validate(s, envschema.Values{}), 1,
		"a carried variable left unset must still be reported missing")
}

// A changeset that has been through JSON is not the one Plan built, and must
// not be applied as though it were.
//
// SecretWrite deliberately serializes Kind, Name and Label but never the value,
// so a round-tripped changeset is structurally valid and carries nothing.
// Storing it would overwrite whatever the user already holds under that key
// with the empty string, and report success. This is the same refusal Apply
// makes for a Change whose Content is nil.
func TestAChangesetThatLostItsValuesIsRefused(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)

	encoded, err := json.Marshal(cs)
	require.NoError(t, err)
	var roundTripped Changeset
	require.NoError(t, json.Unmarshal(encoded, &roundTripped))
	require.NotEmpty(t, roundTripped.Secrets, "the round trip dropped the writes entirely")

	roundTripped.Dir = cs.Dir
	roundTripped.secrets = writer
	_, err = roundTripped.Apply()
	require.ErrorIs(t, err, ErrChangedOnDisk)
	require.Empty(t, writer.stored, "an empty value was written over the vault")
}

// A preview must say that credentials leave the project for a keychain.
//
// It has to reach a list consumers actually render. Updated is derived from
// Changes by report(), a vault write is not a Change, and nothing in the app
// read that field — so the disclosure a conversion is approved on was invisible
// wherever it mattered.
func TestThePreviewSaysCredentialsGoToTheVault(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	cs, err := Plan(dir, Options{SecretWriter: newRecordingWriter()})
	require.NoError(t, err)

	require.True(t, anyContains(cs.Advisories, "encrypted vault"),
		"the preview does not say credentials are stored: %v", cs.Advisories)

	// And it survives Apply, which is what the app renders after converting.
	res, err := cs.Apply()
	require.NoError(t, err)
	require.True(t, anyContains(res.Advisories, "encrypted vault"),
		"the outcome does not say credentials were stored: %v", res.Advisories)
}

// "No longer read from the file" reads like the value moved. It was copied: the
// plaintext is still in a file this run deliberately keeps, and no other list
// mentions it — Removed does not name this file, and Left to do is about what
// failed to carry. So the run has to say the original is still there.
// findAdvisory returns the one advisory containing substr, failing if none or
// several do.
//
// It exists so assertions compare one whole string. Checking substrings against
// the union of the list cannot catch a sentence that is cut short, because each
// substring is free to match a different member and the missing words match
// nothing anyone asked for.
func findAdvisory(t *testing.T, advisories []string, substr string) string {
	t.Helper()
	var found []string
	for _, a := range advisories {
		if strings.Contains(a, substr) {
			found = append(found, a)
		}
	}
	require.Len(t, found, 1, "want exactly one advisory containing %q, got %v", substr, advisories)
	return found[0]
}

func TestThePreviewSaysThePlaintextIsStillInTheFile(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	writer := newRecordingWriter()
	writer.held["batch_size"] = "set-by-hand"
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)

	// One advisory, compared whole: it names the file, names the connection left
	// behind rather than saying "it", and says why the file stays.
	got := findAdvisory(t, cs.Advisories, "plaintext")
	require.Equal(t,
		SettingsRelPath+" still contains batch_size and warehouse in plaintext, and is kept "+
			"because the vault already held batch_size, so the file's value was not carried over it",
		got)

	// It does NOT instruct a deletion. Apply may decline to overwrite a name
	// the vault already holds, which would make the file's copy the only one —
	// so an instruction written here, before that decision, can send someone to
	// destroy it.
	require.NotContains(t, got, "Delete")
	require.NotContains(t, got, "git")

	res, err := cs.Apply()
	require.NoError(t, err)
	require.Equal(t, got, findAdvisory(t, res.Advisories, "plaintext"),
		"the advisory must survive Apply unchanged")

	// The claim has to stay true: the file must actually still be there, since
	// it is the only surviving record of the value the vault kept. Deleted holds
	// decorated labels, not bare paths, so an equality check against the path
	// would pass whatever happened — use the substring helper.
	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.False(t, anyContains(res.Deleted, SettingsRelPath),
		"the file the advisory describes was retired: %v", res.Deleted)
}

// Two carried connections read as prose and stay in a stable order, because the
// ids come from a map walk.
func TestThePlaintextAdvisoryNamesEveryConnection(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: postgres
      conn_password: hunter2
    - conn_id: billing
      conn_type: mysql
      conn_host: billing.example.com
`)
	writer := newRecordingWriter()
	writer.held["warehouse"] = "set-by-hand"
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)

	got := findAdvisory(t, cs.Advisories, "plaintext")
	require.Contains(t, got, "billing and warehouse")
}

// A settings file with no connections has no carried value to describe, so the
// advisory would be noise on every project that only ever declared pools. The
// neighboring case — a caller that supplies no writer at all — is below.
func TestPoolsOnlySettingsGetNoPlaintextAdvisory(t *testing.T) {
	dir := v1WithSettings(t, "airflow:\n  pools:\n    - pool_name: p\n      pool_slot: 1\n")
	cs, err := Plan(dir, Options{SecretWriter: newRecordingWriter()})
	require.NoError(t, err)

	require.False(t, anyContains(cs.Advisories, "plaintext"),
		"described a plaintext value with nothing carried: %v", cs.Advisories)
}

// A caller that supplies no SecretWriter is DECLINING to move credentials, so
// nothing reaches the vault and the earlier branch clears cs.Secrets. This
// advisory must not fire — its subject is a value that was copied, and here
// nothing was.
//
// The file's own copy is then the only one, which is the case where saying
// "delete the entries" would have been actively destructive. The existing Note
// covers it (TestNoWriterLeavesTheValuesInTheFileAndSaysSo); what is pinned
// here is that this advisory stays out of it.
func TestNoWriterMeansNoPlaintextAdvisory(t *testing.T) {
	dir := v1WithSettings(t, settingsWithEverything)
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	require.Empty(t, cs.Secrets, "nothing may be queued for a caller with nowhere to put it")

	require.False(t, anyContains(cs.Advisories, "plaintext"),
		"claimed a value was copied when none was: %v", cs.Advisories)

	res, err := cs.Apply()
	require.NoError(t, err)
	require.False(t, anyContains(res.Advisories, "plaintext"), "%v", res.Advisories)
}

// Airflow chooses a connection's provider from its conn_type, so a record
// without one is not a connection it can resolve. Carrying it stores something
// unusable and declares it required, which stops the project starting over a
// value that would not have worked.
//
// It is also the one place the two codecs disagree: this package's encoder
// accepts an empty conn_type and the app's decoder does not, so one that gets
// through converts from the CLI and fails in the app after the preview was
// approved. Refusing it where the connection is authored settles it for both.
func TestAConnectionWithNoTypeIsRefused(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_host: acct.example.com
      conn_login: dbt
      conn_password: hunter2
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Empty(t, writer.stored, "stored a connection Airflow cannot resolve")
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "warehouse")
	require.True(t, anyContains(res.Notes, "no conn_type"), "the fault was not named: %v", res.Notes)
}

// A connection written as a URI declares the kind the URI says it is. The type
// lives in the scheme there, so reading only the conn_type field would declare
// an empty one beside a value that is a perfectly good postgres connection.
func TestAURIConnectionIsDeclaredAsItsScheme(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: pg
      conn_uri: postgres://user:pw@db.example.com:5432/analytics
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "pg = {conn_type = 'postgres'}")
	require.Contains(t, writer.stored, "pg")
}

// v1StockSettings is the airflow_settings.yaml v1's `astro dev init` wrote,
// byte for byte. The v1 project template kept it out of git, so most v1
// checkouts still hold it untouched.
const v1StockSettings = `# This file allows you to configure Airflow Connections, Pools, and Variables in a single place for local development only.
# NOTE: json dicts can be added to the conn_extra field as yaml key value pairs. See the example below.

# For more information, refer to our docs: https://www.astronomer.io/docs/astro/cli/develop-project#configure-airflow_settingsyaml-local-development-only
# For questions, reach out to: https://support.astronomer.io
# For issues create an issue ticket here: https://github.com/astronomer/astro-cli/issues

airflow:
  connections:
    - conn_id:
      conn_type:
      conn_host:
      conn_schema:
      conn_login:
      conn_password:
      conn_port:
      conn_extra:
        example_extra_field: example-value
  pools:
    - pool_name:
      pool_slot:
      pool_description:
  variables:
    - variable_name:
      variable_value:
`

// The untouched template carries nothing, so it is removed with no note, and
// the report says there was nothing in it rather than that it was migrated.
func TestTheStockV1SettingsFileIsRetired(t *testing.T) {
	dir := v1WithSettings(t, v1StockSettings)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Contains(t, res.Deleted, SettingsRelPath+" (nothing to carry, removed)")
	require.False(t, anyContains(res.Notes, SettingsRelPath), "%v", res.Notes)
	require.Empty(t, res.Advisories)
	require.Empty(t, writer.stored)
	manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "tool.astro.env")
}

// A real entry beside the template's blank ones carries as it would alone, and
// the blank ones are skipped rather than blocking it.
func TestTheStockTemplatesBlankEntriesDoNotBlockARealOne(t *testing.T) {
	dir := v1WithSettings(t, v1StockSettings+`    - variable_name: API_TOKEN
      variable_value: tok-123
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.Equal(t, "tok-123", writer.stored["API_TOKEN"])
	require.Len(t, writer.stored, 1)
	require.NoFileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Contains(t, res.Deleted, SettingsRelPath+" (migrated into the encrypted vault and pyproject.toml, removed)")
	require.False(t, anyContains(res.Notes, SettingsRelPath), "%v", res.Notes)
}

// An entry that has values but no id is a mistake, not a placeholder: the
// file is kept and the note says which entry to fix, without its secrets.
func TestAnEntryWithValuesButNoIDIsReported(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id: warehouse
      conn_type: snowflake
    - conn_id:
      conn_type: postgres
      conn_host: db.example.com
      conn_password: hunter2
  variables:
    - variable_name:
      variable_value: tok-123
`)
	writer := newRecordingWriter()
	cs, err := Plan(dir, Options{SecretWriter: writer})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Empty(t, writer.stored)
	require.Contains(t, res.Notes,
		SettingsRelPath+": connection 2 (conn_type postgres, conn_host db.example.com) has no conn_id. Add one, or delete the entry")
	require.Contains(t, res.Notes, SettingsRelPath+": variable 1 has a variable_value but no variable_name. Add one, or delete the entry")
	require.False(t, anyContains(res.Notes, "hunter2"), "%v", res.Notes)
	require.False(t, anyContains(res.Notes, "tok-123"), "%v", res.Notes)
}

// Only the template's own placeholders count as blank. An id-less entry with a
// real extra, or a nameless pool with slots, holds something the user wrote,
// so the file is kept and the entry named rather than removed with it.
func TestAnEntryWithNoIDIsBlankOnlyWithTheTemplatesPlaceholders(t *testing.T) {
	dir := v1WithSettings(t, `airflow:
  connections:
    - conn_id:
      conn_extra:
        key_path: /keys/sa.json
  pools:
    - pool_name:
      pool_slot: 4
`)
	cs, err := Plan(dir, Options{SecretWriter: newRecordingWriter()})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, SettingsRelPath))
	require.Contains(t, res.Notes, SettingsRelPath+": connection 1 has no conn_id. Add one, or delete the entry")
	require.Contains(t, res.Notes, SettingsRelPath+": pool 1 has no pool_name. Add one, or delete the entry")
}

// The stock template does not share a blocked env schema's fate: it declares
// nothing, so no note says it was not carried.
func TestTheStockTemplateIsNotHeldByABlockedEnvSchema(t *testing.T) {
	dir := v1WithSettings(t, v1StockSettings)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "env.schema.yaml"), []byte("connections: [\n"), 0o600))

	cs, err := Plan(dir, Options{SecretWriter: newRecordingWriter()})
	require.NoError(t, err)
	require.False(t, anyContains(cs.Notes, SettingsRelPath), "%v", cs.Notes)
}
