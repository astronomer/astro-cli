package localshared

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/proxy"
)

// The allocator of an engine without routes picks a port in pkg/proxy's range
// that nothing is listening on.
func TestAllocateListeningPortIsInRangeAndFree(t *testing.T) {
	port, err := AllocateListeningPort()
	require.NoError(t, err)
	n, err := strconv.Atoi(port)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 10000)
	assert.LessOrEqual(t, n, 19999)
	assert.True(t, proxy.IsPortAvailable(port))
}
