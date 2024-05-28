package software

import (
	"bytes"

	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func (s *Suite) TestWorkspaceList() {
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
	s.NoError(err)
	s.Equal(expectedOut, output, err)
}

func (s *Suite) TestWorkspaceCreate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	buf := new(bytes.Buffer)

	err := workspaceCreate(&cobra.Command{}, buf)
	s.ErrorIs(err, errCreateWorkspaceMissingLabel)

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()
	workspaceCreateLabel = "test"

	houstonMock.On("CreateWorkspace", houston.CreateWorkspaceRequest{Label: workspaceCreateLabel, Description: "N/A"}).Return(&houston.Workspace{ID: "test", Label: workspaceCreateLabel}, nil).Once()
	err = workspaceCreate(&cobra.Command{}, buf)
	s.NoError(err)
	s.Contains(buf.String(), workspaceCreateLabel)

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceDelete() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	buf := new(bytes.Buffer)
	wsID := "test-id"

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()

	houstonMock.On("DeleteWorkspace", wsID).Return(nil, nil).Once()
	err := workspaceDelete(&cobra.Command{}, buf, []string{wsID})
	s.NoError(err)
	s.Contains(buf.String(), "Successfully deleted workspace")

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceUpdate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	wsID := "test-id"
	buf := new(bytes.Buffer)

	err := workspaceUpdate(&cobra.Command{}, buf, []string{wsID})
	s.ErrorIs(err, errUpdateWorkspaceInvalidArgs)

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()
	workspaceUpdateLabel = "test"
	workspaceUpdateDescription = "test"

	houstonMock.On("UpdateWorkspace", houston.UpdateWorkspaceRequest{WorkspaceID: wsID, Args: map[string]string{"label": workspaceUpdateLabel, "description": workspaceUpdateDescription}}).Return(&houston.Workspace{ID: wsID, Label: workspaceUpdateLabel}, nil).Once()
	err = workspaceUpdate(&cobra.Command{}, buf, []string{wsID})
	s.NoError(err)
	s.Contains(buf.String(), wsID)
	s.Contains(buf.String(), workspaceUpdateLabel)

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceSwitch() {
	s.Run("Without pagination", func() {
		testUtil.InitTestConfig(testUtil.SoftwarePlatform)
		wsID := "test-id"
		buf := new(bytes.Buffer)

		houstonMock := new(mocks.ClientInterface)
		currentClient := houstonClient
		houstonClient = houstonMock
		defer func() { houstonClient = currentClient }()

		houstonMock.On("ValidateWorkspaceID", wsID).Return(&houston.Workspace{}, nil).Once()

		err := workspaceSwitch(&cobra.Command{}, buf, []string{wsID})
		s.NoError(err)
		s.Contains(buf.String(), wsID)
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("With pagination default pageSize", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), wsID)
		houstonMock.AssertExpectations(s.T())
	})

	s.Run("With pagination, invalid/negative pageSize", func() {
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
		s.NoError(err)
		s.Contains(buf.String(), wsID)
		houstonMock.AssertExpectations(s.T())
	})
}
