package software

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
)

var errMockHouston = errors.New("some houston error")

func (s *Suite) TestWorkspaceUserRemove() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockUser := houston.WorkspaceUserRoleBindings{
		ID:       "ckc0eir8e01gj07608ajmvia1",
		Username: "test@astronomer.io",
		Emails:   []houston.Email{{Address: "test@astronomer.io"}},
		FullName: "test",
		RoleBindings: []houston.RoleBinding{
			{
				Role: houston.WorkspaceAdminRole,
			},
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("GetWorkspaceUserRole", houston.GetWorkspaceUserRoleRequest{WorkspaceID: mockWorkspace.ID, Email: mockUser.Username}).Return(mockUser, nil)
	api.On("DeleteWorkspaceUser", houston.DeleteWorkspaceUserRequest{WorkspaceID: mockWorkspace.ID, UserID: mockUser.ID}).Return(mockWorkspace, nil)
	houstonClient = api

	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
 airflow                       ck05r3bor07h40d02y2hw4n4v                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`

	buf := new(bytes.Buffer)
	cmd := newWorkspaceUserRemoveCmd(buf)
	err := cmd.RunE(cmd, []string{mockUser.Username})
	s.NoError(err)
	s.Equal(expected, buf.String())
}

func (s *Suite) TestNewWorkspaceUserListCmd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString("")),
			Header:     make(http.Header),
		}
	})
	houstonClient = houston.NewClient(client)
	buf := new(bytes.Buffer)
	cmd := newWorkspaceUserListCmd(buf)
	s.NotNil(cmd)
	s.Nil(cmd.Args)
}

func (s *Suite) TestWorkspaceUserAdd() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	houstonMock := new(mocks.ClientInterface)
	houstonMock.On("AddWorkspaceUser", mock.Anything).Return(&houston.Workspace{}, nil).Once()
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	// success case
	buf := new(bytes.Buffer)
	workspaceUserWsRole = houston.WorkspaceAdminRole
	err := workspaceUserAdd(&cobra.Command{}, buf)
	s.NoError(err)

	// houston error case
	houstonMock.On("AddWorkspaceUser", mock.Anything).Return(nil, errMockHouston).Once()
	err = workspaceUserAdd(&cobra.Command{}, buf)
	s.ErrorIs(err, errMockHouston)

	// invalid role case
	workspaceUserWsRole = "invalid-role"
	err = workspaceUserAdd(&cobra.Command{}, buf)
	s.Error(err)
	s.Contains(err.Error(), "failed to find a valid role")

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceUserUpdate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockEmail := "test-email"
	mockRoles := houston.WorkspaceUserRoleBindings{
		RoleBindings: []houston.RoleBinding{
			{
				Role:      houston.WorkspaceViewerRole,
				Workspace: houston.Workspace{ID: "ck05r3bor07h40d02y2hw4n4v"},
			},
		},
	}
	houstonMock := new(mocks.ClientInterface)
	houstonMock.On("UpdateWorkspaceUserRole", mock.Anything).Return("updated", nil).Once()
	houstonMock.On("GetWorkspaceUserRole", mock.Anything).Return(mockRoles, nil)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	// success case
	buf := new(bytes.Buffer)
	workspaceUserWsRole = houston.WorkspaceAdminRole
	err := workspaceUserUpdate(&cobra.Command{}, buf, []string{mockEmail})
	s.NoError(err)

	// houston error case
	houstonMock.On("UpdateWorkspaceUserRole", mock.Anything).Return("", errMockHouston).Once()
	err = workspaceUserUpdate(&cobra.Command{}, buf, []string{mockEmail})
	s.ErrorIs(err, errMockHouston)

	// invalid role case
	workspaceUserWsRole = "invalid-role"
	err = workspaceUserUpdate(&cobra.Command{}, buf, []string{mockEmail})
	s.Error(err)
	s.Contains(err.Error(), "failed to find a valid role")

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceUserList() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	ws, err := coalesceWorkspace()
	s.NoError(err)
	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "test-id-username",
			Username: "test@astronomer.io",
			FullName: "testusername",
			Emails:   []houston.Email{{Address: "test@astronomer.io"}},
			RoleBindings: []houston.RoleBinding{
				{
					Role:      houston.WorkspaceViewerRole,
					Workspace: houston.Workspace{ID: ws},
				},
			},
		},
	}

	houstonMock := new(mocks.ClientInterface)
	houstonMock.On("ListWorkspaceUserAndRoles", mock.Anything).Return(mockResponse, nil)

	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	buf := new(bytes.Buffer)
	err = workspaceUserList(&cobra.Command{}, buf)
	s.NoError(err)
	content := buf.String()
	s.Contains(content, mockResponse[0].Username)
	s.Contains(content, mockResponse[0].ID)
	s.Contains(content, houston.WorkspaceViewerRole)
	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceUserListPaginated() {
	s.Run("with default page size", func() {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		ws, err := coalesceWorkspace()
		s.NoError(err)
		mockResponse := []houston.WorkspaceUserRoleBindings{
			{
				ID:       "test-id-username",
				Username: "test@astronomer.io",
				FullName: "testusername",
				Emails:   []houston.Email{{Address: "test@astronomer.io"}},
				RoleBindings: []houston.RoleBinding{
					{
						Role:      houston.WorkspaceViewerRole,
						Workspace: houston.Workspace{ID: ws},
					},
				},
			},
		}

		houstonMock := new(mocks.ClientInterface)
		houstonMock.On("ListWorkspacePaginatedUserAndRoles", mock.Anything).Return(mockResponse, nil)

		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("q")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		paginated = true
		err = workspaceUserList(&cobra.Command{}, buf)
		s.NoError(err)
		content := buf.String()
		s.Contains(content, mockResponse[0].Username)
		s.Contains(content, mockResponse[0].ID)
		s.Contains(content, houston.WorkspaceViewerRole)
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("with invalid/negative page size", func() {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		ws, err := coalesceWorkspace()
		s.NoError(err)
		mockResponse := []houston.WorkspaceUserRoleBindings{
			{
				ID:       "test-id-username",
				Username: "test@astronomer.io",
				FullName: "testusername",
				Emails:   []houston.Email{{Address: "test@astronomer.io"}},
				RoleBindings: []houston.RoleBinding{
					{
						Role:      houston.WorkspaceViewerRole,
						Workspace: houston.Workspace{ID: ws},
					},
				},
			},
		}

		houstonMock := new(mocks.ClientInterface)
		houstonMock.On("ListWorkspacePaginatedUserAndRoles", mock.Anything).Return(mockResponse, nil)

		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("q")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		paginated = true
		pageSize = -10
		err = workspaceUserList(&cobra.Command{}, buf)
		s.NoError(err)
		content := buf.String()
		s.Contains(content, mockResponse[0].Username)
		s.Contains(content, mockResponse[0].ID)
		s.Contains(content, houston.WorkspaceViewerRole)
		houstonMock.AssertExpectations(s.T())
	})
}
