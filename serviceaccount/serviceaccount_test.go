package deployment

import (
	"bytes"
	"errors"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/stretchr/testify/assert"
)

func TestCreateUsingDeploymentUUID(t *testing.T) {
	testUtil.InitTestConfig()
	mockSA := &houston.DeploymentServiceAccount{
		ID:             "ckbvcbqs1014t0760u4bszmcs",
		APIKey:         "60f2f4f3fa006e3e135dbe99b1391d84",
		Label:          "test",
		Category:       "test",
		EntityType:     "DEPLOYMENT",
		DeploymentUUID: "ck1qg6whg001r08691y117hub",
		LastUsedAt:     "",
		CreatedAt:      "2020-06-25T22:10:42.385Z",
		UpdatedAt:      "2020-06-25T22:10:42.385Z",
		Active:         true,
	}
	expectedRequest := &houston.CreateServiceAccountRequest{
		DeploymentID: mockSA.DeploymentUUID,
		Label:        mockSA.Label,
		Category:     mockSA.Category,
		Role:         "test",
	}

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("CreateDeploymentServiceAccount", expectedRequest).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := CreateUsingDeploymentUUID(mockSA.DeploymentUUID, mockSA.Label, mockSA.Category, "test", api, buf)
		assert.NoError(t, err)
		expectedOut := ` NAME     CATEGORY     ID                            APIKEY                               
 test     test         ckbvcbqs1014t0760u4bszmcs     60f2f4f3fa006e3e135dbe99b1391d84     

 Service account successfully created.
`
		assert.Equal(t, buf.String(), expectedOut)
		api.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		mockError := errors.New("api error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("CreateDeploymentServiceAccount", expectedRequest).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := CreateUsingDeploymentUUID(mockSA.DeploymentUUID, mockSA.Label, mockSA.Category, "test", api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}

func TestCreateUsingWorkspaceUUID(t *testing.T) {
	testUtil.InitTestConfig()

	mockSA := &houston.WorkspaceServiceAccount{
		ID:            "ckbvcbqs1014t0760u4bszmcs",
		APIKey:        "60f2f4f3fa006e3e135dbe99b1391d84",
		Label:         "test",
		Category:      "test",
		EntityType:    "WORKSPACE",
		WorkspaceUUID: "ck1qg6whg001r08691y117hub",
		LastUsedAt:    "",
		CreatedAt:     "2020-06-25T22:10:42.385Z",
		UpdatedAt:     "2020-06-25T22:10:42.385Z",
		Active:        true,
	}
	expectedRequest := &houston.CreateServiceAccountRequest{
		WorkspaceID: mockSA.WorkspaceUUID,
		Label:       mockSA.Label,
		Category:    mockSA.Category,
		Role:        "test",
	}

	label, category, role := "test", "test", "test"

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("CreateWorkspaceServiceAccount", expectedRequest).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := CreateUsingWorkspaceUUID(mockSA.WorkspaceUUID, label, category, role, api, buf)
		assert.NoError(t, err)
		expectedOut := ` NAME     CATEGORY     ID                            APIKEY                               
 test     test         ckbvcbqs1014t0760u4bszmcs     60f2f4f3fa006e3e135dbe99b1391d84     

 Service account successfully created.
`
		assert.Equal(t, buf.String(), expectedOut)
		api.AssertExpectations(t)
	})

	t.Run("api error", func(t *testing.T) {
		mockError := errors.New("api error") //nolint:goerr113

		api := new(mocks.ClientInterface)
		api.On("CreateWorkspaceServiceAccount", expectedRequest).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := CreateUsingWorkspaceUUID(mockSA.WorkspaceUUID, label, category, role, api, buf)
		assert.EqualError(t, err, mockError.Error())
	})
}

func TestDeleteUsingWorkspaceUUID(t *testing.T) {
	testUtil.InitTestConfig()

	mockSA := &houston.ServiceAccount{
		ID: "ckbvcbqs1014t0760u4bszmcs",
	}

	workspaceUUID := "ck1qg6whg001r08691y117hub"

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceServiceAccount", workspaceUUID, mockSA.ID).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := DeleteUsingWorkspaceUUID(mockSA.ID, workspaceUUID, api, buf)
		assert.NoError(t, err)
		expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
		assert.Equal(t, buf.String(), expectedOut)
		api.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		mockError := errors.New("api error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceServiceAccount", workspaceUUID, mockSA.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := DeleteUsingWorkspaceUUID(mockSA.ID, workspaceUUID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}

func TestDeleteUsingDeploymentUUID(t *testing.T) {
	testUtil.InitTestConfig()

	mockSA := &houston.ServiceAccount{
		ID: "ckbvcbqs1014t0760u4bszmcs",
	}
	deploymentUUID := "ck1qg6whg001r08691y117hub"

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentServiceAccount", deploymentUUID, mockSA.ID).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := DeleteUsingDeploymentUUID(mockSA.ID, deploymentUUID, api, buf)
		assert.NoError(t, err)
		expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
		assert.Equal(t, buf.String(), expectedOut)
	})

	t.Run("error", func(t *testing.T) {
		mockError := errors.New("api error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentServiceAccount", deploymentUUID, mockSA.ID).Return(nil, mockError)

		buf := new(bytes.Buffer)
		err := DeleteUsingDeploymentUUID(mockSA.ID, deploymentUUID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}

func TestGetDeploymentServiceAccount(t *testing.T) {
	testUtil.InitTestConfig()
	mockSAs := []houston.ServiceAccount{
		{
			ID:        "ckqvfa2cu1468rn9hnr0bqqfk",
			APIKey:    "658b304f36eaaf19860a6d9eb73f7d8a",
			Label:     "yooo can u see me test",
			Active:    true,
			CreatedAt: "2021-07-08T21:28:57.966Z",
			UpdatedAt: "2021-07-08T21:28:57.966Z",
		},
	}
	deploymentUUID := "ckqvf9spa1189rn9hbh5h439u"

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentServiceAccounts", deploymentUUID).Return(mockSAs, nil)

		buf := new(bytes.Buffer)
		err := GetDeploymentServiceAccounts(deploymentUUID, api, buf)
		assert.NoError(t, err)
		expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
		assert.Contains(t, buf.String(), expectedOut)
		api.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		mockError := errors.New("api error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentServiceAccounts", deploymentUUID).Return([]houston.ServiceAccount{}, mockError)

		buf := new(bytes.Buffer)
		err := GetDeploymentServiceAccounts(deploymentUUID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}

func TestGetWorkspaceServiceAccount(t *testing.T) {
	testUtil.InitTestConfig()

	mockSAs := []houston.ServiceAccount{
		{
			ID:        "ckqvfa2cu1468rn9hnr0bqqfk",
			APIKey:    "658b304f36eaaf19860a6d9eb73f7d8a",
			Label:     "yooo can u see me test",
			Active:    true,
			CreatedAt: "2021-07-08T21:28:57.966Z",
			UpdatedAt: "2021-07-08T21:28:57.966Z",
		},
	}
	workspaceUUID := "ckqvf9spa1189rn9hbh5h439u"

	t.Run("success", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceServiceAccounts", workspaceUUID).Return(mockSAs, nil)

		buf := new(bytes.Buffer)
		err := GetWorkspaceServiceAccounts(workspaceUUID, api, buf)
		assert.NoError(t, err)
		expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
		assert.Contains(t, buf.String(), expectedOut)
		api.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		mockError := errors.New("api error") //nolint:goerr113
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceServiceAccounts", workspaceUUID).Return([]houston.ServiceAccount{}, mockError)

		buf := new(bytes.Buffer)
		err := GetWorkspaceServiceAccounts(workspaceUUID, api, buf)
		assert.EqualError(t, err, mockError.Error())
		api.AssertExpectations(t)
	})
}
