package team

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/pkg/input"
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
	errorNetwork  = errors.New("network error")
	userOrgRole   = astrov1.UserOrganizationRole("ORGANIZATION_MEMBER")
	description   = "mock description"
	user1FullName = "user 1"
	user1         = astrov1.User{
		CreatedAt:        time.Now(),
		FullName:         user1FullName,
		Id:               "user1-id",
		OrganizationRole: &userOrgRole,
		Username:         "user@1.com",
	}
	teamMembers = []astrov1.TeamMember{{
		UserId:   user1.Id,
		Username: user1.Username,
		FullName: &user1.FullName,
	}}
	workspaceRole       = "WORKSPACE_MEMBER"
	workspaceID         = "ck05r3bor07h40d02y2hw4n4v"
	team1WorkspaceRoles = []astrov1.WorkspaceRole{
		{WorkspaceId: workspaceID, Role: astrov1.WorkspaceRoleRole(workspaceRole)},
	}
	team1 = astrov1.Team{
		CreatedAt:        time.Now(),
		Name:             "team 1",
		Description:      &description,
		Id:               "team1-id",
		OrganizationRole: astrov1.TeamOrganizationRole("ORGANIZATION_MEMBER"),
		WorkspaceRoles:   &team1WorkspaceRoles,
	}
	team2 = astrov1.Team{
		CreatedAt:        time.Now(),
		Name:             "team 2",
		Description:      &description,
		Id:               "team2-id",
		OrganizationRole: astrov1.TeamOrganizationRole("ORGANIZATION_MEMBER"),
		IsIdpManaged:     true,
	}
	teams = []astrov1.Team{
		team1,
		team2,
	}
	GetUserWithResponseOK = astrov1.GetUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &user1,
	}
	GetTeamWithResponseOK = astrov1.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
	}
	GetIDPManagedTeamWithResponseOK = astrov1.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team2,
	}
	errorBodyGet, _ = json.Marshal(astrov1.Error{
		Message: "failed to get team",
	})
	GetTeamWithResponseError = astrov1.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBodyGet,
		JSON200: nil,
	}
	emptyMembershipTeam = astrov1.Team{
		Id:               team1.Id,
		Name:             team1.Name,
		OrganizationRole: astrov1.TeamOrganizationRole("ORGANIZATION_MEMBER"),
	}
	GetTeamWithResponseEmptyMembership = astrov1.GetTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &emptyMembershipTeam,
	}
	ListTeamsResponseOK = astrov1.ListTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      teams,
		},
	}
	ListTeamsResponseEmpty = astrov1.ListTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
		},
	}
	ListTeamsWorkspaceResponseOK = astrov1.ListTeamsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.TeamsPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Teams:      []astrov1.Team{team1},
		},
	}
	errorBodyUpdateRole, _ = json.Marshal(astrov1.Error{
		Message: "failed to update team role",
	})
	errorBodyUpdate, _ = json.Marshal(astrov1.Error{
		Message: "failed to update team",
	})
	UpdateTeamRolesResponseOK = astrov1.UpdateTeamRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	UpdateTeamRolesResponseError = astrov1.UpdateTeamRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	UpdateTeamRolesResponseRoleError = astrov1.UpdateTeamRolesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateRole,
	}
	DeleteOrganizationTeamResponseOK = astrov1.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	DeleteOrganizationTeamResponseError = astrov1.DeleteTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	UpdateTeamResponseOK = astrov1.UpdateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	UpdateTeamResponseError = astrov1.UpdateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	CreateTeamResponseOK = astrov1.CreateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &team1,
	}
	CreateTeamResponseError = astrov1.CreateTeamResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdate,
	}
	usersList = []astrov1.User{
		user1,
	}
	ListUsersResponseOK = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Users:      usersList,
		},
	}
	ListUsersResponseEmpty = astrov1.ListUsersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.UsersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
		},
	}
	errorBodyUpdateTeamMembership, _ = json.Marshal(astrov1.Error{
		Message: "failed to update team membership",
	})
	AddTeamMemberResponseOK = astrov1.AddTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	AddTeamMemberResponseError = astrov1.AddTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateTeamMembership,
	}
	RemoveTeamMemberResponseOK = astrov1.RemoveTeamMemberResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	RemoveTeamMemberResponseError = astrov1.RemoveTeamMemberResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body: errorBodyUpdateTeamMembership,
	}
	ListTeamMembersResponseOK = astrov1.ListTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.TeamMembersPaginated{
			Limit:       1,
			Offset:      0,
			TotalCount:  1,
			TeamMembers: teamMembers,
		},
	}
	ListTeamMembersResponseEmpty = astrov1.ListTeamMembersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.TeamMembersPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 0,
		},
	}
)

// deployment teams variables
var (
	deploymentID = "ck05r3bor07h40d02y2hw4n4d"
)

func (s *Suite) TestListOrgTeamsData() {
	s.Run("returns structured team data", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()

		data, err := ListOrgTeamsData(mockClient)
		s.NoError(err)
		s.NotEmpty(data.Teams)
		s.Equal("team 1", data.Teams[0].Name)
		s.Equal("team1-id", data.Teams[0].ID)
	})

	s.Run("returns error on failure", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()

		_, err := ListOrgTeamsData(mockClient)
		s.Error(err)
	})
}

func (s *Suite) TestListOrgTeamsWithFormat() {
	s.Run("json output", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()

		buf := new(bytes.Buffer)
		err := ListOrgTeamsWithFormat(mockClient, testUtil.Renderer{JSON: true, Out: buf})
		s.NoError(err)

		var result TeamList
		s.NoError(json.Unmarshal(buf.Bytes(), &result))
		s.NotEmpty(result.Teams)
		s.Equal("team 1", result.Teams[0].Name)
	})
}

func (s *Suite) TestUpdateWorkspaceTeamRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path UpdateWorkspaceTeamRole", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.NoError(err)
		s.Equal(team1.Id, got.ID)
		s.Equal("WORKSPACE_MEMBER", got.WorkspaceRole)
	})

	s.Run("error path no workspace teams found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseEmpty, nil).Once()
		_, err := UpdateWorkspaceTeamRole("", "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "no teams found in your workspace")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseError, nil).Once()
		_, err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "failed to update team")
	})
	s.Run("error path when isValidRole returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateWorkspaceTeamRole(team1.Id, "test-role", "", mockClient)
		s.ErrorIs(err, user.ErrInvalidWorkspaceRole)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateWorkspaceTeamRole(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.Error(err)
	})

	s.Run("UpdateWorkspaceTeamRole no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsWorkspaceResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()

		got, err := UpdateWorkspaceTeamRole("", "WORKSPACE_MEMBER", "", mockClient)
		s.NoError(err)
		s.Equal(team1.Id, got.ID)
		s.Equal("WORKSPACE_MEMBER", got.WorkspaceRole)
	})
}

func (s *Suite) TestAddWorkspaceTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path AddWorkspaceTeam", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.NoError(err)
		s.Equal(team1.Id, got.ID)
		s.Equal("WORKSPACE_MEMBER", got.WorkspaceRole)
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseError, nil).Once()
		_, err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.EqualError(err, "failed to update team")
	})
	s.Run("error path when isValidRole returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := AddWorkspaceTeam(team1.Id, "test-role", "", mockClient)
		s.ErrorIs(err, user.ErrInvalidWorkspaceRole)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := AddWorkspaceTeam(team1.Id, "WORKSPACE_MEMBER", "", mockClient)
		s.Error(err)
	})

	s.Run("AddWorkspaceTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()

		got, err := AddWorkspaceTeam("", "WORKSPACE_MEMBER", "", mockClient)
		s.NoError(err)
		s.Equal(team1.Id, got.ID)
		s.Equal("WORKSPACE_MEMBER", got.WorkspaceRole)
	})
}

func (s *Suite) TestRemoveWorkspaceTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path DeleteWorkspaceTeam", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := RemoveWorkspaceTeam(team1.Id, "", mockClient)
		s.NoError(err)
		s.Equal(WorkspaceRemoval{ID: team1.Id, Name: team1.Name, WorkspaceID: workspaceID, Action: Removed}, got)
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveWorkspaceTeam(team1.Id, "", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveWorkspaceTeam(team1.Id, "", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseError, nil).Once()
		_, err := RemoveWorkspaceTeam(team1.Id, "", mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := RemoveWorkspaceTeam(team1.Id, "", mockClient)
		s.Error(err)
	})

	s.Run("RemoveWorkspaceTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsWorkspaceResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()

		got, err := RemoveWorkspaceTeam("", "", mockClient)
		s.NoError(err)
		s.Equal(WorkspaceRemoval{ID: team1.Id, Name: team1.Name, WorkspaceID: workspaceID, Action: Removed}, got)
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Delete", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		got, err := Delete(team1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(team1.Id, got.ID)
		s.Equal(team1.Name, got.Name)
		s.Equal(Deleted, got.Action)
	})

	s.Run("happy path Delete with idp managed team", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		got, err := Delete(team2.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(team2.Id, got.ID)
		s.Equal(team2.Name, got.Name)
		s.Equal(Deleted, got.Action)
	})

	s.Run("happy path Delete with idp managed team using force flag", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()
		got, err := Delete(team2.Id, true, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(team2.Id, got.ID)
		s.Equal(team2.Name, got.Name)
		s.Equal(Deleted, got.Action)
	})

	s.Run("error path user reject delete", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		got, err := Delete(team2.Id, false, mockClient)
		s.NoError(err)
		s.Nil(got, "declined, so nothing to report")
	})

	s.Run("a picker that may not ask returns its refusal", func() {
		// Not "invalid team selection": the refusal names what answers it.
		restore := input.SetGuard(func() string { return "with --output json it cannot" })
		defer restore()
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
		got, err := Delete("", false, mockClient)
		s.True(input.IsRequired(err), "got %v", err)
		s.Nil(got)
	})

	s.Run("error path no org teams found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseEmpty, nil).Once()
		_, err := Delete("", false, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := Delete(team1.Id, false, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := Delete(team1.Id, false, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when DeleteTeamWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseError, nil).Once()
		_, err := Delete(team1.Id, false, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := Delete(team1.Id, false, mockClient)
		s.Error(err)
	})
	s.Run("DeleteTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
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

		mockClient.On("DeleteTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&DeleteOrganizationTeamResponseOK, nil).Once()

		got, err := Delete("", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(team1.Id, got.ID)
		s.Equal(team1.Name, got.Name)
		s.Equal(Deleted, got.Action)
	})
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Update", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		got, err := UpdateTeam(team1.Id, "name", "description", "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal("name", got.Team.Name, "the team as the update left it")
		s.False(got.RoleChanged)
	})

	s.Run("happy path Update - with role", func() {
		role := "ORGANIZATION_OWNER"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := UpdateTeam(team1.Id, "name", "description", role, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal("name", got.Team.Name)
		s.True(got.RoleChanged)
		s.Equal(role, got.Team.OrgRole)
	})

	s.Run("unhappy path Update - with invalid role is refused before any request", func() {
		role := "WORKSPACE_VIEWER"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateTeam(team1.Id, "name", "description", role, false, mockClient)
		s.EqualError(err, "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
		s.Empty(mockClient.Calls, "neither the lookup nor the rename is sent")
	})

	s.Run("unhappy path Update - a role the API refuses leaves the name as it was", func() {
		role := "ORGANIZATION_OWNER"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Maybe()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseRoleError, nil).Once()
		_, err := UpdateTeam(team1.Id, "name", "description", role, false, mockClient)
		s.EqualError(err, "failed to update team role")
		mockClient.AssertNotCalled(s.T(), "UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("Update with only a role sends no rename", func() {
		role := "ORGANIZATION_OWNER"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Maybe()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := UpdateTeam(team1.Id, "", "", role, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.True(got.RoleChanged)
		s.Equal(role, got.Team.OrgRole)
		s.Equal(team1.Name, got.Team.Name)
		mockClient.AssertNotCalled(s.T(), "UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("a rename that fails after the role went through says so", func() {
		role := "ORGANIZATION_OWNER"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		got, err := UpdateTeam(team1.Id, "name", "", role, false, mockClient)
		s.EqualError(err, "the team's role was updated to ORGANIZATION_OWNER, but updating its name and description failed: failed to update team")
		s.Require().NotNil(got, "the team as it now is")
		s.True(got.RoleChanged)
		s.Equal(role, got.Team.OrgRole)
		s.Equal(team1.Name, got.Team.Name, "not renamed")
	})

	s.Run("unhappy path Update - with role UpdateTeamRolesWithResponse network error", func() {
		role := "ORGANIZATION_OWNER"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Maybe()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateTeam(team1.Id, "name", "description", role, false, mockClient)
		s.EqualError(err, "network error")
		mockClient.AssertNotCalled(s.T(), "UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("happy path Update with idp managed team", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		got, err := UpdateTeam(team2.Id, "name", "description", "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal("name", got.Team.Name)
		s.False(got.RoleChanged)
	})

	s.Run("happy path Update with idp managed team using force flag", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		got, err := UpdateTeam(team2.Id, "name", "description", "", true, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal("name", got.Team.Name)
		s.False(got.RoleChanged)
	})

	s.Run("happy path user reject Update", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		got, err := UpdateTeam(team2.Id, "name", "description", "", false, mockClient)
		s.NoError(err)
		s.Nil(got, "declined, so nothing to report")
	})

	s.Run("happy path Update no description passed in", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		got, err := UpdateTeam(team1.Id, "name", "", "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal("name", got.Team.Name)
		s.False(got.RoleChanged)
	})

	s.Run("happy path Update no name passed in", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()
		got, err := UpdateTeam(team1.Id, "", "description", "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(team1.Name, got.Team.Name, "no --name keeps the name")
		s.False(got.RoleChanged)
	})

	s.Run("error path no org teams found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseEmpty, nil).Once()
		_, err := UpdateTeam("", "name", "description", "", false, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateTeam(team1.Id, "name", "description", "", false, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateTeam(team1.Id, "name", "description", "", false, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseError, nil).Once()
		_, err := UpdateTeam(team1.Id, "name", "description", "", false, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateTeam(team1.Id, "name", "description", "", false, mockClient)
		s.Error(err)
	})
	s.Run("UpdateTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamResponseOK, nil).Once()

		got, err := UpdateTeam("", "name", "description", "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal("name", got.Team.Name)
		s.False(got.RoleChanged)
	})
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path Update", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseOK, nil).Once()
		got, err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", mockClient)
		s.NoError(err)
		s.Equal(team1.Name, got.Name)
		s.Equal("ORGANIZATION_MEMBER", got.OrgRole)
	})

	s.Run("happy path no name passed so user types one in when prompted", func() {
		teamName := "Test Team Name"
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		// The name typed is the name sent; the team returned is what the
		// API answered.
		named := mock.MatchedBy(func(r astrov1.CreateTeamJSONRequestBody) bool { return r.Name == teamName })
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, named).Return(&CreateTeamResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), teamName)()
		got, err := CreateTeam("", *team1.Description, "ORGANIZATION_MEMBER", mockClient)
		s.NoError(err)
		s.Equal(team1.Id, got.ID)
		s.Equal("ORGANIZATION_MEMBER", got.OrgRole)
	})

	s.Run("error path when CreateTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when CreateTeamWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("CreateTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&CreateTeamResponseError, nil).Once()
		_, err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := CreateTeam(team1.Name, *team1.Description, "ORGANIZATION_MEMBER", mockClient)
		s.Error(err)
	})
	s.Run("error path no name passed in and user doesn't type one in when prompted", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := CreateTeam("", *team1.Description, "ORGANIZATION_MEMBER", mockClient)
		s.EqualError(err, "you must give your Team a name")
	})

	s.Run("error path invalid org role", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := CreateTeam("", *team1.Description, "WORKSPACE_OWNER", mockClient)
		s.EqualError(err, "requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	})
}

func (s *Suite) TestAddUser() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path AddUser", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		got, err := AddUser(team1.Id, user1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team1.Name, got.TeamName)
		s.Equal(Added, got.Action)
	})

	s.Run("happy path AddUser with idp managed team", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		got, err := AddUser(team2.Id, user1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team2.Name, got.TeamName)
		s.Equal(Added, got.Action)
	})

	s.Run("happy path AddUser with idp managed team using force flag", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()
		got, err := AddUser(team2.Id, user1.Id, true, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team2.Name, got.TeamName)
		s.Equal(Added, got.Action)
	})

	s.Run("user reject AddUser", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		got, err := AddUser(team2.Id, user1.Id, false, mockClient)
		s.NoError(err)
		s.Nil(got, "declined, so nothing to report")
	})

	s.Run("error path no org teams found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseEmpty, nil).Once()
		_, err := AddUser("", user1.Id, false, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path when AddTeamMembersWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddUser(team1.Id, user1.Id, false, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when AddTeamMembersWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()
		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseError, nil).Once()
		_, err := AddUser(team1.Id, user1.Id, false, mockClient)
		s.EqualError(err, "failed to update team membership")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := AddUser(team1.Id, user1.Id, false, mockClient)
		s.Error(err)
	})

	s.Run("AddUser no team-id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
		mockClient.On("GetUserWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetUserWithResponseOK, nil).Once()

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

		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()

		got, err := AddUser("", user1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team1.Name, got.TeamName)
		s.Equal(Added, got.Action)
	})
	s.Run("AddUser refuses a picked team with no ID", func() {
		// It used to return nil here, so the command exited 0 having added
		// no one.
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		noID := team1
		noID.Id = ""
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListTeamsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.TeamsPaginated{Limit: 1, TotalCount: 1, Teams: []astrov1.Team{noID}},
		}, nil).Once()
		defer testUtil.MockUserInput(s.T(), "1")()

		got, err := AddUser("", user1.Id, false, mockClient)
		s.ErrorIs(err, ErrInvalidTeamKey)
		s.Nil(got)
		mockClient.AssertNotCalled(s.T(), "AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
	s.Run("AddUser no user_id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListUsersResponseOK, nil).Once()

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

		mockClient.On("AddTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&AddTeamMemberResponseOK, nil).Once()

		got, err := AddUser(team1.Id, "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team1.Name, got.TeamName)
		s.Equal(Added, got.Action)
	})

	s.Run("AddUser no user_id passed no org users found", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListUsersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListUsersResponseEmpty, nil).Once()

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

		_, err = AddUser(team1.Id, "", false, mockClient)
		s.EqualError(err, "no users found in your organization")
	})
}

func (s *Suite) TestRemoveUser() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path RemoveUser", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		got, err := RemoveUser(team1.Id, user1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team1.Name, got.TeamName)
		s.Equal(Removed, got.Action)
	})

	s.Run("RemoveUser of a user who is not a member says so", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		_, err := RemoveUser(team1.Id, "user-nobody", false, mockClient)
		s.EqualError(err, "user user-nobody is not a member of team "+team1.Name)
		mockClient.AssertNotCalled(s.T(), "RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("happy path RemoveUser with idp managed team", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "y")()
		got, err := RemoveUser(team2.Id, user1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team2.Name, got.TeamName)
		s.Equal(Removed, got.Action)
	})

	s.Run("happy path RemoveUser with idp managed team using force flag", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()
		got, err := RemoveUser(team2.Id, user1.Id, true, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team2.Name, got.TeamName)
		s.Equal(Removed, got.Action)
	})

	s.Run("user reject RemoveUser with idp managed team", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetIDPManagedTeamWithResponseOK, nil).Once()
		defer testUtil.MockUserInput(s.T(), "n")()
		got, err := RemoveUser(team2.Id, user1.Id, false, mockClient)
		s.NoError(err)
		s.Nil(got, "declined, so nothing to report")
	})

	s.Run("error path no org teams found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseEmpty, nil).Once()
		_, err := RemoveUser("", user1.Id, false, mockClient)
		s.EqualError(err, "no teams found in your organization")
	})

	s.Run("error path no team members found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseEmptyMembership, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseEmpty, nil).Once()
		_, err := RemoveUser(team1.Id, user1.Id, false, mockClient)
		s.EqualError(err, "no team members found in team")
	})

	s.Run("error path when RemoveTeamMemberWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveUser(team1.Id, user1.Id, false, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when RemoveTeamMemberWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseError, nil).Once()
		_, err := RemoveUser(team1.Id, user1.Id, false, mockClient)
		s.EqualError(err, "failed to update team membership")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := RemoveUser(team1.Id, user1.Id, false, mockClient)
		s.Error(err)
	})

	s.Run("RemoveUser no team-id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()

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

		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()

		got, err := RemoveUser("", user1.Id, false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team1.Name, got.TeamName)
		s.Equal(Removed, got.Action)
	})
	s.Run("RemoveUser no user_id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()

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

		mockClient.On("RemoveTeamMemberWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&RemoveTeamMemberResponseOK, nil).Once()

		got, err := RemoveUser(team1.Id, "", false, mockClient)
		s.NoError(err)
		s.Require().NotNil(got)
		s.Equal(user1.Id, got.UserID)
		s.Equal(team1.Name, got.TeamName)
		s.Equal(Removed, got.Action)
	})
}

func (s *Suite) TestListTeamUsers() {
	s.Run("happy path ListTeamUsers", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()
		got, err := ListTeamUsers(team1.Id, mockClient)
		s.NoError(err)
		s.Equal([]Member{{ID: user1.Id, FullName: user1.FullName, Email: user1.Username}}, got.Members)
	})

	s.Run("happy path ListTeamUsers team with no membership", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseEmptyMembership, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseEmpty, nil).Once()
		got, err := ListTeamUsers(team1.Id, mockClient)
		s.NoError(err)
		s.NotNil(got.Members, "empty, not nil, so it publishes []")
		s.Empty(got.Members)
	})

	s.Run("error path when GetTeamWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseError, nil).Once()

		_, err := ListTeamUsers(team1.Id, mockClient)
		s.EqualError(err, "failed to get team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := ListTeamUsers(team1.Id, mockClient)
		s.Error(err)
	})

	s.Run("ListTeamUsers no team-id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
		mockClient.On("ListTeamMembersWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamMembersResponseOK, nil).Once()

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

		got, err := ListTeamUsers("", mockClient)
		s.NoError(err)
		s.Len(got.Members, 1)
	})
}

func (s *Suite) TestGetTeam() {
	s.Run("happy path GetTeam", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		_, err := GetTeam(mockClient, team1.Id)
		s.NoError(err)
	})

	s.Run("error path when GetTeamWithResponse returns a network error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()

		_, err := GetTeam(mockClient, team1.Id)
		s.EqualError(err, "network error")
	})

	s.Run("error path when GetTeamWithResponse returns an error", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseError, nil).Once()

		_, err := GetTeam(mockClient, team1.Id)
		s.EqualError(err, "failed to get team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := GetTeam(mockClient, team1.Id)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestGetWorkspaceTeams() {
	s.Run("happy path get WorkspaceTeams pulls workspace from context", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsWorkspaceResponseOK, nil).Once()
		_, err := GetWorkspaceTeams(mockClient, "", 10)
		s.NoError(err)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := GetWorkspaceTeams(mockClient, "", 10)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}

func (s *Suite) TestUpdateDeploymentTeamRole() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path UpdateDeploymentTeamRole", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.NoError(err)
		s.Equal("team1-id", got.ID)
		s.Equal("DEPLOYMENT_ADMIN", got.DeploymentRole)
	})

	s.Run("error path no deployment teams found", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseEmpty, nil).Once()
		_, err := UpdateDeploymentTeamRole("", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "no teams found in your deployment")
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseError, nil).Once()
		_, err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := UpdateDeploymentTeamRole(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.Error(err)
	})

	s.Run("UpdateDeploymentTeamRole no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsWorkspaceResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()

		got, err := UpdateDeploymentTeamRole("", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.NoError(err)
		s.Equal("team1-id", got.ID)
		s.Equal("DEPLOYMENT_ADMIN", got.DeploymentRole)
	})
}

func (s *Suite) TestAddDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path AddDeploymentTeam", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.NoError(err)
		s.Equal("team1-id", got.ID)
		s.Equal("DEPLOYMENT_ADMIN", got.DeploymentRole)
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseError, nil).Once()
		_, err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := AddDeploymentTeam(team1.Id, "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.Error(err)
	})

	s.Run("AddDeploymentTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()

		got, err := AddDeploymentTeam("", "DEPLOYMENT_ADMIN", deploymentID, mockClient)
		s.NoError(err)
		s.Equal("team1-id", got.ID)
		s.Equal("DEPLOYMENT_ADMIN", got.DeploymentRole)
	})
}

func (s *Suite) TestRemoveDeploymentTeam() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("happy path DeleteDeploymentTeam", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()
		got, err := RemoveDeploymentTeam(team1.Id, deploymentID, mockClient)
		s.NoError(err)
		s.Equal(DeploymentRemoval{ID: team1.Id, Name: team1.Name, DeploymentID: deploymentID, Action: Removed}, got)
	})

	s.Run("error path when GetTeamWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveDeploymentTeam(team1.Id, deploymentID, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse return network error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errorNetwork).Once()
		_, err := RemoveDeploymentTeam(team1.Id, deploymentID, mockClient)
		s.EqualError(err, "network error")
	})

	s.Run("error path when UpdateTeamRolesWithResponse returns an error", func() {
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("GetTeamWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetTeamWithResponseOK, nil).Once()
		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseError, nil).Once()
		_, err := RemoveDeploymentTeam(team1.Id, deploymentID, mockClient)
		s.EqualError(err, "failed to update team")
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := RemoveDeploymentTeam(team1.Id, deploymentID, mockClient)
		s.Error(err)
	})

	s.Run("RemoveDeploymentTeam no id passed", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsWorkspaceResponseOK, nil).Once()
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

		mockClient.On("UpdateTeamRolesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&UpdateTeamRolesResponseOK, nil).Once()

		got, err := RemoveDeploymentTeam("", deploymentID, mockClient)
		s.NoError(err)
		s.Equal(DeploymentRemoval{ID: team1.Id, Name: team1.Name, DeploymentID: deploymentID, Action: Removed}, got)
	})
}

func (s *Suite) TestGetDeploymentTeams() {
	s.Run("happy path get DeploymentTeams pulls deployment from context", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		mockClient.On("ListTeamsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListTeamsWorkspaceResponseOK, nil).Once()
		_, err := GetDeploymentTeams(mockClient, deploymentID, 10)
		s.NoError(err)
	})

	s.Run("error path when getting current context returns an error", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOutMessage := ""
		out := new(bytes.Buffer)
		mockClient := astrov1_mocks.NewClientWithResponsesInterface(s.T())
		_, err := GetDeploymentTeams(mockClient, deploymentID, 10)
		s.Error(err)
		s.Equal(expectedOutMessage, out.String())
	})
}
