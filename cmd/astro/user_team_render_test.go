package astro

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/team"
)

// A team's name is printed as text: an escape in it (here a clipboard write
// and a screen clear) does not reach the terminal. json keeps it as returned.
func TestTeamUpdateLinesCarryNoEscapes(t *testing.T) {
	upd := &team.Update{Team: team.TeamInfo{Name: "ops\x1b]52;c;aGk=\x07\x1b[2J"}}

	var buf bytes.Buffer
	require.NoError(t, renderTeamUpdate(cliout.FormatText, &buf, io.Discard, upd, nil))
	assert.Equal(t, "Updated team ops\n", buf.String())

	buf.Reset()
	require.NoError(t, renderTeamUpdate(cliout.FormatJSON, &buf, io.Discard, upd, nil))
	assert.Contains(t, buf.String(), `ops\u001b]52;c;aGk=\u0007\u001b[2J`)
}
