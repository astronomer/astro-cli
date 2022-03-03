package settings

import (
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/stretchr/testify/mock"
)

func TestAddConnections(t *testing.T) {
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
