package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/credentials"
	"github.com/astronomer/astro-cli/pkg/keychain"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestLoadStoredToken_LoadsToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	store := keychain.NewTestStore()
	err := store.SetCredentials("astronomer_dev.com", keychain.Credentials{Token: "test-token"})
	assert.NoError(t, err)

	holder := &credentials.CurrentCredentials{}
	assert.NoError(t, loadStoredToken(store, holder))
	assert.Equal(t, "test-token", holder.Get())
}

// errStore fails every read, standing in for a locked keychain, an ACL prompt
// the user denied after a binary update, or a dead D-Bus session.
type errStore struct{ err error }

func (e errStore) GetCredentials(string) (keychain.Credentials, error) {
	return keychain.Credentials{}, e.err
}
func (e errStore) SetCredentials(string, keychain.Credentials) error { return e.err }
func (e errStore) DeleteCredentials(string) error                    { return e.err }

// A store that cannot be read must not look like "not logged in": swallowing
// the error leaves the token holder empty and sends unauthenticated requests to
// Houston, which surface as a confusing 401 instead of a credential problem.
func TestLoadStoredToken_ReadFailureIsNotSilent(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	holder := &credentials.CurrentCredentials{}
	err := loadStoredToken(errStore{err: errors.New("keychain is locked")}, holder)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keychain is locked")
	assert.Empty(t, holder.Get())
}

// Not being logged in is ordinary, not an error — leave the holder empty and let
// whatever actually needs auth report it.
func TestLoadStoredToken_NotFoundIsNotAnError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	holder := &credentials.CurrentCredentials{}
	err := loadStoredToken(errStore{err: keychain.ErrNotFound}, holder)

	assert.NoError(t, err)
	assert.Empty(t, holder.Get())
}

// `astro dev` defines its own PersistentPreRunE for the container runtime, and
// cobra replaces the root's rather than chaining them — so the root hook that
// normally fills creds never runs. Request editors read the token only from
// creds now, so the dev sub-commands that reach the Astro API must load it
// themselves or they send unauthenticated requests.
func TestLoadDevCredentials_LoadsTokenWhenTargetingDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	c, err := context.GetCurrentContext()
	require.NoError(t, err)

	store := keychain.NewTestStore()
	require.NoError(t, store.SetCredentials(c.Domain, keychain.Credentials{Token: "Bearer dev-token"}))

	cmd := &cobra.Command{Use: "start"}
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("deployment-id", "", "")
	require.NoError(t, cmd.Flags().Set("deployment-id", "cl-test-deployment"))

	creds := &credentials.CurrentCredentials{}
	require.NoError(t, loadDevCredentials(store, creds)(cmd, nil))
	assert.Equal(t, "Bearer dev-token", creds.Get())
}

// Plain `astro dev start` targets nothing on Astro, so it must keep working
// logged out — and must not touch the keychain (which on macOS would mean an
// unprompted-for credential prompt just to start Airflow locally).
func TestLoadDevCredentials_SkipsWhenNoAstroTarget(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	cmd := &cobra.Command{Use: "start"}
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("deployment-id", "", "")

	creds := &credentials.CurrentCredentials{}
	// errStore fails any read; reaching it at all would fail this test.
	require.NoError(t, loadDevCredentials(errStore{err: errors.New("must not touch the store")}, creds)(cmd, nil))
	assert.Empty(t, creds.Get())
}
