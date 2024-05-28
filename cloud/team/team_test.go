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
	"github.com/stretchr/testify/suite"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
}

func TestTeam(t *testing.T) {
	suite.Run(t, new(Suite))
}

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

// deployment teams variables

var (
	deploymentID                  = "ck05r3bor07h40d02y2hw4n4d"
	ListDeploymentTeamsResponseOK = astrocore.ListDeploymentTeamsResponse{
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
	ListDeploymentTeamsResponseEmpty = astrocore.ListDeploymentTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
		},
	}
	ListDeploymentTeamsResponseError = astrocore.ListDeploymentTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyList,
		JSON200: nil,
	}
	MutateDeploymentTeamRoleResponseOK = astrocore.MutateDeploymentTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.TeamRole{
			Role: "DEPLOYMENT_ADMIN",
		},
	}
	MutateDeploymentTeamRoleResponseError = astrocore.MutateDeploymentTeamRoleResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyUpdate,
		JSON200: nil,
	}
	DeleteDeploymentTeamResponseOK = astrocore.DeleteDeploymentTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteDeploymentTeamResponseError = astrocore.DeleteDeploymentTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
)

func (s *Suite) TestListOrgTeam() {
	s.Run("happy path TestListOrgTeam", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		err := ListOrgTeams(out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when ListOrganizationTeamsWithResponse return network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListOrgTeams(out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListOrganizationTeamsWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseError, nil).Twice()
		err := ListOrgTeams(out, mockClient)
		s.EqualError(err, "failed to list teams")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListOrgTeams(out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestListWorkspaceTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path TestListWorkspaceTeam", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		err := ListWorkspaceTeams(out, mockClient, "")
		s.NoError(err)
	})

	s.Run("error path when ListWorkspaceTeamsWithResponse return network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListWorkspaceTeams(out, mockClient, "")
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListWorkspaceTeamsWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseError, nil).Twice()
		err := ListWorkspaceTeams(out, mockClient, "")
		s.EqualError(err, "failed to list teams")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListWorkspaceTeams(out, mockClient, "")
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestUpdateWorkspaceTeamRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path UpdateWorkspaceTeamRole", func() {
		expectedOutMessage := "The workspace team team1-id role was successfully updated to WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path no workspace teams found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseEmpty, nil).Twice()
		err := UpdateWorkspaceTeamRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "no teams found in your workspace")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateWorkspaceTeamRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateWorkspaceTeamRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseError, nil).Once()
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "failed to update team")
	})
	s.Run("error path when isValidRole returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceTeamRole(team1.Id, "test-role", "", out, mockClient)
		s.ErrorIs(err, user.ErrInvalidWorkspaceRole)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("UpdateWorkspaceTeamRole no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The workspace team team1-id role was successfully updated to WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()

		err = UpdateWorkspaceTeamRole("", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestAddWorkspaceTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path AddWorkspaceTeam", func() {
		expectedOutMessage := "The team team1-id was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateWorkspaceTeamRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateWorkspaceTeamRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseError, nil).Once()
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.EqualError(err, "failed to update team")
	})
	s.Run("error path when isValidRole returns an error", func() {
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceTeam(team1.Id, "test-role", "", out, mockClient)
		s.ErrorIs(err, user.ErrInvalidWorkspaceRole)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("AddWorkspaceTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The team team1-id was successfully added to the workspace with the role WORKSPACE_MEMBER\n"
		mockClient.On("MutateWorkspaceTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateWorkspaceTeamRoleResponseOK, nil).Once()

		err = AddWorkspaceTeam("", "WORKSPACE_MEMBER", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestRemoveWorkspaceTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path DeleteWorkspaceTeam", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from workspace %s\n", team1.Name, workspaceID)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteWorkspaceTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteWorkspaceTeamWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseError, nil).Once()
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveWorkspaceTeam(team1.Id, "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("RemoveWorkspaceTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from workspace %s\n", team1.Name, workspaceID)
		mockClient.On("DeleteWorkspaceTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteWorkspaceTeamResponseOK, nil).Once()

		err = RemoveWorkspaceTeam("", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Delete", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		err := Delete(team1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path Delete with idp managed team", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		err := Delete(team2.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path user reject delete", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		err := Delete(team2.Id, out, mockClient)
		s.NoError(err)
		s.Equal("", out.String())
	})

	s.Run("error path no org teams found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := Delete("", out, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := Delete(team1.Id, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := Delete(team1.Id, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteTeamWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseError, nil).Once()
		err := Delete(team1.Id, out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := Delete(team1.Id, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
	s.Run("DeleteTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully deleted\n", team1.Name)
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()

		err = Delete("", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Update", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path Update - with role", func() {
		role := "ORGANIZATION_OWNER"
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\nAstro Team role %s was successfully updated to %s\n", team1.Name, team1.Name, role)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgTeamRoleResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("unhappy path Update - with invalid role", func() {
		role := "WORKSPACE_VIEWER"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		s.EqualError(err, "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	})

	s.Run("unhappy path Update - with role MutateOrgTeamRoleWithResponse error", func() {
		role := "ORGANIZATION_OWNER"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateOrgTeamRoleResponseError, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		s.EqualError(err, "failed to update team role")
	})

	s.Run("unhappy path Update - with role MutateOrgTeamRoleWithResponse network error", func() {
		role := "ORGANIZATION_OWNER"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("MutateOrgTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateTeam(team1.Id, "name", "description", role, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("happy path Update with idp managed team", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		err := UpdateTeam(team2.Id, "name", "description", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path user reject Update", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		err := UpdateTeam(team2.Id, "name", "description", "", out, mockClient)
		s.NoError(err)
		s.Equal("", out.String())
	})

	s.Run("happy path Update no description passed in", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "name", "", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path Update no name passed in", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		err := UpdateTeam(team1.Id, "", "description", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path no org teams found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := UpdateTeam("", "name", "description", "", out, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateTeam(team1.Id, "name", "description", "", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
	s.Run("UpdateTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully updated\n", team1.Name)
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()

		err = UpdateTeam("", "name", "description", "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Update", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully created\n", team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path no name passed so user types one in when prompted", func() {
		teamName := "Test Team Name"
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully created\n", teamName)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), teamName)()
		err := CreateTeam("", *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when CreateTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when CreateTeamWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseError, nil).Once()
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
	s.Run("error path no name passed in and user doesn't type one in when prompted", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam("", *team1.Description, "ORGANIZATION_MEMBER", out, mockClient)
		s.EqualError(err, "you must give your Team a name")
	})

	s.Run("error path invalid org role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := CreateTeam("", *team1.Description, "WORKSPACE_OWNER", out, mockClient)
		s.EqualError(err, "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	})
}

func (s *Suite) TestAddUser() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path AddUser", func() {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path AddUser with idp managed team", func() {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		err := AddUser(team2.Id, user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("user reject AddUser", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		err := AddUser(team2.Id, user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal("", out.String())
	})

	s.Run("error path no org teams found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := AddUser("", user1.Id, out, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path when AddTeamMembersWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when AddTeamMembersWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseError, nil).Once()
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		s.EqualError(err, "failed to update team membership")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddUser(team1.Id, user1.Id, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("AddUser no team-id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()

		err = AddUser("", user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
	s.Run("AddUser no user_id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully added to team %s \n", user1.Id, team1.Name)
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()

		err = AddUser(team1.Id, "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})

	s.Run("AddUser no user_id passed no org users found", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseEmpty, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = AddUser(team1.Id, "", out, mockClient)
		s.EqualError(err, "no users found in your organization")
	})
}

func (s *Suite) TestRemoveUser() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path RemoveUser", func() {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("happy path RemoveUser with idp managed team", func() {
		expectedOutMessage := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team2.Name)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		err := RemoveUser(team2.Id, user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("user reject RemoveUser with idp managed team", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		err := RemoveUser(team2.Id, user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal("", out.String())
	})

	s.Run("error path no org teams found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseEmpty, nil).Twice()
		err := RemoveUser("", user1.Id, out, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path no team members found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseEmptyMembership, nil).Twice()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		s.EqualError(err, "no team members found in team")
	})

	s.Run("error path when RemoveTeamMemberWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when RemoveTeamMemberWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseError, nil).Once()
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		s.EqualError(err, "failed to update team membership")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveUser(team1.Id, user1.Id, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("RemoveUser no team-id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()

		err = RemoveUser("", user1.Id, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
	s.Run("RemoveUser no user_id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("ListOrgUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrgUsersResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := fmt.Sprintf("Astro User %s was successfully removed from team %s \n", user1.Id, team1.Name)
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()

		err = RemoveUser(team1.Id, "", out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestListTeamUsers() {
	s.Run("happy path ListTeamUsers", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		err := ListTeamUsers(team1.Id, out, mockClient)
		s.NoError(err)
	})

	s.Run("happy path ListTeamUsers team with no membership", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseEmptyMembership, nil).Twice()
		err := ListTeamUsers(team1.Id, out, mockClient)
		s.NoError(err)
	})

	s.Run("error path when GetTeamWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseError, nil).Twice()

		err := ListTeamUsers(team1.Id, out, mockClient)
		s.EqualError(err, "failed to get team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListTeamUsers(team1.Id, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("ListTeamUsers no team-id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()

		// mock os.Stdin
		expectedInput := []byte("1")

		// select team
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()

		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		err = ListTeamUsers("", out, mockClient)
		s.NoError(err)
	})
}

func (s *Suite) TestGetTeam() {
	s.Run("happy path GetTeam", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		_, err := GetTeam(mockClient, team1.Id)
		s.NoError(err)
	})

	s.Run("error path when GetTeamWithResponse returns a network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()

		_, err := GetTeam(mockClient, team1.Id)
		s.EqualError(err, "network error")
	})

	s.Run("error path when GetTeamWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseError, nil).Twice()

		_, err := GetTeam(mockClient, team1.Id)
		s.EqualError(err, "failed to get team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := GetTeam(mockClient, team1.Id)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestGetWorkspaceTeams() {
	s.Run("happy path get WorkspaceTeams pulls workspace from context", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListWorkspaceTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspaceTeamsResponseOK, nil).Twice()
		_, err := GetWorkspaceTeams(mockClient, "", 10)
		s.NoError(err)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := GetWorkspaceTeams(mockClient, "", 10)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestListDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path TestListDeploymentTeam", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseOK, nil).Twice()
		err := ListDeploymentTeams(out, mockClient, deploymentID)
		s.NoError(err)
	})

	s.Run("error path when ListDeploymentTeamsWithResponse return network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := ListDeploymentTeams(out, mockClient, deploymentID)
		s.EqualError(err, "network error")
	})

	s.Run("error path when ListDeploymentTeamsWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseError, nil).Twice()
		err := ListDeploymentTeams(out, mockClient, deploymentID)
		s.EqualError(err, "failed to list teams")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := ListDeploymentTeams(out, mockClient, deploymentID)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestUpdateDeploymentTeamRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path UpdateDeploymentTeamRole", func() {
		expectedOutMessage := "The deployment team team1-id role was successfully updated to DEPLOYMENT_ADMIN\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path no deployment teams found", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseEmpty, nil).Twice()
		err := UpdateDeploymentTeamRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "no teams found in your deployment")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateDeploymentTeamRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateDeploymentTeamRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseError, nil).Once()
		err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("UpdateDeploymentTeamRole no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The deployment team team1-id role was successfully updated to DEPLOYMENT_ADMIN\n"
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()

		err = UpdateDeploymentTeamRole("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestAddDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path AddDeploymentTeam", func() {
		expectedOutMessage := "The team team1-id was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()
		err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateDeploymentTeamRoleWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when MutateDeploymentTeamRoleWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseError, nil).Once()
		err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("AddDeploymentTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListOrganizationTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOut := "The team team1-id was successfully added to the deployment with the role DEPLOYMENT_ADMIN\n"
		mockClient.On("MutateDeploymentTeamRoleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&MutateDeploymentTeamRoleResponseOK, nil).Once()

		err = AddDeploymentTeam("", "DEPLOYMENT_ADMIN", deploymentID, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOut, out.String())
	})
}

func (s *Suite) TestRemoveDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path DeleteDeploymentTeam", func() {
		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from deployment %s\n", team1.Name, deploymentID)
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseOK, nil).Once()
		err := RemoveDeploymentTeam(team1.Id, deploymentID, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Twice()
		err := RemoveDeploymentTeam(team1.Id, deploymentID, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteDeploymentTeamWithResponse return network error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		err := RemoveDeploymentTeam(team1.Id, deploymentID, out, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteDeploymentTeamWithResponse returns an error", func() {
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Twice()
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseError, nil).Once()
		err := RemoveDeploymentTeam(team1.Id, deploymentID, out, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		err := RemoveDeploymentTeam(team1.Id, deploymentID, out, mockClient)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})

	s.Run("RemoveDeploymentTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		out := new(bytes.Buffer)

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseOK, nil).Twice()
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		expectedOutMessage := fmt.Sprintf("Astro Team %s was successfully removed from deployment %s\n", team1.Name, deploymentID)
		mockClient.On("DeleteDeploymentTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&DeleteDeploymentTeamResponseOK, nil).Once()

		err = RemoveDeploymentTeam("", deploymentID, out, mockClient)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestGetDeploymentTeams() {
	s.Run("happy path get DeploymentTeams pulls deployment from context", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListDeploymentTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListDeploymentTeamsResponseOK, nil).Twice()
		_, err := GetDeploymentTeams(mockClient, deploymentID, 10)
		s.NoError(err)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		_, err := GetDeploymentTeams(mockClient, deploymentID, 10)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}
