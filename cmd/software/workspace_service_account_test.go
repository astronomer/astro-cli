package software

import (
	"bytes"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
)

func (s *Suite) TestWorkspaceSaRootCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err := execWorkspaceCmd("service-account")
	s.NoError(err)
	s.Contains(output, "workspace service-account")
}

func (s *Suite) TestWorkspaceSAListCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
	mockSA := houston.ServiceAccount{
		ID:         "ckqvfa2cu1468rn9hnr0bqqfk",
		APIKey:     "658b304f36eaaf19860a6d9eb73f7d8a",
		Label:      "yooo can u see me test",
		Category:   "",
		CreatedAt:  "2021-07-08T21:28:57.966Z",
		UpdatedAt:  "2021-07-08T21:28:57.966Z",
		LastUsedAt: "",
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("ListWorkspaceServiceAccounts", mockWorkspace.ID).Return([]houston.ServiceAccount{mockSA}, nil)
	houstonClient = api

	output, err := execWorkspaceCmd("sa", "list", "--workspace-id="+mockWorkspace.ID)
	s.NoError(err)
	s.Contains(output, expectedOut)
}

func (s *Suite) TestWorkspaceSaCreate() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	buf := new(bytes.Buffer)

	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()
	workspaceSARole = houston.WorkspaceAdminRole
	houstonMock.On("CreateWorkspaceServiceAccount", mock.Anything).Return(&houston.WorkspaceServiceAccount{}, nil).Once()

	err := workspaceSaCreate(&cobra.Command{}, buf)
	s.NoError(err)

	workspaceSARole = "invalid-role"
	err = workspaceSaCreate(&cobra.Command{}, buf)
	s.Error(err)
	s.Contains(err.Error(), "failed to find a valid role")

	houstonMock.AssertExpectations(s.T())
}

func (s *Suite) TestWorkspaceSaDelete() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	buf := new(bytes.Buffer)
	saID := "test-id"
	houstonMock := new(mocks.ClientInterface)
	currentClient := houstonClient
	houstonClient = houstonMock
	defer func() { houstonClient = currentClient }()
	workspaceSARole = houston.WorkspaceAdminRole
	houstonMock.On("DeleteWorkspaceServiceAccount", mock.Anything).Return(&houston.ServiceAccount{}, nil).Once()

	err := workspaceSaDelete(&cobra.Command{}, buf, []string{saID})
	s.NoError(err)

	houstonMock.AssertExpectations(s.T())
}
