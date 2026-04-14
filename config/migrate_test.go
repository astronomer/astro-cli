package config

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/keychain"
)

func TestMigrateLegacyCredentials_NothingToMigrate(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: astronomer_io
contexts:
  astronomer_io:
    domain: astronomer.io
    workspace: ws-1
`)
	err := afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	require.NoError(t, err)
	InitConfig(fs)

	store := keychain.NewTestStore()
	migrated, err := MigrateLegacyCredentials(store)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated)

	_, err = store.GetCredentials("astronomer.io")
	assert.ErrorIs(t, err, keychain.ErrNotFound)
}

func TestMigrateLegacyCredentials_SingleContext(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: astronomer_io
contexts:
  astronomer_io:
    domain: astronomer.io
    token: "Bearer old-token"
    refreshtoken: "old-refresh"
    user_email: "user@example.com"
    workspace: ws-1
`)
	err := afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	require.NoError(t, err)
	InitConfig(fs)

	store := keychain.NewTestStore()
	migrated, err := MigrateLegacyCredentials(store)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated)

	creds, err := store.GetCredentials("astronomer.io")
	require.NoError(t, err)
	assert.Equal(t, "Bearer old-token", creds.Token)
	assert.Equal(t, "old-refresh", creds.RefreshToken)
	assert.Equal(t, "user@example.com", creds.UserEmail)

	// Confirm credential fields are fully removed (not just empty strings)
	ctxMap := viperHome.GetStringMap("contexts.astronomer_io")
	assert.NotContains(t, ctxMap, "token")
	assert.NotContains(t, ctxMap, "refreshtoken")
	assert.NotContains(t, ctxMap, "user_email")
	assert.NotContains(t, ctxMap, "expiresin")
	// Non-credential fields survive
	assert.Contains(t, ctxMap, "domain")
	assert.Contains(t, ctxMap, "workspace")
}

func TestMigrateLegacyCredentials_MultipleContexts(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: astronomer_io
contexts:
  astronomer_io:
    domain: astronomer.io
    token: "Bearer token-a"
    refreshtoken: "refresh-a"
    user_email: "a@example.com"
  astronomer_stage_io:
    domain: astronomer-stage.io
    token: "Bearer token-b"
    refreshtoken: "refresh-b"
    user_email: "b@example.com"
`)
	err := afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	require.NoError(t, err)
	InitConfig(fs)

	store := keychain.NewTestStore()
	migrated, err := MigrateLegacyCredentials(store)
	require.NoError(t, err)
	assert.Equal(t, 2, migrated)

	credsA, err := store.GetCredentials("astronomer.io")
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-a", credsA.Token)

	credsB, err := store.GetCredentials("astronomer-stage.io")
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-b", credsB.Token)
}

func TestMigrateLegacyCredentials_Idempotent(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: astronomer_io
contexts:
  astronomer_io:
    domain: astronomer.io
    token: "Bearer old-token"
    refreshtoken: "old-refresh"
    user_email: "user@example.com"
`)
	err := afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	require.NoError(t, err)
	InitConfig(fs)

	store := keychain.NewTestStore()
	migrated, err := MigrateLegacyCredentials(store)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated)

	// Second call: nothing in config.yaml to migrate
	migrated, err = MigrateLegacyCredentials(store)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated)
}
