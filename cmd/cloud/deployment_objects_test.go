package cloud

import (
	"bytes"
	"os"
	"testing"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	airflowclient_mocks "github.com/astronomer/astro-cli/airflow-client/mocks"
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
	_, err := execWorkspaceCmd(cmdArgs...)
	assert.NoError(t, err)
}

func TestConnectionList(t *testing.T) {
	expectedHelp := "list connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints list help", func(t *testing.T) {
		cmdArgs := []string{"connection", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and connections are not listed", func(t *testing.T) {
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetConnections", mock.AnythingOfType("string")).Return(mockResp, errTest).Once()
		cmdArgs := []string{"connection", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "error")
	})
	t.Run("any context errors from api are returned and connections are not listed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"connection", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestConnectionCreate(t *testing.T) {
	expectedHelp := "Create connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints create  help", func(t *testing.T) {
		cmdArgs := []string{"connection", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("any errors from api are returned and connections are not created", func(t *testing.T) {
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		cmdArgs := []string{"connection", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to create connections")
	})
	t.Run("any context errors from api are returned and connections are not created", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
}

func TestConnectionUpdate(t *testing.T) {
	expectedHelp := "Update connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints update help", func(t *testing.T) {
		cmdArgs := []string{"connection", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and connections are not updated", func(t *testing.T) {
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		cmdArgs := []string{"connection", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to update connections")
	})

	t.Run("any context errors from api are returned and connections are not updated", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("successful connection update", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "update", "my-connection", "--conn-type", "http", "--conn-host", "http://example.com", "--conn-port", "8080"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, "Successfully updated connection")
	})
}

func TestConnectionCopy(t *testing.T) {
	expectedHelp := "Copy connections from one Astro Deployment to another"
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("-h prints copy help", func(t *testing.T) {
		cmdArgs := []string{"connection", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("any errors from api are returned and connections are not copied", func(t *testing.T) {
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CopyConnections", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(errTest).Once()
		cmdArgs := []string{"connection", "copy", "source-deployment", "destination-deployment"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.EqualError(t, err, "failed to copy connections")
	})

	t.Run("any context errors from api are returned and connections are not copied", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CopyConnections", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
		cmdArgs := []string{"connection", "copy", "source-deployment", "destination-deployment"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})

	t.Run("successful connection copy", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient := new(airflowclient_mocks.Client)
		mockClient.On("CopyConnections", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
		cmdArgs := []string{"connection", "copy", "source-deployment", "destination-deployment"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, "Successfully copied connections")
	})
}
