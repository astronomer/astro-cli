package cloud

import (
	"bytes"
	"os"

	airflowclient "github.com/astronomer/astro-cli/airflow-client"
	airflowclient_mocks "github.com/astronomer/astro-cli/airflow-client/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/pkg/errors"
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

func (s *Suite) TestDeploymentConnectionRootCommand() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(os.Stdout)
	cmd.SetOut(buf)
	cmdArgs := []string{"connection", "-h"}
	_, err := execDeploymentCmd(cmdArgs...)
	s.NoError(err)
}

func (s *Suite) TestConnectionList() {
	expectedHelp := "list connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints list help", func() {
		cmdArgs := []string{"connection", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and connections are not listed", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"connection", "list", "-d", "test-deployment-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})
	s.Run("any context errors from api are returned and connections are not listed", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"connection", "list"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})
}

func (s *Suite) TestConnectionCreate() {
	expectedHelp := "Create Airflow connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints create  help", func() {
		cmdArgs := []string{"connection", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and connections are not created", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		cmdArgs := []string{"connection", "create", "-d", "test-deployment-id", "--conn-id", "conn-id", "--conn-type", "conn-type"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})
	s.Run("any context errors from api are returned and connections are not created", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})
	s.Run("successful airflow variable create", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreateConnection", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"connection", "create", "-d", "test-deployment-id", "--conn-id", "conn-id", "--conn-type", "conn-type"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestConnectionUpdate() {
	expectedHelp := "Update existing Airflow connections for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints update help", func() {
		cmdArgs := []string{"connection", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("any errors from api are returned and connections are not updated", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(errTest).Once()
		cmdArgs := []string{"connection", "update", "-d", "test-deployment-id", "--conn-id", "conn-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})

	s.Run("any context errors from api are returned and connections are not updated", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful connection update", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Once()
		cmdArgs := []string{"connection", "update", "-d", "test-deployment-id", "--conn-id", "conn-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestConnectionCopy() {
	expectedHelp := "Copy Airflow connections from one Astro Deployment to another"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints copy help", func() {
		cmdArgs := []string{"connection", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("any errors from api are returned and connections are not copied", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"connection", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})

	s.Run("any context errors from api are returned and connections are not copied", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		mockClient.On("CopyConnections", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
		cmdArgs := []string{"connection", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful connection copy", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient.On("GetConnections", mock.Anything).Return(mockResp, nil).Twice()
		mockClient.On("UpdateConnection", mock.AnythingOfType("string"), mock.AnythingOfType("*airflowclient.Connection")).Return(nil).Twice()
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		cmdArgs := []string{"connection", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestVariableList() {
	expectedHelp := "list Airflow variables stored in an Astro Deployment's metadata database"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints list help", func() {
		cmdArgs := []string{"airflow-variable", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and variables are not listed", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"airflow-variable", "list", "-d", "test-deployment-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})
	s.Run("any context errors from api are returned and variables are not listed", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"airflow-variable", "list", "-d", "test-deployment-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})
}

func (s *Suite) TestVariableUpdate() {
	expectedHelp := "Update Airflow variables for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints update help", func() {
		cmdArgs := []string{"airflow-variable", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("any errors from api are returned and variables are not updated", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"airflow-variable", "update", "-d", "test-deployment-id", "--key", "KEY"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})

	s.Run("any context errors from api are returned and airflow variables are not updated", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful airflow variable update", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "update", "-d", "test-deployment-id", "--key", "KEY"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestVaraibleCreate() {
	expectedHelp := "Create Airflow variables for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints create  help", func() {
		cmdArgs := []string{"airflow-variable", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and variables are not created", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreateVariable", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"airflow-variable", "create", "-d", "test-deployment-id", "--key", "KEY", "--value", "VAR"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})
	s.Run("any context errors from api are returned and variables are not created", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful airflow variable create", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "create", "-d", "test-deployment-id", "--key", "KEY", "--value", "VAR"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestVariableCopy() {
	expectedHelp := "Copy Airflow variables from one Astro Deployment to another Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints copy help", func() {
		cmdArgs := []string{"airflow-variable", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("any errors from api are returned and variables are not copied", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"airflow-variable", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})

	s.Run("any context errors from api are returned and variables are not copied", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		mockClient.On("Copyvariables", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
		cmdArgs := []string{"airflow-variable", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful variable copy", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient.On("GetVariables", mock.Anything).Return(mockResp, nil).Twice()
		mockClient.On("UpdateVariable", mock.AnythingOfType("string"), mock.Anything).Return(nil).Twice()
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		cmdArgs := []string{"airflow-variable", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestPoolList() {
	expectedHelp := "list Airflow pools for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints list help", func() {
		cmdArgs := []string{"pool", "list", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and pools are not listed", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("GetPools", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"pool", "list", "-d", "test-deployment-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})
	s.Run("any context errors from api are returned and pools are not listed", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("GetPools", mock.Anything).Return(mockResp, nil).Once()
		cmdArgs := []string{"pool", "list", "-d", "test-deployment-id"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})
}

func (s *Suite) TestPoolUpdate() {
	expectedHelp := "Update Airflow pools for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints update help", func() {
		cmdArgs := []string{"pool", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("any errors from api are returned and pools are not updated", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"pool", "update", "-d", "test-deployment-id", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})

	s.Run("any context errors from api are returned and pools are not updated", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "update"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful pool update", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "update", "-d", "test-deployment-id", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestPoolCreate() {
	expectedHelp := "Create Airflow pools for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints create  help", func() {
		cmdArgs := []string{"pool", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("any errors from api are returned and pools are not created", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreatePool", mock.AnythingOfType("string"), mock.Anything).Return(errTest).Once()
		cmdArgs := []string{"pool", "create", "-d", "test-deployment-id", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})
	s.Run("any context errors from api are returned and pools are not created", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "create"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful pool create", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		mockClient.On("CreatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Once()
		cmdArgs := []string{"pool", "create", "-d", "test-deployment-id", "--name", "name"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}

func (s *Suite) TestPoolCopy() {
	expectedHelp := "Copy Airflow pools from one Astro Deployment to another Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(airflowclient_mocks.Client)
	airflowAPIClient = mockClient
	mockAstroClient := new(astro_mocks.Client)
	astroClient = mockAstroClient

	s.Run("-h prints copy help", func() {
		cmdArgs := []string{"pool", "copy", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("any errors from api are returned and pools are not copied", func() {
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		mockClient.On("GetPools", mock.Anything).Return(mockResp, errTest).Once()
		cmdArgs := []string{"pool", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.EqualError(err, "error")
	})

	s.Run("any context errors from api are returned and pools are not copied", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		cmdArgs := []string{"pool", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
	})

	s.Run("successful pool copy", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		mockClient.On("GetPools", mock.Anything).Return(mockResp, nil).Twice()
		mockClient.On("UpdatePool", mock.AnythingOfType("string"), mock.Anything).Return(nil).Twice()
		mockAstroClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Twice()
		cmdArgs := []string{"pool", "copy", "--source-id", "test-deployment-id", "--target-id", "test-deployment-id-1"}
		_, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
	})
}
