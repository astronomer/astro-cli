package local

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// `astro run` is a tombstone: every old invocation fails as a usage error and
// names the replacement, with its old flags reaching the guidance.
func TestRunStubNamesTheReplacement(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"dag id", []string{"run", "my_dag"}, "astro local run airflow dags test my_dag"},
		{"old flags", []string{"run", "my_dag", "--execution-date", "2026-01-01", "--dag-file", "dags/a.py", "--no-cache"}, "astro local run airflow dags test my_dag"},
		{"bare", []string{"run"}, "astro local run airflow dags test <dag-id>"},
		{"flag first", []string{"run", "--verbose", "my_dag"}, "astro local run airflow dags test <dag-id>"},
		{"not an id", []string{"run", "a b;c"}, "astro local run airflow dags test <dag-id>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, out := testDeps(t)
			err := execute(t, d, tc.args...)
			if err == nil {
				t.Fatal("astro run must fail")
			}
			if !cliout.IsUsage(err) {
				t.Errorf("want a usage error (exit 2, as an unknown command was), got %T", err)
			}
			if !strings.Contains(err.Error(), "astro run was removed in Astro CLI v2") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not name %s:\n%s", tc.want, err)
			}
			if out.Len() != 0 {
				t.Errorf("text mode wrote to stdout: %q", out.String())
			}
		})
	}
}

// Under --output json the failure is the one error object every command
// publishes, so an agent asking for json learns the replacement too.
func TestRunStubJSONOutput(t *testing.T) {
	d, out := testDeps(t)
	if err := execute(t, d, "run", "my_dag", "-o", "json"); err == nil {
		t.Fatal("json mode must still fail")
	}
	var obj cliout.ErrorObject
	if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out.String())
	}
	if obj.Code != 2 || obj.Kind != "usage" {
		t.Errorf("code, kind = %d, %q; want 2, usage", obj.Code, obj.Kind)
	}
	if !strings.Contains(obj.Error, "astro local run airflow dags test my_dag") {
		t.Errorf("error does not name the replacement: %q", obj.Error)
	}
}

// The replacement names a command this tree has.
func TestRunStubReplacementIsARealCommand(t *testing.T) {
	d, _ := testDeps(t)
	found, _, err := newRootCmd(d).Find([]string{"local", nameRun})
	if err != nil || found.CommandPath() != "astro local run" {
		t.Errorf("%q does not resolve: %v", replaceRunDag, err)
	}
}
