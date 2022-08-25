package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddConnectionsAirflowOneOld(t *testing.T) {
	testConn := OldConnection{
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
	oldSettings.OldAirflow.OldConnections = []OldConnection{testConn}

	expectedAddCmd := "airflow connections -a  --conn_id 'test-id' --conn_type 'test-type' --conn_uri 'test-uri' --conn_extra 'test-extras' --conn_host 'test-host' --conn_login 'test-login' --conn_password 'test-password' --conn_schema 'test-schema' --conn_port 1"
	expectedListCmd := "airflow connections -l "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd}, airflowCommand)
		return ""
	}
	AddConnectionsOld("test-conn-id", 1)
}

func TestAddConnectionsAirflowTwoOld(t *testing.T) {
	testConn := OldConnection{
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
	oldSettings.OldAirflow.OldConnections = []OldConnection{testConn}

	expectedAddCmd := "airflow connections add   'test-id' --conn-type 'test-type' --conn-uri 'test-uri' --conn-extra 'test-extras' --conn-host 'test-host' --conn-login 'test-login' --conn-password 'test-password' --conn-schema 'test-schema' --conn-port 1"
	expectedDelCmd := "airflow connections delete   \"test-id\""
	expectedListCmd := "airflow connections list "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri' 'test-extras'"
		}
		return ""
	}
	AddConnectionsOld("test-conn-id", 2)
}

func TestAddVariableAirflowOneOld(t *testing.T) {
	oldSettings.OldAirflow.Variables = Variables{
		{
			Variable_Name:  "test-var-name",
			Variable_Value: "test-var-val",
		},
	}

	expectedAddCmd := "airflow variables -s test-var-name'test-var-val'"
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Equal(t, expectedAddCmd, airflowCommand)
		return ""
	}
	AddVariablesOld("test-conn-id", 1)
}

func TestAddVariableAirflowTwoOld(t *testing.T) {
	oldSettings.OldAirflow.Variables = Variables{
		{
			Variable_Name:  "test-var-name",
			Variable_Value: "test-var-val",
		},
	}

	expectedAddCmd := "airflow variables set test-var-name 'test-var-val'"
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Equal(t, expectedAddCmd, airflowCommand)
		return ""
	}
	AddVariablesOld("test-conn-id", 2)
}

func TestAddPoolsAirflowOneOld(t *testing.T) {
	oldSettings.OldAirflow.Pools = Pools{
		{
			Pool_Name:        "test-pool-name",
			Pool_Slot:        1,
			Pool_Description: "test-pool-description",
		},
	}

	expectedAddCmd := "airflow pool -s  test-pool-name 1 'test-pool-description' "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Equal(t, expectedAddCmd, airflowCommand)
		return ""
	}
	AddPoolsOld("test-conn-id", 1)
}

func TestAddPoolsAirflowTwoOld(t *testing.T) {
	oldSettings.OldAirflow.Pools = Pools{
		{
			Pool_Name:        "test-pool-name",
			Pool_Slot:        1,
			Pool_Description: "test-pool-description",
		},
	}

	expectedAddCmd := "airflow pools set  test-pool-name 1 'test-pool-description' "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Equal(t, expectedAddCmd, airflowCommand)
		return ""
	}
	AddPoolsOld("test-conn-id", 2)
}
