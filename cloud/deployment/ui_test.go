package deployment

import (
	"bytes"
	"errors"

	"github.com/stretchr/testify/mock"

	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestOpenUI() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	var openedURL string
	origOpenURL := openURL
	defer func() { openURL = origOpenURL }()

	s.Run("opens the Airflow UI", func() {
		openedURL = ""
		openURL = func(url string) error {
			openedURL = url
			return nil
		}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		out := new(bytes.Buffer)
		err := OpenUI(ws, "test-id-1", "", out, mockV1Client)
		s.NoError(err)
		s.Equal("https://test-url", openedURL)
		s.Contains(out.String(), "https://test-url")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("prints the link when the browser cannot be opened", func() {
		openURL = func(string) error { return errors.New("no browser") }
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		out := new(bytes.Buffer)
		err := OpenUI(ws, "test-id-1", "", out, mockV1Client)
		s.NoError(err)
		s.Contains(out.String(), "Unable to open the Airflow UI, please visit the following link: https://test-url")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("no Deployments in the Workspace", func() {
		openedURL = ""
		openURL = func(url string) error {
			openedURL = url
			return nil
		}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		out := new(bytes.Buffer)
		err := OpenUI(ws, "", "", out, mockV1Client)
		s.NoError(err)
		s.Contains(out.String(), NoDeploymentInWSMsg)
		s.Empty(openedURL)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("Deployment name not found", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		err := OpenUI(ws, "", "missing", new(bytes.Buffer), mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)
		mockV1Client.AssertExpectations(s.T())
	})
}
