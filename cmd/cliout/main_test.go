package cliout

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain unsets the environment variables that change what the CLI does
// (testenv.Clear) for the package run. CI among them: a run with CI set asks
// nothing (input.Refusal), so a case that expects a prompt would fail on a CI
// runner. The cases about CI, or any other variable, set it themselves.
func TestMain(m *testing.M) {
	testenv.Clear()
	m.Run()
}
