package cloud

import (
	"bytes"
	"os"
	"testing"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	airflowclient_mocks "github.com/astronomer/astro-cli/airflow-client/mocks"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockResp = airflowclient.Response{
		Connections: []airflowclient.Connection{
			{ConnID: "conn1", ConnType: "type1"},
			{ConnID: "conn2", ConnType: "type2"},
		},
	}
	errTest = errors.New("error")
)

func TestDeploymentConnectionRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"connection", "-h"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestConnectionList(t *testing.T) {
	expectedHelp := "list connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"connection", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and connections are not listed", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"connection", "list", "-d", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("any context errors from api are returned and connections are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"connection", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestConnectionCreate(t *testing.T) {
	expectedHelp := "Create Airflow connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints create  help", func(t *testing.T) {
		cmdArgs := []string{"connection", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and connections are not created", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		cmdArgs := []string{"connection", "create", "-d", "test-id-1", "--conn-id", "conn-id", "--conn-type", "conn-type"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("any context errors from api are returned and connections are not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		cmdArgs := []string{"connection", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("successful airflow variable create", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"connection", "create", "-d", "test-id-1", "--conn-id", "conn-id", "--conn-type", "conn-type"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestConnectionUpdate(t *testing.T) {
	expectedHelp := "Update existing Airflow connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"connection", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and connections are not updated", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		cmdArgs := []string{"connection", "update", "-d", "test-id-1", "--conn-id", "conn-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("any context errors from api are returned and connections are not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful connection update", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "update", "-d", "test-id-1", "--conn-id", "conn-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestConnectionCopy(t *testing.T) {
	expectedHelp := "Copy Airflow connections from one Astro Deployment to another"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints copy help", func(t *testing.T) {
		cmdArgs := []string{"connection", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and connections are not copied", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"connection", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("any context errors from api are returned and connections are not copied", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("CopyConnections", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
		cmdArgs := []string{"connection", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful connection copy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, nil).Twice()
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Twice()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"connection", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestVariableList(t *testing.T) {
	expectedHelp := "list Airflow variables stored in an Astro Deployment's metadata database"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"airflow-variable", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and variables are not listed", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"airflow-variable", "list", "-d", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("any context errors from api are returned and variables are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"airflow-variable", "list", "-d", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestVariableUpdate(t *testing.T) {
	expectedHelp := "Update Airflow variables for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"airflow-variable", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and variables are not updated", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"airflow-variable", "update", "-d", "test-id-1", "--key", "KEY"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("any context errors from api are returned and airflow variables are not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		cmdArgs := []string{"airflow-variable", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful airflow variable update", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "update", "-d", "test-id-1", "--key", "KEY"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestVaraibleCreate(t *testing.T) {
	expectedHelp := "Create Airflow variables for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints create  help", func(t *testing.T) {
		cmdArgs := []string{"airflow-variable", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and variables are not created", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("CreateVariable", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"airflow-variable", "create", "-d", "test-id-1", "--key", "KEY", "--value", "VAR"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("any context errors from api are returned and variables are not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("CreateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful airflow variable create", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("CreateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "create", "-d", "test-id-1", "--key", "KEY", "--value", "VAR"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestVariableCopy(t *testing.T) {
	expectedHelp := "Copy Airflow variables from one Astro Deployment to another Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints copy help", func(t *testing.T) {
		cmdArgs := []string{"airflow-variable", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and variables are not copied", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"airflow-variable", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("any context errors from api are returned and variables are not copied", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("Copyvariables", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful variable copy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, nil).Twice()
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Twice()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"airflow-variable", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestPoolList(t *testing.T) {
	expectedHelp := "list Airflow pools for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"pool", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and pools are not listed", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("GetPools", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"pool", "list", "-d", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("any context errors from api are returned and pools are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("GetPools", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"pool", "list", "-d", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestPoolUpdate(t *testing.T) {
	expectedHelp := "Update Airflow pools for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"pool", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and pools are not updated", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"pool", "update", "-d", "test-id-1", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("any context errors from api are returned and pools are not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful pool update", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "update", "-d", "test-id-1", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestPoolCreate(t *testing.T) {
	expectedHelp := "Create Airflow pools for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints create  help", func(t *testing.T) {
		cmdArgs := []string{"pool", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and pools are not created", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("CreatePool", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"pool", "create", "-d", "test-id-1", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("any context errors from api are returned and pools are not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient.On("CreatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful pool create", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockClient.On("CreatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "create", "-d", "test-id-1", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestPoolCopy(t *testing.T) {
	expectedHelp := "Copy Airflow pools from one Astro Deployment to another Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	platformCoreClient = mockPlatformCoreClient

	t.Run("-h prints copy help", func(t *testing.T) {
		cmdArgs := []string{"pool", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and pools are not copied", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		mockClient.On("GetPools", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"pool", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("any context errors from api are returned and pools are not copied", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		cmdArgs := []string{"pool", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
	})

	t.Run("successful pool copy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient.On("GetPools", mock.Anything).Return(mockResp, nil).Twice()
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Twice()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"pool", "copy", "--source-id", "test-id-1", "--target-id", "test-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockPlatformCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}
