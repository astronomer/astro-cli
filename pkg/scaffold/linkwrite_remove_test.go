package scaffold

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Removing the last link removes the table it leaves empty, in every spelling a
// link arrives in, and keeps the comments on the sections around it.
func TestRemoveLinkRemovesTheTableItEmpties(t *testing.T) {
	const after = "\n# where our Composer environments live\n[tool.astro.targets.composer]\nproject = 'acme-data' # the data project\n"
	for name, links := range map[string]string{
		"inline":             "\n[tool.astro.deployments]\nprod = {deployment = 'dep-prod'}\n",
		"header":             "\n[tool.astro.deployments.prod]\ndeployment = 'dep-prod'\n",
		"header under table": "\n[tool.astro.deployments]\n\n[tool.astro.deployments.prod]\ndeployment = 'dep-prod'\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, linkFixture+"workspace = 'W'\n"+links+after)
			removed, err := RemoveLink(dir, nil, "prod")
			require.NoError(t, err)
			require.True(t, removed)
			out := readFile(t, path)
			assert.NotContains(t, out, "deployments", "an empty links table was left:\n%s", out)
			assert.Contains(t, out, userComment)
			assert.Contains(t, out, "# where our Composer environments live")
			assert.Contains(t, out, "project = 'acme-data' # the data project")
			assert.Empty(t, loadLinks(t, path).Astro.Deployments)
		})
	}
}

// A link that is not the last leaves the table, and its neighbors, in place.
func TestRemoveLinkKeepsATableWithLinksLeft(t *testing.T) {
	dir, path := linkProject(t, linkFixture+"workspace = 'W'\n\n[tool.astro.deployments]\n# staging first\nstage = {deployment = 'd1'}\nprod = {deployment = 'd2'}\n")
	_, err := RemoveLink(dir, nil, "prod")
	require.NoError(t, err)
	out := readFile(t, path)
	assert.True(t, strings.Contains(out, "[tool.astro.deployments]\n# staging first\nstage = {deployment = 'd1'}"), out)
}

// Removing a link that is not there writes nothing, even beside an empty
// links table: the cleanup belongs to the removal.
func TestRemoveLinkOfNothingLeavesAnEmptyTableAlone(t *testing.T) {
	body := linkFixture + "\n[tool.astro.deployments]\n"
	dir, path := linkProject(t, body)
	removed, err := RemoveLink(dir, nil, "nope")
	require.NoError(t, err)
	assert.False(t, removed)
	assert.Equal(t, body, readFile(t, path))
}
