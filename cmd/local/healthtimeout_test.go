package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// What the environment variable is read as.
//
// A unit test because every other test of this feature needs a real Airflow or
// a real Docker, which puts them in the nightly run — so a wrong variable name,
// a dropped guard or a parse that stopped working would reach trunk and wait a
// day to be caught. This runs on every pull request and costs nothing.
func TestHealthTimeoutReadsTheEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  string
		want time.Duration
	}{
		{name: "unset means the engine default", set: "", want: 0},
		{name: "a duration", set: "10m", want: 10 * time.Minute},
		{name: "a short one", set: "500ms", want: 500 * time.Millisecond},
		{
			// The most natural spelling, and the one houston.dial_timeout
			// uses. time.ParseDuration refuses it for want of a unit, and a
			// start is a bad place to fail over a tuning knob — so the default
			// stands and the warning below is what tells them.
			name: "bare seconds, which is not a duration",
			set:  "600",
			want: 0,
		},
		{name: "nonsense", set: "soon", want: 0},
		{name: "zero is not a wait", set: "0s", want: 0},
		{name: "negative is not a wait", set: "-1s", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(healthTimeoutEnv, tc.set)
			if got := healthTimeout(); got != tc.want {
				t.Errorf("healthTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}

// And the value reaches the runtime the CLI builds.
//
// The composition root is where this feature is wired, and wiring is what
// silently does nothing: the engines keep their defaults and every start looks
// normal until somebody measures one.
func TestTheRuntimeIsBuiltWithTheConfiguredTimeout(t *testing.T) {
	t.Setenv(healthTimeoutEnv, "7m")

	// Reading it back out of a built Runtime would mean exporting the field,
	// so this asserts the value the constructor is handed — which is the step
	// that was missing, and the one a refactor drops.
	cfg := localrt.Config{
		RoutesDir:     t.TempDir(),
		HealthTimeout: healthTimeout(),
	}
	if cfg.HealthTimeout != 7*time.Minute {
		t.Errorf("HealthTimeout = %v, want 7m", cfg.HealthTimeout)
	}
	if rt := localrt.New(cfg); rt == nil {
		t.Error("New() = nil for a config carrying a health timeout")
	}
}

// And a start that ran out of time says where the knob is.
//
// The engines do not name the variable: pkg/localrt and pkg/airflowrt are also
// Astro Desktop's, and desktop sets the duration from its own settings and
// never reads an environment variable. So the sentinel travels and the CLI
// adds the sentence — which only works while the CLI is actually asking.
func TestAHealthTimeoutIsToldHowToWaitLonger(t *testing.T) {
	t.Run("the advice names the variable", func(t *testing.T) {
		// As either engine reports it: the sentinel wrapped in that engine's
		// own words. Matching is on the sentinel, so the words are free to
		// differ and this test does not pin them.
		engineErr := fmt.Errorf("%w after 5m0s — Airflow may still be starting", localrt.ErrHealthTimeout)

		err := adviseHealthTimeout(engineErr)
		if !strings.Contains(err.Error(), healthTimeoutEnv) {
			t.Errorf("adviseHealthTimeout() = %q, which never names %s", err, healthTimeoutEnv)
		}
		if !strings.Contains(err.Error(), "5m0s") {
			t.Errorf("adviseHealthTimeout() = %q, which dropped what the engine said", err)
		}
		if !errors.Is(err, localrt.ErrHealthTimeout) {
			t.Error("the advice swallowed the sentinel, so nothing downstream can still recognize it")
		}
	})

	t.Run("anything else is passed through untouched", func(t *testing.T) {
		// Every other way a start fails — a bad Dockerfile, a port conflict,
		// an Airflow that died on import — reaches the same line. Advice about
		// a timeout on any of them is noise pointing the wrong way.
		other := errors.New("Airflow exited while starting; its output is in airflow.log")

		if err := adviseHealthTimeout(other); err.Error() != other.Error() {
			t.Errorf("adviseHealthTimeout() = %q, want it unchanged", err)
		}
	})

	t.Run("a start that worked stays worked", func(t *testing.T) {
		if err := adviseHealthTimeout(nil); err != nil {
			t.Errorf("adviseHealthTimeout(nil) = %v, want nil", err)
		}
	})
}

// timingOutRuntime is a runtime whose Airflow never answers. The error is
// shaped the way both engines shape it — the sentinel, wrapped in words of
// their own — because that shape is the contract adviseHealthTimeout matches
// on, not any particular sentence.
type timingOutRuntime struct{ fakeRuntime }

func (timingOutRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return nil, fmt.Errorf("%w after 5m0s — Airflow may still be starting", localrt.ErrHealthTimeout)
}

// restartTimingOut is the same, reachable through restart: runRestart reads the
// status and attaches before it starts anything, so without both of these the
// command bails long before the line under test.
type restartTimingOut struct{ timingOutRuntime }

func (restartTimingOut) Attach(string) (localrt.Airflow, error) { return fakeAirflow{}, nil }

func (restartTimingOut) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{State: localrt.StateRunning, Mode: localrt.ModeStandalone}, nil
}

// And the commands ask.
//
// adviseHealthTimeout being right counts for nothing if a start hands the
// engine's error straight past it, and that is the failure wiring has: nothing
// breaks, the advice simply stops appearing. Both commands that start Airflow
// reach their own copy of the line, so a test of one says nothing about the
// other — warnEnvValues learned that here already.
func TestAStartThatRunsOutOfTimeNamesTheVariable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime Runtime
		args    []string
	}{
		{name: "start", runtime: timingOutRuntime{}, args: []string{"local", "start"}},
		{name: "restart", runtime: restartTimingOut{}, args: []string{"local", "restart"}},
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
			if !strings.Contains(err.Error(), healthTimeoutEnv) {
				t.Errorf("astro %s reported %q, which never names %s",
					strings.Join(tc.args, " "), err, healthTimeoutEnv)
			}
		})
	}
}
