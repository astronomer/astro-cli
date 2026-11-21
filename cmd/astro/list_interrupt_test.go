package astro

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// cloudListCommands are the cloud lists that page, each with the root that
// holds it and a run whose first call to the API is the list itself, so a mock
// with no expectations proves nothing was asked.
type cloudListCase struct {
	name string
	root func(io.Writer) *cobra.Command
	args []string
}

var cloudListCommands = []cloudListCase{
	{"organization list", newOrganizationCmd, []string{"list"}},
	{"organization user list", newOrganizationCmd, []string{"user", "list"}},
	{"organization team list", newOrganizationCmd, []string{"team", "list"}},
	{"organization token list", newOrganizationCmd, []string{"token", "list"}},
	{"organization role list", newOrganizationCmd, []string{"role", "list"}},
	{"organization cluster list", newOrganizationCmd, []string{"cluster", "list"}},
	{"workspace list", newWorkspaceCmd, []string{"list"}},
	{"workspace user list", newWorkspaceCmd, []string{"user", "list", "--workspace", "ws-1"}},
	{"workspace team list", newWorkspaceCmd, []string{"team", "list", "--workspace", "ws-1"}},
	{"workspace token list", newWorkspaceCmd, []string{"token", "list", "--workspace", "ws-1"}},
	{"workspace organization-token list", newWorkspaceCmd, []string{"token", "organization-token", "list", "--workspace", "ws-1"}},
	{"deployment user list", newDeploymentRootCmd, []string{"user", "list", "--deployment", "dep-1"}},
	{"deployment team list", newDeploymentRootCmd, []string{"team", "list", "--deployment", "dep-1"}},
	{"deployment token list", newDeploymentRootCmd, []string{"token", "list", "--deployment", "dep-1"}},
	{"deployment organization-token list", newDeploymentRootCmd, []string{"token", "organization-token", "list", "--deployment", "dep-1"}},
	{"deployment workspace-token list", newDeploymentRootCmd, []string{"token", "workspace-token", "list", "--deployment", "dep-1"}},
}

func runCloudList(ctx context.Context, t *testing.T, root func(io.Writer) *cobra.Command, args []string, client astrov1.APIClient) (string, error) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	testUtil.SetupOSArgsForGinkgo()
	prev := astroV1Client
	astroV1Client = client
	t.Cleanup(func() { astroV1Client = prev })

	var out, errOut bytes.Buffer
	cmd := root(&out)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SilenceUsage = true
	var verbosity string
	cmd.PersistentFlags().StringVar(&verbosity, "verbosity", "", "")
	cmd.SetArgs(args)
	_, err := cmd.ExecuteContextC(ctx)
	return out.String(), err
}

// Ctrl-C ends a listing: a canceled run asks the API for nothing, prints no
// table, and fails with the cancellation (which the exit status turns into
// 130, or 128 plus the signal).
func TestCloudListsStopWhenTheRunIsCanceled(t *testing.T) {
	for _, tc := range cloudListCommands {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			out, err := runCloudList(ctx, t, tc.root, tc.args, new(astrov1_mocks.ClientWithResponsesInterface))
			require.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, out, "no partial table is printed as if it were the whole list")
			assert.Equal(t, cliout.ExitInterrupted, cliout.ExitCode(ctx, err))
		})
	}
}

// A page that is in flight when Ctrl-C arrives is not the whole list: the
// listing fails instead of printing what had been read.
func TestAListingCanceledBetweenPagesPrintsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Run(func(mock.Arguments) { cancel() }).
		Return(&astrov1.ListUsersResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.UsersPaginated{TotalCount: 500, Users: []astrov1.User{{Id: "u1", Username: "a@x.io", FullName: "A"}}},
		}, nil).Once()
	out, err := runCloudList(ctx, t, newOrganizationCmd, []string{"user", "list"}, m)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, out)
	m.AssertExpectations(t)
}

// Every list that pages takes --limit the way the workspace and deployment
// lists do: below one, or not a number, is refused before the API is asked.
func TestCloudListLimitBelowOneIsAUsageError(t *testing.T) {
	limited := append(append([]cloudListCase{}, cloudListCommands...),
		cloudListCase{"organization team user list", newOrganizationCmd, []string{"team", "user", "list", "--team-id", "t1"}},
		cloudListCase{"deployment bundle list", newDeploymentRootCmd, []string{"bundle", "list", "--deployment", "dep-1"}},
	)
	for _, tc := range limited {
		for _, bad := range []string{"0", "-2", "many"} {
			t.Run(tc.name+" --limit "+bad, func(t *testing.T) {
				args := append(append([]string{}, tc.args...), "--limit", bad)
				_, err := runCloudList(context.Background(), t, tc.root, args, new(astrov1_mocks.ClientWithResponsesInterface))
				require.Error(t, err)
				if bad != "many" {
					assert.True(t, cliout.IsUsage(err), "--limit %s: %v", bad, err)
					assert.Contains(t, err.Error(), "--limit")
				}
			})
		}
	}
}

// A limit is asked of the API as the page size, in a stable name order, and
// the listing stops once it has that many.
func TestCloudListLimitAsksForThatManyInANameOrder(t *testing.T) {
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1.ListUsersParams) bool {
		return p.Limit != nil && *p.Limit == 2 && p.Offset != nil && *p.Offset == 0 &&
			p.Sorts != nil && len(*p.Sorts) > 0 && (*p.Sorts)[0] == astrov1.ListUsersParamsSortsFullNameAsc
	})).Return(&astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.UsersPaginated{TotalCount: 50, Users: []astrov1.User{
			{Id: "u1", Username: "a@x.io", FullName: "Ada"},
			{Id: "u2", Username: "b@x.io", FullName: "Bo"},
		}},
	}, nil).Once()
	out, err := runCloudList(context.Background(), t, newOrganizationCmd, []string{"user", "list", "--limit", "2"}, m)
	require.NoError(t, err)
	assert.Contains(t, out, "a@x.io")
	assert.Contains(t, out, "b@x.io")
	m.AssertExpectations(t)
}
