package astro

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/ide"
)

// An IDE project's name is printed as text in the import and export lines: an
// escape in it (here a clipboard write and a screen clear) does not reach the
// terminal.
func TestIDEProjectLinesCarryNoEscapes(t *testing.T) {
	const name = "etl\x1b]52;c;aGk=\x07\x1b[2J"
	r := func(buf *bytes.Buffer) cliout.Renderer { return cliout.Renderer{Format: cliout.FormatText, Out: buf} }

	var buf bytes.Buffer
	require.NoError(t, emitIDEImport(r(&buf), &ide.Import{ProjectID: "p1", ProjectName: name}))
	assert.Equal(t, "Successfully exported project from etl\n", buf.String())

	buf.Reset()
	require.NoError(t, emitIDEExport(r(&buf), &ide.Export{ProjectID: "p1", ProjectName: name}))
	assert.Equal(t, "Successfully exported project to etl\n", buf.String())
}
