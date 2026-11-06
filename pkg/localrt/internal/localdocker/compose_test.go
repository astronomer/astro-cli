package localdocker

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
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
	got := passEnv([]string{"MY_SECRET", "ZED", "ALPHA"}, env)
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

// --- SecretEnv: never in the file, delivered by the handoff ---

// The property the field exists for, across both halves that produce it: a
// consumer whose values come from a keyring must be able to deliver them without
// decrypting secrets onto disk as a side effect of starting Airflow. Absent from
// the file, entirely: not recorded, and not declared either, since a declared
// key's value is resolved into the container's configuration, which the daemon
// keeps on disk. Present in what the handoff delivers.
//
// Deliberately asserts both halves rather than just the absence. Absence alone
// passes trivially — airflowEnv never had the key to begin with, since it builds
// from planEnv — so a test that only checked the file would still pass with the
// delivery broken, and the variable would silently never reach the container.
// The withholding logic in airflowEnv is the guard for a caller that puts a key in
// BOTH maps, which TestSecretEnvWinsOverEnv covers.
func TestSecretEnvIsDeliveredButNotInTheFile(t *testing.T) {
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

	assert.Empty(t, passEnv(nil, env), "a secret must not be declared for compose to resolve into the container's configuration")
	assert.Equal(t, secretEnv, handoffEnv(secretEnv, env), "the handoff must carry the secret instead")
}

// A secret colliding with a setting the engine keeps is not applied in either
// mode. In docker mode that means leaving it out of the handoff too, or the
// loader would override the engine's value inside the container.
func TestHandoffEnvLeavesOutWhatTheEngineKeeps(t *testing.T) {
	secretEnv := map[string]string{
		"AIRFLOW__CORE__EXECUTOR": "CeleryExecutor", // the engine's
		"AIRFLOW_CONN_DB":         "postgres://u:pw@h/db",
	}
	env := airflowEnv("proj", 8080, "3", nil, secretEnv)
	assert.Equal(t, map[string]string{"AIRFLOW_CONN_DB": "postgres://u:pw@h/db"}, handoffEnv(secretEnv, env))
}

// With secret values, the file declares one secret read from the compose
// process's environment, and every container that runs Airflow names it, runs
// its command through the loader, and carries the start's revision. The file
// names the variable, never a value.
func TestGenerateComposeWithSecretHandoff(t *testing.T) {
	in := goldenInput()
	in.SecretEnv = newSecretEnvSpec("rev1")
	in.DBCommand = withSecretEnv(true, in.DBCommand)
	for i := range in.Services {
		in.Services[i].Command = withSecretEnv(true, in.Services[i].Command)
	}
	got, err := generateCompose(in)
	require.NoError(t, err)

	var doc struct {
		Secrets  map[string]map[string]string `yaml:"secrets"`
		Services map[string]struct {
			Command []string            `yaml:"command"`
			Labels  map[string]string   `yaml:"labels"`
			Secrets []map[string]string `yaml:"secrets"`
			Volumes []string            `yaml:"volumes"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got), &doc), got)

	assert.Equal(t, map[string]map[string]string{secretEnvName: {"environment": secretEnvPayloadVar}}, doc.Secrets,
		"one secret, read from the compose process's environment")

	for _, name := range []string{"db-migration", "scheduler", "api-server"} {
		svc := doc.Services[name]
		require.GreaterOrEqual(t, len(svc.Command), 5, name)
		assert.Equal(t, []string{"python3", "-c", secretEnvLoader, secretEnvFile, "required"}, svc.Command[:5],
			"%s must run its command through the loader, intact after YAML", name)
		assert.Equal(t, []map[string]string{{"source": secretEnvName, "target": secretEnvFile, "mode": secretEnvMode}}, svc.Secrets,
			"%s must take the secret at a path podman does not mount over, readable whoever it runs as", name)
		assert.Equal(t, "rev1", svc.Labels["io.astronomer.docker.secret-env-rev"],
			"%s must carry the start's revision, so a re-start recreates it onto the new values", name)
	}
	assert.Contains(t, doc.Services["scheduler"].Volumes, "/home/me/demo/dags:/usr/local/airflow/dags:z",
		"the project mounts must survive next to the secret")
	assert.Nil(t, doc.Services["postgres"].Secrets, "postgres has no use for the values")
	assert.NotContains(t, got, "pw@", "the file must never carry a value")
}

// Docker Compose from v2.6.1 takes a secret from the environment, including
// through `podman compose`'s banner; podman-compose and anything unrecognized
// do not.
func TestComposeTakesEnvSecrets(t *testing.T) {
	for out, want := range map[string]bool{
		"Docker Compose version v2.6.1":  true,
		"Docker Compose version v2.40.3": true,
		"Docker Compose version v5.5.0":  true,
		"Docker Compose version 2.29.7":  true,
		">>>> Executing external compose provider \"/usr/local/bin/docker-compose\". <<<<\n\nDocker Compose version v5.5.0": true,
		"Docker Compose version v2.6.0":                      false,
		"Docker Compose version v1.29.2":                     false,
		"podman version 5.8.2\npodman-compose version 1.6.0": false,
		"": false,
	} {
		assert.Equal(t, want, composeTakesEnvSecrets(out), "%q", out)
	}
}

// Only a service whose command the override sets is named: an override that
// touches something else leaves the loader in place.
func TestOverrideReplacedCommands(t *testing.T) {
	project := t.TempDir()
	assert.Nil(t, overrideReplacedCommands(project, []string{"scheduler"}), "no override, nothing replaced")

	override := "services:\n  scheduler:\n    command: [airflow, scheduler]\n  api-server:\n    environment:\n      X: y\n"
	require.NoError(t, os.WriteFile(filepath.Join(project, rt.ComposeOverrideFile), []byte(override), 0o600))
	assert.Equal(t, []string{"scheduler"},
		overrideReplacedCommands(project, []string{"db-migration", "scheduler", "api-server"}))
}

// Without secret values the file has no trace of the handoff, so a project that
// never uses one starts exactly as it did before it existed.
func TestGenerateComposeWithoutSecretsHasNoHandoff(t *testing.T) {
	got, err := generateCompose(goldenInput())
	require.NoError(t, err)
	assert.NotContains(t, got, "secrets:")
	assert.NotContains(t, got, secretEnvName)
	assert.NotContains(t, got, "python3")
}

// The loader is real Python, run here against a real interpreter when one is
// on PATH: it loads the file into the environment of the command it execs, a
// required run refuses to start without the file, and an optional run execs the
// command untouched.
func TestSecretEnvLoaderRunsUnderPython(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "env.json")
	value := `pa$$ 'w"o` + "\nrd"
	require.NoError(t, os.WriteFile(file, []byte(`{"HANDOFF_TEST":`+strconv.Quote(value)+`}`), 0o600))

	run := func(path, mode string) (string, error) {
		cmd := exec.Command(py, "-I", "-c", secretEnvLoader, path, mode, py, "-I", "-c", "import os;print(os.environ.get('HANDOFF_TEST','<unset>'),end='')")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run(file, "required")
	require.NoError(t, err, out)
	assert.Equal(t, value, out, "the value must arrive byte for byte")

	out, err = run(filepath.Join(dir, "missing.json"), "required")
	require.Error(t, err, "a required run must not start without its values")
	assert.Contains(t, out, "astro local start --docker")

	out, err = run(filepath.Join(dir, "missing.json"), "optional")
	require.NoError(t, err, out)
	assert.Equal(t, "<unset>", out)
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
		assert.Equal(t, secret, handoffEnv(secret, env))
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
	assert.Equal(t, map[string]string{key: "from-secret"}, handoffEnv(map[string]string{key: "from-secret"}, env))
}

// RunInImage's values have to reach its compose process, since that is what
// resolves its bare -e flags.
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
