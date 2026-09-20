//go:build e2e && !windows

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Airflow 2 in Docker: the other generation, which is a different compose file
// and a different startup, not a different tag.
//
// T7.2, and the last of T7. Everything else in tier 3 runs the generation the
// scaffold pins, so the Airflow 2 arms of three separate decisions had no case
// at all: which components compose brings up, what the one-shot database
// service runs before them, and which API generation answers afterwards. Each
// is a branch in the runtime keyed on the major, and a project pinned to 2 is
// the only thing that reaches any of them.
func TestAirflow2ComesUpInDocker(t *testing.T) {
	tier(t, 3)
	p := airflow2DockerProject(t, "af2docker")
	needsDocker(t, p)

	p.runSlow("local", "start", "--docker").requireSuccess()
	if st := p.status(); st.State != "running" {
		t.Fatalf("state = %q, want running", st.State)
	}

	// The service set, which is where the generations differ most visibly:
	// Airflow 2 publishes a webserver and has no dag-processor, Airflow 3 has
	// an api-server and a dag-processor. Naming both absences as well as the
	// presence, because a compose file that brought up BOTH sets would satisfy
	// "the webserver is there" while being exactly wrong.
	project := composeProject(t, p)
	names := strings.Join(containersFor(t, project), " ")
	if !strings.Contains(names, "webserver") {
		t.Errorf("no webserver among the Airflow 2 containers: %s", names)
	}
	for _, three := range []string{"api-server", "dag-processor"} {
		if strings.Contains(names, three) {
			t.Errorf("an Airflow 3 component (%s) came up for an Airflow 2 project: %s", three, names)
		}
	}

	// The generation that actually answers, which is the claim the service set
	// only implies. A compose file can name whatever it likes; this is the
	// running API saying which one it is.
	var report struct {
		Version struct {
			Generation string `json:"generation"`
			Error      string `json:"error"`
		} `json:"version"`
	}
	p.runSlow("local", "af", "health", "--output", "json").
		requireSuccess().
		requireJSON(&report)
	if report.Version.Error != "" {
		t.Fatalf("health could not read the version: %s", report.Version.Error)
	}
	if report.Version.Generation != "2" {
		t.Errorf("api generation = %q, want 2 for a project pinned to Airflow 2", report.Version.Generation)
	}

	// And the admin account the Airflow 2 startup seeds.
	//
	// Airflow 2 is the only generation that has one: its one-shot database
	// service runs `airflow users create` after the migration and sync-perm
	//, Airflow 3 seeds nothing, and so nothing else in this suite
	// exercises that command at all.
	//
	// Two halves, because either alone is worthless. The CLI reading the API is
	// only evidence of an account if an anonymous caller would have been
	// refused — and on this runtime /api/v1/version answers 200 to anybody,
	// which is why the version read asserted above is NOT the proof and an
	// earlier draft of this case that leaned on it proved nothing. Measured,
	// not assumed.
	st := p.status()
	const protectedPath = "/api/v1/dags"
	anon := getURL(t, fmt.Sprintf("http://localhost:%d%s", st.Port, protectedPath))
	switch anon {
	case http.StatusUnauthorized, http.StatusForbidden:
		// Protected, so the read below has to have authenticated.
	case http.StatusOK:
		t.Fatalf("GET %s answered 200 without credentials, so nothing the CLI reads from this "+
			"API can show that the seeded admin account works", protectedPath)
	default:
		t.Fatalf("GET %s answered %d, which is neither a refusal nor a read; this case cannot "+
			"tell whether the admin account works", protectedPath, anon)
	}

	// The other half: the CLI reaches the same family, authenticating as the
	// account the database service created. Only that it succeeds — the DAGs
	// themselves are the scheduler's business and arrive when it has parsed
	// them, which is a different claim with different timing.
	p.runSlow("local", "af", "dags", "list", "--output", "json").requireSuccess()
}

// airflow2DockerProject scaffolds a docker project pinned to Airflow 2.
//
// dockerProject takes the scaffold's default, which is an Airflow 3 pin, and
// the pin is the only thing that selects any of the branches under test.
func airflow2DockerProject(t *testing.T, name string) *project {
	t.Helper()
	p := newNamedProject(t, name)
	p.run("init", "--name", name, "--airflow-version", "2").requireSuccess()
	t.Cleanup(func() { p.runSlow("local", "reset", "--yes").requireSuccess() })
	return p
}
