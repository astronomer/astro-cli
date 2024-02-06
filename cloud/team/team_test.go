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
	team2 = astrocore.Team{
		CreatedAt:    time.Now(),
		Name:         "team 2",
		Description:  &description,
		Id:           "team2-id",
		Members:      &teamMembers,
		IsIdpManaged: true,
	}
	teams = []astrocore.Team{
		team1,
		team2,
	}
	GetUserWithResponseOK = astrocore.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &user1,
	}
	GetTeamWithResponseOK = astrocore.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
	}
	GetIDPManagedTeamWithResponseOK = astrocore.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team2,
	}
	errorBodyGet, _ = json.Marshal(astrocore.Error{
		Message: "failed to get team",
	})
	GetTeamWithResponseError = astrocore.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyGet,
		JSON200: nil,
	}
	emptyMembershipTeam = astrocore.Team{
		Id: team1.Id,
	}
	GetTeamWithResponseEmptyMembership = astrocore.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &emptyMembershipTeam,
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
	errorBodyUpdateRole, _ = json.Marshal(astrocore.Error{
		Message: "failed to update team role",
	})
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
	MutateOrgTeamRoleResponseOK = astrocore.MutateOrgTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	MutateOrgTeamRoleResponseError = astrocore.MutateOrgTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateRole,
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
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		err := ListOrgTeams(out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when ListOrganizationTeamsWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgTeams(out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListOrganizationTeamsWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseError, nil).Twice()
		err := ListOrgTeams(out, mockClient)
		assert.EqualError(t, err, "failed to list teams")
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path TestListWorkspaceTeam", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.NoError(t, err)
	})

	t.Run("error path when ListWorkspaceTeamsWithResponse return network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when ListWorkspaceTeamsWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseError, nil).Twice()
		err := ListWorkspaceTeams(out, mockClient, "")
		assert.EqualError(t, err, "failed to list teams")
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path UpdateWorkspaceTeamRole", func(t *testing.T) {
		expectedOutMessage := "The workspace team team1-id role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no workspace teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseEmpty, nil).Twice()
		err := UpdateWorkspaceTeamRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "no teams found in your workspace")
	})

	t.Run("error path when GetTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
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

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("UpdateWorkspaceTeamRole no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddWorkspaceTeam", func(t *testing.T) {
		expectedOutMessage := "The team team1-id was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when GetTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when MutateWorkspaceTeamRoleWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
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

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("AddWorkspaceTeam no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path DeleteWorkspaceTeam", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from workspace %s\n", team1.Name, workspaceID)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when GetTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteWorkspaceTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteWorkspaceTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseError, nil).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
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

	t.Run("RemoveWorkspaceTeam no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
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

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from workspace %s\n", team1.Name, workspaceID)
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()

		err = RemoveWorkspaceTeam("", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Delete", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		err := Delete(team1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path Delete with idp managed team", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "y")()
		err := Delete(team2.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path user reject delete", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		err := Delete(team2.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "", out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := Delete("", out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path when GetTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when DeleteTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseError, nil).Once()
		err := Delete(team1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to update team")
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
	t.Run("DeleteTeam no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
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

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()

		err = Delete("", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Update", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path Update - with role", func(t *testing.T) {
		role := "ORGANIZATION_OWNER"
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\nAstro Team role %s was successfully updated to %s\n", team1.Name, team1.Name, role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgTeamRoleResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("unhappy path Update - with invalid role", func(t *testing.T) {
		role := "WORKSPACE_VIEWER"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		assert.EqualError(t, err, "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	})

	t.Run("unhappy path Update - with role MutateOrgTeamRoleWithResponse error", func(t *testing.T) {
		role := "ORGANIZATION_OWNER"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgTeamRoleResponseError, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		assert.EqualError(t, err, "failed to update team role")
	})

	t.Run("unhappy path Update - with role MutateOrgTeamRoleWithResponse network error", func(t *testing.T) {
		role := "ORGANIZATION_OWNER"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("happy path Update with idp managed team", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "y")()
		err := UpdateTeam(team2.Id, "name", "description", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path user reject Update", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		err := UpdateTeam(team2.Id, "name", "description", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "", out.String())
	})

	t.Run("happy path Update no description passed in", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path Update no name passed in", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "", "description", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := UpdateTeam("", "name", "description", "", out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path when GetTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when UpdateTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("UpdateTeam no id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
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

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()

		err = UpdateTeam("", "name", "description", "", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path Update", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully created\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path no name passed so user types one in when prompted", func(t *testing.T) {
		teamName := "Test Team Name"
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully created\n", teamName)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, teamName)()
		err := CreateTeam("", *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("error path when CreateTeamWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when CreateTeamWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseError, nil).Once()
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "failed to update team")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
	t.Run("error path no name passed in and user doesn't type one in when prompted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam("", *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		assert.EqualError(t, err, "you must give your Team a name")
	})

	t.Run("error path invalid org role", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam("", *team1.Description, "WORKSPACE_OWNER", out, mockClient)
		assert.EqualError(t, err, "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	})
}

func TestAddUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path AddUser", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path AddUser with idp managed team", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "y")()
		err := AddUser(team2.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("user reject AddUser", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		err := AddUser(team2.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "", out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := AddUser("", user1.Id, out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path when AddTeamMembersWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when AddTeamMembersWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseError, nil).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to update team membership")
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

	t.Run("AddUser no team-id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()

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
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
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

	t.Run("AddUser no user_id passed no org users found", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseEmpty, nil).Twice()

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

		err = AddUser(team1.Id, "", out, mockClient)
		assert.EqualError(t, err, "no users found in your organization")
	})
}

func TestRemoveUser(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("happy path RemoveUser", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("happy path RemoveUser with idp managed team", func(t *testing.T) {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "y")()
		err := RemoveUser(team2.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})

	t.Run("user reject RemoveUser with idp managed team", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		err := RemoveUser(team2.Id, user1.Id, out, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, "", out.String())
	})

	t.Run("error path no org teams found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := RemoveUser("", user1.Id, out, mockClient)
		assert.EqualError(t, err, "no teams found in your organization")
	})

	t.Run("error path no team members found", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseEmptyMembership, nil).Twice()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "no team members found in team")
	})

	t.Run("error path when RemoveTeamMemberWithResponse return network error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when RemoveTeamMemberWithResponse returns an error", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseError, nil).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to update team membership")
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

	t.Run("RemoveUser no team-id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
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
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		err := ListTeamUsers(team1.Id, out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("happy path ListTeamUsers team with no membership", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseEmptyMembership, nil).Twice()
		err := ListTeamUsers(team1.Id, out, mockClient)
		assert.NoError(t, err)
	})

	t.Run("error path when GetTeamWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseError, nil).Twice()

		err := ListTeamUsers(team1.Id, out, mockClient)
		assert.EqualError(t, err, "failed to get team")
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

	t.Run("ListTeamUsers no team-id passed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()

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

		err = ListTeamUsers("", out, mockClient)
		assert.NoError(t, err)
	})
}

func TestGetTeam(t *testing.T) {
	t.Run("happy path GetTeam", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		_, err := GetTeam(mockClient, team1.Id)
		assert.NoError(t, err)
	})

	t.Run("error path when GetTeamWithResponse returns a network error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()

		_, err := GetTeam(mockClient, team1.Id)
		assert.EqualError(t, err, "network error")
	})

	t.Run("error path when GetTeamWithResponse returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseError, nil).Twice()

		_, err := GetTeam(mockClient, team1.Id)
		assert.EqualError(t, err, "failed to get team")
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := GetTeam(mockClient, team1.Id)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}

func TestGetWorkspaceTeams(t *testing.T) {
	t.Run("happy path get WorkspaceTeams pulls workspace from context", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		_, err := GetWorkspaceTeams(mockClient, "", 10)
		assert.NoError(t, err)
	})

	t.Run("error path when getting current context returns an error", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := GetWorkspaceTeams(mockClient, "", 10)
		assert.Error(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
	})
}
