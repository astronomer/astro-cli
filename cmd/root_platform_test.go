package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An APC root asks Houston for the platform's version and feature flags only
// when the line runs an APC command. A host that does not answer used to hold
// up every command, `astro --help` and a typo included, for two dial timeouts
// (#2289).
func TestAPCRootAsksThePlatformOnlyForAPCCommands(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	for _, args := range [][]string{
		nil,
		{"--help"},
		{"version"},
		{"jjklsjdfklsjfklsdf"},
		{"--jklsdjkfljsfd"},
		{"context", "list"},
		{"local", "start", "--help"},
	} {
		client := new(houston_mocks.ClientInterface) // no expectations: any call panics
		require.NotPanics(t, func() {
			newRootCmd(&rootOptions{platform: apcPlatform, loggedIn: true, houstonClient: client, out: new(bytes.Buffer), args: args})
		}, "%q", args)
	}

	client := stubHoustonAt(t, newestAPCVersion)
	root := newRootCmd(&rootOptions{platform: apcPlatform, loggedIn: true, houstonClient: client, out: new(bytes.Buffer), args: []string{"deployment", "list"}})
	client.AssertCalled(t, "GetPlatformVersion", nil)
	client.AssertCalled(t, "GetAppConfig", mock.Anything)
	// And the tree is built against what it answered: --mode is gated at 2.1.0.
	create, _, err := root.Find([]string{"deployment", "create"})
	require.NoError(t, err)
	require.NotNil(t, create.Flags().Lookup("mode"))
}
