package cloud

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestNewDeploymentInspectCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client

	t.Run("returns deployment in yaml format when a deployment name was provided", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		cmdArgs := []string{"inspect", "-n", "test"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, deploymentResponse.JSON200.Namespace)
		assert.Contains(t, resp, deploymentResponse.JSON200.Name)
		assert.Contains(t, resp, deploymentResponse.JSON200.RuntimeVersion)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns deployment in yaml format when a deployment id was provided", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		cmdArgs := []string{"inspect", "test-id-1"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, deploymentResponse.JSON200.Namespace)
		assert.Contains(t, resp, deploymentResponse.JSON200.Name)
		assert.Contains(t, resp, deploymentResponse.JSON200.RuntimeVersion)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns deployment template in yaml format when a deployment id was provided", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		cmdArgs := []string{"inspect", "test-id-1", "--template"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, deploymentResponse.JSON200.RuntimeVersion)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns a deployment's specific field", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		cmdArgs := []string{"inspect", "-n", "test", "-k", "metadata.cluster_id"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns empty workload identity when show-workload-identity flag is not passed", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		cmdArgs := []string{"inspect", "-n", "test"}
		out, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, out, `workload_identity: ""`)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns workload identity when show-workload-identity flag is passed", func(t *testing.T) {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		cmdArgs := []string{"inspect", "-n", "test", "--show-workload-identity"}
		out, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, out, fmt.Sprintf(`workload_identity: %s`, mockWorkloadIdentity))
		mockV1Client.AssertExpectations(t)
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"inspect", "-n", "doesnotexist"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.NotContains(t, resp, expectedOut)
	})
}
