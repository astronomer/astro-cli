package software

import (
	"bytes"
	"testing"

	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
)

var mockWorkspace = &houston.Workspace{
	ID:           "ck05r3bor07h40d02y2hw4n4v",
	Label:        "airflow",
	Description:  "test description",
	Users:        nil,
	CreatedAt:    "2019-10-16T21:14:22.105Z",
	UpdatedAt:    "2019-10-16T21:14:22.105Z",
	RoleBindings: nil,
}

func execWorkspaceCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newWorkspaceCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestWorkspaceList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := " NAME           ID                            \n" +
		"\x1b[1;32m airflow        ck05r3bor07h40d02y2hw4n4v     \x1b[0m\n " +
		"airflow123     XXXXXXXXXXXXXXX               \n"

	mockWorkspaces := []houston.Workspace{
		*mockWorkspace,
		{
			ID:          "XXXXXXXXXXXXXXX",
			Label:       "airflow123",
			Description: "test description 123",
		},
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(&houston.AppConfig{}, nil)
	api.On("ListWorkspaces", nil).Return(mockWorkspaces, nil)

	houstonClient = api

	output, err := execWorkspaceCmd("list")
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output, err)
}

func TestWorkspaceCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	buf := new(bytes.Buffer)

	err := workspaceCreate(&cobra.Command{}, buf)
	assert.ErrorIs(t, err, errCreateWorkspaceMissingLabel)

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()
	workspaceCreateLabel = "test"

	houstonMock.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: workspaceCreateLabel, Description: "N/A"}).Return(&houston.Workspace{ID: "test", Label: workspaceCreateLabel}, nil).Once()
	err = workspaceCreate(&cobra.Command{}, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), workspaceCreateLabel)

	houstonMock.AssertExpectations(t)
}

func TestWorkspaceDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	buf := new(bytes.Buffer)
	wsID := "test-id"

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	houstonMock.On("DeleteWorkspace", wsID).Return(nil, nil).Once()
	err := workspaceDelete(&cobra.Command{}, buf, []string{wsID})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Successfully deleted workspace")

	houstonMock.AssertExpectations(t)
}

func TestWorkspaceUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	wsID := "test-id"
	buf := new(bytes.Buffer)

	err := workspaceUpdate(&cobra.Command{}, buf, []string{wsID})
	assert.ErrorIs(t, err, errUpdateWorkspaceInvalidArgs)

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()
	workspaceUpdateLabel = "test"
	workspaceUpdateDescription = "test"

	houstonMock.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: wsID, Args: map[string]string{"label": workspaceUpdateLabel, "description": workspaceUpdateDescription}}).Return(&houston.Workspace{ID: wsID, Label: workspaceUpdateLabel}, nil).Once()
	err = workspaceUpdate(&cobra.Command{}, buf, []string{wsID})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), wsID)
	assert.Contains(t, buf.String(), workspaceUpdateLabel)

	houstonMock.AssertExpectations(t)
}

func TestWorkspaceSwitch(t *testing.T) {
	t.Run("Without pagination", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		wsID := "test-id"
		buf := new(bytes.Buffer)

		houstonMock := new(mocks.ClientInterface)
		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		houstonMock.On("ValidateWorkspaceID", wsID).Return(&houston.Workspace{}, nil).Once()

		err := workspaceSwitch(&cobra.Command{}, buf, []string{wsID})
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), wsID)
		houstonMock.AssertExpectations(t)
	})

	t.Run("With pagination default pageSize", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		wsID := "test-id"
		buf := new(bytes.Buffer)
		workspacePaginated = true

		houstonMock := new(mocks.ClientInterface)
		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		houstonMock.On("ValidateWorkspaceID", wsID).Return(&houston.Workspace{}, nil).Once()

		err := workspaceSwitch(&cobra.Command{}, buf, []string{wsID})
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), wsID)
		houstonMock.AssertExpectations(t)
	})

	t.Run("With pagination, invalid/negative pageSize", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		wsID := "test-id"
		workspacePaginated = true
		workspacePageSize = -10
		buf := new(bytes.Buffer)

		houstonMock := new(mocks.ClientInterface)
		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		houstonMock.On("ValidateWorkspaceID", wsID).Return(&houston.Workspace{}, nil).Once()

		err := workspaceSwitch(&cobra.Command{}, buf, []string{wsID})
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), wsID)
		houstonMock.AssertExpectations(t)
	})
}
