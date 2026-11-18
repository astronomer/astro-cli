package usageerr

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain isolates the package run from the machine (testenv.Isolate).
func TestMain(m *testing.M) { testenv.Main(m) }
