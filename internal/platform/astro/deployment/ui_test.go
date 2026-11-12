package deployment

import (
	"errors"

	"github.com/stretchr/testify/mock"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestUI() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("finds the Airflow UI", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		res, err := UI(ws, "test-id-1", "", mockV1Client)
		s.NoError(err)
		s.Equal("https://test-url", res.URL)
		s.Equal("test-id-1", res.DeploymentID)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("no Deployments in the Workspace", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		_, err := UI(ws, "", "", mockV1Client)
		var none *NoDeploymentsError
		s.ErrorAs(err, &none)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("Deployment name not found", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := UI(ws, "", "missing", mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestOpenInBrowser() {
	origOpenURL := openURL
	defer func() { openURL = origOpenURL }()

	var opened string
	openURL = func(url string) error {
		opened = url
		return nil
	}
	s.True(OpenInBrowser("https://test-url"))
	s.Equal("https://test-url", opened)

	openURL = func(string) error { return errors.New("no browser") }
	s.False(OpenInBrowser("https://test-url"))
}
