package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/credentials"
	"github.com/astronomer/astro-cli/pkg/keychain"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestLoadSoftwareToken_LoadsToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	store := keychain.NewTestStore()
	err := store.SetCredentials("astronomer_dev.com", keychain.Credentials{Token: "test-token"})
	assert.NoError(t, err)

	holder := &credentials.CurrentCredentials{}
	assert.NoError(t, loadSoftwareToken(store, holder))
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
func TestLoadSoftwareToken_ReadFailureIsNotSilent(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	holder := &credentials.CurrentCredentials{}
	err := loadSoftwareToken(errStore{err: errors.New("keychain is locked")}, holder)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keychain is locked")
	assert.Empty(t, holder.Get())
}

// Not being logged in is ordinary, not an error — leave the holder empty and let
// whatever actually needs auth report it.
func TestLoadSoftwareToken_NotFoundIsNotAnError(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	holder := &credentials.CurrentCredentials{}
	err := loadSoftwareToken(errStore{err: keychain.ErrNotFound}, holder)

	assert.NoError(t, err)
	assert.Empty(t, holder.Get())
}
