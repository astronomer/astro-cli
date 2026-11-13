package auth

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain unsets the CI variables for the package run: a run with one set
// asks nothing (input.Refusal), so a case that expects the Press Enter prompt
// would fail on a CI runner, where CI is set. The cases about CI set it
// themselves.
func TestMain(m *testing.M) {
	testenv.Clear()
	m.Run()
}
