package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

// Hostname logic is thoroughly tested in pkg/proxy/hostname_test.go.
// These tests verify the CLI wrapper delegates correctly.

func TestDeriveHostname_Delegates(t *testing.T) {
	// Verify the wrapper produces the same result as the pkg function.
	hostname, err := DeriveHostname("/home/user/my-project")
	require.NoError(t, err)

	expected, err := pkgproxy.DeriveHostname("/home/user/my-project")
	require.NoError(t, err)

	assert.Equal(t, expected, hostname)
}
