package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The engine reports the database and the CLI names the way out, for the reason
// TestAHealthTimeoutIsToldHowToWaitLonger gives: the engines are also
// Astro Desktop's, which resets a project from its own UI.
func TestADatabaseANewerAirflowUpgradedIsToldAboutReset(t *testing.T) {
	t.Run("the advice names reset", func(t *testing.T) {
		engineErr := fmt.Errorf("%w: it is at migration 1d6611b6ab7c", localrt.ErrDatabaseNewerThanAirflow)

		err := adviseDatabaseNewer(engineErr)
		if !strings.Contains(err.Error(), "astro local reset") {
			t.Errorf("adviseDatabaseNewer() = %q, which never names astro local reset", err)
		}
		// Both places the version can come from. A project with a declared
		// Dockerfile runs its base image whatever the pin says, so advice naming
		// only the pin sends that user to edit a file that changes nothing.
		for _, want := range []string{"pyproject.toml", "Dockerfile"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("adviseDatabaseNewer() = %q, which never names %s as where to go back to the newer Airflow", err, want)
			}
		}
		if !strings.Contains(err.Error(), "1d6611b6ab7c") {
			t.Errorf("adviseDatabaseNewer() = %q, which dropped what the engine said", err)
		}
		if !errors.Is(err, localrt.ErrDatabaseNewerThanAirflow) {
			t.Error("the advice swallowed the sentinel, so the json kind can no longer be read from it")
		}
		if got := ProblemKinds.Of(err); got != KindDatabaseNewerThanAirflow {
			t.Errorf("ProblemKinds.Of() = %q, want %q", got, KindDatabaseNewerThanAirflow)
		}
	})

	t.Run("anything else is passed through untouched", func(t *testing.T) {
		// Advice to wipe the database on a failure that has nothing to do with
		// it would be the most expensive wrong advice the CLI could give.
		other := errors.New("starting project containers: exit status 1")
		if err := adviseDatabaseNewer(other); err.Error() != other.Error() {
			t.Errorf("adviseDatabaseNewer() = %q, want it unchanged", err)
		}
		if err := adviseDatabaseNewer(nil); err != nil {
			t.Errorf("adviseDatabaseNewer(nil) = %v, want nil", err)
		}
	})
}

// newerDatabaseRuntime is a runtime whose Airflow refuses the project's
// database, shaped as the docker engine shapes it: the sentinel in its own words.
type newerDatabaseRuntime struct{ fakeRuntime }

func (newerDatabaseRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return nil, fmt.Errorf("%w: it is at migration 1d6611b6ab7c", localrt.ErrDatabaseNewerThanAirflow)
}

// restartNewerDatabase reaches the same failure through restart, which reads the
// status and attaches before it starts anything.
type restartNewerDatabase struct{ newerDatabaseRuntime }

func (restartNewerDatabase) Attach(string) (localrt.Airflow, error) { return fakeAirflow{}, nil }

func (restartNewerDatabase) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{State: localrt.StateRunning, Mode: localrt.ModeDocker}, nil
}

// And both commands that start Airflow ask.
func TestAStartOnANewerDatabaseNamesReset(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime Runtime
		args    []string
	}{
		{name: "start", runtime: newerDatabaseRuntime{}, args: []string{"local", "start"}},
		{name: "restart", runtime: restartNewerDatabase{}, args: []string{"local", "restart"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			d.Runtime = tc.runtime
			dir := t.TempDir()
			m := `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
`
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			isolateEnvSources(t)

			err := execute(t, d, tc.args...)
			if err == nil {
				t.Fatal("the runtime refused to start; the command must fail")
			}
			if !strings.Contains(err.Error(), "astro local reset") {
				t.Errorf("astro %s reported %q, which never names astro local reset",
					strings.Join(tc.args, " "), err)
			}
		})
	}
}
