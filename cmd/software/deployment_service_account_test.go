package software

import (
	"testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestDeploymentSaRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	output, err := execDeploymentCmd("service-account")
	assert.NoError(t, err)
	assert.Contains(t, output, "deployment service-account")
}

func TestDeploymentSAListCommand(t *testing.T) {
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
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("ListDeploymentServiceAccounts", mockDeployment.ID).Return([]houston.ServiceAccount{mockSA}, nil)

	houstonClient = api
	output, err := execDeploymentCmd("sa", "list", "--deployment-id="+mockDeployment.ID)
	assert.NoError(t, err)
	assert.Contains(t, output, mockSA.Label)
	assert.Contains(t, output, mockSA.ID)
	assert.Contains(t, output, mockSA.APIKey)
}

func TestDeploymentSaDeleteWoKeyIdCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	_, err := execDeploymentCmd("service-account", "delete", "--deployment-id=1234")
	assert.Error(t, err)
	assert.EqualError(t, err, "accepts 1 arg(s), received 0")
}

func TestDeploymentSaDeleteWoDeploymentIdCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	_, err := execDeploymentCmd("service-account", "delete", "key-test-id")
	assert.Error(t, err)
	assert.EqualError(t, err, `required flag(s) "deployment-id" not set`)
}

func TestDeploymentSaDeleteRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("DeleteDeploymentServiceAccount", houston.DeleteServiceAccountRequest{DeploymentID: "1234", ServiceAccountID: mockDeploymentSA.ID}).Return(mockDeploymentSA, nil)
	houstonClient = api
	output, err := execDeploymentCmd("service-account", "delete", mockDeploymentSA.ID, "--deployment-id=1234")
	assert.NoError(t, err)
	assert.Contains(t, output, "Service Account my_label (q1w2e3r4t5y6u7i8o9p0) successfully deleted")
}

func TestDeploymentSaCreateCommand(t *testing.T) {
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
	api.On("GetAppConfig", nil).Return(mockAppConfig, nil)
	api.On("CreateDeploymentServiceAccount", expectedSARequest).Return(mockSA, nil)

	houstonClient = api
	output, err := execDeploymentCmd(
		"service-account",
		"create",
		"--deployment-id="+expectedSARequest.DeploymentID,
		"--label="+expectedSARequest.Label,
		"--role=DEPLOYMENT_VIEWER",
	)
	assert.NoError(t, err)
	assert.Equal(t, expectedOut, output)
}
