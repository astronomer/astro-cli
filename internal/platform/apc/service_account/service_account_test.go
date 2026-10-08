package serviceaccount

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var errMock = errors.New("api error")

type Suite struct {
	suite.Suite
}

func TestServiceAccount(t *testing.T) {
	suite.Run(t, new(Suite))
}

// created is what Houston returns for a create: the new row with no role
// bindings loaded, so entityType reads SYSTEM and the deploymentUuid or
// workspaceUuid is null, both being derived from the bindings. The API
// key is whole.
var created = ServiceAccount{
	ID:        "ckbvcbqs1014t0760u4bszmcs",
	APIKey:    "60f2f4f3fa006e3e135dbe99b1391d84",
	Label:     "test",
	Category:  "test",
	CreatedAt: "2020-06-25T22:10:42.385Z",
	Active:    true,
}

var createdRow = houston.ServiceAccount{
	ID: created.ID, APIKey: created.APIKey, Label: created.Label, Category: created.Category,
	CreatedAt: created.CreatedAt, UpdatedAt: created.CreatedAt, Active: true,
}

func (s *Suite) TestCreateUsingDeploymentUUID() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockSA := &houston.DeploymentServiceAccount{ServiceAccount: createdRow, EntityType: "SYSTEM"}
	expectedRequest := &houston.CreateServiceAccountRequest{
		DeploymentID: "ck1qg6whg001r08691y117hub",
		Label:        "test",
		Category:     "test",
		Role:         houston.DeploymentViewerRole,
	}

	s.Run("returns the account with its key", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeploymentServiceAccount", expectedRequest).Return(mockSA, nil)

		got, err := CreateUsingDeploymentUUID(expectedRequest.DeploymentID, "test", "test", houston.DeploymentViewerRole, api)
		s.NoError(err)
		s.Equal(created, got)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateDeploymentServiceAccount", expectedRequest).Return(nil, errMock)

		_, err := CreateUsingDeploymentUUID(expectedRequest.DeploymentID, "test", "test", houston.DeploymentViewerRole, api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestCreateUsingWorkspaceUUID() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	mockSA := &houston.WorkspaceServiceAccount{ServiceAccount: createdRow, EntityType: "SYSTEM"}
	expectedRequest := &houston.CreateServiceAccountRequest{
		WorkspaceID: "ck1qg6whg001r08691y117hub",
		Label:       "test",
		Category:    "test",
		Role:        houston.WorkspaceViewerRole,
	}

	s.Run("returns the account with its key", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateWorkspaceServiceAccount", expectedRequest).Return(mockSA, nil)

		got, err := CreateUsingWorkspaceUUID(expectedRequest.WorkspaceID, "test", "test", houston.WorkspaceViewerRole, api)
		s.NoError(err)
		s.Equal(created, got)
		api.AssertExpectations(s.T())
	})

	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("CreateWorkspaceServiceAccount", expectedRequest).Return(nil, errMock)

		_, err := CreateUsingWorkspaceUUID(expectedRequest.WorkspaceID, "test", "test", houston.WorkspaceViewerRole, api)
		s.ErrorIs(err, errMock)
	})
}

// A list returns each key whole for ten minutes after its create and masked
// after that: its first six characters, then * to the full length.
// lastUsedAt is null until the account is used.
var listed = []houston.ServiceAccount{
	{ID: "sa-1", APIKey: "60f2f4**************************", Label: "ci", Category: "default", CreatedAt: "2020-06-25T22:10:42.385Z", LastUsedAt: "2020-06-26T10:00:00.000Z", Active: true},
	{ID: "sa-2", APIKey: "8d1e0a3b5c7f9e2d4a6b8c0d1e2f3a4b", Label: "new", CreatedAt: "2020-06-27T22:10:42.385Z", Active: true},
}

func (s *Suite) TestGetServiceAccounts() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	want := []ServiceAccount{
		{ID: "sa-1", APIKey: "60f2f4**************************", Label: "ci", Category: "default", CreatedAt: "2020-06-25T22:10:42.385Z", LastUsedAt: "2020-06-26T10:00:00.000Z", Active: true},
		{ID: "sa-2", APIKey: "8d1e0a3b5c7f9e2d4a6b8c0d1e2f3a4b", Label: "new", CreatedAt: "2020-06-27T22:10:42.385Z", Active: true},
	}

	s.Run("deployment", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentServiceAccounts", "dep-1").Return(listed, nil)
		got, err := GetDeploymentServiceAccounts("dep-1", api)
		s.NoError(err)
		s.Equal(want, got)
	})
	s.Run("workspace", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceServiceAccounts", "ws-1").Return(listed, nil)
		got, err := GetWorkspaceServiceAccounts("ws-1", api)
		s.NoError(err)
		s.Equal(want, got)
	})
	s.Run("none is empty, not nil", func() {
		api := new(mocks.ClientInterface)
		api.On("ListWorkspaceServiceAccounts", "ws-1").Return([]houston.ServiceAccount{}, nil)
		got, err := GetWorkspaceServiceAccounts("ws-1", api)
		s.NoError(err)
		s.NotNil(got)
		s.Empty(got)
	})
	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("ListDeploymentServiceAccounts", "dep-1").Return(nil, errMock)
		_, err := GetDeploymentServiceAccounts("dep-1", api)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	deleted := &houston.ServiceAccount{ID: "sa-1", Label: "ci", Category: "default", Active: true}

	s.Run("deployment", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteDeploymentServiceAccount", houston.DeleteServiceAccountRequest{DeploymentID: "dep-1", ServiceAccountID: "sa-1"}).Return(deleted, nil)
		got, err := DeleteUsingDeploymentUUID("sa-1", "dep-1", api)
		s.NoError(err)
		s.Equal(ServiceAccount{ID: "sa-1", Label: "ci", Category: "default", Active: true}, got)
	})
	s.Run("workspace", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceServiceAccount", houston.DeleteServiceAccountRequest{WorkspaceID: "ws-1", ServiceAccountID: "sa-1"}).Return(deleted, nil)
		got, err := DeleteUsingWorkspaceUUID("sa-1", "ws-1", api)
		s.NoError(err)
		s.Equal("sa-1", got.ID)
	})
	// An unknown account is an error, never a null answer.
	s.Run("error", func() {
		api := new(mocks.ClientInterface)
		api.On("DeleteWorkspaceServiceAccount", houston.DeleteServiceAccountRequest{WorkspaceID: "ws-1", ServiceAccountID: "sa-1"}).Return(nil, errMock)
		_, err := DeleteUsingWorkspaceUUID("sa-1", "ws-1", api)
		s.ErrorIs(err, errMock)
	})
}

// Every create and delete mutation returns a nullable ServiceAccount
// (createDeploymentServiceAccount, createWorkspaceServiceAccount,
// deleteDeploymentServiceAccount, deleteWorkspaceServiceAccount). A null
// answer with no error is refused, not read.
func (s *Suite) TestNullAnswers() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	api := new(mocks.ClientInterface)
	api.On("CreateDeploymentServiceAccount", mock.Anything).Return(nil, nil)
	api.On("CreateWorkspaceServiceAccount", mock.Anything).Return(nil, nil)
	api.On("DeleteDeploymentServiceAccount", mock.Anything).Return(nil, nil)
	api.On("DeleteWorkspaceServiceAccount", mock.Anything).Return(nil, nil)

	_, err := CreateUsingDeploymentUUID("dep-1", "ci", "default", houston.DeploymentViewerRole, api)
	s.ErrorIs(err, errNoServiceAccount)
	_, err = CreateUsingWorkspaceUUID("ws-1", "ci", "default", houston.WorkspaceViewerRole, api)
	s.ErrorIs(err, errNoServiceAccount)
	_, err = DeleteUsingDeploymentUUID("sa-1", "dep-1", api)
	s.ErrorIs(err, errNoServiceAccount)
	_, err = DeleteUsingWorkspaceUUID("sa-1", "ws-1", api)
	s.ErrorIs(err, errNoServiceAccount)
}
