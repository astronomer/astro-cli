//go:build linux

package keychain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CI runs in a Docker container with no D-Bus / Secret Service, so New()
// always takes the fileStore fallback path. We can't test the keyring
// success path without a running Secret Service daemon.
func TestNew_FallsBackToFileStore(t *testing.T) {
	store, err := New()
	require.NoError(t, err)
	assert.IsType(t, &fileStore{}, store)
}
