package software

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errMockHouston = errors.New("some houston error")

func TestWorkspaceUserRemove(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockUser := houston.WorkspaceUserRoleBindings{
		ID:       "ckc0eir8e01gj07608ajmvia1",
		Username: "test@astronomer.io",
		Emails:   []houston.Email{{Address: "test@astronomer.io"}},
		FullName: "test",
		RoleBindings: []houston.RoleBindingWorkspace{
			{
				Role: houston.WorkspaceAdminRole,
			},
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig").Return(mockAppConfig, nil)
	api.On("GetWorkspaceUserRole", mockWorkspace.ID, mockUser.Username).Return(mockUser, nil)
	api.On("DeleteWorkspaceUser", mockWorkspace.ID, mockUser.ID).Return(mockWorkspace, nil)
	houstonClient = api

	expected := ` NAME                          WORKSPACE ID                                      USER_ID                                           
 airflow                       ck05r3bor07h40d02y2hw4n4v                         ckc0eir8e01gj07608ajmvia1                         
Successfully removed user from workspace
`

	buf := new(bytes.Buffer)
	cmd := newWorkspaceUserRemoveCmd(buf)
	err := cmd.RunE(cmd, []string{mockUser.Username})
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())
}

func TestNewWorkspaceUserListCmd(t *testing.T) {
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
	assert.NotNil(t, cmd)
	assert.Nil(t, cmd.Args)
}

func TestWorkspaceUserAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	houstonMock := new(mocks.ClientInterface)
	houstonMock.On("AddWorkspaceUser", mock.Anything, mock.Anything, mock.Anything).Return(&houston.Workspace{}, nil).Once()
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	// success case
	buf := new(bytes.Buffer)
	workspaceUserWsRole = houston.WorkspaceAdminRole
	err := workspaceUserAdd(&cobra.Command{}, buf)
	assert.NoError(t, err)

	// houston error case
	houstonMock.On("AddWorkspaceUser", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMockHouston).Once()
	err = workspaceUserAdd(&cobra.Command{}, buf)
	assert.ErrorIs(t, err, errMockHouston)

	// invalid role case
	workspaceUserWsRole = "invalid-role"
	err = workspaceUserAdd(&cobra.Command{}, buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find a valid role")

	houstonMock.AssertExpectations(t)
}

func TestWorkspaceUserUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockEmail := "test-email"
	mockRoles := houston.WorkspaceUserRoleBindings{
		RoleBindings: []houston.RoleBindingWorkspace{
			{
				Role: houston.WorkspaceViewerRole,
				Workspace: struct {
					ID string `json:"id"`
				}{ID: "ck05r3bor07h40d02y2hw4n4v"},
			},
		},
	}
	houstonMock := new(mocks.ClientInterface)
	houstonMock.On("UpdateWorkspaceUserRole", mock.Anything, mockEmail, mock.Anything).Return("updated", nil).Once()
	houstonMock.On("GetWorkspaceUserRole", mock.Anything, mockEmail).Return(mockRoles, nil)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	// success case
	buf := new(bytes.Buffer)
	workspaceUserWsRole = houston.WorkspaceAdminRole
	err := workspaceUserUpdate(&cobra.Command{}, buf, []string{mockEmail})
	assert.NoError(t, err)

	// houston error case
	houstonMock.On("UpdateWorkspaceUserRole", mock.Anything, mockEmail, mock.Anything).Return("", errMockHouston).Once()
	err = workspaceUserUpdate(&cobra.Command{}, buf, []string{mockEmail})
	assert.ErrorIs(t, err, errMockHouston)

	// invalid role case
	workspaceUserWsRole = "invalid-role"
	err = workspaceUserUpdate(&cobra.Command{}, buf, []string{mockEmail})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find a valid role")

	houstonMock.AssertExpectations(t)
}

func TestWorkspaceUserList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResponse := []houston.WorkspaceUserRoleBindings{
		{
			ID:       "test-id-username",
			Username: "test@astronomer.io",
			FullName: "testusername",
			Emails:   []houston.Email{{Address: "test@astronomer.io"}},
			RoleBindings: []houston.RoleBindingWorkspace{
				{
					Role: houston.WorkspaceViewerRole,
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
	err := workspaceUserList(&cobra.Command{}, buf)
	assert.NoError(t, err)
	content := buf.String()
	assert.Contains(t, content, mockResponse[0].Username)
	assert.Contains(t, content, mockResponse[0].ID)
	assert.Contains(t, content, houston.WorkspaceViewerRole)
	houstonMock.AssertExpectations(t)
}

func TestWorkspaceUserListPaginated(t *testing.T) {
	t.Run("with default page size", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)

		mockResponse := []houston.WorkspaceUserRoleBindings{
			{
				ID:       "test-id-username",
				Username: "test@astronomer.io",
				FullName: "testusername",
				Emails:   []houston.Email{{Address: "test@astronomer.io"}},
				RoleBindings: []houston.RoleBindingWorkspace{
					{
						Role: houston.WorkspaceViewerRole,
					},
				},
			},
		}

		houstonMock := new(mocks.ClientInterface)
		houstonMock.On("ListWorkspacePaginatedUserAndRoles", mock.Anything, mock.Anything, mock.Anything).Return(mockResponse, nil)

		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("q")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		paginated = true
		err = workspaceUserList(&cobra.Command{}, buf)
		assert.NoError(t, err)
		content := buf.String()
		assert.Contains(t, content, mockResponse[0].Username)
		assert.Contains(t, content, mockResponse[0].ID)
		assert.Contains(t, content, houston.WorkspaceViewerRole)
		houstonMock.AssertExpectations(t)
	})

	t.Run("with invalid/negative page size", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)

		mockResponse := []houston.WorkspaceUserRoleBindings{
			{
				ID:       "test-id-username",
				Username: "test@astronomer.io",
				FullName: "testusername",
				Emails:   []houston.Email{{Address: "test@astronomer.io"}},
				RoleBindings: []houston.RoleBindingWorkspace{
					{
						Role: houston.WorkspaceViewerRole,
					},
				},
			},
		}

		houstonMock := new(mocks.ClientInterface)
		houstonMock.On("ListWorkspacePaginatedUserAndRoles", mock.Anything, mock.Anything, mock.Anything).Return(mockResponse, nil)

		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		buf := new(bytes.Buffer)
		// mock os.Stdin
		input := []byte("q")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		paginated = true
		pageSize = -10
		err = workspaceUserList(&cobra.Command{}, buf)
		assert.NoError(t, err)
		content := buf.String()
		assert.Contains(t, content, mockResponse[0].Username)
		assert.Contains(t, content, mockResponse[0].ID)
		assert.Contains(t, content, houston.WorkspaceViewerRole)
		houstonMock.AssertExpectations(t)
	})
}
