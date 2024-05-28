package serviceaccount

import (
	"bytes"
	"errors"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/houston"
	mocks "github.com/astronomer/astro-cli/houston/mocks"
)

var errMock = errors.New("api error")

type Suite struct {
	suite.Suite
}

func TestServiceAccount(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreateUsingDeploymentUUID() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeploymentServiceAccount", expectedRequest).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := CreateUsingDeploymentUUID(mockSA.DeploymentUUID, mockSA.Label, mockSA.Category, "test", api, buf)
		s.NoError(err)
		expectedOut := ` NAME     CATEGORY     ID                            APIKEY                               
 test     test         ckbvcbqs1014t0760u4bszmcs     60f2f4f3fa006e3e135dbe99b1391d84     

 Service account successfully created.
`
		s.Equal(buf.String(), expectedOut)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeploymentServiceAccount", expectedRequest).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := CreateUsingDeploymentUUID(mockSA.DeploymentUUID, mockSA.Label, mockSA.Category, "test", api, buf)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCreateUsingWorkspaceUUID() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

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

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateWorkspaceServiceAccount", expectedRequest).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := CreateUsingWorkspaceUUID(mockSA.WorkspaceUUID, label, category, role, api, buf)
		s.NoError(err)
		expectedOut := ` NAME     CATEGORY     ID                            APIKEY                               
 test     test         ckbvcbqs1014t0760u4bszmcs     60f2f4f3fa006e3e135dbe99b1391d84     

 Service account successfully created.
`
		s.Equal(buf.String(), expectedOut)
		api.AssertExpectations(s.T())
	})

	s.Run("api error", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateWorkspaceServiceAccount", expectedRequest).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := CreateUsingWorkspaceUUID(mockSA.WorkspaceUUID, label, category, role, api, buf)
		s.EqualError(err, errMock.Error())
	})
}

func (s *Suite) TestDeleteUsingWorkspaceUUID() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockSA := &houston.ServiceAccount{
		ID: "ckbvcbqs1014t0760u4bszmcs",
	}

	workspaceUUID := "ck1qg6whg001r08691y117hub"

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceServiceAccount", houston.DeleteServiceAccountRequest{WorkspaceID: workspaceUUID, ServiceAccountID: mockSA.ID}).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := DeleteUsingWorkspaceUUID(mockSA.ID, workspaceUUID, api, buf)
		s.NoError(err)
		expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
		s.Equal(buf.String(), expectedOut)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceServiceAccount", houston.DeleteServiceAccountRequest{WorkspaceID: workspaceUUID, ServiceAccountID: mockSA.ID}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := DeleteUsingWorkspaceUUID(mockSA.ID, workspaceUUID, api, buf)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDeleteUsingDeploymentUUID() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockSA := &houston.ServiceAccount{
		ID: "ckbvcbqs1014t0760u4bszmcs",
	}
	deploymentUUID := "ck1qg6whg001r08691y117hub"

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentServiceAccount", houston.DeleteServiceAccountRequest{DeploymentID: deploymentUUID, ServiceAccountID: mockSA.ID}).Return(mockSA, nil)

		buf := new(bytes.Buffer)
		err := DeleteUsingDeploymentUUID(mockSA.ID, deploymentUUID, api, buf)
		s.NoError(err)
		expectedOut := `Service Account  (ckbvcbqs1014t0760u4bszmcs) successfully deleted
`
		s.Equal(buf.String(), expectedOut)
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentServiceAccount", houston.DeleteServiceAccountRequest{DeploymentID: deploymentUUID, ServiceAccountID: mockSA.ID}).Return(nil, errMock)

		buf := new(bytes.Buffer)
		err := DeleteUsingDeploymentUUID(mockSA.ID, deploymentUUID, api, buf)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeploymentServiceAccount() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
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

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentServiceAccounts", deploymentUUID).Return(mockSAs, nil)

		buf := new(bytes.Buffer)
		err := GetDeploymentServiceAccounts(deploymentUUID, api, buf)
		s.NoError(err)
		expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
		s.Contains(buf.String(), expectedOut)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentServiceAccounts", deploymentUUID).Return([]houston.ServiceAccount{}, errMock)

		buf := new(bytes.Buffer)
		err := GetDeploymentServiceAccounts(deploymentUUID, api, buf)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetWorkspaceServiceAccount() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

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

	s.Run("success", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceServiceAccounts", workspaceUUID).Return(mockSAs, nil)

		buf := new(bytes.Buffer)
		err := GetWorkspaceServiceAccounts(workspaceUUID, api, buf)
		s.NoError(err)
		expectedOut := ` yooo can u see me test                  ckqvfa2cu1468rn9hnr0bqqfk     658b304f36eaaf19860a6d9eb73f7d8a`
		s.Contains(buf.String(), expectedOut)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceServiceAccounts", workspaceUUID).Return([]houston.ServiceAccount{}, errMock)

		buf := new(bytes.Buffer)
		err := GetWorkspaceServiceAccounts(workspaceUUID, api, buf)
		s.EqualError(err, errMock.Error())
		api.AssertExpectations(s.T())
	})
}
