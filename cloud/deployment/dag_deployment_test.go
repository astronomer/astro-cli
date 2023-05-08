package deployment

import (
	"errors"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/stretchr/testify/mock"
)

var errorMock = errors.New("mock error")

func (s *Suite) TestInitiate() {
	initiatedDagDeploymentID := "test-dag-deployment-id"
	dagURL := "test-dag-url"
	runtimeID := "test-id"
	s.Run("initiate dag deployment with correct deployment ID", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Once()

		initiateDagDeployment, err := Initiate(runtimeID, mockClient)
		s.NoError(err)
		s.Equal(initiatedDagDeploymentID, initiateDagDeployment.ID)
		s.Equal(dagURL, initiateDagDeployment.DagURL)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{RuntimeID: runtimeID}).Return(astro.InitiateDagDeployment{}, errorMock).Once()

		_, err := Initiate(runtimeID, mockClient)
		s.ErrorIs(err, errorMock)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestReportDagDeploymentStatus() {
	initiatedDagDeploymentID := "test-dag-deployment-id"
	dagDeploymentStatusID := "test-dag-deployment-status-id"
	runtimeID := "test-id"
	action := "UPLOAD"
	versionID := "version-id"
	status := "SUCCESS"
	message := "some-message"
	createdAt := "created-date"
	initiatorID := "initiator-id"
	initiatorType := "user"

	s.Run("successfully reports dag deployment status", func() {
		mockClient := new(astro_mocks.Client)
		mockResponse := astro.DagDeploymentStatus{
			ID:            dagDeploymentStatusID,
			RuntimeID:     runtimeID,
			Action:        action,
			VersionID:     versionID,
			Status:        status,
			Message:       message,
			CreatedAt:     createdAt,
			InitiatorID:   initiatorID,
			InitiatorType: initiatorType,
		}
		mockClient.On("ReportDagDeploymentStatus", mock.Anything).Return(mockResponse, nil).Once()

		dagDeploymentStatus, err := ReportDagDeploymentStatus(initiatedDagDeploymentID, runtimeID, action, versionID, status, message, mockClient)
		s.NoError(err)
		s.Equal(dagDeploymentStatusID, dagDeploymentStatus.ID)
		s.Equal(runtimeID, dagDeploymentStatus.RuntimeID)
		s.Equal(action, dagDeploymentStatus.Action)
		s.Equal(versionID, dagDeploymentStatus.VersionID)
		s.Equal(status, dagDeploymentStatus.Status)
		s.Equal(message, dagDeploymentStatus.Message)
		s.Equal(createdAt, dagDeploymentStatus.CreatedAt)
		s.Equal(initiatorID, dagDeploymentStatus.InitiatorID)
		s.Equal(initiatorType, dagDeploymentStatus.InitiatorType)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ReportDagDeploymentStatus", mock.Anything).Return(astro.DagDeploymentStatus{}, errorMock).Once()

		_, err := ReportDagDeploymentStatus(initiatedDagDeploymentID, runtimeID, action, versionID, status, message, mockClient)
		s.ErrorIs(err, errorMock)
		mockClient.AssertExpectations(s.T())
	})
}
