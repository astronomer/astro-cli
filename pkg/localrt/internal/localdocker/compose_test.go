package localdocker

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

func goldenInput() composeInput {
	return goldenInputFor(airflow3, "astrocrpublic.azurecr.io/runtime:3.1-2")
}

func goldenInputFor(major, image string) composeInput {
	return composeInput{
		ProjectName:   "astro-demo-abc123",
		Image:         image,
		PostgresImage: postgresImage,
		WebPort:       8081,
		PostgresPort:  15432,
		Env: airflowEnv("astro-demo-abc123", 8081, major, map[string]string{
			"AIRFLOW__CORE__LOAD_EXAMPLES": "True", // user override wins
			"MY_SECRET":                    "it's quoted",
		}, nil),
		PassEnv: []string{"SHELL_ONLY_TOKEN"},
		Mounts: []mount{
			{Host: "/home/me/demo/dags", Container: "/usr/local/airflow/dags"},
			{Host: "/home/me/demo/include", Container: "/usr/local/airflow/include"},
		},
		DBCommand: dbCommand(major),
		Services:  airflowServices(major),
	}
}

func TestGenerateComposeGolden(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  composeInput
		golden string
	}{
		{"airflow3", goldenInput(), "compose_golden.yaml"},
		{"airflow2", goldenInputFor(airflow2, "quay.io/astronomer/astro-runtime:12.9.0"), "compose_golden_af2.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := generateCompose(tc.input)
			require.NoError(t, err)

			golden := filepath.Join("testdata", tc.golden)
			if *updateGolden {
				require.NoError(t, os.WriteFile(golden, []byte(got), 0o644))
			}
			want, err := os.ReadFile(golden)
			require.NoError(t, err)
			assert.Equal(t, string(want), got, "run `go test ./pkg/localrt/internal/localdocker -update` after intentional template changes")
		})
	}
}

// Airflow 2 runs the components that release has, authenticates through
// Flask-AppBuilder, and gets none of Airflow 3's api-server settings.
func TestGenerateComposeAirflow2(t *testing.T) {
	got, err := generateCompose(goldenInputFor(airflow2, "quay.io/astronomer/astro-runtime:12.9.0"))
	require.NoError(t, err)

	var doc struct {
		Services map[string]struct {
			Command     []string          `yaml:"command"`
			Ports       []string          `yaml:"ports"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []string          `yaml:"volumes"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got), &doc))

	assert.NotContains(t, doc.Services, "api-server")
	assert.NotContains(t, doc.Services, "dag-processor")
	web, ok := doc.Services["webserver"]
	require.True(t, ok)
	assert.Equal(t, []string{"airflow", "webserver"}, web.Command)
	assert.Equal(t, []string{"127.0.0.1:8081:8080"}, web.Ports)
	assert.Contains(t, web.Volumes, "/home/me/demo/dags:/usr/local/airflow/dags:z")

	assert.Equal(t, "http://localhost:8081", web.Environment["AIRFLOW__WEBSERVER__BASE_URL"])
	assert.Contains(t, web.Environment["AIRFLOW__API__AUTH_BACKENDS"], "basic_auth")
	for _, af3Only := range []string{
		"AIRFLOW__API__BASE_URL",
		"AIRFLOW__CORE__AUTH_MANAGER",
		"AIRFLOW__CORE__EXECUTION_API_SERVER_URL",
		"AIRFLOW__SCHEDULER__STANDALONE_DAG_PROCESSOR",
	} {
		assert.NotContains(t, web.Environment, af3Only, "an Airflow 3 setting must not reach an Airflow 2 container")
	}

	// The migration also seeds the admin account every caller authenticates with,
	// which now comes from airflowrt (the one package the macOS standalone shim
	// can also reach). Asserted as a LITERAL on purpose: building the expected
	// string from the same constants dbCommand uses is a tautology that passes
	// whatever the value is, which is what the first version of this did — and
	// the only remaining signal was the golden file, whose failure message tells
	// you to regenerate it.
	db := doc.Services["db-migration"].Command
	require.Len(t, db, 3)
	assert.Equal(t, []string{"bash", "-c"}, db[:2])
	assert.Contains(t, db[2], "airflow db migrate")
	// Trailing " --email" on purpose: "admin9" contains "admin", so an assertion
	// that stops at the password passes for a changed value. Verified by mutation.
	assert.Contains(t, db[2], "--username admin --password admin --email")
}

func TestHealthURLs(t *testing.T) {
	assert.Equal(t, []string{"http://localhost:8081/api/v2/monitor/health"}, healthURLs(8081, airflow3))
	// Older Astronomer Airflow 2 runtimes serve /health at the root instead.
	assert.Equal(t, []string{
		"http://localhost:8081/api/v2/monitor/health",
		"http://localhost:8081/health",
	}, healthURLs(8081, airflow2))
}

func TestAirflowMajor(t *testing.T) {
	assert.Equal(t, "2", airflowMajor("2.11.2"))
	assert.Equal(t, "3", airflowMajor("3.1-2"))
	assert.Equal(t, "", airflowMajor(""))
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

// A pass-through name renders as a null-valued environment entry — compose's
// "resolve from the invoking environment" form — with the value nowhere in
// the file.
func TestGenerateComposePassEnv(t *testing.T) {
	got, err := generateCompose(goldenInput())
	require.NoError(t, err)

	var doc struct {
		Services map[string]struct {
			Environment map[string]*string `yaml:"environment"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got), &doc))
	env := doc.Services["scheduler"].Environment
	val, present := env["SHELL_ONLY_TOKEN"]
	require.True(t, present, "the pass-through name must appear in the common env")
	assert.Nil(t, val, "the pass-through entry must carry no value")
}

// passEnv drops names the value-carrying env already holds: a duplicate YAML
// key would be invalid, and the on-disk value already reaches the container.
func TestPassEnvDedupes(t *testing.T) {
	env := []envVar{{Name: "MY_SECRET", Value: "'x'"}}
	got := passEnv([]string{"MY_SECRET", "ZED", "ALPHA"}, nil, env)
	assert.Equal(t, []string{"ALPHA", "ZED"}, got)
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

// --- SecretEnv: declared in the file, never recorded in it ---

// The property the field exists for, across both halves that produce it: a
// consumer whose values come from a keyring must be able to deliver them without
// decrypting secrets onto disk as a side effect of starting Airflow. Absent from
// what the file records, present in what it declares.
//
// Deliberately asserts both halves rather than just the absence. Absence alone
// passes trivially — airflowEnv never had the key to begin with, since it builds
// from planEnv — so a test that only checked the file would still pass with the
// declaration broken, and the variable would silently never reach the container.
// The withholding logic in airflowEnv is the guard for a caller that puts a key in
// BOTH maps, which TestSecretEnvWinsOverEnv covers.
func TestSecretEnvIsDeclaredButNotRecorded(t *testing.T) {
	const secret = "AIRFLOW_CONN_DB"
	secretEnv := map[string]string{secret: "postgres://user:pw@host/db"}
	env := airflowEnv("proj", 8080, "3", map[string]string{"PLAIN": "written"}, secretEnv)

	var sawPlain bool
	for _, e := range env {
		if e.Name == secret {
			t.Fatalf("a secret reached the compose file: %s=%s", e.Name, e.Value)
		}
		if strings.Contains(e.Value, "pw@host") {
			t.Fatalf("a secret value reached the compose file under %s", e.Name)
		}
		if e.Name == "PLAIN" {
			sawPlain = true
		}
	}
	if !sawPlain {
		t.Error("a non-secret value must still be written; only SecretEnv is withheld")
	}

	declared := passEnv(nil, secretEnv, env)
	if len(declared) != 1 || declared[0] != secret {
		t.Fatalf("passEnv = %v, want %s declared so compose has an entry to resolve", declared, secret)
	}
	if got := secretEnviron(secretEnv); len(got) != 1 || got[0] != secret+"=postgres://user:pw@host/db" {
		t.Fatalf("secretEnviron = %v, want the value handed to the compose process instead", got)
	}
}

// DAGs start unpaused with the scheduler's own runs off, in both generations,
// and a project that sets either key, plainly or as a secret, keeps its own
// value.
func TestAirflowEnvRunsDAGsOnlyWhenTriggered(t *testing.T) {
	const paused, schedule = "AIRFLOW__CORE__DAGS_ARE_PAUSED_AT_CREATION", "AIRFLOW__SCHEDULER__USE_JOB_SCHEDULE"
	value := func(env []envVar, key string) string {
		for _, e := range env {
			if e.Name == key {
				return e.Value
			}
		}
		return ""
	}
	for _, major := range []string{airflow2, airflow3} {
		env := airflowEnv("proj", 8080, major, nil, nil)
		assert.Equal(t, "'False'", value(env, paused), major)
		assert.Equal(t, "'False'", value(env, schedule), major)

		env = airflowEnv("proj", 8080, major, map[string]string{paused: "True", schedule: "True"}, nil)
		assert.Equal(t, "'True'", value(env, paused), major)
		assert.Equal(t, "'True'", value(env, schedule), major)

		secret := map[string]string{schedule: "True"}
		env = airflowEnv("proj", 8080, major, nil, secret)
		assert.Empty(t, value(env, schedule), "a secret value must replace the default, not lose to it")
		assert.Contains(t, passEnv(nil, secret, env), schedule)
	}
}

// A caller that puts the same key in both maps is contradicting itself. Secret
// wins, because the other reading writes a secret to disk.
func TestSecretEnvWinsOverEnv(t *testing.T) {
	const key = "AIRFLOW_VAR_TOKEN"
	env := airflowEnv("proj", 8080, "3",
		map[string]string{key: "from-env"},
		map[string]string{key: "from-secret"})

	for _, e := range env {
		if e.Name == key {
			t.Fatalf("%s was written as %s; a key in both maps must be treated as secret", key, e.Value)
		}
	}
	if got := passEnv(nil, map[string]string{key: "from-secret"}, env); len(got) != 1 || got[0] != key {
		t.Fatalf("passEnv = %v, want %s declared", got, key)
	}
}

// PassthroughEnv and SecretEnv both render as a valueless entry, so a key in both
// lists must not be declared twice — duplicate keys in a YAML mapping are invalid.
func TestPassEnvDoesNotDuplicateAcrossBothSources(t *testing.T) {
	got := passEnv([]string{"SHARED"}, map[string]string{"SHARED": "v"}, nil)
	// Identity, not just count: a result of length one holding the wrong key
	// satisfies a length check while declaring a variable nobody asked for.
	assert.Equal(t, []string{"SHARED"}, got)
}

// The values have to reach the compose process, since that is what resolves the
// declarations at container-creation time.
func TestSecretEnvironRendersSortedKeyValues(t *testing.T) {
	got := secretEnviron(map[string]string{"B_KEY": "2", "A_KEY": "1"})
	want := []string{"A_KEY=1", "B_KEY=2"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("secretEnviron = %v, want %v", got, want)
	}
	if secretEnviron(nil) != nil {
		t.Error("no secrets means no extra environment, not an empty entry")
	}
}
