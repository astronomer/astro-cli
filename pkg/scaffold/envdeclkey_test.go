package scaffold

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// EnvDeclarationKey names the key the writer would edit, found in the writer's
// own order: as given, then by case for a variable or connection, then folded.
func TestEnvDeclarationKey(t *testing.T) {
	dir, _ := writeEditFixture(t, envFixture+"AIRFLOW_VAR_ZONE = {}\n", 0o644)

	for _, tc := range []struct {
		section      envschema.Section
		name, want   string
		wantDeclared bool
	}{
		{envschema.SectionConnection, "db_main", "DB_Main", true},
		{envschema.SectionConnection, "AIRFLOW_CONN_DB_MAIN", "DB_Main", true},
		{envschema.SectionAirflowVariable, "REGION", "region", true},
		{envschema.SectionAirflowVariable, "AIRFLOW_VAR_ZONE", "AIRFLOW_VAR_ZONE", true},
		{envschema.SectionAirflowVariable, "AIRFLOW_VAR_NEW", "new", false},
		{envschema.SectionEnvVar, "LOG_LEVEL", "LOG_LEVEL", true},
		// Env var names are case-sensitive, so another case is another name.
		{envschema.SectionEnvVar, "log_level", "log_level", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, ok, err := EnvDeclarationKey(dir, tc.section, tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.want, key)
			assert.Equal(t, tc.wantDeclared, ok)
		})
	}

	_, _, err := EnvDeclarationKey(t.TempDir(), envschema.SectionEnvVar, "X")
	assert.True(t, errors.Is(err, manifest.ErrNotFound), "err = %v", err)
}
