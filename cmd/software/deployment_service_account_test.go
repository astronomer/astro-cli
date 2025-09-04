package software

import (
	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestDeploymentSaRootCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err := execDeploymentCmd("service-account")
	s.NoError(err)
	s.Contains(output, "deployment service-account")
}

func (s *Suite) TestDeploymentSAListCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockSA := houston.ServiceAccount{
		ID:        "ckqvfa2cu1468rn9hnr0bqqfk",
		APIKey:    "658b304f36eaaf19860a6d9eb73f7d8a",
		Label:     "yooo can u see me test",
		Category:  "default",
		CreatedAt: "2021-07-08T21:28:57.966Z",
		UpdatedAt: "2021-07-08T21:28:57.966Z",
		Active:    true,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("ListDeploymentServiceAccounts", mockDeployment.ID).Return([]houston.ServiceAccount{mockSA}, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd("sa", "list", "--deployment-id="+mockDeployment.ID)
	s.NoError(err)
	s.Contains(output, mockSA.Label)
	s.Contains(output, mockSA.ID)
	s.Contains(output, mockSA.APIKey)
}

func (s *Suite) TestDeploymentSaDeleteWoKeyIdCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	_, err := execDeploymentCmd("service-account", "delete", "--deployment-id=1234")
	s.Error(err)
	s.EqualError(err, "accepts 1 arg(s), received 0")
}

func (s *Suite) TestDeploymentSaDeleteWoDeploymentIdCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	_, err := execDeploymentCmd("service-account", "delete", "key-test-id")
	s.Error(err)
	s.EqualError(err, `required flag(s) "deployment-id" not set`)
}

func (s *Suite) TestDeploymentSaDeleteRootCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("DeleteDeploymentServiceAccount", houston.DeleteServiceAccountRequest{DeploymentID: "1234", ServiceAccountID: mockDeploymentSA.ID}).Return(mockDeploymentSA, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)
	houstonClient = api
	output, err := execDeploymentCmd("service-account", "delete", mockDeploymentSA.ID, "--deployment-id=1234")
	s.NoError(err)
	s.Contains(output, "Service Account my_label (q1w2e3r4t5y6u7i8o9p0) successfully deleted")
}

func (s *Suite) TestDeploymentSaCreateCommand() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	expectedOut := ` NAME         CATEGORY     ID                       APIKEY                       
 my_label     default      q1w2e3r4t5y6u7i8o9p0     000000000000000000000000     

 Service account successfully created.
`

	mockSA := &houston.DeploymentServiceAccount{
		ID:             mockDeploymentSA.ID,
		APIKey:         mockDeploymentSA.APIKey,
		Label:          mockDeploymentSA.Label,
		Category:       mockDeploymentSA.Category,
		EntityType:     "DEPLOYMENT",
		DeploymentUUID: mockDeployment.ID,
		CreatedAt:      mockDeploymentSA.CreatedAt,
		UpdatedAt:      mockDeploymentSA.UpdatedAt,
		Active:         true,
	}

	expectedSARequest := &houston.CreateServiceAccountRequest{
		DeploymentID: "ck1qg6whg001r08691y117hub",
		Label:        "my_label",
		Category:     "default",
		Role:         houston.DeploymentViewerRole,
	}

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", "").Return(mockAppConfig, nil)
	api.On("CreateDeploymentServiceAccount", expectedSARequest).Return(mockSA, nil)
	api.On("GetPlatformVersion", nil).Return("0.25.0", nil)

	houstonClient = api
	output, err := execDeploymentCmd(
		"service-account",
		"create",
		"--deployment-id="+expectedSARequest.DeploymentID,
		"--label="+expectedSARequest.Label,
		"--role=DEPLOYMENT_VIEWER",
	)
	s.NoError(err)
	s.Equal(expectedOut, output)
}
