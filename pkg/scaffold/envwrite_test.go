package scaffold

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// envFixture is a [tool.astro.env] a person wrote: comments above and beside
// declarations, the string shorthand, inline tables, one declaration under its
// own header, and a connection id in mixed case.
const envFixture = `# the orders team's project
[project]
name = 'orders'
dependencies = []

[tool.astro]
airflow = '3.1'

[tool.astro.env]
# every environment starts at this level
LOG_LEVEL = 'info'
API_TOKEN = { sensitive = true } # from the vault
DEFAULTED = { default = 'x', description = 'has a default' }

[tool.astro.env.WAREHOUSE_URI]
# ask the data team
type = 'url'
description = 'the warehouse'

[tool.astro.env.connections]
DB_Main = { conn_type = 'postgres' }

[tool.astro.env.airflow_variables]
region = {} # set per environment
`

// fixtureComments are the comments in envFixture, every one of which an edit
// elsewhere must leave in place.
var fixtureComments = []string{
	"# the orders team's project",
	"# every environment starts at this level",
	"# from the vault",
	"# ask the data team",
	"# set per environment",
}

// loadSchema parses the manifest at path the way `astro local start` does.
func loadSchema(t *testing.T, path string) *envschema.Schema {
	t.Helper()
	m, err := manifest.Load(path)
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	return s
}

func assertCommentsKept(t *testing.T, got string, except ...string) {
	t.Helper()
	for _, c := range fixtureComments {
		if !slices.Contains(except, c) {
			assert.Contains(t, got, c, "an edit elsewhere dropped a comment")
		}
	}
}

// assertOthersUnchanged checks that every declaration but the edited one
// parses exactly as the fixture's did.
func assertOthersUnchanged(t *testing.T, got *envschema.Schema, section envschema.Section, edited string) {
	t.Helper()
	want := loadSchemaFrom(t, envFixture)
	for _, sec := range []struct {
		section   envschema.Section
		got, want map[string]envschema.ValueSpec
	}{
		{envschema.SectionEnvVar, got.EnvVars, want.EnvVars},
		{envschema.SectionAirflowVariable, got.AirflowVariables, want.AirflowVariables},
		{envschema.SectionConnection, got.Connections, want.Connections},
	} {
		for name, spec := range sec.want {
			if sec.section == section && name == edited {
				continue
			}
			assert.Equal(t, spec, sec.got[name], "%s %s changed", sec.section, name)
		}
	}
}

func loadSchemaFrom(t *testing.T, body string) *envschema.Schema {
	t.Helper()
	m, err := manifest.Parse([]byte(body))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	return s
}

// SetEnvDeclaration declares name in section with exactly spec's annotations,
// or with none for a nil spec, replacing the ones it had: the whole-spec edit
// these tests drive EditEnvDeclaration with.
func SetEnvDeclaration(dir string, wrap func(run func() error) error, section envschema.Section, name string, spec *envschema.ValueSpec) error {
	return EditEnvDeclaration(dir, wrap, section, name, func(s *envschema.ValueSpec, _ bool) error {
		if spec == nil {
			*s = envschema.ValueSpec{}
			return nil
		}
		*s = cloneSpec(spec)
		return nil
	})
}

func TestAddEnvDeclarationWritesItsAnnotations(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	spec := &envschema.ValueSpec{Sensitive: true, Optional: true, Description: "for the alerts channel", Type: envschema.TypeURL}
	require.NoError(t, AddEnvDeclaration(dir, nil, envschema.SectionEnvVar, "SLACK_WEBHOOK", spec))

	got := loadSchema(t, path)
	assert.Equal(t, envschema.ValueSpec{
		Sensitive: true, HasSensitive: true, Optional: true, Description: "for the alerts channel", Type: envschema.TypeURL,
	}, got.EnvVars["SLACK_WEBHOOK"])
	assertOthersUnchanged(t, got, envschema.SectionEnvVar, "SLACK_WEBHOOK")
	assertCommentsKept(t, readFile(t, path))
}

func TestAddEnvDeclarationWithNoSpecIsBare(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, AddEnvDeclaration(dir, nil, envschema.SectionEnvVar, "PLAIN", nil))

	assert.Contains(t, readFile(t, path), "PLAIN = {}")
	assert.Equal(t, envschema.ValueSpec{}, loadSchema(t, path).EnvVars["PLAIN"])
}

// Adding a name that is declared already leaves what a person wrote alone,
// whatever the call asked for.
func TestAddEnvDeclarationKeepsAnExistingOne(t *testing.T) {
	for _, tc := range []struct {
		section envschema.Section
		name    string
	}{
		{envschema.SectionEnvVar, "LOG_LEVEL"},
		{envschema.SectionEnvVar, "WAREHOUSE_URI"},
		// Found under the case it was written in, not declared a second time.
		{envschema.SectionConnection, "db_main"},
		{envschema.SectionConnection, "AIRFLOW_CONN_DB_MAIN"},
		{envschema.SectionAirflowVariable, "REGION"},
		{envschema.SectionAirflowVariable, "AIRFLOW_VAR_REGION"},
	} {
		dir, path := writeEditFixture(t, envFixture, 0o644)
		err := AddEnvDeclaration(dir, nil, tc.section, tc.name, &envschema.ValueSpec{Description: "overwritten?"})
		require.NoError(t, err, tc.name)
		assert.Equal(t, envFixture, readFile(t, path), "adding %s %s rewrote the file", tc.section, tc.name)
	}
}

// An env-form key is declared by its plain name: AIRFLOW_VAR_BATCH_SIZE as the
// variable batch_size, not airflow_var_batch_size, whose env key matches
// nothing.
func TestAddEnvDeclarationTakesTheEnvForm(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, AddEnvDeclaration(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_BATCH_SIZE", nil))
	require.NoError(t, AddEnvDeclaration(dir, nil, envschema.SectionConnection, "AIRFLOW_CONN_WAREHOUSE",
		&envschema.ValueSpec{ConnType: "snowflake"}))

	got := loadSchema(t, path)
	assert.Contains(t, got.AirflowVariables, "batch_size")
	assert.NotContains(t, got.AirflowVariables, "airflow_var_batch_size")
	require.Contains(t, got.Connections, "warehouse")
	assert.Equal(t, "snowflake", got.Connections["warehouse"].ConnType)
	assert.Equal(t, []string{
		"AIRFLOW_CONN_DB_MAIN", "AIRFLOW_CONN_WAREHOUSE", "AIRFLOW_VAR_BATCH_SIZE", "AIRFLOW_VAR_REGION",
		"API_TOKEN", "DEFAULTED", "LOG_LEVEL", "WAREHOUSE_URI",
	}, envschema.DeclaredEnvKeys(got))
	assertCommentsKept(t, readFile(t, path))
}

func TestRemoveEnvDeclaration(t *testing.T) {
	for _, tc := range []struct {
		section envschema.Section
		name    string
		gone    string // the comment that goes with it, if any
	}{
		{envschema.SectionEnvVar, "API_TOKEN", "# from the vault"},
		{envschema.SectionEnvVar, "WAREHOUSE_URI", "# ask the data team"},
		{envschema.SectionConnection, "AIRFLOW_CONN_DB_MAIN", ""},
		{envschema.SectionAirflowVariable, "Region", "# set per environment"},
	} {
		dir, path := writeEditFixture(t, envFixture, 0o644)

		require.NoError(t, RemoveEnvDeclaration(dir, nil, tc.section, tc.name), tc.name)

		got := loadSchema(t, path)
		name, err := envschema.DeclarationName(tc.section, tc.name)
		require.NoError(t, err)
		for _, decls := range []map[string]envschema.ValueSpec{got.EnvVars, got.AirflowVariables, got.Connections} {
			for k := range decls {
				assert.False(t, strings.EqualFold(k, name), "%s is still declared", tc.name)
			}
		}
		assertOthersUnchanged(t, got, tc.section, map[envschema.Section]string{
			envschema.SectionEnvVar: tc.name, envschema.SectionConnection: "DB_Main", envschema.SectionAirflowVariable: "region",
		}[tc.section])
		assertCommentsKept(t, readFile(t, path), tc.gone)
	}
}

func TestRemoveEnvDeclarationThatIsNotThereWritesNothing(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, "NOT_DECLARED"))

	assert.Equal(t, envFixture, readFile(t, path))
}

// The parser accepts a variable a person wrote in env form, AIRFLOW_VAR_ZONE,
// so that name addresses that declaration first. Only when it is not declared
// does it fall back to the plain name, zone.
func TestEnvFormNameAddressesItsOwnDeclarationFirst(t *testing.T) {
	withEnvForm := strings.Replace(envFixture, "region = {} # set per environment",
		"region = {} # set per environment\nAIRFLOW_VAR_ZONE = {}", 1)
	withBoth := strings.Replace(withEnvForm, "AIRFLOW_VAR_ZONE = {}",
		"AIRFLOW_VAR_ZONE = {}\nzone = { description = 'plain' }", 1)
	zone := envschema.ValueSpec{Description: "plain"}

	t.Run("remove finds the env-form one", func(t *testing.T) {
		dir, path := writeEditFixture(t, withEnvForm, 0o644)
		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_ZONE"))
		assert.NotContains(t, loadSchema(t, path).AirflowVariables, "AIRFLOW_VAR_ZONE")
	})
	t.Run("remove leaves the plain one beside it", func(t *testing.T) {
		dir, path := writeEditFixture(t, withBoth, 0o644)
		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_ZONE"))
		got := loadSchema(t, path).AirflowVariables
		assert.NotContains(t, got, "AIRFLOW_VAR_ZONE")
		assert.Equal(t, zone, got["zone"], "removing AIRFLOW_VAR_ZONE removed zone")
	})
	t.Run("an edit changes the env-form one", func(t *testing.T) {
		dir, path := writeEditFixture(t, withBoth, 0o644)
		require.NoError(t, DeclareEnvFromWorkspace(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_ZONE", ""))
		require.NoError(t, SetEnvDeclaration(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_ZONE",
			&envschema.ValueSpec{Source: envschema.SourceWorkspace, Description: "env form"}))
		got := loadSchema(t, path).AirflowVariables
		assert.Equal(t, envschema.ValueSpec{Source: envschema.SourceWorkspace, Description: "env form"}, got["AIRFLOW_VAR_ZONE"])
		assert.Equal(t, zone, got["zone"], "editing AIRFLOW_VAR_ZONE edited zone")
	})
	t.Run("an add keeps the env-form one", func(t *testing.T) {
		dir, path := writeEditFixture(t, withEnvForm, 0o644)
		require.NoError(t, AddEnvDeclaration(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_ZONE", nil))
		assert.Equal(t, withEnvForm, readFile(t, path), "a second declaration of the same variable was added")
	})
}

// A declaration under a name the grammar refuses breaks the whole section, and
// removing it is the fix, so Remove does not refuse the name it is fixing.
func TestRemoveEnvDeclarationFixesABadName(t *testing.T) {
	body := strings.Replace(envFixture, "LOG_LEVEL = 'info'", "LOG_LEVEL = 'info'\nMY-VAR = {}", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, "MY-VAR"))

	assert.NotContains(t, loadSchema(t, path).EnvVars, "MY-VAR")
}

// envSectionLoads reports whether the section at path parses, failing the test
// if the manifest itself does not.
func envSectionLoads(t *testing.T, path string) bool {
	t.Helper()
	m, err := manifest.Load(path)
	require.NoError(t, err)
	_, err = envschema.ParseSchema(m.Astro.Env)
	return err == nil
}

// ParseSchema refuses the whole section over any one problem, so a section
// with several is fixed one removal at a time: each removal that leaves the
// section no worse is written, though what remains still does not load.
func TestRemoveEnvDeclarationFixesSeveralProblemsInTurn(t *testing.T) {
	t.Run("two bad names", func(t *testing.T) {
		body := strings.Replace(envFixture, "LOG_LEVEL = 'info'", "LOG_LEVEL = 'info'\nMY-VAR = {}\nOTHER-VAR = {}", 1)
		dir, path := writeEditFixture(t, body, 0o644)

		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, "MY-VAR"))
		assert.NotContains(t, readFile(t, path), "MY-VAR")
		assert.False(t, envSectionLoads(t, path), "OTHER-VAR is still there")

		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, "OTHER-VAR"))
		assert.True(t, envSectionLoads(t, path))
	})
	t.Run("a bad name, an unknown field and a sensitive connection", func(t *testing.T) {
		body := strings.Replace(envFixture, "LOG_LEVEL = 'info'", "LOG_LEVEL = 'info'\nMY-VAR = {}", 1)
		body = strings.Replace(body, "API_TOKEN = { sensitive = true }", "API_TOKEN = { sensitive = true, shade = 'red' }", 1)
		body = strings.Replace(body, "DB_Main = { conn_type = 'postgres' }", "DB_Main = { conn_type = 'postgres', sensitive = true }", 1)
		dir, path := writeEditFixture(t, body, 0o644)

		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, "MY-VAR"))
		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, "API_TOKEN"))
		assert.False(t, envSectionLoads(t, path), "DB_Main is still there")
		require.NoError(t, RemoveEnvDeclaration(dir, nil, envschema.SectionConnection, "db_main"))

		got := loadSchema(t, path)
		assert.NotContains(t, got.EnvVars, "API_TOKEN")
		assert.NotContains(t, got.Connections, "DB_Main")
		assert.Contains(t, got.EnvVars, "LOG_LEVEL")
	})
}

// Removal's judge accepts only the problems the file already had. An edit
// that fixes one and adds another is refused, and the file is unchanged.
func TestNoNewEnvProblemsRefusesANewProblem(t *testing.T) {
	body := strings.Replace(envFixture, "LOG_LEVEL = 'info'", "LOG_LEVEL = 'info'\nMY-VAR = {}", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	err := editManifestJudged(dir, nil, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		ed.Delete([]string{"tool", "astro", "env", "MY-VAR"})
		return ed.Set([]string{"tool", "astro", "env", "NEW-VAR"}, map[string]any{})
	}, noNewEnvProblems)

	require.ErrorIs(t, err, ErrEditRefused)
	var schema *envschema.SchemaError
	require.ErrorAs(t, err, &schema)
	assert.Equal(t, "tool.astro.env.NEW-VAR", schema.Problems[0].Key)
	assert.Equal(t, body, readFile(t, path))
}

// A removal is held to manifest.Parse as always: the relaxed judge covers the
// env section's problems and nothing else.
func TestNoNewEnvProblemsStillRequiresTheManifestToParse(t *testing.T) {
	body := strings.Replace(envFixture, "LOG_LEVEL = 'info'", "LOG_LEVEL = 'info'\nMY-VAR = {}", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	err := editManifestJudged(dir, nil, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		ed.Delete([]string{"tool", "astro", "env", "MY-VAR"})
		return ed.Set([]string{"tool", "astro", "domain"}, "astronomer.io")
	}, noNewEnvProblems)

	require.ErrorIs(t, err, ErrEditRefused)
	var invalid *manifest.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, body, readFile(t, path))
}

// connections and airflow_variables, as env var names, would address the
// sub-sections themselves. No operation treats either as a declaration:
// removing one would drop every connection or variable, and the result still
// parses, so the round trip cannot catch it.
func TestNoEnvOperationAddressesASubSection(t *testing.T) {
	ops := map[string]func(dir, name string) error{
		"remove": func(dir, name string) error {
			return RemoveEnvDeclaration(dir, nil, envschema.SectionEnvVar, name)
		},
		"add": func(dir, name string) error {
			return AddEnvDeclaration(dir, nil, envschema.SectionEnvVar, name, nil)
		},
		"set": func(dir, name string) error {
			return SetEnvDeclaration(dir, nil, envschema.SectionEnvVar, name, nil)
		},
		"declare": func(dir, name string) error {
			return DeclareEnvFromWorkspace(dir, nil, envschema.SectionEnvVar, name, "")
		},
	}
	for _, name := range []string{"connections", "airflow_variables"} {
		for op, run := range ops {
			dir, path := writeEditFixture(t, envFixture, 0o644)
			require.Error(t, run(dir, name), "%s %s", op, name)
			assert.Equal(t, envFixture, readFile(t, path), "%s %s changed the file", op, name)
		}
	}
}

// Annotating a declaration under its own header changes the lines that
// changed, and keeps the header, the comment in it, and the line that did not.
func TestSetEnvDeclarationChangesOnlyWhatChanged(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, SetEnvDeclaration(dir, nil, envschema.SectionEnvVar, "WAREHOUSE_URI", &envschema.ValueSpec{
		Type: envschema.TypeURL, Description: "the analytics warehouse", Optional: true,
	}))

	raw := readFile(t, path)
	assert.Contains(t, raw, "[tool.astro.env.WAREHOUSE_URI]\n# ask the data team\ntype = 'url'\n")
	assert.Contains(t, raw, "the analytics warehouse")
	assert.NotContains(t, raw, "'the warehouse'")
	got := loadSchema(t, path)
	assert.Equal(t, envschema.ValueSpec{Type: envschema.TypeURL, Description: "the analytics warehouse", Optional: true},
		got.EnvVars["WAREHOUSE_URI"])
	assertOthersUnchanged(t, got, envschema.SectionEnvVar, "WAREHOUSE_URI")
	assertCommentsKept(t, raw)
}

// Taking an annotation away deletes its key, and the rest of the table stays.
func TestSetEnvDeclarationDropsAnAnnotation(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, SetEnvDeclaration(dir, nil, envschema.SectionEnvVar, "WAREHOUSE_URI",
		&envschema.ValueSpec{Type: envschema.TypeURL}))

	raw := readFile(t, path)
	assert.NotContains(t, raw, "the warehouse")
	assert.Equal(t, envschema.ValueSpec{Type: envschema.TypeURL}, loadSchema(t, path).EnvVars["WAREHOUSE_URI"])
	assertCommentsKept(t, raw)
}

// Taking every annotation away leaves the name declared, whichever way the
// table was written. A dotted-key declaration is nothing but its keys, so
// deleting them one by one would otherwise undeclare it.
func TestSetEnvDeclarationToBareKeepsItDeclared(t *testing.T) {
	body := strings.Replace(envFixture, "LOG_LEVEL = 'info'",
		"LOG_LEVEL = 'info'\nDOTTED.type = 'url'\nDOTTED.description = 'dotted'", 1)
	for _, tc := range []struct {
		section envschema.Section
		name    string
	}{
		{envschema.SectionEnvVar, "DOTTED"},
		{envschema.SectionEnvVar, "WAREHOUSE_URI"},
		{envschema.SectionEnvVar, "API_TOKEN"},
		{envschema.SectionConnection, "DB_Main"},
	} {
		dir, path := writeEditFixture(t, body, 0o644)

		require.NoError(t, SetEnvDeclaration(dir, nil, tc.section, tc.name, nil), tc.name)

		got := loadSchema(t, path)
		decls := map[envschema.Section]map[string]envschema.ValueSpec{
			envschema.SectionEnvVar: got.EnvVars, envschema.SectionConnection: got.Connections,
		}[tc.section]
		require.Contains(t, decls, tc.name, "setting %s to bare undeclared it", tc.name)
		want := envschema.ValueSpec{Sensitive: tc.section == envschema.SectionConnection}
		assert.Equal(t, want, decls[tc.name], tc.name)
	}
}

// The string shorthand stays a string while its default is all it says, and
// becomes a table, default kept, once it says more.
func TestEditEnvDeclarationOnTheShorthand(t *testing.T) {
	t.Run("a new default stays a string", func(t *testing.T) {
		dir, path := writeEditFixture(t, envFixture, 0o644)
		require.NoError(t, SetEnvDeclaration(dir, nil, envschema.SectionEnvVar, "LOG_LEVEL",
			&envschema.ValueSpec{Default: "debug", HasDefault: true}))
		assert.Contains(t, readFile(t, path), "LOG_LEVEL = 'debug'")
		assertCommentsKept(t, readFile(t, path))
	})
	t.Run("an annotation makes it a table", func(t *testing.T) {
		dir, path := writeEditFixture(t, envFixture, 0o644)
		require.NoError(t, EditEnvDeclaration(dir, nil, envschema.SectionEnvVar, "LOG_LEVEL",
			func(s *envschema.ValueSpec, declared bool) error {
				require.True(t, declared)
				s.Type, s.Enum = envschema.TypeEnum, []string{"debug", "info"}
				return nil
			}))
		got := loadSchema(t, path)
		assert.Equal(t, envschema.ValueSpec{
			Default: "info", HasDefault: true, Type: envschema.TypeEnum, Enum: []string{"debug", "info"},
		}, got.EnvVars["LOG_LEVEL"])
		assertOthersUnchanged(t, got, envschema.SectionEnvVar, "LOG_LEVEL")
		raw := readFile(t, path)
		assert.Contains(t, raw, "# every environment starts at this level\nLOG_LEVEL = {", "the declaration moved")
		assertCommentsKept(t, raw)
	})
	t.Run("no change writes nothing", func(t *testing.T) {
		dir, path := writeEditFixture(t, envFixture, 0o644)
		require.NoError(t, SetEnvDeclaration(dir, nil, envschema.SectionEnvVar, "LOG_LEVEL",
			&envschema.ValueSpec{Default: "info", HasDefault: true}))
		assert.Equal(t, envFixture, readFile(t, path))
	})
}

// source = 'workspace' goes inside the table that is there, and everything the
// table already said stays.
func TestDeclareEnvFromWorkspaceKeepsTheTable(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, DeclareEnvFromWorkspace(dir, nil, envschema.SectionEnvVar, "API_TOKEN", ""))
	require.NoError(t, DeclareEnvFromWorkspace(dir, nil, envschema.SectionEnvVar, "WAREHOUSE_URI", ""))
	// Found under DB_Main; the conn_type it has is kept, not replaced.
	require.NoError(t, DeclareEnvFromWorkspace(dir, nil, envschema.SectionConnection, "AIRFLOW_CONN_DB_MAIN", "mysql"))

	got := loadSchema(t, path)
	assert.Equal(t, envschema.ValueSpec{Sensitive: true, HasSensitive: true, Source: envschema.SourceWorkspace},
		got.EnvVars["API_TOKEN"])
	assert.Equal(t, envschema.ValueSpec{Type: envschema.TypeURL, Description: "the warehouse", Source: envschema.SourceWorkspace},
		got.EnvVars["WAREHOUSE_URI"])
	assert.Equal(t, envschema.ValueSpec{Sensitive: true, ConnType: "postgres", Source: envschema.SourceWorkspace},
		got.Connections["DB_Main"])
	assert.NotContains(t, got.Connections, "db_main")
	raw := readFile(t, path)
	assert.Contains(t, raw, "[tool.astro.env.WAREHOUSE_URI]\n# ask the data team\n")
	assertCommentsKept(t, raw)
}

func TestDeclareEnvFromWorkspaceDeclaresANewName(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	require.NoError(t, DeclareEnvFromWorkspace(dir, nil, envschema.SectionAirflowVariable, "AIRFLOW_VAR_TEAM", "ignored"))
	require.NoError(t, DeclareEnvFromWorkspace(dir, nil, envschema.SectionConnection, "AIRFLOW_CONN_LAKE", "aws"))

	got := loadSchema(t, path)
	assert.Equal(t, envschema.ValueSpec{Source: envschema.SourceWorkspace}, got.AirflowVariables["team"])
	assert.Equal(t, envschema.ValueSpec{Sensitive: true, ConnType: "aws", Source: envschema.SourceWorkspace},
		got.Connections["lake"])
}

// A workspace-sourced name never falls back to a default, so giving one to a
// declaration that has a default would drop the value the default commits to.
// Refused, string shorthand and table alike, with the file as it was.
func TestDeclareEnvFromWorkspaceRefusesADefault(t *testing.T) {
	for _, name := range []string{"LOG_LEVEL", "DEFAULTED"} {
		dir, path := writeEditFixture(t, envFixture, 0o644)

		err := DeclareEnvFromWorkspace(dir, nil, envschema.SectionEnvVar, name, "")

		require.ErrorIs(t, err, ErrWorkspaceSourceWithDefault, name)
		assert.Equal(t, envFixture, readFile(t, path), "a refused declare changed the file")
	}
}

// The rule is about what an edit introduces. A declaration someone wrote with
// both keeps loading, and an unrelated annotation on it is not refused.
func TestEditEnvDeclarationLeavesAnExistingWorkspaceDefault(t *testing.T) {
	body := strings.Replace(envFixture, "DEFAULTED = { default = 'x', description = 'has a default' }",
		"DEFAULTED = { default = 'x', source = 'workspace' }", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	require.NoError(t, EditEnvDeclaration(dir, nil, envschema.SectionEnvVar, "DEFAULTED",
		func(s *envschema.ValueSpec, _ bool) error { s.Description = "noted"; return nil }))

	assert.Equal(t, "noted", loadSchema(t, path).EnvVars["DEFAULTED"].Description)
}

// A declaration the parser would refuse is not written, and the file is left
// exactly as it was: EditManifest's ParseSchema round trip is what refuses it.
func TestEnvDeclarationThatWouldNotLoadIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		section envschema.Section
		key     string
		spec    envschema.ValueSpec
		code    envschema.ProblemCode
	}{
		{
			"sensitive with a default", envschema.SectionEnvVar, "API_TOKEN",
			envschema.ValueSpec{Sensitive: true, Default: "hunter2", HasDefault: true},
			envschema.CodeSensitiveDefault,
		},
		{
			"enum without its type", envschema.SectionEnvVar, "NEW_ONE",
			envschema.ValueSpec{Enum: []string{"a"}},
			envschema.CodeEnumNeedsType,
		},
		{
			"conn_type on a variable", envschema.SectionAirflowVariable, "region",
			envschema.ValueSpec{ConnType: "postgres"},
			envschema.CodeConnTypeOutsideConnections,
		},
		{
			"a connection with a default", envschema.SectionConnection, "lake",
			envschema.ValueSpec{Default: "s3://", HasDefault: true},
			envschema.CodeSensitiveDefault,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := writeEditFixture(t, envFixture, 0o644)

			err := SetEnvDeclaration(dir, nil, tc.section, tc.key, &tc.spec)

			require.ErrorIs(t, err, ErrEditRefused)
			var schema *envschema.SchemaError
			require.ErrorAs(t, err, &schema)
			require.Len(t, schema.Problems, 1)
			assert.Equal(t, tc.code, schema.Problems[0].Code)
			assert.Equal(t, envFixture, readFile(t, path), "a refused edit changed the file")
		})
	}
}

// A name the grammar refuses is refused before the file is read.
func TestEnvDeclarationRefusesABadName(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	for _, name := range []string{"MY-VAR", "connections", ""} {
		require.Error(t, AddEnvDeclaration(dir, nil, envschema.SectionEnvVar, name, nil), name)
	}
	require.Error(t, AddEnvDeclaration(dir, nil, envschema.Section("secrets"), "X", nil))
	assert.Equal(t, envFixture, readFile(t, path))
}

// A declaration that does not load cannot be read to edit, so editing it is
// refused with the parser's reason, and the file is unchanged.
func TestEditEnvDeclarationRefusesOneThatDoesNotLoad(t *testing.T) {
	body := strings.Replace(envFixture, "API_TOKEN = { sensitive = true }", "API_TOKEN = { sensitive = 'yes' }", 1)
	dir, path := writeEditFixture(t, body, 0o644)

	err := DeclareEnvFromWorkspace(dir, nil, envschema.SectionEnvVar, "API_TOKEN", "")

	var schema *envschema.SchemaError
	require.ErrorAs(t, err, &schema)
	assert.Equal(t, envschema.CodeExpectedBool, schema.Problems[0].Code)
	assert.Equal(t, body, readFile(t, path))
}

// The read happens inside the wrapper: a writer that changed the file while
// holding the lock is seen, not overwritten.
func TestEnvDeclarationRunsInsideTheWrapper(t *testing.T) {
	dir, path := writeEditFixture(t, envFixture, 0o644)

	calls := 0
	wrap := func(run func() error) error {
		calls++
		// Another writer's edit, made while the lock is held.
		if err := AddEnvDeclaration(dir, nil, envschema.SectionEnvVar, "OTHER_WRITER", nil); err != nil {
			return err
		}
		return run()
	}
	require.NoError(t, DeclareEnvFromWorkspace(dir, wrap, envschema.SectionEnvVar, "API_TOKEN", ""))

	assert.Equal(t, 1, calls)
	got := loadSchema(t, path)
	assert.Contains(t, got.EnvVars, "OTHER_WRITER", "the edit read the file before the wrapper ran")
	assert.Equal(t, envschema.SourceWorkspace, got.EnvVars["API_TOKEN"].Source)

	// A wrapper that never calls run is an error, not a silent success.
	err := RemoveEnvDeclaration(dir, func(func() error) error { return nil }, envschema.SectionEnvVar, "API_TOKEN")
	require.Error(t, err)
	assert.Contains(t, loadSchema(t, path).EnvVars, "API_TOKEN")

	// And the wrapper's own error is the result.
	sentinel := errors.New("locked out")
	err = SetEnvDeclaration(dir, func(func() error) error { return sentinel }, envschema.SectionEnvVar, "X", nil)
	require.ErrorIs(t, err, sentinel)
}
