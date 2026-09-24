package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

func writeV1EnvSchema(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "env.schema.yaml"), []byte(body), 0o600))
}

func v1ProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM quay.io/astronomer/astro-runtime:9\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))
	return dir
}

// The declarations reach [tool.astro.env] and the file goes, so the project has
// one source for its environment rather than two.
func TestRunCarriesTheEnvSchemaAndRetiresTheFile(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, `env_vars:
  - { key: API_URL, type: url, required: true }
  - { key: SAYS_NOTHING }
airflow_variables:
  - { key: region, type: string, required: true }
connections:
  - { conn_id: warehouse, conn_type: snowflake, required: true }
`)

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)

	assert.Equal(t, envschema.TypeURL, s.EnvVars["API_URL"].Type)
	assert.False(t, s.EnvVars["API_URL"].Optional, "a required declaration still gates")
	assert.Equal(t, envschema.TypeString, s.AirflowVariables["region"].Type)
	assert.Equal(t, "snowflake", s.Connections["warehouse"].ConnType)
	assert.True(t, s.Connections["warehouse"].Sensitive)

	assert.NoFileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"),
		"the file is retired once everything it said is in the manifest")
}

// The row nearly every declaration is on. `required` and `optional` are
// opposite flags with disagreeing zero values, so a declaration spelling
// neither must come out NOT gated — carrying the field across by name makes a
// converted project refuse to start until every documented name is set.
func TestRunInvertsTheGateFlagOnConversion(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, "env_vars:\n  - { key: SAYS_NOTHING }\n  - { key: GATED, required: true }\n")

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)

	assert.True(t, s.EnvVars["SAYS_NOTHING"].Optional,
		"a declaration that spelled no flag was not gated by the v1 file")
	assert.False(t, s.EnvVars["GATED"].Optional)

	// The end of it: nothing new is missing after a conversion that carried
	// nothing but shape.
	assert.Len(t, envschema.Validate(s, envschema.Values{
		EnvVars: map[string]string{"GATED": "x"},
	}), 0, "conversion added a gate the project did not have")
}

// A default was documentation in the v1 file and is a live value in the
// manifest. It is carried, because refusing would leave a file that can never
// retire, but the preview says so: the project's environment changes and
// nothing else on screen shows it.
func TestRunNotesThatACarriedDefaultBecomesLive(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, "env_vars:\n  - { key: BATCH_SIZE, type: int, default: \"100\" }\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	joined := strings.Join(res.Advisories, "\n")
	assert.Contains(t, joined, "BATCH_SIZE")
	assert.Contains(t, joined, "composed into the environment")
	assert.NotContains(t, strings.Join(res.Notes, "\n"), "BATCH_SIZE",
		"an advisory is a change already made, not work outstanding")

	// Noting it does not keep the file: the declaration WAS carried.
	assert.NoFileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"))

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	assert.Equal(t, "100", s.EnvVars["BATCH_SIZE"].Default)
}

// One fault stops the whole carry, and the reason is not tidiness.
//
// Carrying the rest would create [tool.astro.env], and a manifest with that
// section is the declaration source from then on — so the file kept to preserve
// TOKEN would be a file nothing ever reads again, and TOKEN would have silently
// stopped being required. Keeping a file is only protection while it still
// speaks.
func TestRunCarriesNothingWhenOneDeclarationCannotBeCarried(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, `env_vars:
  - { key: FINE, type: string }
  - { key: TOKEN, sensitive: true, default: "sk-live" }
`)

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	joined := strings.Join(res.Notes, "\n")
	assert.Contains(t, joined, "TOKEN")
	assert.Contains(t, joined, envschema.LegacyRelPath)
	assert.FileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"))

	// Nothing was written, so the file is still what the project declares by.
	body, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "[tool.astro.env",
		"a partial carry would make the kept file unreadable by every consumer")
}

// Every fault, not the first. This feeds a preview of a destructive operation,
// so a user who fixes what it names should not be told about a second fault on
// the next run.
func TestRunReportsEveryFaultAtOnce(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, `env_vars:
  - { key: MY-VAR, type: string }
  - { key: TOKEN, sensitive: true, default: "sk-live" }
connections:
  - { conn_id: my-warehouse, conn_type: snowflake }
`)

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	joined := strings.Join(res.Notes, "\n")
	for _, want := range []string{"MY-VAR", "TOKEN", "my-warehouse"} {
		assert.Contains(t, joined, want, "the preview named only some of the faults")
	}
}

// A name the manifest grammar refuses is refused BEFORE anything is written.
//
// The parser rejects an illegal name by failing the whole [tool.astro.env]
// section, so carrying one and deleting the file leaves a project whose every
// declaration is unreadable and whose only other copy is gone. ValueSpec.Check
// judges annotations and never sees the name, so it cannot catch this.
func TestRunRefusesNamesTheManifestGrammarRejects(t *testing.T) {
	for _, body := range []string{
		"env_vars:\n  - { key: MY-VAR, type: string }\n",
		"env_vars:\n  - { key: 1ST, type: string }\n",
		"airflow_variables:\n  - { key: batch.size }\n",
		"connections:\n  - { conn_id: my-warehouse, conn_type: snowflake }\n",
		// Directly under [tool.astro.env] these two name the sub-sections, so
		// an env var called either would be read back as a section.
		"env_vars:\n  - { key: connections }\n",
		"env_vars:\n  - { key: airflow_variables }\n",
	} {
		dir := v1ProjectDir(t)
		writeV1EnvSchema(t, dir, body)

		_, err := Run(dir, Options{})
		require.NoError(t, err, body)

		assert.FileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"), body)
		m, lerr := manifest.Load(filepath.Join(dir, "pyproject.toml"))
		require.NoError(t, lerr, body)
		_, perr := envschema.ParseSchema(m.Astro.Env)
		assert.NoError(t, perr, "the manifest this run wrote does not read back: %s", body)
	}
}

// The v1 format is a list and the manifest is a table, so a repeated key loses
// one of the two. Silently, and then the file holding the other is deleted.
func TestRunRefusesAFileThatWouldLoseADeclaration(t *testing.T) {
	for _, body := range []string{
		"env_vars:\n  - { key: API_URL, type: url, required: true }\n  - { key: API_URL }\n",
		"env_vars:\n  - { type: string }\n",
		"connections:\n  - { conn_id: warehouse }\n  - { conn_id: warehouse, conn_type: snowflake }\n",
	} {
		dir := v1ProjectDir(t)
		writeV1EnvSchema(t, dir, body)

		res, err := Run(dir, Options{})
		require.NoError(t, err, body)

		assert.Contains(t, strings.Join(res.Notes, "\n"), envschema.LegacyRelPath, body)
		assert.FileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"), body)
	}
}

// The conversion writes through envschema.DeclarationTable, and the round trip
// is asserted here too rather than assumed: everything it writes, ParseSchema
// reads back as the same declaration. Without this, a change on either side
// breaks the carry and the failure surfaces after the source file is deleted.
func TestCarriedDeclarationsRoundTripThroughTheParser(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, `env_vars:
  - { key: PLAIN }
  - { key: TYPED, type: port, required: true }
  - { key: WITH_DEFAULT, type: int, default: "100" }
  - { key: SECRET, type: string, sensitive: true, required: true }
  - { key: DESCRIBED, type: string, description: "what it is for" }
  - { key: PICKED, type: enum, enum: [a, b], required: true }
airflow_variables:
  - { key: region, type: string, required: true }
connections:
  - { conn_id: warehouse, conn_type: snowflake, required: true, description: "the warehouse" }
`)
	want, err := envschema.ParseLegacy([]byte(mustRead(t, filepath.Join(dir, ".astro", "env.schema.yaml"))))
	require.NoError(t, err)

	_, err = Run(dir, Options{})
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	got, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)

	assert.Equal(t, want.EnvVars, got.EnvVars)
	assert.Equal(t, want.AirflowVariables, got.AirflowVariables)
	assert.Equal(t, want.Connections, got.Connections)
}

// The converter must never write `sensitive` under connections, because the
// grammar refuses it there whichever value it carries — so emitting it would
// make every converted project's own manifest unloadable.
//
// DeclarationTable omits it; this pins that at the conversion, because the
// cost of the omission regressing is a conversion that produces a file the next
// command rejects, rather than one noisy key.
func TestConversionNeverWritesSensitiveUnderConnections(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, `connections:
  - { conn_id: warehouse, conn_type: snowflake, required: true }
  - { conn_id: lake, conn_type: s3 }
`)
	_, err := Run(dir, Options{})
	require.NoError(t, err)

	path := filepath.Join(dir, "pyproject.toml")
	raw := mustRead(t, path)
	_, conns, found := strings.Cut(raw, "[tool.astro.env.connections")
	require.True(t, found, "the conversion wrote no connections section:\n%s", raw)
	assert.NotContains(t, conns, "sensitive",
		"a connection is sensitive by section, and saying so is refused on the way back in")

	// The real assertion: what conversion wrote, the parser accepts.
	m, err := manifest.Load(path)
	require.NoError(t, err)
	_, err = envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err, "a converted manifest must load")
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// The adopt arm: a repo that already has a pyproject.toml for its own
// packaging. It is a different manifest renderer from the greenfield one every
// other test here takes, and it is the arm where setEnvDeclarations runs before
// anything else has created [tool.astro].
func TestAdoptArmCarriesTheEnvSchema(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(
		"[project]\nname = 'demo'\nversion = '0.1.0'\nrequires-python = '>=3.10'\ndependencies = []\n\n"+
			"[tool.other]\nx = 1\n"), 0o600))
	writeV1EnvSchema(t, dir, "env_vars:\n  - { key: API_URL, type: url, required: true }\n"+
		"connections:\n  - { conn_id: warehouse, conn_type: snowflake }\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	assert.Equal(t, envschema.TypeURL, s.EnvVars["API_URL"].Type)
	assert.Equal(t, "snowflake", s.Connections["warehouse"].ConnType)
	// The airflow pin is written after the env section; both have to be there.
	assert.NotEmpty(t, m.Astro.AirflowVersion)
	assert.NoFileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"))
	assert.Contains(t, strings.Join(res.Updated, "\n"), "[tool.astro.env]")
}

// Both arms say the schema moved. The greenfield one is what a real v1 project
// takes, and it said nothing — a change performed but unreported cannot be
// reviewed, which is the rule Plan's own comment states.
func TestBothArmsReportTheMigration(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, "env_vars:\n  - { key: API_URL, type: url }\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Created, "\n")+strings.Join(res.Updated, "\n"),
		"[tool.astro.env]")
}

// An advisory names a DECLARATION, and planRetirements spares any file whose
// name appears in a note. So a declaration named after a file must not keep
// that file alive.
func TestAnAdvisoryDoesNotSpareAFileItHappensToName(t *testing.T) {
	// A runtime tag that names the Airflow minor, so the Dockerfile draws no
	// note of its own and would retire. "Dockerfile" is the only v1 filename
	// that is also a legal env-var name, so it is the only collision possible.
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-1\n"), 0o600))
	writeV1EnvSchema(t, dir, "env_vars:\n  - { key: Dockerfile, type: string, default: \"x\" }\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	assert.Contains(t, strings.Join(res.Advisories, "\n"), "Dockerfile",
		"the advisory is still shown")
	assert.NoFileExists(t, filepath.Join(dir, "Dockerfile"),
		"an env var named after a file kept that file from retiring")
}

// A file that will not parse blocks nothing: the conversion runs, says it
// carried nothing, and keeps the file.
func TestRunSurvivesAnUnreadableEnvSchema(t *testing.T) {
	dir := v1ProjectDir(t)
	writeV1EnvSchema(t, dir, "env_vars: [not a list of maps\n")

	res, err := Run(dir, Options{})
	require.NoError(t, err, "a broken v1 file must not fail the conversion")

	assert.Contains(t, strings.Join(res.Notes, "\n"), envschema.LegacyRelPath)
	assert.FileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"))
}

// A project with no v1 env schema gets no [tool.astro.env] at all, rather than
// an empty table — an empty table is a claim, and it would make the app treat
// the manifest as the source before anything declares there.
func TestRunWritesNoEnvSectionWithoutAV1File(t *testing.T) {
	dir := v1ProjectDir(t)

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(body), "[tool.astro.env")
}

// A manifest that already declares an environment is already an Astro project,
// so conversion refuses it rather than merging into it. Asserted because the
// carry looks like it needs a "do not overwrite" rule and does not: the state
// that rule would protect cannot be reached.
func TestRunRefusesAManifestThatAlreadyDeclares(t *testing.T) {
	dir := v1ProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(
		"[project]\nname = 'demo'\nversion = '0.1.0'\nrequires-python = '>=3.10'\ndependencies = []\n\n"+
			"[tool.astro.env]\nAPI_URL = { type = 'string' }\n"), 0o600))
	writeV1EnvSchema(t, dir, "env_vars:\n  - { key: NEW_ONE }\n")

	_, err := Run(dir, Options{})
	require.ErrorIs(t, err, ErrAlreadyAstroProject)

	// And nothing was touched on the way to refusing.
	assert.FileExists(t, filepath.Join(dir, ".astro", "env.schema.yaml"))
}
