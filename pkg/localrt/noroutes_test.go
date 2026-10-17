//go:build !windows

package localrt

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A runtime with no routes dir still lists, reads and prunes the records:
// the routes are a join on top of them, not where they live.
func TestNoRoutesStillListsReadsAndPrunes(t *testing.T) {
	isolatedLevers(t)
	rtime := New(Config{NoRoutes: errors.New("finding the home directory for the proxy routes: no home")})

	project := filepath.Join(t.TempDir(), "homeless")
	saveStale(t, project, 8085)

	statuses, err := rtime.List()
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Equal(t, "homeless.localhost", statuses[0].Hostname, "the record's own hostname, with no routes to join")

	st, err := rtime.ReadStatus(project)
	require.NoError(t, err)
	assert.Equal(t, project, st.ProjectPath)

	removed, err := rtime.PruneStale()
	require.NoError(t, err, "no route was registered, so there is none to fail to remove")
	require.Len(t, removed, 1)
}
