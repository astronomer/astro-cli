package astro

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
