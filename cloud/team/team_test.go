package team

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/astronomer/astro-cli/cloud/user"
	"net/http"
	"os"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/config"
	"github.com/stretchr/testify/mock"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

// org team variables
var (
	errorNetwork = errors.New("network error")
	errorInvite  = errors.New("test-inv-error")
	orgRole      = "ORGANIZATION_MEMBER"
	description  = "mock description"
	team1        = astrocore.Team{
		CreatedAt:   time.Now(),
		Name:        "team 1",
		Description: &description,
		Id:          "team1-id",
	}
	teams = []astrocore.Team{
		team1,
	}
	ListOrganizationTeamsResponseOK = astrocore.ListOrganizationTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      teams,
		},
	}
	errorBodyList, _ = json.Marshal(astrocore.Error{
		Message: "failed to list teams",
	})
	ListOrganizationTeamsResponseError = astrocore.ListOrganizationTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
)

// workspace teams variables
var (
	workspaceRole = "WORKSPACE_MEMBER"
	workspaceId   = "workspace_cuid"
	roles         = []astrocore.TeamRole{
		{EntityType: "WORKSPACE", EntityId: workspaceId, Role: workspaceRole},
	}
	workspaceTeam1 = astrocore.Team{
		CreatedAt:   time.Now(),
		Name:        "team 1",
		Id:          "team1-id",
		Description: &description,
		Roles:       &roles,
	}
	workspaceTeams = []astrocore.Team{
		workspaceTeam1,
	}
	ListWorkspaceTeamsResponseOK = astrocore.ListWorkspaceTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      workspaceTeams,
		},
	}
	ListWorkspaceTeamsResponseError = astrocore.ListWorkspaceTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateWorkspaceTeamRoleResponseOK = astrocore.MutateWorkspaceTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamRole{
			Role: "WORKSPACE_MEMBER",
		},
	}
	errorBodyUpdate, _ = json.Marshal(astrocore.Error{
		Message: "failed to update team",
	})
	MutateWorkspaceTeamRoleResponseError = astrocore.MutateWorkspaceTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	//DeleteWorkspaceTeamResponseOK = astrocore.DeleteWorkspaceTeamResponse{
	//	HTTPResponse: &http.Response{
	//		StatusCode: 200,
	//	},
	//	JSON200: &workspaceTeam1,
	//}
	//DeleteWorkspaceTeamResponseError = astrocore.DeleteWorkspaceTeamResponse{
	//	HTTPResponse: &http.Response{
	//		StatusCode: 500,
	//	},
	//	Body:    errorBodyUpdate,
	//	JSON200: nil,
	//}
)

type testWriter struct {
	Error error
}

func (t testWriter) Write(p []byte) (n int, err error) {
	return 0, t.Error
}

//func TestCreateInvite(t *testing.T) {
//	testUtil.InitTestConfig(testUtil.CloudPlatform)
//	inviteTeamID := "team_cuid"
//	createInviteResponseOK := astrocore.CreateTeamInviteResponse{
//		HTTPResponse: &http.Response{
//			StatusCode: 200,
//		},
//		JSON200: &astrocore.Invite{
//			InviteId: "",
//			TeamId:   &inviteTeamID,
//		},
//	}
//	errorBody, _ := json.Marshal(astrocore.Error{
//		Message: "failed to create invite: test-inv-error",
//	})
//	createInviteResponseError := astrocore.CreateTeamInviteResponse{
//		HTTPResponse: &http.Response{
//			StatusCode: 500,
//		},
//		Body:    errorBody,
//		JSON200: nil,
//	}
//	t.Run("happy path", func(t *testing.T) {
//		expectedOutMessage := "invite for test-email@test.com with role ORGANIZATION_MEMBER created\n"
//		createInviteRequest := astrocore.CreateTeamInviteRequest{
//			InviteeEmail: "test-email@test.com",
//			Role:         "ORGANIZATION_MEMBER",
//		}
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseOK, nil).Once()
//		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
//		assert.NoError(t, err)
//		assert.Equal(t, expectedOutMessage, out.String())
//	})
//
//	t.Run("error path when CreateTeamInviteWithResponse return network error", func(t *testing.T) {
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		createInviteRequest := astrocore.CreateTeamInviteRequest{
//			InviteeEmail: "test-email@test.com",
//			Role:         "ORGANIZATION_MEMBER",
//		}
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(nil, errorNetwork).Once()
//		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
//		assert.EqualError(t, err, "network error")
//	})
//
//	t.Run("error path when CreateTeamInviteWithResponse returns an error", func(t *testing.T) {
//		expectedOutMessage := "failed to create invite: test-inv-error"
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		createInviteRequest := astrocore.CreateTeamInviteRequest{
//			InviteeEmail: "test-email@test.com",
//			Role:         "ORGANIZATION_MEMBER",
//		}
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, createInviteRequest).Return(&createInviteResponseError, nil).Once()
//		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
//		assert.EqualError(t, err, expectedOutMessage)
//	})
//	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
//		expectedOutMessage := ""
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
//		err := CreateInvite("test-email@test.com", "test-role", out, mockClient)
//		assert.ErrorIs(t, err, ErrInvalidRole)
//		assert.Equal(t, expectedOutMessage, out.String())
//	})
//
//	t.Run("error path when no organization shortname found", func(t *testing.T) {
//		testUtil.InitTestConfig(testUtil.CloudPlatform)
//		c, err := config.GetCurrentContext()
//		assert.NoError(t, err)
//		err = c.SetContextKey("organization_short_name", "")
//		assert.NoError(t, err)
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
//		err = CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
//		assert.ErrorIs(t, err, ErrNoShortName)
//	})
//
//	t.Run("error path when getting current context returns an error", func(t *testing.T) {
//		testUtil.InitTestConfig(testUtil.Initial)
//		expectedOutMessage := ""
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
//		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", out, mockClient)
//		assert.Error(t, err)
//		assert.Equal(t, expectedOutMessage, out.String())
//	})
//	t.Run("error path when email is blank returns an error", func(t *testing.T) {
//		expectedOutMessage := ""
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseOK, nil).Once()
//		err := CreateInvite("", "test-role", out, mockClient)
//		assert.ErrorIs(t, err, ErrInvalidEmail)
//		assert.Equal(t, expectedOutMessage, out.String())
//	})
//	t.Run("error path when writing output returns an error", func(t *testing.T) {
//		testUtil.InitTestConfig(testUtil.CloudPlatform)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("CreateTeamInviteWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&createInviteResponseError, nil).Once()
//		err := CreateInvite("test-email@test.com", "ORGANIZATION_MEMBER", testWriter{Error: errorInvite}, mockClient)
//		assert.EqualError(t, err, "failed to create invite: test-inv-error")
//	})
//}

func TestListOrgTeam(t *testing.T) {
	t.Run("happy path TestListOrgTeam", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		err := ListOrgTeams(out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when ListOrganizationTeamsWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgTeams(out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListOrganizationTeamsWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseError, nil).Twice()
		err := ListOrgTeams(out, mockClient)
		assert.EqualError(t, err, "failed to list teams")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = ListOrgTeams(out, mockClient)
		assert.ErrorIs(t, err, ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListOrgTeams(out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestListWorkspaceTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path TestListWorkspaceTeam", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.NoError(t, err)
	})

	t.Run("error path when ListWorkspaceTeamsWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListWorkspaceTeamsWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseError, nil).Twice()
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.EqualError(t, err, "failed to list teams")
	})

	t.Run("error path when no Workspaceanization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = ListWorkspaceTeams(out, mockClient, "")
		assert.ErrorIs(t, err, ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestUpdateWorkspaceTeamRole(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path UpdateWorkspaceTeamRole", func(t *testing.T) {
		expectedOutMessage := "The workspace team team1-id role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseError, nil).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceTeamRole(team1.Id, "test-role", "", out, mockClient)
		assert.ErrorIs(t, err, user.ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.ErrorIs(t, err, ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateWorkspaceTeamRole no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The workspace team team1-id role was successfully updated to WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()

		err = UpdateWorkspaceTeamRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestAddWorkspaceTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path AddWorkspaceTeam", func(t *testing.T) {
		expectedOutMessage := "The team team1-id was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseError, nil).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})
	t.Run("error path when isValidRole returns an error", func(t *testing.T) {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceTeam(team1.Id, "test-role", "", out, mockClient)
		assert.ErrorIs(t, err, user.ErrInvalidWorkspaceRole)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.ErrorIs(t, err, ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddWorkspaceTeam no email passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The team team1-id was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()

		err = AddWorkspaceTeam("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

//func TestDeleteWorkspaceTeam(t *testing.T) {
//	testUtil.InitTestConfig(testUtil.CloudPlatform)
//	t.Run("happy path DeleteWorkspaceTeam", func(t *testing.T) {
//		expectedOutMessage := "The team team@1.com was successfully removed from the workspace\n"
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
//		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
//		err := RemoveWorkspaceTeam("team@1.com", "", out, mockClient)
//		assert.NoError(t, err)
//		assert.Equal(t, expectedOutMessage, out.String())
//	})
//
//	t.Run("error path when DeleteWorkspaceTeamWithResponse return network error", func(t *testing.T) {
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
//		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
//		err := RemoveWorkspaceTeam("team@1.com", "", out, mockClient)
//		assert.EqualError(t, err, "network error")
//	})
//
//	t.Run("error path when DeleteWorkspaceTeamWithResponse returns an error", func(t *testing.T) {
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
//		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseError, nil).Once()
//		err := RemoveWorkspaceTeam("team@1.com", "", out, mockClient)
//		assert.EqualError(t, err, "failed to update team")
//	})
//
//	t.Run("error path when no organization shortname found", func(t *testing.T) {
//		testUtil.InitTestConfig(testUtil.CloudPlatform)
//		c, err := config.GetCurrentContext()
//		assert.NoError(t, err)
//		err = c.SetContextKey("organization_short_name", "")
//		assert.NoError(t, err)
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		err = RemoveWorkspaceTeam("team@1.com", "", out, mockClient)
//		assert.ErrorIs(t, err, ErrNoShortName)
//	})
//
//	t.Run("error path when getting current context returns an error", func(t *testing.T) {
//		testUtil.InitTestConfig(testUtil.Initial)
//		expectedOutMessage := ""
//		out := new(bytes.Buffer)
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		err := RemoveWorkspaceTeam("team@1.com", "", out, mockClient)
//		assert.Error(t, err)
//		assert.Equal(t, expectedOutMessage, out.String())
//	})
//
//	t.Run("DeleteWorkspaceTeam no email passed", func(t *testing.T) {
//		testUtil.InitTestConfig(testUtil.CloudPlatform)
//		out := new(bytes.Buffer)
//
//		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
//		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
//		// mock os.Stdin
//		expectedInput := []byte("1")
//		r, w, err := os.Pipe()
//		assert.NoError(t, err)
//		_, err = w.Write(expectedInput)
//		assert.NoError(t, err)
//		w.Close()
//		stdin := os.Stdin
//		// Restore stdin right after the test.
//		defer func() { os.Stdin = stdin }()
//		os.Stdin = r
//
//		expectedOut := "The team team@1.com was successfully removed from the workspace\n"
//		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
//
//		err = RemoveWorkspaceTeam("", "", out, mockClient)
//		assert.NoError(t, err)
//		assert.Equal(t, expectedOut, out.String())
//	})
//}
