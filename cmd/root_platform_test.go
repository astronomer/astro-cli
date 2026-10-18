package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrocontext "github.com/astronomer/astro-cli/context"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// Which platform the root builds is decided from the current context's domain,
// never its login: resolving a login can read the keyring or the vault,
// rewrite the config and warn on stderr, and every command, `astro version`
// and the offline core tree included, decides the platform first. A command
// that needs the login reads it for itself.
func TestTheRootDecidesThePlatformWithoutTheLogin(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		config, platform string
	}{
		{testUtil.CloudPlatform, cloudPlatform},
		{testUtil.SoftwarePlatform, apcPlatform},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			testUtil.InitTestConfig(tc.config)
			resolved, restore := config.CountLoginResolutions()
			defer restore()

			o := detectRootOptions()
			require.Equal(t, tc.platform, o.platform)
			require.Zero(t, resolved(), "detecting the root's options resolved the login")

			for _, args := range [][]string{{"version"}, {"local", "ps", "-o", "json"}} {
				o.out, o.args = new(bytes.Buffer), args
				// Executed only for what the root reaches on the way: the core
				// command fails, with no project here.
				_, _, _ = executeRoot(newRootCmd(&o), args...)
				require.Zero(t, resolved(), "astro %s resolved the login", strings.Join(args, " "))
			}

			// The control: the counter sees a command that reads the login.
			_, err := astrocontext.GetCurrentContext()
			require.NoError(t, err)
			require.Equal(t, 1, resolved())
		})
	}
}

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
		// Tab-completing a root word: the word being completed is not the command.
		{cobra.ShellCompRequestCmd, ""},
		{cobra.ShellCompRequestCmd, "de"},
		{cobra.ShellCompNoDescRequestCmd},
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

	// Completing a word under an APC command asks, so the flags it offers are the platform's.
	client = stubHoustonAt(t, newestAPCVersion)
	newRootCmd(&rootOptions{platform: apcPlatform, loggedIn: true, houstonClient: client, out: new(bytes.Buffer), args: []string{cobra.ShellCompRequestCmd, "deployment", "create", "--"}})
	client.AssertCalled(t, "GetPlatformVersion", nil)
}
