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

// A context entry's `domain:` field is optional — real config.yaml files have
// entries without it, and they resolve at runtime because GetCurrentContext
// fills Domain in from the top-level `context:` key. Migration walks every
// context rather than just the current one, so it must recover the domain from
// the context key; otherwise all such contexts land under "" and clobber each
// other, silently logging the user out of all but one.
func TestMigrateLegacyCredentials_ContextsWithoutDomainField(t *testing.T) {
	fs := afero.NewMemMapFs()
	configRaw := []byte(`
context: astronomer-stage.io
contexts:
  astronomer-dev_io:
    token: "Bearer token-dev"
    refreshtoken: "refresh-dev"
  astronomer-stage_io:
    token: "Bearer token-stage"
    refreshtoken: "refresh-stage"
`)
	err := afero.WriteFile(fs, HomeConfigFile, configRaw, 0o777)
	require.NoError(t, err)
	InitConfig(fs)

	store := keychain.NewTestStore()
	migrated, err := MigrateLegacyCredentials(store)
	require.NoError(t, err)
	assert.Equal(t, 2, migrated)

	credsDev, err := store.GetCredentials("astronomer-dev.io")
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-dev", credsDev.Token)
	assert.Equal(t, "refresh-dev", credsDev.RefreshToken)

	credsStage, err := store.GetCredentials("astronomer-stage.io")
	require.NoError(t, err)
	assert.Equal(t, "Bearer token-stage", credsStage.Token)
	assert.Equal(t, "refresh-stage", credsStage.RefreshToken)

	_, err = store.GetCredentials("")
	assert.Error(t, err, "no credentials should be filed under an empty domain")
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
