package apc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// A workspace list written to a pipe is the header and one tab-separated line
// per workspace: no padding, and no color on the current one.
func TestWorkspaceListIsTabSeparatedWhenPiped(t *testing.T) {
	var out bytes.Buffer
	r := cliout.Renderer{Format: cliout.FormatText, Out: &out}
	require.NoError(t, emitWorkspaceList(r, []houston.Workspace{wsCurrent, wsBare}, currentWS))
	assert.Equal(t, "NAME\tID\nairflow\t"+currentWS+"\nthird\tckc0j8y1101xo0760or02jdi7\n", out.String())
}
