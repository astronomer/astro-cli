//go:build e2e

package e2e

import (
	"os"
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

// `astro local run` builds the venv when nothing is running, as uv run does:
// a fresh astro init project with no .venv, no uv sync and nothing started
// still runs a command that imports Airflow. The build's progress is on
// stderr; stdout is the command's alone. --no-sync keeps the old refusal.
func TestRunBuildsTheEnvironmentWhenNothingIsRunning(t *testing.T) {
	tier(t, 1)
	needsUV(t)

	p := newProject(t)
	p.run("init", "--name", "tier1").requireSuccess()
	venv := filepath.Join(p.Dir, ".venv", "bin", "python")

	r := p.run("local", "run", "--no-sync", "python", "-c", "print(1)").requireFailure()
	if !strings.Contains(r.output(), "no environment yet") {
		t.Errorf("--no-sync on a project with no venv did not refuse\n%s", r.output())
	}
	if _, err := os.Stat(venv); err == nil {
		t.Fatal("--no-sync built a venv")
	}

	r = p.runSlow("local", "run", "python", "-c", "import airflow; print('airflow-ok')").requireSuccess()
	if got := strings.TrimSpace(r.Stdout); got != "airflow-ok" {
		t.Errorf("stdout = %q, want only the command's output\n%s", r.Stdout, r.output())
	}
	if !strings.Contains(r.Stderr, "Building this project's environment") {
		t.Errorf("stderr does not report the build\n%s", r.output())
	}
	if _, err := os.Stat(venv); err != nil {
		t.Errorf("no venv after the run: %v", err)
	}

	// Up to date: the second run is quiet.
	r = p.run("local", "run", "python", "-c", "print('again')").requireSuccess()
	if strings.TrimSpace(r.Stdout) != "again" || strings.TrimSpace(r.Stderr) != "" {
		t.Errorf("a current venv should run silently\n%s", r.output())
	}
}
