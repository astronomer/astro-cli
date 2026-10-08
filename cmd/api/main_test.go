package api

import (
	"os"
	"testing"
)

// TestMain watches what ls and describe publish through cliout.Renderer.Emit,
// and fails a passing run that published a shape no golden pins
// (schema_test.go). The requests themselves print the API's response and go
// around Emit, so the watch sees only ls, describe and their failures.
func TestMain(m *testing.M) {
	watching := emitWatch().Arm()
	os.Exit(watching.Finish(m.Run()))
}
