package astro

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// --limit below one is a usage error before anything is listed, and a limit
// that is met stops the paging: the workspace list asks for one row only.
func TestWorkspaceListLimit(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	for _, bad := range []string{"0", "-3"} {
		_, err := execWorkspaceCmd("list", "--limit", bad)
		require.Error(t, err, bad)
		assert.True(t, cliout.IsUsage(err), "--limit %s: %v", bad, err)
	}
	_, err := execWorkspaceCmd("list", "--limit", "many")
	require.Error(t, err)

	mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
	mockClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	astroV1Client = mockClient
	resp, err := execWorkspaceCmd("list", "--limit", "1")
	require.NoError(t, err)
	assert.Contains(t, resp, "test-workspace")
	mockClient.AssertExpectations(t)
}

func TestAddListLimitValidatesBeforeRunning(t *testing.T) {
	var got []int
	cmd := &cobra.Command{Use: "x"}
	addListLimit(cmd, "things", func(_ *cobra.Command, limit int) error {
		got = append(got, limit)
		return nil
	})
	require.NoError(t, cmd.ParseFlags(nil))
	require.NoError(t, cmd.RunE(cmd, nil))
	require.NoError(t, cmd.ParseFlags([]string{"--limit", "5"}))
	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Equal(t, []int{0, 5}, got, "left off is 0, which pages through everything")
	assert.Contains(t, cmd.Flags().Lookup("limit").Usage, "at least 1")
	assert.Contains(t, cmd.Flags().Lookup("limit").Usage, "default: all")

	require.NoError(t, cmd.ParseFlags([]string{"--limit", "0"}))
	err := cmd.RunE(cmd, nil)
	assert.True(t, cliout.IsUsage(err))
	assert.Equal(t, []int{0, 5}, got, "a refused limit runs nothing")
}

// --basic-token - reads the token from a pipe, so it never reaches argv.
func TestMetricsBasicTokenReadsStdin(t *testing.T) {
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	_, err = w.WriteString("tok-from-pipe\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	cmd := &cobra.Command{Use: "set"}
	metricsCommonFlags(cmd)
	require.NoError(t, cmd.ParseFlags([]string{"--basic-token", "-"}))
	defer resetEnvFlags()

	in, err := buildMetricsInput(cmd)
	require.NoError(t, err)
	require.NotNil(t, in.BasicToken)
	assert.Equal(t, "tok-from-pipe", *in.BasicToken)
}

func TestMetricsBasicTokenFlagValueIsUsedAsGiven(t *testing.T) {
	cmd := &cobra.Command{Use: "set"}
	metricsCommonFlags(cmd)
	require.NoError(t, cmd.ParseFlags([]string{"--basic-token", "literal"}))
	defer resetEnvFlags()

	in, err := buildMetricsInput(cmd)
	require.NoError(t, err)
	require.NotNil(t, in.BasicToken)
	assert.Equal(t, "literal", *in.BasicToken)
}

// --log-count below one is a usage error before any Deployment is looked up.
func TestDeploymentLogsCountBelowOneIsRefused(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer func() { logCount = 500 }()
	for _, bad := range []string{"0", "-1"} {
		_, err := execDeploymentCmd("logs", "--log-count", bad, "some-id")
		require.Error(t, err, bad)
		assert.True(t, cliout.IsUsage(err), "--log-count %s: %v", bad, err)
	}
}
