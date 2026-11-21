package astro

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
)

// The receipt names the token as text: a rotated token's name comes from the
// API, and its escapes are not passed to the terminal.
func TestTokenReceiptNamesTheTokenAsText(t *testing.T) {
	var out, errOut bytes.Buffer
	tok := &apitoken.Token{Name: "ci", Token: "s3cret"}
	require.NoError(t, renderTokenSecret(cliout.FormatText, &out, &errOut, tok, "Workspace", "Rotated", "ci\x1b]0;title\x07\nfake"))
	assert.Contains(t, errOut.String(), "\nRotated Workspace API token ci fake\n")
	assert.NotContains(t, errOut.String(), "\x1b")
}
