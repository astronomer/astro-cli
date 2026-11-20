package astro

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
)

// A Workspace's name is printed as text in the created and deleted lines: an
// escape in it (here a clipboard write and a screen clear) does not reach the
// terminal.
func TestWorkspaceLinesCarryNoEscapes(t *testing.T) {
	const name = "ops\x1b]52;c;aGk=\x07\x1b[2J"
	r := func(buf *bytes.Buffer) cliout.Renderer { return cliout.Renderer{Format: cliout.FormatText, Out: buf} }

	var buf bytes.Buffer
	require.NoError(t, emitWorkspace(r(&buf), &workspace.WorkspaceInfo{Name: name}, "Astro Workspace "+name+" was successfully created"))
	assert.Equal(t, "Astro Workspace ops was successfully created\n", buf.String())

	buf.Reset()
	require.NoError(t, emitWorkspaceRemoval(r(&buf), &workspace.Removal{Name: name}))
	assert.Equal(t, "Astro Workspace ops was successfully deleted\n", buf.String())
}
