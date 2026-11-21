package astro

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
)

// requireOnlyDeprecationWarnings fails on anything on stderr that is not a
// deprecation warning: a run through a deprecated spelling says so there, and
// nothing else is allowed to.
func requireOnlyDeprecationWarnings(t *testing.T, stderr string) {
	t.Helper()
	for line := range strings.Lines(stderr) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, "Warning: ") && strings.Contains(line, " is deprecated: "), "stderr holds only deprecation warnings, not %q", line)
	}
}

// The token tables' columns stay where they have always been, and KIND, added
// later, is last: a script reading a table by position finds every earlier
// column in its place. The rows line up with the header.
func TestTokenTablesKeepTheirColumnsAndAddKindLast(t *testing.T) {
	for _, role := range []string{deploymentRoleHeader, workspaceRoleHeader, organizationRoleHeader} {
		assert.Equal(t, []string{"ID", "NAME", "DESCRIPTION", "SCOPE", role, "CREATED", "CREATED BY", "KIND"}, tokenHeader(role))
	}
	row := tokenRow(&apitoken.Token{ID: "id", Name: "n", Description: "d", Scope: "WORKSPACE", Role: "r", CreatedBy: "by", Kind: "DIRECT_ACCESS"})
	assert.Len(t, row, len(tokenHeader(workspaceRoleHeader)))
	assert.Equal(t, []string{"id", "n", "d", "WORKSPACE", "r"}, row[:5])
	assert.Equal(t, "by", row[6])
	assert.Equal(t, "DIRECT_ACCESS", row[7])
}

// Piped, a token list gives a script the creation moment, not "3 days ago",
// and one tab between cells.
func TestTokenListPipedHasTimestampsAndTabs(t *testing.T) {
	created := time.Date(2026, 11, 18, 22, 57, 8, 0, time.FixedZone("x", 3600))
	var out bytes.Buffer
	tokens := []apitoken.Token{{ID: "tok", Name: "n", Scope: "WORKSPACE", Role: "r", CreatedAt: created, CreatedBy: "by", Kind: "DIRECT_ACCESS"}}
	require.NoError(t, renderTokenList(cliout.FormatText, &out, tokens, workspaceRoleHeader))
	assert.Equal(t, "ID\tNAME\tDESCRIPTION\tSCOPE\tWORKSPACE ROLE\tCREATED\tCREATED BY\tKIND\n"+
		"tok\tn\t\tWORKSPACE\tr\t2026-11-18T21:57:08Z\tby\tDIRECT_ACCESS\n", out.String())
}

// A create's stdout is the secret and a newline, never the receipt.
func TestTokenSecretIsAloneOnStdout(t *testing.T) {
	var out, errOut bytes.Buffer
	tok := &apitoken.Token{Name: "ci", Token: "s3cret"}
	require.NoError(t, renderTokenSecret(cliout.FormatText, &out, &errOut, tok, "Workspace", "Created", "ci"))
	assert.Equal(t, "s3cret\n", out.String())
	assert.Equal(t, "\nCreated Workspace API token ci\nCopy this API token for your records.\nThis API token value is not shown again.\n", errOut.String())

	// A secret that could not be written is not announced as given.
	errOut.Reset()
	require.Error(t, renderTokenSecret(cliout.FormatText, failingWriter{}, &errOut, tok, "Workspace", "Created", "ci"))
	assert.Empty(t, errOut.String())
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
