package software

import (
	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestDeploymentUserAddCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` DEPLOYMENT ID                 USER                       ROLE                  
 ckggvxkw112212kc9ebv8vu6p     somebody@astronomer.io     DEPLOYMENT_VIEWER     

 Successfully added somebody@astronomer.io as a DEPLOYMENT_VIEWER
`
	expectedAddUserRequest := houston.UpdateDeploymentUserRequest{
		Email:        mockDeploymentUserRole.User.Username,
		Role:         mockDeploymentUserRole.Role,
		DeploymentID: mockDeploymentUserRole.Deployment.ID,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("AddDeploymentUser", expectedAddUserRequest).Return(mockDeploymentUserRole, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"user",
		"add",
		"--deployment-id="+mockDeploymentUserRole.Deployment.ID,
		"--email="+mockDeploymentUserRole.User.Username,
	)
	s.NoError(err)
	s.Equal(expectedOut, output)
}

func (s *Suite) TestDeploymentUserDeleteCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` DEPLOYMENT ID                 USER                       ROLE                  
 ckggvxkw112212kc9ebv8vu6p     somebody@astronomer.io     DEPLOYMENT_VIEWER     

 Successfully removed the DEPLOYMENT_VIEWER role for somebody@astronomer.io from deployment ckggvxkw112212kc9ebv8vu6p
`

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("DeleteDeploymentUser", houston.DeleteDeploymentUserRequest{DeploymentID: mockDeploymentUserRole.Deployment.ID, Email: mockDeploymentUserRole.User.Username}).
		Return(mockDeploymentUserRole, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd(
		"user",
		"remove",
		"--deployment-id="+mockDeploymentUserRole.Deployment.ID,
		mockDeploymentUserRole.User.Username,
	)
	s.NoError(err)
	s.Equal(expectedOut, output)
}

func (s *Suite) TestDeploymentUserList() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockUser := []houston.DeploymentUser{
		{
			ID:           "test-id",
			Emails:       []houston.Email{{Address: "test-email"}},
			FullName:     "test-name",
			RoleBindings: []houston.RoleBinding{{Role: houston.DeploymentViewerRole, Deployment: houston.Deployment{ID: "test-id"}}},
		},
	}
	api := new(mocks.ClientInterface)
	api.On("ListDeploymentUsers", houston.ListDeploymentUsersRequest{UserID: "test-user-id", Email: "test-email", FullName: "test-name", DeploymentID: "test-id"}).Return(mockUser, nil).Once()
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd("user", "list", "--deployment-id", "test-id", "-u", "test-user-id", "-e", "test-email", "-n", "test-name")
	s.NoError(err)
	s.Contains(output, "test-id")
	s.Contains(output, "test-name")
	api.AssertExpectations(s.T())
}

func (s *Suite) TestDeploymentUserUpdateCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedNewRole := houston.DeploymentAdminRole
	expectedOut := `Successfully updated somebody@astronomer.io to a ` + expectedNewRole
	mockResponseUserRole := *mockDeploymentUserRole
	mockResponseUserRole.Role = expectedNewRole

	expectedUpdateUserRequest := houston.UpdateDeploymentUserRequest{
		Email:        mockResponseUserRole.User.Username,
		Role:         expectedNewRole,
		DeploymentID: mockDeploymentUserRole.Deployment.ID,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("UpdateDeploymentUser", expectedUpdateUserRequest).Return(&mockResponseUserRole, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)

	houstonClient = api
	output, err := execDeploymentCmd(
		"user",
		"update",
		"--deployment-id="+mockResponseUserRole.Deployment.ID,
		"--role="+expectedNewRole,
		mockResponseUserRole.User.Username,
	)
	s.NoError(err)
	s.Contains(output, expectedOut)
}
