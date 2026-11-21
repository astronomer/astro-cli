package team

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A role written, then an interrupt before the rename: the error still is the
// interrupt, and says the role changed, with the team as it now is. No rename
// is sent.
func TestInterruptBetweenTheRoleAndTheRename(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := astrov1_mocks.NewClientWithResponsesInterface(t)
	client.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
	client.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(context.Context, string, string, astrov1.UpdateTeamRolesJSONRequestBody, ...astrov1.RequestEditorFn) (*astrov1.UpdateTeamRolesResponse, error) {
			cancel()
			return &UpdateTeamRolesResponseOK, nil
		}).Once()

	got, err := UpdateTeam(ctx, team1.Id, "name", "description", "ORGANIZATION_OWNER", false, client)
	require.ErrorIs(t, err, context.Canceled)
	require.Error(t, err)
	require.Contains(t, err.Error(), "updated the team's role to ORGANIZATION_OWNER")
	require.NotNil(t, got)
	require.True(t, got.RoleChanged)
	client.AssertNotCalled(t, "UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// An interrupt that lands while the team is looked up sends no Deployment role.
func TestInterruptSendsNoDeploymentTeamRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for name, run := range map[string]func(client astrov1.APIClient) error{
		"add": func(c astrov1.APIClient) error {
			_, err := AddDeploymentTeam(ctx, team1.Id, "DEPLOYMENT_ADMIN", deploymentID, c)
			return err
		},
		"update": func(c astrov1.APIClient) error {
			_, err := UpdateDeploymentTeamRole(ctx, team1.Id, "DEPLOYMENT_ADMIN", deploymentID, c)
			return err
		},
		"remove": func(c astrov1.APIClient) error {
			_, err := RemoveDeploymentTeam(ctx, team1.Id, deploymentID, c)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			client := astrov1_mocks.NewClientWithResponsesInterface(t)
			client.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(
				func(context.Context, string, string, ...astrov1.RequestEditorFn) (*astrov1.GetTeamResponse, error) {
					cancel()
					return &GetTeamWithResponseOK, nil
				}).Once()
			require.ErrorIs(t, run(client), context.Canceled)
			client.AssertNotCalled(t, "UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
