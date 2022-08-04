package deployment

import (
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errorMock = errors.New("mock error")

func TestInitiate(t *testing.T) {
	initiatedDagDeploymentID := "test-dag-deployment-id"
	dagURL := "test-dag-url"
	deploymentID := "test-id"
	t.Run("initiate dag deployment with correct deployment ID", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{DeploymentID: deploymentID}).Return(astro.InitiateDagDeployment{ID: initiatedDagDeploymentID, DagURL: dagURL}, nil).Once()

		initiateDagDeployment, err := Initiate(deploymentID, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, initiatedDagDeploymentID, initiateDagDeployment.ID)
		assert.Equal(t, dagURL, initiateDagDeployment.DagURL)
		mockClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("InitiateDagDeployment", astro.InitiateDagDeploymentInput{DeploymentID: deploymentID}).Return(astro.InitiateDagDeployment{}, errorMock).Once()

		_, err := Initiate(deploymentID, mockClient)
		assert.ErrorIs(t, err, errorMock)
		mockClient.AssertExpectations(t)
	})
}

func TestReportDagDeploymentStatus(t *testing.T) {
	initiatedDagDeploymentID := "test-dag-deployment-id"
	dagDeploymentStatusID := "test-dag-deployment-status-id"
	deploymentID := "test-id"
	action := "UPLOAD"
	versionID := "version-id"
	status := "SUCCESS"
	message := "some-message"
	createdAt := "created-date"
	initiatorID := "initiator-id"
	initiatorType := "user"

	t.Run("successfully reports dag deployment status", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockResponse := astro.DagDeploymentStatus{
			ID:            dagDeploymentStatusID,
			DeploymentID:  deploymentID,
			Action:        action,
			VersionID:     versionID,
			Status:        status,
			Message:       message,
			CreatedAt:     createdAt,
			InitiatorID:   initiatorID,
			InitiatorType: initiatorType,
		}
		mockClient.On("ReportDagDeploymentStatus", mock.Anything).Return(mockResponse, nil).Once()

		dagDeploymentStatus, err := ReportDagDeploymentStatus(initiatedDagDeploymentID, deploymentID, action, versionID, status, message, mockClient)
		assert.NoError(t, err)
		assert.Equal(t, dagDeploymentStatusID, dagDeploymentStatus.ID)
		assert.Equal(t, deploymentID, dagDeploymentStatus.DeploymentID)
		assert.Equal(t, action, dagDeploymentStatus.Action)
		assert.Equal(t, versionID, dagDeploymentStatus.VersionID)
		assert.Equal(t, status, dagDeploymentStatus.Status)
		assert.Equal(t, message, dagDeploymentStatus.Message)
		assert.Equal(t, createdAt, dagDeploymentStatus.CreatedAt)
		assert.Equal(t, initiatorID, dagDeploymentStatus.InitiatorID)
		assert.Equal(t, initiatorType, dagDeploymentStatus.InitiatorType)

		mockClient.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("ReportDagDeploymentStatus", mock.Anything).Return(astro.DagDeploymentStatus{}, errorMock).Once()

		_, err := ReportDagDeploymentStatus(initiatedDagDeploymentID, deploymentID, action, versionID, status, message, mockClient)
		assert.ErrorIs(t, err, errorMock)
		mockClient.AssertExpectations(t)
	})
}
