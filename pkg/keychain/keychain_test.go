package keychain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/keychain"
)

func TestSetAndGetCredentials(t *testing.T) {
	store := keychain.NewTestStore()
	creds := keychain.Credentials{
		Token:        "Bearer access-token",
		RefreshToken: "refresh-token",
		UserEmail:    "user@example.com",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
	}

	err := store.SetCredentials("astronomer.io", creds)
	require.NoError(t, err)

	got, err := store.GetCredentials("astronomer.io")
	require.NoError(t, err)
	// Round-trip strips monotonic clock; normalise before comparing.
	creds.ExpiresAt = creds.ExpiresAt.UTC()
	got.ExpiresAt = got.ExpiresAt.UTC()
	assert.Equal(t, creds, got)
}

func TestGetCredentials_NotFound(t *testing.T) {
	store := keychain.NewTestStore()
	_, err := store.GetCredentials("notexist.io")
	assert.ErrorIs(t, err, keychain.ErrNotFound)
}

func TestDeleteCredentials(t *testing.T) {
	store := keychain.NewTestStore()
	creds := keychain.Credentials{Token: "Bearer tok"}

	require.NoError(t, store.SetCredentials("astronomer.io", creds))
	require.NoError(t, store.DeleteCredentials("astronomer.io"))

	_, err := store.GetCredentials("astronomer.io")
	assert.ErrorIs(t, err, keychain.ErrNotFound)
}

func TestDeleteCredentials_NotFound_NoError(t *testing.T) {
	store := keychain.NewTestStore()
	err := store.DeleteCredentials("notexist.io")
	assert.NoError(t, err)
}

func TestIsolation(t *testing.T) {
	store := keychain.NewTestStore()
	require.NoError(t, store.SetCredentials("a.io", keychain.Credentials{Token: "token-a"}))
	require.NoError(t, store.SetCredentials("b.io", keychain.Credentials{Token: "token-b"}))

	a, err := store.GetCredentials("a.io")
	require.NoError(t, err)
	assert.Equal(t, "token-a", a.Token)

	b, err := store.GetCredentials("b.io")
	require.NoError(t, err)
	assert.Equal(t, "token-b", b.Token)
}
