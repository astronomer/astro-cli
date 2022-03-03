package settings

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errContainer = errors.New("some container engine error")

func TestAddConnectionsAirflowOne(t *testing.T) {
	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
		ConnURI:      "test-uri",
		ConnExtra:    "test-extras",
	}
	settings.Airflow.Connections = []Connection{testConn}

	mockContainer := new(mocks.ContainerHandler)
	mockContainer.On("ExecCommand", mock.Anything, mock.Anything).Return("no connections found", nil).Once()
	expectedAddCmd := "airflow connections -a  --conn_id \"test-id\" --conn_type \"test-type\" --conn_uri 'test-uri' --conn_extra 'test-extras' --conn_host 'test-host' --conn_login 'test-login' --conn_password 'test-password' --conn_schema 'test-schema' --conn_port 1"
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("connections added", nil).Once()
	AddConnections(mockContainer, "test-conn-id", 1)
	mockContainer.AssertExpectations(t)

	mockContainer.On("ExecCommand", mock.Anything, mock.Anything).Return("'test-id' connection exists", nil).Once()
	expectedDelCmd := "airflow connections -d  --conn_id \"test-id\""
	mockContainer.On("ExecCommand", mock.Anything, expectedDelCmd).Return("connection deleted", nil).Once()
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("connections added", nil).Once()
	AddConnections(mockContainer, "test-conn-id", 1)
	mockContainer.AssertExpectations(t)
}

func TestAddConnectionsAirflowTwo(t *testing.T) {
	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
		ConnURI:      "test-uri",
		ConnExtra:    "test-extras",
	}
	settings.Airflow.Connections = []Connection{testConn}

	mockContainer := new(mocks.ContainerHandler)
	mockContainer.On("ExecCommand", mock.Anything, mock.Anything).Return("no connections found", nil).Once()
	expectedAddCmd := "airflow connections add   \"test-id\" --conn-type \"test-type\" --conn-uri 'test-uri' --conn-extra 'test-extras' --conn-host 'test-host' --conn-login 'test-login' --conn-password 'test-password' --conn-schema 'test-schema' --conn-port 1"
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("connections added", nil).Once()
	AddConnections(mockContainer, "test-conn-id", 2)
	mockContainer.AssertExpectations(t)

	mockContainer.On("ExecCommand", mock.Anything, mock.Anything).Return("'test-id' connection exists", nil).Once()
	expectedDelCmd := "airflow connections delete   \"test-id\""
	mockContainer.On("ExecCommand", mock.Anything, expectedDelCmd).Return("connection deleted", nil).Once()
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("connections added", nil).Once()
	AddConnections(mockContainer, "test-conn-id", 2)
	mockContainer.AssertExpectations(t)
}

func TestAddConnectionsFailure(t *testing.T) {
	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
		ConnURI:      "test-uri",
		ConnExtra:    "test-extras",
	}
	settings.Airflow.Connections = []Connection{testConn}

	mockContainer := new(mocks.ContainerHandler)
	mockContainer.On("ExecCommand", mock.Anything, mock.Anything).Return("no connections found", nil).Once()
	expectedAddCmd := "airflow connections add   \"test-id\" --conn-type \"test-type\" --conn-uri 'test-uri' --conn-extra 'test-extras' --conn-host 'test-host' --conn-login 'test-login' --conn-password 'test-password' --conn-schema 'test-schema' --conn-port 1"
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("", errContainer).Once()

	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	AddConnections(mockContainer, "test-conn-id", 2)

	w.Close()
	out, _ := ioutil.ReadAll(r)
	os.Stdout = stdout
	mockContainer.AssertExpectations(t)
	assert.Contains(t, string(out), errContainer.Error())
	assert.Contains(t, string(out), "Error adding connection")
}

func TestAddVariableAirflowOne(t *testing.T) {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	mockContainer := new(mocks.ContainerHandler)
	expectedAddCmd := "airflow variables -s test-var-name 'test-var-val'"
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("variable added", nil).Once()
	AddVariables(mockContainer, "test-conn-id", 1)
	mockContainer.AssertExpectations(t)
}

func TestAddVariableAirflowTwo(t *testing.T) {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	mockContainer := new(mocks.ContainerHandler)
	expectedAddCmd := "airflow variables set test-var-name 'test-var-val'"
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("variable added", nil).Once()
	AddVariables(mockContainer, "test-conn-id", 2)
	mockContainer.AssertExpectations(t)
}

func TestAddVariableFailure(t *testing.T) {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	mockContainer := new(mocks.ContainerHandler)
	expectedAddCmd := "airflow variables set test-var-name 'test-var-val'"
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("", errContainer).Once()

	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	AddVariables(mockContainer, "test-conn-id", 2)

	w.Close()
	out, _ := ioutil.ReadAll(r)
	os.Stdout = stdout
	mockContainer.AssertExpectations(t)
	assert.Contains(t, string(out), errContainer.Error())
	assert.Contains(t, string(out), "Error adding variable")
}

func TestAddPoolsAirflowOne(t *testing.T) {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
		},
	}

	mockContainer := new(mocks.ContainerHandler)
	expectedAddCmd := "airflow pool -s  test-pool-name 1 'test-pool-description' "
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("pool added", nil).Once()
	AddPools(mockContainer, "test-conn-id", 1)
	mockContainer.AssertExpectations(t)
}

func TestAddPoolsAirflowTwo(t *testing.T) {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
		},
	}

	mockContainer := new(mocks.ContainerHandler)
	expectedAddCmd := "airflow pools set  test-pool-name 1 'test-pool-description' "
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("variable added", nil).Once()
	AddPools(mockContainer, "test-conn-id", 2)
	mockContainer.AssertExpectations(t)
}

func TestAddPoolsFailure(t *testing.T) {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
		},
	}

	mockContainer := new(mocks.ContainerHandler)
	expectedAddCmd := "airflow pools set  test-pool-name 1 'test-pool-description' "
	mockContainer.On("ExecCommand", mock.Anything, expectedAddCmd).Return("", errContainer).Once()

	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	AddPools(mockContainer, "test-conn-id", 2)

	w.Close()
	out, _ := ioutil.ReadAll(r)
	os.Stdout = stdout
	mockContainer.AssertExpectations(t)
	assert.Contains(t, string(out), errContainer.Error())
	assert.Contains(t, string(out), "Error adding pool")
}
