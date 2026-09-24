package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// declared reads the project's declarations back the way start does, failing
// the test when they do not load.
func declared(t *testing.T, dir string) *envschema.Schema {
	t.Helper()
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	schema, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	return schema
}

func readManifest(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	return string(b)
}

// declareJSON runs one declare/undeclare with --output json and decodes the
// status it prints.
func declareJSON(t *testing.T, dir string, args ...string) envDeclarationResult {
	t.Helper()
	d, out, _ := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, append(append([]string{"local", "env"}, args...), "--output", "json")...))
	var res envDeclarationResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &res), "decode %q", out.String())
	return res
}

// A new name is declared in its own kind's section, with the annotations its
// flags give, for every noun.
func TestDeclareANewName(t *testing.T) {
	for _, tc := range []struct {
		noun  string
		args  []string
		kind  localenv.Kind
		decls func(*envschema.Schema) map[string]envschema.ValueSpec
		want  envschema.ValueSpec
	}{
		{
			noun:  "variable",
			args:  []string{"API_URL", "--type", "url", "--optional", "--description", "The API"},
			kind:  localenv.KindEnv,
			decls: func(s *envschema.Schema) map[string]envschema.ValueSpec { return s.EnvVars },
			want:  envschema.ValueSpec{Type: envschema.TypeURL, Optional: true, Description: "The API"},
		},
		{
			noun:  "connection",
			args:  []string{"db_main", "--type", "postgres", "--description", "Warehouse"},
			kind:  localenv.KindConn,
			decls: func(s *envschema.Schema) map[string]envschema.ValueSpec { return s.Connections },
			want:  envschema.ValueSpec{ConnType: "postgres", Description: "Warehouse", Sensitive: true},
		},
		{
			noun:  "airflow-variable",
			args:  []string{"batch_size", "--type", "int", "--default", "500"},
			kind:  localenv.KindVar,
			decls: func(s *envschema.Schema) map[string]envschema.ValueSpec { return s.AirflowVariables },
			want:  envschema.ValueSpec{Type: envschema.TypeInt, Default: "500", HasDefault: true},
		},
	} {
		t.Run(tc.noun, func(t *testing.T) {
			dir := envProject(t, "")
			res := declareJSON(t, dir, append([]string{tc.noun, "declare"}, tc.args...)...)
			assert.Equal(t, envDeclarationResult{
				Kind: tc.kind, Name: tc.args[0], Status: declStatusDeclared,
				Manifest: filepath.Join(dir, "pyproject.toml"),
			}, res)
			got, ok := tc.decls(declared(t, dir))[tc.args[0]]
			require.True(t, ok, "%s %s was not declared", tc.noun, tc.args[0])
			assert.Equal(t, tc.want, got)
		})
	}
}

// A bare declare writes an empty table, which is a required name with no
// annotations, and a second one changes nothing and says so.
func TestDeclareBareIsIdempotent(t *testing.T) {
	dir := envProject(t, "")
	assert.Equal(t, declStatusDeclared, declareJSON(t, dir, "variable", "declare", "TOKEN").Status)
	before := readManifest(t, dir)
	assert.Equal(t, declStatusUnchanged, declareJSON(t, dir, "variable", "declare", "TOKEN").Status)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Equal(t, envschema.ValueSpec{}, declared(t, dir).EnvVars["TOKEN"])
}

// Only the annotations passed change. A hand-written declaration keeps the rest
// of its keys, and the comments around it.
func TestDeclareKeepsWhatItWasNotGiven(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\n# the API's token\n"+
		"API_TOKEN = { type = 'string', optional = true, description = 'Token', source = 'workspace' }  # from EM\n")

	res := declareJSON(t, dir, "variable", "declare", "API_TOKEN", "--sensitive")
	assert.Equal(t, declStatusDeclared, res.Status)

	assert.Equal(t, envschema.ValueSpec{
		Type: envschema.TypeString, Optional: true, Description: "Token",
		Source: envschema.SourceWorkspace, Sensitive: true, HasSensitive: true,
	}, declared(t, dir).EnvVars["API_TOKEN"])
	body := readManifest(t, dir)
	assert.Contains(t, body, "# the API's token")
	assert.Contains(t, body, "# from EM")
}

// --optional=false and an empty --description take back what an earlier
// declare gave, rather than being read as "not passed".
func TestDeclareClearsAnAnnotationWhenAskedTo(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.airflow_variables]\nregion = { optional = true, description = 'Where', type = 'string' }\n")
	declareJSON(t, dir, "airflow-variable", "declare", "region", "--optional=false", "--description", "")
	assert.Equal(t, envschema.ValueSpec{Type: envschema.TypeString}, declared(t, dir).AirflowVariables["region"])
}

// A connection's --type is its connection type, as it is for `connection set`,
// and never the value type a connection may not declare.
func TestDeclareConnectionTypeIsTheConnType(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.connections]\ndb = { description = 'kept' }\n")
	declareJSON(t, dir, "connection", "declare", "db", "--type", "snowflake")
	assert.Equal(t, envschema.ValueSpec{ConnType: "snowflake", Description: "kept", Sensitive: true},
		declared(t, dir).Connections["db"])
}

// --enum implies type enum, and moving an enum to another type drops the list,
// which only type enum may carry.
func TestDeclareEnumAndType(t *testing.T) {
	dir := envProject(t, "")
	declareJSON(t, dir, "variable", "declare", "LEVEL", "--enum", "debug,info")
	assert.Equal(t, envschema.ValueSpec{Type: envschema.TypeEnum, Enum: []string{"debug", "info"}},
		declared(t, dir).EnvVars["LEVEL"])

	declareJSON(t, dir, "variable", "declare", "LEVEL", "--type", "string")
	assert.Equal(t, envschema.ValueSpec{Type: envschema.TypeString}, declared(t, dir).EnvVars["LEVEL"])
}

// --source workspace on a name with no other flag goes through
// DeclareEnvFromWorkspace: an existing declaration keeps every annotation, and
// a new one is declared with the source alone.
func TestDeclareFromTheWorkspace(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.connections]\ndb = { conn_type = 'postgres', optional = true }\n")

	declareJSON(t, dir, "connection", "declare", "db", "--source", "workspace")
	declareJSON(t, dir, "airflow-variable", "declare", "zone", "--source", "workspace")

	s := declared(t, dir)
	assert.Equal(t, envschema.ValueSpec{
		ConnType: "postgres", Optional: true, Source: envschema.SourceWorkspace, Sensitive: true,
	}, s.Connections["db"])
	assert.Equal(t, envschema.ValueSpec{Source: envschema.SourceWorkspace}, s.AirflowVariables["zone"])

	// And --source local takes it back.
	declareJSON(t, dir, "airflow-variable", "declare", "zone", "--source", "local")
	assert.Equal(t, envschema.ValueSpec{}, declared(t, dir).AirflowVariables["zone"])
}

// The project here links no workspace, so the name resolves from nothing
// remote yet. That is a note, not a refusal: a local value still satisfies it.
func TestDeclareFromTheWorkspaceNotesAMissingWorkspace(t *testing.T) {
	dir := envProject(t, "")
	d, _, stderr := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, "local", "env", "variable", "declare", "TOKEN", "--source", "workspace"))
	assert.Contains(t, stderr.String(), "sets no workspace")
}

// A name with a default is not given a workspace source, since it would never
// fall back to that default. The refusal names the flag that removes it, and
// the file is not touched. Both spellings of a default count.
func TestDeclareFromTheWorkspaceRefusesADefault(t *testing.T) {
	for name, body := range map[string]string{
		"string shorthand": "LEVEL = 'info'\n",
		"table default":    "LEVEL = { default = 'info', description = 'd' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := envProject(t, "\n[tool.astro.env]\n"+body)
			before := readManifest(t, dir)

			d, _, _ := envDeps(t, dir, "")
			err := execute(t, d, "local", "env", "variable", "declare", "LEVEL", "--source", "workspace")
			require.Error(t, err)
			assert.True(t, errors.Is(err, scaffold.ErrWorkspaceSourceWithDefault), "err = %v", err)
			assert.Contains(t, err.Error(), "--no-default")
			assert.Equal(t, before, readManifest(t, dir), "a refused declare wrote the manifest")

			// The way through the refusal is the one it names.
			declareJSON(t, dir, "variable", "declare", "LEVEL", "--source", "workspace", "--no-default")
			spec := declared(t, dir).EnvVars["LEVEL"]
			assert.False(t, spec.HasDefault)
			assert.Equal(t, envschema.SourceWorkspace, spec.Source)
		})
	}
}

// An env-form key is declared, and reported, by the name Airflow reads it by.
func TestDeclareEnvFormNames(t *testing.T) {
	dir := envProject(t, "")

	res := declareJSON(t, dir, "airflow-variable", "declare", "AIRFLOW_VAR_REGION", "--description", "Where")
	assert.Equal(t, "region", res.Name)
	res = declareJSON(t, dir, "connection", "declare", "AIRFLOW_CONN_DB_MAIN")
	assert.Equal(t, "db_main", res.Name)

	s := declared(t, dir)
	assert.Equal(t, "Where", s.AirflowVariables["region"].Description)
	assert.Contains(t, s.Connections, "db_main")
	assert.NotContains(t, s.AirflowVariables, "airflow_var_region")

	res = declareJSON(t, dir, "airflow-variable", "undeclare", "AIRFLOW_VAR_REGION")
	assert.Equal(t, envDeclarationResult{
		Kind: localenv.KindVar, Name: "region", Status: declStatusUndeclared,
		Manifest: filepath.Join(dir, "pyproject.toml"),
	}, res)
	assert.NotContains(t, declared(t, dir).AirflowVariables, "region")
}

// A variable a person did declare in env form is the one that form addresses,
// and is reported under the name it is declared by.
func TestDeclareAddressesALiteralEnvFormDeclaration(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.airflow_variables]\nAIRFLOW_VAR_ZONE = {}\n")
	res := declareJSON(t, dir, "airflow-variable", "declare", "AIRFLOW_VAR_ZONE", "--optional")
	assert.Equal(t, "AIRFLOW_VAR_ZONE", res.Name)
	s := declared(t, dir)
	assert.True(t, s.AirflowVariables["AIRFLOW_VAR_ZONE"].Optional)
	assert.NotContains(t, s.AirflowVariables, "zone")
}

// undeclare removes the declaration and nothing else, and an undeclare of a
// name that is not declared reports unchanged rather than failing.
func TestUndeclare(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\n# keep me\nLOG_LEVEL = 'info'\nAPI_TOKEN = { sensitive = true }\n")

	res := declareJSON(t, dir, "variable", "undeclare", "API_TOKEN")
	assert.Equal(t, declStatusUndeclared, res.Status)
	s := declared(t, dir)
	assert.NotContains(t, s.EnvVars, "API_TOKEN")
	assert.Contains(t, s.EnvVars, "LOG_LEVEL")
	assert.Contains(t, readManifest(t, dir), "# keep me")

	before := readManifest(t, dir)
	assert.Equal(t, declStatusUnchanged, declareJSON(t, dir, "variable", "undeclare", "API_TOKEN").Status)
	assert.Equal(t, before, readManifest(t, dir))
}

// undeclare can remove the declaration that stops a section loading, which is
// how that section gets fixed; declare on the same file is refused.
func TestUndeclareFixesABrokenDeclaration(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nBAD = { sensitive = 'yes' }\nGOOD = {}\n")

	d, _, _ := envDeps(t, dir, "")
	require.Error(t, execute(t, d, "local", "env", "variable", "declare", "BAD", "--optional"))

	assert.Equal(t, declStatusUndeclared, declareJSON(t, dir, "variable", "undeclare", "BAD").Status)
	assert.Contains(t, declared(t, dir).EnvVars, "GOOD")
}

// A manifest that does not parse is refused as it stands, and left alone.
func TestDeclareRefusesAnUnparseableManifest(t *testing.T) {
	for name, body := range map[string]string{
		"toml error":   "\n[tool.astro.env\nX = {}\n",
		"schema error": "\n[tool.astro.env]\nX = { sensitive = 'yes' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := envProject(t, body)
			before := readManifest(t, dir)
			d, out, _ := envDeps(t, dir, "")
			require.Error(t, execute(t, d, "local", "env", "variable", "declare", "X", "--optional"))
			assert.Equal(t, before, readManifest(t, dir))
			assert.Empty(t, out.String(), "a refused declare printed a result")
		})
	}
}

// Outside a project there is no manifest to edit, and --global has none either.
func TestDeclareNeedsAProject(t *testing.T) {
	outside := t.TempDir()
	envProject(t, "") // isolates HOME and ASTRO_HOME
	for _, verb := range []string{"declare", "undeclare"} {
		d, _, _ := envDeps(t, outside, "")
		err := execute(t, d, "local", "env", "variable", verb, "X")
		require.Error(t, err, verb)
		_, statErr := os.Stat(filepath.Join(outside, "pyproject.toml"))
		assert.True(t, os.IsNotExist(statErr), "%s created a manifest outside a project", verb)
	}

	dir := envProject(t, "")
	before := readManifest(t, dir)
	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "declare", "X", "--global")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--global")
	assert.Equal(t, before, readManifest(t, dir))
}

// A refusal over a sensitive default never echoes the default, which is the
// credential the rule exists to keep out of the file.
func TestDeclareDoesNotEchoASensitiveDefault(t *testing.T) {
	const secret = "hunter2-do-not-print"
	for name, tc := range map[string]struct {
		body string
		args []string
	}{
		"given with --sensitive": {"", []string{"TOKEN", "--sensitive", "--default", secret}},
		"already in the file":    {"\n[tool.astro.env]\nTOKEN = '" + secret + "'\n", []string{"TOKEN", "--sensitive"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := envProject(t, tc.body)
			before := readManifest(t, dir)
			d, out, stderr := envDeps(t, dir, "")
			err := execute(t, d, append([]string{"local", "env", "variable", "declare"}, tc.args...)...)
			require.Error(t, err)
			for what, s := range map[string]string{"error": err.Error(), "stdout": out.String(), "stderr": stderr.String()} {
				if strings.Contains(s, secret) {
					t.Errorf("the %s echoes the default (%d bytes)", what, len(s))
				}
			}
			assert.Equal(t, before, readManifest(t, dir))
		})
	}
}

// Declaring a name sensitive while its value sits in the plain .env says so,
// since the .env copy still wins at start until set moves it.
func TestDeclareSensitiveNotesAPlaintextCopy(t *testing.T) {
	dir := envProject(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("API_TOKEN=x\n"), 0o600))
	d, _, stderr := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, "local", "env", "variable", "declare", "API_TOKEN", "--sensitive"))
	assert.Contains(t, stderr.String(), "astro local env variable set API_TOKEN")
}

// A --source other than the two the manifest can say is refused before the
// file is read.
func TestDeclareRefusesAnUnknownSource(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "declare", "X", "--source", "cloud")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace or local")
	assert.NotContains(t, declared(t, dir).EnvVars, "X")
}

// A variable declared under another case is the one the writer edits, so it is
// the name reported, in text and JSON, for declare and for undeclare.
func TestDeclareReportsTheCaseTheManifestUses(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.airflow_variables]\nREGION = {}\n\n[tool.astro.env.connections]\nDB_Main = {}\n")

	res := declareJSON(t, dir, "airflow-variable", "declare", "region", "--optional")
	assert.Equal(t, "REGION", res.Name)
	assert.True(t, declared(t, dir).AirflowVariables["REGION"].Optional)

	d, out, _ := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, "local", "env", "connection", "declare", "db_main", "--description", "d"))
	assert.Contains(t, out.String(), "connection DB_Main ")

	res = declareJSON(t, dir, "airflow-variable", "undeclare", "region")
	assert.Equal(t, envDeclarationResult{
		Kind: localenv.KindVar, Name: "REGION", Status: declStatusUndeclared,
		Manifest: filepath.Join(dir, "pyproject.toml"),
	}, res)
	assert.NotContains(t, declared(t, dir).AirflowVariables, "REGION")
}

// --default with --source workspace is refused by the writer. The hint then
// cannot be --no-default, which the command line already rules out beside
// --default; it says the two flags do not combine instead.
func TestDeclareFromTheWorkspaceWithADefaultFlag(t *testing.T) {
	dir := envProject(t, "")
	before := readManifest(t, dir)
	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "declare", "API_URL", "--source", "workspace", "--default", "https://x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, scaffold.ErrWorkspaceSourceWithDefault), "err = %v", err)
	assert.NotContains(t, err.Error(), "--no-default")
	assert.Contains(t, err.Error(), "cannot be combined")
	assert.Equal(t, before, readManifest(t, dir))
}
