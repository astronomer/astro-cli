package apc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// A Deployment's log line goes to a file or a pipe as its code wrote it. (To a
// terminal, cliout.Renderer drops what the terminal would act on.)
func TestLogEntryToAPipeIsExact(t *testing.T) {
	l := houston.DeploymentLog{Log: "line\x1b]52;c;aGk=\x07\x1b[2J", CreatedAt: "2026-01-01T00:00:00Z"}
	run := func() string {
		var buf bytes.Buffer
		require.NoError(t, emitLogEntry(cliout.Renderer{Format: cliout.FormatText, Out: &buf}, "scheduler", l, false, false))
		return buf.String()
	}
	assert.Equal(t, "line\x1b]52;c;aGk=\x07\x1b[2J\n", run(), "a pipe gets the log as it came")
}
