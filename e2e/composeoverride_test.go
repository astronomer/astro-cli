//go:build e2e && !windows

package e2e

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Docker mode merges a project's docker-compose.override.yml, as v1 did: a
// service it adds runs beside Airflow on the network the Airflow services
// share, a service it extends gets the extra setting, and stop and reset take
// the added service and its volume down with Airflow's.
//
// The sidecar runs the postgres image the stack already pulls, so the case
// costs no download and leaves no image of its own behind.
func TestDockerModeMergesTheComposeOverride(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "dkoverride")
	needsDocker(t, p)
	write(t, filepath.Join(p.Dir, "docker-compose.override.yml"), `services:
  sidecar:
    image: docker.io/postgres:12.6
    entrypoint: ["sleep", "infinity"]
    networks: [airflow]
    volumes:
      - sidecar_data:/data
  scheduler:
    environment:
      OVERRIDE_MARKER: ${OVERRIDE_MARKER:-from-override}
volumes:
  sidecar_data:
`)

	p.runSlow("local", "start", "--docker").requireSuccess()
	project := startedDockerStack(t, p)

	sidecar := containersMatching(t, project, "--filter", "status=running", "--filter", "label=com.docker.compose.service=sidecar")
	if len(sidecar) != 1 {
		t.Fatalf("want the override's sidecar running, found %v", sidecar)
	}
	networks, err := dockerLines(t.Context(), "inspect", sidecar[0],
		"--format", "{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}")
	if err != nil {
		t.Fatalf("inspecting %s: %v", sidecar[0], err)
	}
	if !slices.Contains(networks, project+"_airflow") {
		t.Errorf("the sidecar is not on the Airflow services' network %s_airflow: %v", project, networks)
	}

	scheduler := containersMatching(t, project, "--filter", "label=com.docker.compose.service=scheduler")
	if len(scheduler) != 1 {
		t.Fatalf("want one scheduler container, found %v", scheduler)
	}
	env, err := dockerLines(t.Context(), "inspect", scheduler[0], "--format", "{{range .Config.Env}}{{println .}}{{end}}")
	if err != nil {
		t.Fatalf("inspecting %s: %v", scheduler[0], err)
	}
	if !slices.Contains(env, "OVERRIDE_MARKER=from-override") {
		t.Errorf("the override did not extend the scheduler's environment: %v", env)
	}

	sidecarVolume := project + "_sidecar_data"
	if vols := volumesFor(t, project); !slices.Contains(vols, sidecarVolume) {
		t.Fatalf("the override's volume %s was not created: %v", sidecarVolume, vols)
	}

	p.runSlow("local", "stop").requireSuccess()
	waitFor(t, "the stopped project's containers, the sidecar's included, to go", func() bool {
		return len(containersFor(t, project)) == 0
	})
	if !slices.Contains(volumesFor(t, project), sidecarVolume) {
		t.Errorf("a plain stop removed the override's volume; it keeps volumes, as it keeps the database")
	}

	p.runSlow("local", "reset", "--yes").requireSuccess()
	if left := volumesFor(t, project); len(left) != 0 {
		t.Errorf("reset left %d volume(s) behind: %v", len(left), left)
	}
	left, err := dockerLines(t.Context(), "network", "ls",
		"--filter", "label=com.docker.compose.project="+project, "--format", "{{.Name}}")
	if err != nil {
		t.Fatalf("listing networks for %s: %v", project, err)
	}
	if len(left) != 0 {
		t.Errorf("reset left network(s) behind: %s", strings.Join(left, ", "))
	}
}
