package local

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/envschema"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// writeEnvFile writes the project's plain .env.
func writeEnvFile(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600))
}

// deleteText runs a delete in dir and returns its stdout.
func deleteText(t *testing.T, dir string, args ...string) string {
	t.Helper()
	d, out, _ := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, append([]string{"local", "env"}, args...)...))
	return out.String()
}

// deleteJSON runs a delete in dir with --output json and decodes it.
func deleteJSON(t *testing.T, dir string, args ...string) envResult {
	t.Helper()
	d, out, _ := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, append(append([]string{"local", "env"}, args...), "--output", "json")...))
	var res envResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &res), "decode %q", out.String())
	return res
}

func TestDeleteOfAnOptionalDeclaredNameSaysItIsAbsent(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nDEMO_VAR2 = { optional = true }\n")
	writeEnvFile(t, dir, "DEMO_VAR2=x\n")

	out := deleteText(t, dir, "variable", "delete", "DEMO_VAR2")
	manifestPath := filepath.Join(dir, "pyproject.toml")
	assert.Contains(t, out, "deleted variable DEMO_VAR2 from project")
	assert.Contains(t, out, "DEMO_VAR2 is still declared in "+manifestPath+", so `astro local env list` shows it as absent. "+
		"Remove the declaration with `astro local env variable undeclare DEMO_VAR2`.")
	assert.NotContains(t, out, "next `astro local start`")
	assert.Contains(t, declared(t, dir).EnvVars, "DEMO_VAR2", "delete removed the declaration")

	writeEnvFile(t, dir, "DEMO_VAR2=x\n")
	res := deleteJSON(t, dir, "variable", "delete", "DEMO_VAR2")
	assert.Equal(t, envschema.RemainderAbsent, res.Remainder)
	assert.Equal(t, "astro local env variable set DEMO_VAR2", res.SetHint)
	assert.Equal(t, "astro local env variable undeclare DEMO_VAR2", res.UndeclareHint)
	assert.Equal(t, manifestPath, res.Manifest)
	assert.Empty(t, res.Source)
	assert.False(t, res.Undeclared)
}

func TestDeleteOfARequiredDeclaredNameSaysStartRefuses(t *testing.T) {
	// OTHER is supplied, and says nothing about API_URL.
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = {}\nOTHER = {}\n")
	writeEnvFile(t, dir, "API_URL=s3cr3t-url\nOTHER=x\n")

	out := deleteText(t, dir, "variable", "delete", "API_URL")
	assert.Contains(t, out, "API_URL is still declared in "+filepath.Join(dir, "pyproject.toml")+
		", and required: the next `astro local start` refuses until a value is set with `astro local env variable set API_URL`. "+
		"Remove the declaration with `astro local env variable undeclare API_URL`.")
	assert.NotContains(t, out, "s3cr3t-url")

	writeEnvFile(t, dir, "API_URL=s3cr3t-url\n")
	res := deleteJSON(t, dir, "variable", "delete", "API_URL")
	assert.Equal(t, envschema.RemainderRequired, res.Remainder)
	assert.Equal(t, "astro local env variable set API_URL", res.SetHint)
	assert.Equal(t, "astro local env variable undeclare API_URL", res.UndeclareHint)
}

// The other nouns report under the key the manifest declares, which for an
// Airflow variable is its plain name, not its env key.
func TestDeleteRemainderForTheOtherNouns(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.connections]\nwarehouse = {}\n\n[tool.astro.env.airflow_variables]\nregion = { optional = true }\n")
	writeEnvFile(t, dir, "AIRFLOW_CONN_WAREHOUSE={\"conn_type\":\"postgres\"}\nAIRFLOW_VAR_REGION=us\n")

	res := deleteJSON(t, dir, "connection", "delete", "warehouse")
	assert.Equal(t, envschema.RemainderRequired, res.Remainder)
	assert.Equal(t, "astro local env connection undeclare warehouse", res.UndeclareHint)

	res = deleteJSON(t, dir, "airflow-variable", "delete", "region")
	assert.Equal(t, envschema.RemainderAbsent, res.Remainder)
	assert.Equal(t, "astro local env airflow-variable undeclare region", res.UndeclareHint)
	assert.Equal(t, "astro local env airflow-variable set region", res.SetHint)
}

// The hints name the key the manifest declares, whatever spelling the delete
// was given.
func TestDeleteRemainderNamesTheDeclaredKey(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env.airflow_variables]\nregion = {}\n")
	writeEnvFile(t, dir, "AIRFLOW_VAR_REGION=us\n")

	res := deleteJSON(t, dir, "airflow-variable", "delete", "REGION")
	assert.Equal(t, envschema.RemainderRequired, res.Remainder)
	assert.Equal(t, "astro local env airflow-variable set region", res.SetHint)
	assert.Equal(t, "astro local env airflow-variable undeclare region", res.UndeclareHint)
}

// A value of another kind under the same name does not supply this one.
func TestDeleteRemainderLooksOnlyAtItsOwnKind(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nREGION = {}\n\n[tool.astro.env.airflow_variables]\nREGION = {}\n")
	writeEnvFile(t, dir, "REGION=x\nAIRFLOW_VAR_REGION=y\n")

	res := deleteJSON(t, dir, "variable", "delete", "REGION")
	assert.Equal(t, envschema.RemainderRequired, res.Remainder)
}

// undeclare, which delete --undeclare shares a path with, says in text
// whether it changed the file.
func TestUndeclareTextSaysWhetherItChanged(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = {}\n")
	manifestPath := filepath.Join(dir, "pyproject.toml")
	assert.Equal(t, "undeclared variable API_URL in "+manifestPath+"; any value set for it is kept\n",
		deleteText(t, dir, "variable", "undeclare", "API_URL"))
	assert.Equal(t, "variable API_URL is not declared in "+manifestPath+"\n",
		deleteText(t, dir, "variable", "undeclare", "API_URL"))
}

// Outside a project there is no declaration to report on.
func TestDeleteOutsideAProjectReportsOnlyTheDelete(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "global-value\n")
	require.NoError(t, execute(t, d, "local", "env", "variable", "set", "API_URL", "--global", "--plain", "--auto-link"))

	res := deleteJSON(t, t.TempDir(), "variable", "delete", "API_URL", "--global")
	assert.Equal(t, "deleted", res.Status)
	assert.Empty(t, res.Remainder)
	assert.Empty(t, res.Manifest)
}

func TestDeleteOfANameAnotherTierSuppliesNamesThatTier(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = {}\n")
	d, _, _ := envDeps(t, dir, "global-s3cr3t\n")
	require.NoError(t, execute(t, d, "local", "env", "variable", "set", "API_URL", "--global", "--plain", "--auto-link"))
	writeEnvFile(t, dir, "API_URL=project-s3cr3t\n")

	out := deleteText(t, dir, "variable", "delete", "API_URL")
	assert.Contains(t, out, "API_URL is still declared in "+filepath.Join(dir, "pyproject.toml")+", and now resolves from "+vaultenv.SourceGlobal+".")
	assert.NotContains(t, out, "s3cr3t")
	assert.NotContains(t, out, "undeclare")

	writeEnvFile(t, dir, "API_URL=project-s3cr3t\n")
	res := deleteJSON(t, dir, "variable", "delete", "API_URL")
	assert.Equal(t, envschema.RemainderSupplied, res.Remainder)
	assert.Equal(t, vaultenv.SourceGlobal, res.Source)
	assert.Empty(t, res.SetHint)
	assert.Empty(t, res.UndeclareHint)
}

func TestDeleteOfANameWithADefaultSaysTheDefaultApplies(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nLOG_LEVEL = 'info'\n")
	writeEnvFile(t, dir, "LOG_LEVEL=debug\n")

	out := deleteText(t, dir, "variable", "delete", "LOG_LEVEL")
	assert.Contains(t, out, "LOG_LEVEL is still declared in "+filepath.Join(dir, "pyproject.toml")+", and its declared default now applies.")
	assert.NotContains(t, out, "now resolves from")
}

// A workspace-sourced name the workspace does not supply still lists under
// the workspace's label, and is no less absent for it.
func TestDeleteOfAnUnresolvedWorkspaceNameIsNotSupplied(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = { source = 'workspace', optional = true }\n")
	writeEnvFile(t, dir, "API_URL=x\n")

	res := deleteJSON(t, dir, "variable", "delete", "API_URL")
	assert.Equal(t, envschema.RemainderAbsent, res.Remainder)
}

func TestDeleteOfAnUndeclaredNameReportsOnlyTheDelete(t *testing.T) {
	dir := envProject(t, "")
	writeEnvFile(t, dir, "LEFTOVER=x\n")

	out := deleteText(t, dir, "variable", "delete", "LEFTOVER")
	assert.Equal(t, 1, strings.Count(out, "\n"), out)
	writeEnvFile(t, dir, "LEFTOVER=x\n")
	res := deleteJSON(t, dir, "variable", "delete", "LEFTOVER")
	assert.Empty(t, res.Remainder)
	assert.Empty(t, res.Manifest)
}

func TestDeleteUndeclareRemovesTheDeclarationToo(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\n# keep me\nLOG_LEVEL = 'info'\nDEMO_VAR2 = { optional = true }\n")
	writeEnvFile(t, dir, "DEMO_VAR2=x\nOTHER=y\n")

	out := deleteText(t, dir, "variable", "delete", "DEMO_VAR2", "--undeclare")
	manifestPath := filepath.Join(dir, "pyproject.toml")
	assert.Contains(t, out, "deleted variable DEMO_VAR2 from project")
	assert.Contains(t, out, "undeclared variable DEMO_VAR2 in "+manifestPath)
	assert.NotContains(t, out, "still declared")
	s := declared(t, dir)
	assert.NotContains(t, s.EnvVars, "DEMO_VAR2")
	assert.Contains(t, s.EnvVars, "LOG_LEVEL")
	assert.Contains(t, readManifest(t, dir), "# keep me")
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "OTHER=y\n", string(env))

	// Already undeclared: refused, and the value stays.
	writeEnvFile(t, dir, "DEMO_VAR2=x\n")
	d, _, _ := envDeps(t, dir, "")
	err = execute(t, d, "local", "env", "variable", "delete", "DEMO_VAR2", "--undeclare")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable DEMO_VAR2 is not declared in "+manifestPath+", so nothing was deleted")
	assertEnvFile(t, dir, "DEMO_VAR2=x\n")
}

func assertEnvFile(t *testing.T, dir, want string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, want, string(b))
}

// A manifest that does not load refuses --undeclare before the value goes.
func TestDeleteUndeclareRefusesABrokenManifest(t *testing.T) {
	for name, body := range map[string]string{
		"toml":   "\n[tool.astro.env\nAPI_URL = {}\n",
		"schema": "\n[tool.astro.env]\nAPI_URL = { type = 'nope' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := envProject(t, body)
			writeEnvFile(t, dir, "API_URL=x\n")
			before := readManifest(t, dir)
			d, _, _ := envDeps(t, dir, "")
			err := execute(t, d, "local", "env", "variable", "delete", "API_URL", "--undeclare")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "so nothing was deleted")
			assertEnvFile(t, dir, "API_URL=x\n")
			assert.Equal(t, before, readManifest(t, dir))
		})
	}
}

// A delete reads no workspace, so an offline one cannot hold up the report,
// which says the workspace may still supply the name.
func TestDeleteRemainderDoesNotReadTheWorkspace(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	dir := workspaceEnvProject(t)
	m := readManifest(t, dir) + "DEMO_VAR2 = { optional = true }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(m), 0o600))
	writeEnvFile(t, dir, "DEMO_VAR2=x\n")
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("dial tcp: no route to host"))

	d, out, _ := envDeps(t, dir, "")
	d.WorkspaceClients = workspaceClients(mc)
	require.NoError(t, execute(t, d, "local", "env", "variable", "delete", "DEMO_VAR2"))
	assert.Contains(t, out.String(), "DEMO_VAR2 is still declared in")
	assert.Contains(t, out.String(), "shows it as absent")
	assert.Contains(t, out.String(), "It may still come from workspace cmws, which a delete does not read.")
	mc.AssertNotCalled(t, "ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything)

	writeEnvFile(t, dir, "DEMO_VAR2=x\n")
	res := deleteJSON(t, dir, "variable", "delete", "DEMO_VAR2")
	assert.Equal(t, "cmws", res.Workspace)
}

// A workspace-sourced name the workspace did not supply lists under the
// workspace label, and still gets both ways out.
func TestListNotesAnUnsuppliedWorkspaceName(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = { source = 'workspace', optional = true }\n")

	d, out, _ := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, "local", "env", "list"))
	assert.Contains(t, out.String(), "declared, no value; set: astro local env variable set API_URL; "+
		"or remove the declaration: astro local env variable undeclare API_URL")
}

func TestDeleteUndeclareJSON(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = {}\n")
	writeEnvFile(t, dir, "API_URL=x\n")

	res := deleteJSON(t, dir, "variable", "delete", "API_URL", "--undeclare")
	assert.Equal(t, "deleted", res.Status)
	assert.True(t, res.Undeclared)
	assert.Equal(t, filepath.Join(dir, "pyproject.toml"), res.Manifest)
	assert.Empty(t, res.Remainder)
	assert.NotContains(t, declared(t, dir).EnvVars, "API_URL")
}

// With nothing to delete, --undeclare changes nothing either, and names the
// command that removes only the declaration.
func TestDeleteUndeclareOfAnUnsetNameChangesNothing(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = {}\n")
	before := readManifest(t, dir)

	d, _, _ := envDeps(t, dir, "")
	err := execute(t, d, "local", "env", "variable", "delete", "API_URL", "--undeclare")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "astro local env variable undeclare API_URL")
	assert.Equal(t, before, readManifest(t, dir))
}

func TestDeleteUndeclareGlobalEditsOnlyTheCurrentProject(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nAPI_URL = {}\n")
	other := t.TempDir()
	otherManifest := "[project]\nname = 'other'\n\n[tool.astro.env]\nAPI_URL = {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(other, "pyproject.toml"), []byte(otherManifest), 0o600))
	d, _, _ := envDeps(t, dir, "global-value\n")
	require.NoError(t, execute(t, d, "local", "env", "variable", "set", "API_URL", "--global", "--plain", "--auto-link"))
	writeEnvFile(t, dir, "API_URL=project-value\n")

	out := deleteText(t, dir, "variable", "delete", "API_URL", "--global", "--undeclare")
	assert.Contains(t, out, "deleted variable API_URL from "+vaultenv.SourceGlobal)
	assert.Contains(t, out, "undeclared variable API_URL in "+filepath.Join(dir, "pyproject.toml"))
	assert.NotContains(t, declared(t, dir).EnvVars, "API_URL")
	b, err := os.ReadFile(filepath.Join(other, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, otherManifest, string(b), "another project's declaration was edited")
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "API_URL=project-value\n", string(env), "a --global delete removed the project value")
}

func TestDeleteUndeclareOutsideAProjectRefuses(t *testing.T) {
	dir := envProject(t, "")
	d, _, _ := envDeps(t, dir, "global-value\n")
	require.NoError(t, execute(t, d, "local", "env", "variable", "set", "API_URL", "--global", "--plain", "--auto-link"))
	outside := t.TempDir()

	for _, args := range [][]string{
		{"variable", "delete", "API_URL", "--global", "--undeclare"},
		{"variable", "delete", "API_URL", "--undeclare"},
	} {
		d, _, _ = envDeps(t, outside, "")
		err := execute(t, d, append([]string{"local", "env"}, args...)...)
		require.Error(t, err, args)
		assert.Contains(t, err.Error(), "there is no project here, so nothing was deleted", args)
	}
	// The global value is still there.
	d, out, _ := envDeps(t, outside, "")
	require.NoError(t, execute(t, d, "local", "env", "variable", "get", "API_URL", "--global"))
	assert.Equal(t, "global-value\n", out.String())
}

func TestListNotesBothWaysOutOfAnAbsentName(t *testing.T) {
	dir := envProject(t, "\n[tool.astro.env]\nDEMO_VAR2 = { optional = true }\nPRESENT = {}\n")
	writeEnvFile(t, dir, "PRESENT=x\n")

	d, out, _ := envDeps(t, dir, "")
	require.NoError(t, execute(t, d, "local", "env", "list"))
	var demo, present string
	for _, line := range strings.Split(out.String(), "\n") {
		switch {
		case strings.Contains(line, "DEMO_VAR2"):
			demo = line
		case strings.Contains(line, "PRESENT"):
			present = line
		}
	}
	assert.Contains(t, demo, "declared, no value; set: astro local env variable set DEMO_VAR2; "+
		"or remove the declaration: astro local env variable undeclare DEMO_VAR2")
	assert.NotContains(t, present, "declared, no value")

	d, out, _ = envDeps(t, dir, "")
	require.NoError(t, execute(t, d, "local", "env", "list", "--output", "json"))
	rows := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var row map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		rows[row["name"].(string)] = row
	}
	assert.Equal(t, "astro local env variable set DEMO_VAR2", rows["DEMO_VAR2"]["set_hint"])
	assert.Equal(t, "astro local env variable undeclare DEMO_VAR2", rows["DEMO_VAR2"]["undeclare_hint"])
	assert.NotContains(t, rows["PRESENT"], "set_hint")
	assert.NotContains(t, rows["PRESENT"], "undeclare_hint")
}
