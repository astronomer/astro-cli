//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// A stop with nothing running succeeds, as `docker compose stop` does, so a
// script can stop a project without first asking whether it is up.
func TestStopWithNothingRunningSucceeds(t *testing.T) {
	tier(t, 0)

	p := newProject(t)
	p.run("init", "--name", "demo").requireSuccess()

	r := p.run("local", "stop").requireSuccess()
	if !strings.Contains(r.Stdout, "already stopped") {
		t.Errorf("stop did not say the project was already stopped\n%s", r.output())
	}
}

// `astro local run` needs no running Airflow in standalone mode: the command
// runs in the project's venv, under the project's .env, as it would under a
// start.
func TestRunNeedsNoRunningAirflow(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	write(t, filepath.Join(p.Dir, ".env"), "ASTRO_E2E_DOTENV=sandbox\n")

	r := p.run("local", "run", "python", "-c", "import os, airflow; print(os.environ['ASTRO_E2E_DOTENV'])").requireSuccess()
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if got := lines[len(lines)-1]; got != "sandbox" {
		t.Errorf("the command saw ASTRO_E2E_DOTENV=%q, want sandbox\n%s", got, r.output())
	}
}
