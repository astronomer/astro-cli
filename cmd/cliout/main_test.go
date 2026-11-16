package cliout

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain isolates the package run (testenv.Isolate): the environment variables
// that change what the CLI does are unset, and HOME is a temporary directory.
// CI is among the variables: a run with CI set asks
// nothing (input.Refusal), so a case that expects a prompt would fail on a CI
// runner. The cases about CI, or any other variable, set it themselves.
func TestMain(m *testing.M) { testenv.Main(m) }
