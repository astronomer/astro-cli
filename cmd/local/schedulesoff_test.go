package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

type startingRuntime struct{ attachableRuntime }

func (startingRuntime) Start(context.Context, localrt.Plan, localrt.Callbacks) (localrt.Airflow, error) {
	return fakeAirflow{}, nil
}

func TestSchedulesOn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plan  localrt.Plan
		shell string
		mode  localrt.Mode
		want  bool
	}{
		{name: "unset", mode: localrt.ModeStandalone},
		{name: "env true", plan: localrt.Plan{Env: map[string]string{useJobSchedule: "True"}}, mode: localrt.ModeDocker, want: true},
		{name: "env false", plan: localrt.Plan{Env: map[string]string{useJobSchedule: "False"}}, mode: localrt.ModeStandalone},
		{name: "airflow spelling of true", plan: localrt.Plan{Env: map[string]string{useJobSchedule: " t "}}, mode: localrt.ModeStandalone, want: true},
		{name: "secret in standalone", plan: localrt.Plan{SecretEnv: map[string]string{useJobSchedule: "1"}}, mode: localrt.ModeStandalone, want: true},
		{name: "secret in docker", plan: localrt.Plan{SecretEnv: map[string]string{useJobSchedule: "True"}}, mode: localrt.ModeDocker, want: true},
		{name: "shell in standalone", shell: "true", mode: localrt.ModeStandalone, want: true},
		{name: "shell ignored in docker", shell: "true", mode: localrt.ModeDocker},
		{name: "env over shell", plan: localrt.Plan{Env: map[string]string{useJobSchedule: "False"}}, shell: "true", mode: localrt.ModeStandalone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateEnvSources(t, useJobSchedule)
			if tc.shell != "" {
				t.Setenv(useJobSchedule, tc.shell)
			}
			if got := schedulesOn(tc.plan, tc.mode); got != tc.want {
				t.Errorf("schedulesOn = %v, want %v", got, tc.want)
			}
		})
	}
}

// Start and restart both say schedules are off, in text mode only, and say
// nothing once the project's .env turns them on.
func TestStartNotesSchedulesOff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		dotenv string
		want   bool
	}{
		{name: "start", args: []string{"local", "start"}, want: true},
		{name: "restart", args: []string{"local", "restart"}, want: true},
		{name: "json", args: []string{"local", "start", "--output", "json"}},
		{name: "turned on", args: []string{"local", "start"}, dotenv: useJobSchedule + "=True\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := testDeps(t)
			stderr := &bytes.Buffer{}
			d.Stderr = stderr
			d.Runtime = startingRuntime{}
			dir := t.TempDir()
			m := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(m), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.dotenv != "" {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.dotenv), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			isolateEnvSources(t, useJobSchedule)

			if err := execute(t, d, tc.args...); err != nil {
				t.Fatal(err)
			}
			if got := bytes.Contains(stderr.Bytes(), []byte(schedulesOffNote)); got != tc.want {
				t.Errorf("note printed = %v, want %v; stderr was %q", got, tc.want, stderr.String())
			}
		})
	}
}
