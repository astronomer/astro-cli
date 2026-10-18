package localdocker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func writeOverride(t *testing.T, projectPath, content string) string {
	t.Helper()
	path := filepath.Join(projectPath, rt.ComposeOverrideFile)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestStartMergesTheProjectsComposeOverride(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	override := writeOverride(t, p.ProjectPath, "services: {}\n")

	af, name := startProject(t, e, p)
	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	generated := filepath.Join(stateDir, composeFileName)
	assert.Equal(t, fmt.Sprintf("docker compose --file %s --file %s --project-directory %s --project-name %s up --detach --quiet-pull",
		generated, override, p.ProjectPath, name), cmd.calls[len(cmd.calls)-1],
		"the override goes after the generated file, so compose lets it extend and replace what that file sets")

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{Clean: true}))
	assert.Contains(t, cmd.calls, "docker compose --project-name "+name+" down --timeout 10 --volumes --remove-orphans",
		"down works from the project label, which the override's containers carry too")
}

func TestRunInImageMergesTheProjectsComposeOverride(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	project := t.TempDir()
	composePath := withComposeFile(t, project)
	override := writeOverride(t, project, "services: {}\n")

	require.NoError(t, e.RunInImage(context.Background(), project, rt.ImageRun{Argv: []string{"true"}}))
	require.Len(t, cmd.calls, 1)
	assert.Contains(t, cmd.calls[0], "--file "+composePath+" --file "+override+" --project-directory "+project)
}

func TestComposeFilesWithoutAnOverride(t *testing.T) {
	assert.Equal(t, []string{"/state/docker-compose.yaml"}, composeFiles("/state/docker-compose.yaml", t.TempDir()))
}

// mergedService is the part of `docker compose config --format json` these
// tests read.
type mergedService struct {
	Image       string            `json:"image"`
	Environment map[string]string `json:"environment"`
	Networks    map[string]any    `json:"networks"`
	Volumes     []struct {
		Type   string `json:"type"`
		Source string `json:"source"`
		Target string `json:"target"`
	} `json:"volumes"`
	Deploy struct {
		Replicas *int `json:"replicas"`
	} `json:"deploy"`
}

type mergedProject struct {
	Services map[string]mergedService `json:"services"`
	Networks map[string]struct {
		Name string `json:"name"`
	} `json:"networks"`
	Volumes map[string]struct {
		Name string `json:"name"`
	} `json:"volumes"`
}

// mergeWithCompose renders the compose file Start would write for projectPath
// and asks compose itself what the project becomes once the override is merged
// over it: the same files, directory and name the up is given.
func mergeWithCompose(t *testing.T, projectPath, major string, env []string) mergedProject {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the merge is compose's own; its Windows path handling is not what this checks")
	}
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skipf("docker compose is not available: %v", err)
	}
	name, err := composeProjectName(projectPath)
	require.NoError(t, err)
	in := goldenInputFor(major, "astro-local/test:latest")
	in.ProjectName = name
	in.Mounts = projectMounts(projectPath)
	generated, err := generateCompose(in)
	require.NoError(t, err)
	generatedPath := filepath.Join(t.TempDir(), composeFileName)
	require.NoError(t, os.WriteFile(generatedPath, []byte(generated), 0o600))

	line := composeLine{files: composeFiles(generatedPath, projectPath), name: name, projectDir: projectPath}
	c := exec.Command("docker", line.argv("config", "--format", "json")...)
	c.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	require.NoError(t, err, "compose config: %s", stderr.String())
	var merged mergedProject
	require.NoError(t, json.Unmarshal(out, &merged))
	return merged
}

// An override written against v1 keeps working: it adds a service to the
// network the Airflow services share, and extends an Airflow service by the
// name v1 gave it, with ${VAR} resolved from the project's .env and the shell
// as `docker compose` resolves it.
func TestComposeOverrideMergesOverTheGeneratedProject(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "dags"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".env"), []byte("SIDECAR_REPLICAS=1\nFROM_DOTENV=dotenv\n"), 0o600))
	writeOverride(t, project, `services:
  sidecar:
    image: busybox
    command: ["sleep", "infinity"]
    networks: [airflow]
    deploy:
      replicas: ${SIDECAR_REPLICAS:-0}
    volumes:
      - ./sidecar:/data
      - sidecar_cache:/cache
  scheduler:
    environment:
      EXTRA_SETTING: ${FROM_DOTENV}
      FROM_SHELL: ${OVERRIDE_TEST_SHELL}
    volumes:
      - ./extra:/usr/local/airflow/extra
volumes:
  sidecar_cache:
`)

	merged := mergeWithCompose(t, project, airflow3, []string{"OVERRIDE_TEST_SHELL=shell"})

	sidecar, ok := merged.Services["sidecar"]
	require.True(t, ok, "the override's service is part of the project: %v", merged.Services)
	assert.Equal(t, "busybox", sidecar.Image)
	assert.Contains(t, sidecar.Networks, "airflow", "the sidecar joins the network the Airflow services use")
	require.NotNil(t, sidecar.Deploy.Replicas)
	assert.Equal(t, 1, *sidecar.Deploy.Replicas, "the project's .env resolves the override's variables")
	require.Len(t, sidecar.Volumes, 2)
	assert.Equal(t, filepath.Join(project, "sidecar"), sidecar.Volumes[0].Source,
		"a relative path resolves against the project, where the override lives")

	name, err := composeProjectName(project)
	require.NoError(t, err)
	assert.Equal(t, name+"_airflow", merged.Networks["airflow"].Name)
	assert.Equal(t, name+"_sidecar_cache", merged.Volumes["sidecar_cache"].Name,
		"the override's volume is the project's, so a --clean stop removes it with the database")

	scheduler := merged.Services["scheduler"]
	assert.Equal(t, "dotenv", scheduler.Environment["EXTRA_SETTING"])
	assert.Equal(t, "shell", scheduler.Environment["FROM_SHELL"])
	assert.Equal(t, "LocalExecutor", scheduler.Environment["AIRFLOW__CORE__EXECUTOR"],
		"extending the environment keeps what the generated file sets")
	var targets []string
	for _, v := range scheduler.Volumes {
		targets = append(targets, v.Target)
	}
	assert.ElementsMatch(t, []string{"/usr/local/airflow/dags", "/usr/local/airflow/extra"}, targets,
		"extending the volumes keeps the project mounts")

	_, extended := merged.Services["api-server"].Environment["EXTRA_SETTING"]
	assert.False(t, extended, "only the service the override names changes")
}

// The Airflow 2 service set carries v1's names too, so an override extending
// the webserver finds it.
func TestComposeOverrideExtendsTheAirflow2Webserver(t *testing.T) {
	project := t.TempDir()
	writeOverride(t, project, `services:
  webserver:
    environment:
      EXTRA_SETTING: "on"
`)
	merged := mergeWithCompose(t, project, airflow2, nil)
	assert.Equal(t, "on", merged.Services["webserver"].Environment["EXTRA_SETTING"])
}
