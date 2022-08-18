package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddConnectionsAirflowOne(t *testing.T) {
	var testExtra map[string]string

	testConn := Connection{
		Conn_ID:       "test-id",
		Conn_Type:     "test-type",
		Conn_Host:     "test-host",
		Conn_Schema:   "test-schema",
		Conn_Login:    "test-login",
		Conn_Password: "test-password",
		Conn_Port:     1,
		Conn_URI:      "test-uri",
		Conn_Extra:    testExtra,
	}
	settings.Airflow.Connections = []Connection{testConn}

	expectedAddCmd := "airflow connections -a  --conn_id 'test-id' --conn_type 'test-type' --conn_uri 'test-uri' --conn_host 'test-host' --conn_login 'test-login' --conn_password 'test-password' --conn_schema 'test-schema' --conn_port 1"
	expectedListCmd := "airflow connections -l "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd}, airflowCommand)
		return ""
	}
	AddConnections("test-conn-id", 1)
}

func TestAddConnectionsAirflowTwo(t *testing.T) {
	var testExtra map[string]string

	testConn := Connection{
		Conn_ID:       "test-id",
		Conn_Type:     "test-type",
		Conn_Host:     "test-host",
		Conn_Schema:   "test-schema",
		Conn_Login:    "test-login",
		Conn_Password: "test-password",
		Conn_Port:     1,
		Conn_URI:      "test-uri",
		Conn_Extra:    testExtra,
	}
	settings.Airflow.Connections = []Connection{testConn}

	expectedAddCmd := "airflow connections add   'test-id' --conn-type 'test-type' --conn-uri 'test-uri' --conn-host 'test-host' --conn-login 'test-login' --conn-password 'test-password' --conn-schema 'test-schema' --conn-port 1"
	expectedDelCmd := "airflow connections delete   \"test-id\""
	expectedListCmd := "airflow connections list -o plain"
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'"
		}
		return ""
	}
	AddConnections("test-conn-id", 2)
}

func TestAddVariableAirflowOne(t *testing.T) {
	settings.Airflow.Variables = Variables{
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
	AddVariables("test-conn-id", 1)
}

func TestAddVariableAirflowTwo(t *testing.T) {
	settings.Airflow.Variables = Variables{
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
	AddVariables("test-conn-id", 2)
}

func TestAddPoolsAirflowOne(t *testing.T) {
	settings.Airflow.Pools = Pools{
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
	AddPools("test-conn-id", 1)
}

func TestAddPoolsAirflowTwo(t *testing.T) {
	settings.Airflow.Pools = Pools{
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
	AddPools("test-conn-id", 2)
}

func TestInitSettingsSuccess(t *testing.T) {
	WorkingPath = "./testfiles/"
	ConfigFileName := "airflow_settings.yaml"
	err := InitSettings(ConfigFileName)
	assert.NoError(t, err)
}

func TestInitSettingsFailure(t *testing.T) {
	WorkingPath = "./testfiles/"
	ConfigFileName := "airflow_settings_invalid"
	err := InitSettings(ConfigFileName)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to decode into struct")
}
