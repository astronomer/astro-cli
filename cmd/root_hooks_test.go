package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/keychain"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestLoadSoftwareToken_LoadsToken(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	store := keychain.NewTestStore()
	err := store.SetCredentials("astronomer_dev.com", keychain.Credentials{Token: "test-token"})
	assert.NoError(t, err)

	holder := &httputil.TokenHolder{}
	loadSoftwareToken(store, holder)
	assert.Equal(t, "test-token", holder.Get())
}
