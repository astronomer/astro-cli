//go:build linux

package keychain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CI runs in a Docker container with no D-Bus / Secret Service, so New() takes
// the fallback path. We can't test the keyring success path without a running
// Secret Service or KWallet daemon. HOME is redirected so these never touch a
// real ~/.astro/credentials.json.

func TestNew_FallsBackToFileStoreWhenAllowed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := New(true)
	require.NoError(t, err)

	cached, ok := store.(*cachedStore)
	require.True(t, ok, "fallback store should be cached like the keyring path")
	assert.IsType(t, &fileStore{}, cached.inner)
}

// Refusing the fallback has to fail rather than quietly writing secrets to
// disk — a silent, unrefusable downgrade is the part of this design the GitHub
// CLI regrets shipping (cli/cli#10108).
func TestNew_RefusesFallbackWhenDisallowed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := New(false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInsecureFallbackRefused)
}
