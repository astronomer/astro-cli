package localdocker

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

func goldenInput() composeInput {
	return composeInput{
		ProjectName:   "astro-demo-abc123",
		Image:         "astrocrpublic.azurecr.io/runtime:3.1-2",
		PostgresImage: postgresImage,
		APIServerPort: 8081,
		PostgresPort:  15432,
		Env: airflowEnv("astro-demo-abc123", 8081, map[string]string{
			"AIRFLOW__CORE__LOAD_EXAMPLES": "True", // user override wins
			"MY_SECRET":                    "it's quoted",
		}),
		Mounts: []mount{
			{Host: "/home/me/demo/dags", Container: "/usr/local/airflow/dags"},
			{Host: "/home/me/demo/include", Container: "/usr/local/airflow/include"},
		},
	}
}

func TestGenerateComposeGolden(t *testing.T) {
	got, err := generateCompose(goldenInput())
	require.NoError(t, err)

	golden := filepath.Join("testdata", "compose_golden.yaml")
	if *updateGolden {
		require.NoError(t, os.WriteFile(golden, []byte(got), 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), got, "run `go test ./internal/localdocker -update` after intentional template changes")
}

// The generated file must be valid YAML whose values land where compose
// reads them, including with hostile env values.
func TestGenerateComposeParses(t *testing.T) {
	got, err := generateCompose(goldenInput())
	require.NoError(t, err)

	var doc struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			Ports       []string          `yaml:"ports"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []string          `yaml:"volumes"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got), &doc))

	api, ok := doc.Services["api-server"]
	require.True(t, ok)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1-2", api.Image)
	assert.Equal(t, []string{"127.0.0.1:8081:8080"}, api.Ports)
	assert.Equal(t, "http://localhost:8081", api.Environment["AIRFLOW__API__BASE_URL"], "the host port must reach the env, not a hardcoded 8080")
	assert.Equal(t, "True", api.Environment["AIRFLOW__CORE__LOAD_EXAMPLES"], "plan env overrides the baseline")
	assert.Equal(t, "it's quoted", api.Environment["MY_SECRET"])
	assert.Contains(t, api.Volumes, "/home/me/demo/dags:/usr/local/airflow/dags:z")

	pg, ok := doc.Services["postgres"]
	require.True(t, ok)
	assert.Equal(t, []string{"127.0.0.1:15432:5432"}, pg.Ports)
	assert.Contains(t, pg.Volumes, "postgres_data:/var/lib/postgresql/data")

	for _, svc := range []string{"db-migration", "scheduler", "dag-processor", "triggerer"} {
		assert.Contains(t, doc.Services, svc)
	}
	assert.Empty(t, doc.Services["db-migration"].Volumes, "one-shot migration needs no project mounts")
}

func TestGenerateComposeNoMounts(t *testing.T) {
	in := goldenInput()
	in.Mounts = nil
	got, err := generateCompose(in)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(got), &doc), "an empty mount list must still render valid YAML")
}

func TestComposeProjectName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Demo")
	require.NoError(t, os.Mkdir(dir, 0o755))
	name, err := composeProjectName(dir)
	require.NoError(t, err)
	assert.Regexp(t, `^astro-my-demo-[0-9a-f]{6}$`, name)

	other := filepath.Join(t.TempDir(), "My Demo")
	require.NoError(t, os.Mkdir(other, 0o755))
	otherName, err := composeProjectName(other)
	require.NoError(t, err)
	assert.NotEqual(t, name, otherName, "same directory name in different paths must not collide")
}

func TestProjectMounts(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "dags"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(project, "tests"), 0o755))
	// A file with a mountable name is not a directory mount.
	require.NoError(t, os.WriteFile(filepath.Join(project, "plugins"), []byte("x"), 0o644))

	got := projectMounts(project)
	assert.Equal(t, []mount{
		{Host: filepath.Join(project, "dags"), Container: "/usr/local/airflow/dags"},
		{Host: filepath.Join(project, "tests"), Container: "/usr/local/airflow/tests"},
	}, got)
}
