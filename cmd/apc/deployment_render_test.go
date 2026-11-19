package apc

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// A Deployment's log line is whatever its code wrote: to a terminal it goes
// without escapes, to a pipe as it came.
func TestLogEntryReachesATerminalWithoutEscapes(t *testing.T) {
	l := houston.DeploymentLog{Log: "line\x1b]52;c;aGk=\x07\x1b[2J", CreatedAt: "2026-01-01T00:00:00Z"}
	run := func() string {
		var buf bytes.Buffer
		require.NoError(t, emitLogEntry(cliout.Renderer{Format: cliout.FormatText, Out: &buf}, "scheduler", l, false, false))
		return buf.String()
	}
	assert.Equal(t, "line\x1b]52;c;aGk=\x07\x1b[2J\n", run(), "a pipe gets the log as it came")

	orig := logsToTerminal
	t.Cleanup(func() { logsToTerminal = orig })
	logsToTerminal = func(io.Writer) bool { return true }
	assert.Equal(t, "line\n", run())
}
