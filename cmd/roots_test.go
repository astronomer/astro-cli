package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
)

// stubHouston answers every construction-time Houston call offline, with every
// feature flag on.
//
// The flags matter more than they look: cmd/apc mounts `deployment runtime`
// only when AstroRuntimeEnabled and `deployment logs triggerer` only when
// TriggererEnabled, so the APC tree's shape depends on what this returns. A
// mock with the zero AppConfig builds a smaller tree and a tree-wide test over
// it silently covers less than it claims to.
func stubHouston(t *testing.T) *houston_mocks.ClientInterface {
	t.Helper()
	client := new(houston_mocks.ClientInterface)
	client.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{
		Flags: houston.FeatureFlags{
			NfsMountDagDeployment:  true,
			HardDeleteDeployment:   true,
			ManualNamespaceNames:   true,
			TriggererEnabled:       true,
			GitSyncEnabled:         true,
			NamespaceFreeFormEntry: true,
			BYORegistryEnabled:     true,
			AstroRuntimeEnabled:    true,
			DagOnlyDeployment:      true,
		},
	}, nil)
	client.On("GetPlatformVersion", nil).Return("1.0.0", nil)
	return client
}

// rootsUnderTest builds the fully assembled root once per platform branch, so a
// tree-wide invariant is checked against both. The alternative — calling
// NewRootCmd() — reads whatever context the ambient config holds, which is why
// every tree-wide test before this one covered one branch per run and depended
// on test ordering to pick which.
//
// The APC root is built last on purpose: apcCmd.AddCmds stores its client,
// app config and platform version in package-level variables, so building it
// leaves those set for whatever runs next.
func rootsUnderTest(t *testing.T) map[string]*cobra.Command {
	t.Helper()
	roots := map[string]*cobra.Command{}
	for _, platform := range []string{cloudPlatform, apcPlatform} {
		roots[platform] = newRootCmd(rootOptions{
			platform:      platform,
			loggedIn:      true,
			houstonClient: stubHouston(t),
			out:           new(bytes.Buffer),
		})
	}
	return roots
}
