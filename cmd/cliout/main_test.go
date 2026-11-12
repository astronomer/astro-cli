package cliout

import (
	"os"
	"testing"
)

// TestMain unsets CI for the package run: a run with CI set asks nothing
// (input.Refusal), so a case that expects a prompt would fail on a CI runner,
// where CI is set. The cases about CI set it themselves.
func TestMain(m *testing.M) {
	os.Unsetenv("CI") //nolint:usetesting // for the whole package run
	for _, k := range []string{"JENKINS_URL", "HUDSON_URL", "TF_BUILD"} {
		os.Unsetenv(k) //nolint:usetesting // other CI systems decide it too
	}
	os.Exit(m.Run())
}
