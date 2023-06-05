package team

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/cloud/user"

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
	orgRole      = "ORGANIZATION_MEMBER"
	description  = "mock description"
	user1        = astrocore.User{
		CreatedAt: time.Now(),
		FullName:  "user 1",
		Id:        "user1-id",
		OrgRole:   &orgRole,
		Username:  "user@1.com",
	}
	teamMembers = []astrocore.TeamMember{{
		UserId:   user1.Id,
		Username: user1.Username,
		FullName: &user1.FullName,
	}}
	team1 = astrocore.Team{
		CreatedAt:   time.Now(),
		Name:        "team 1",
		Description: &description,
		Id:          "team1-id",
		Members:     &teamMembers,
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
	ListOrganizationTeamsResponseEmpty = astrocore.ListOrganizationTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
		},
	}
	ListOrganizationTeamsResponseEmptyMembership = astrocore.ListOrganizationTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams: []astrocore.Team{
				{Id: team1.Id},
			},
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
	workspaceID   = "ck05r3bor07h40d02y2hw4n4v"
	roles         = []astrocore.TeamRole{
		{EntityType: "WORKSPACE", EntityId: workspaceID, Role: workspaceRole},
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
	ListWorkspaceTeamsResponseEmpty = astrocore.ListWorkspaceTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
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
	DeleteWorkspaceTeamResponseOK = astrocore.DeleteWorkspaceTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteWorkspaceTeamResponseError = astrocore.DeleteWorkspaceTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	DeleteOrganizationTeamResponseOK = astrocore.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteOrganizationTeamResponseError = astrocore.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	UpdateTeamResponseOK = astrocore.UpdateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	UpdateTeamResponseError = astrocore.UpdateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	CreateTeamResponseOK = astrocore.CreateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &workspaceTeam1,
	}
	CreateTeamResponseError = astrocore.CreateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	users = []astrocore.User{
		user1,
	}
	ListOrgUsersResponseOK = astrocore.ListOrgUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      users,
		},
	}
	ListOrgUsersResponseEmpty = astrocore.ListOrgUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
		},
	}
	errorBodyUpdateTeamMembership, _ = json.Marshal(astrocore.Error{
		Message: "failed to update team membership",
	})
	AddTeamMemberResponseOK = astrocore.AddTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	AddTeamMemberResponseError = astrocore.AddTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateTeamMembership,
	}
	RemoveTeamMemberResponseOK = astrocore.RemoveTeamMemberResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	RemoveTeamMemberResponseError = astrocore.RemoveTeamMemberResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateTeamMembership,
	}
)

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

	t.Run("error path no workspace teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseEmpty, nil).Twice()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "no teams found in your workspace")
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
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
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

func TestRemoveWorkspaceTeam(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path DeleteWorkspaceTeam", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from workspace %s\n", team1.Name, workspaceID)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when DeleteWorkspaceTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteWorkspaceTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseError, nil).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path Delete", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		err := Delete(team1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path when DeleteTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseError, nil).Once()
		err := Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Delete(team1.Id, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path Update", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := UpdateTeam(team1.Id, "name", "description", out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path when UpdateTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateTeam(team1.Id, "name", "description", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = UpdateTeam(team1.Id, "name", "description", out, mockClient)
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateTeam(team1.Id, "name", "description", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path Update", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully created\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		err := CreateTeam(team1.Name, *team1.Description, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := CreateTeam(team1.Name, *team1.Description, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseError, nil).Once()
		err := CreateTeam(team1.Name, *team1.Description, out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = CreateTeam(team1.Name, *team1.Description, out, mockClient)
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam(team1.Name, *team1.Description, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestAddUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path AddUser", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path no org users found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseEmpty, nil).Twice()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "no users found in your organization")
	})

	t.Run("error path when AddTeamMembersWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when AddTeamMembersWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseError, nil).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to update team membership")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddUser no team_id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()

		err = AddUser("", user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
	t.Run("AddUser no user_id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()

		err = AddUser(team1.Id, "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestRemoveUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("happy path RemoveUser", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path no team members found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmptyMembership, nil).Twice()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "no team members found in team")
	})

	t.Run("error path when RemoveTeamMemberWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when RemoveTeamMemberWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseError, nil).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to update team membership")
	})

	t.Run("error path when no organization shortname found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		c, err := config.GetCurrentContext()
		assert.NoError(t, err)
		err = c.SetContextKey("organization_short_name", "")
		assert.NoError(t, err)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err = RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "cannot retrieve organization short name from context")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("RemoveUser no team_id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()

		err = RemoveUser("", user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
	t.Run("RemoveUser no user_id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()

		err = RemoveUser(team1.Id, "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOut, out.String())
	})
}

func TestListTeamUsers(t *testing.T) {
	t.Run("happy path ListTeamUsers", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		err := ListTeamUsers(team1.Id, out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when ListOrganizationTeamsWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListTeamUsers(team1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListOrganizationTeamsWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseError, nil).Twice()
		err := ListTeamUsers(team1.Id, out, mockClient)
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
		err = ListTeamUsers(team1.Id, out, mockClient)
		assert.ErrorIs(t, err, ErrNoShortName)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTeamUsers(team1.Id, out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}
