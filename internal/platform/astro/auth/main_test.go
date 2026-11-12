package auth

import (
	"os"
	"testing"
)

// TestMain unsets the CI variables for the package run: a run with one set
// asks nothing (input.Refusal), so a case that expects the Press Enter prompt
// would fail on a CI runner, where CI is set. The cases about CI set it
// themselves.
func TestMain(m *testing.M) {
	for _, k := range []string{"CI", "JENKINS_URL", "HUDSON_URL", "TF_BUILD"} {
		os.Unsetenv(k) //nolint:usetesting // for the whole package run
	}
	os.Exit(m.Run())
}
