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

	expectedAddCmd := "airflow connections -a  --conn_id 'test-id' --conn_type 'test-type' --conn_host 'test-host' --conn_login 'test-login' --conn_password 'test-password' --conn_schema 'test-schema' --conn_port 1"
	expectedListCmd := "airflow connections -l "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd}, airflowCommand)
		return ""
	}
	AddConnections("test-conn-id", 1, true)
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

	expectedAddCmd := "airflow connections add   'test-id' --conn-type 'test-type' --conn-host 'test-host' --conn-login 'test-login' --conn-password 'test-password' --conn-schema 'test-schema' --conn-port 1"
	expectedDelCmd := "airflow connections delete   \"test-id\""
	expectedListCmd := "airflow connections list -o plain"
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'"
		}
		return ""
	}
	AddConnections("test-conn-id", 2, true)
}

func TestAddConnectionsAirflowTwoURI(t *testing.T) {
	testConn := Connection{
		Conn_URI: "test-uri",
	}
	settings.Airflow.Connections = []Connection{testConn}

	expectedAddCmd := "airflow connections add   'test-id' --conn-uri 'test-uri'"
	expectedDelCmd := "airflow connections delete   \"test-id\""
	expectedListCmd := "airflow connections list -o plain"
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'"
		}
		return ""
	}
	AddConnections("test-conn-id", 2, true)
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
	AddVariables("test-conn-id", 1, true)
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
	AddVariables("test-conn-id", 2, true)
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
	AddPools("test-conn-id", 1, true)
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
	AddPools("test-conn-id", 2, true)
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
	assert.Contains(t, err.Error(), "unable to decode file")
}

func TestEnvExport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case "airflow variables export tmp.var":
				return "1 variables successfully exported to tmp.var"
			case "cat tmp.var":
				return `{
					"myvar": "myval"
				}`
			case "airflow connections export tmp.connections --file-format env":
				return "Connections successfully exported to tmp.json"
			case "cat tmp.connections":
				return "local_postgres=postgres://username:password@example.db.example.com:5432/schema"
			default:
				return ""
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, true, true, true)
		assert.NoError(t, err)
	})
	t.Run("missing id", func(t *testing.T) {
		err := EnvExport("", "", 2, true, true, true)
		assert.ErrorIs(t, err, errNoID)
	})

	t.Run("variable failure", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case "airflow variables export tmp.var":
				return ""
			default:
				return ""
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, false, true, true)
		assert.Contains(t, err.Error(), "there was an error during env export")
	})

	t.Run("connection failure", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case "airflow connections export tmp.connections --file-format env":
				return ""
			default:
				return ""
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, true, false, true)
		assert.Contains(t, err.Error(), "there was an error during env export")
	})

	t.Run("not airflow 2", func(t *testing.T) {
		err := EnvExport("id", "", 1, true, true, true)
		assert.Contains(t, err.Error(), "Command must be used with Airflow 2.X")
	})
}

func TestExport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case "airflow connections list -o yaml":
				return `
- conn_id: local_postgres
  conn_type: postgres
  description: null
  extra_dejson:
  get_uri: postgres://username:password@example.db.example.com:5432/schema
  host: example.db.example.com
  id: '11'
  is_encrypted: 'True'
  is_extra_encrypted: 'True'
  login: username
  password: password
  port: '5432'
  schema: schema`
			case "airflow variables export tmp.var":
				return "1 variables successfully exported to tmp.var"
			case "cat tmp.var":
				return `{
					"myvar": "myval"
				}`
			case "airflow pools list -o yaml":
				return `
- description: Default pool
  pool: default_pool
  slots: '128'
- description: ''
  pool: subdag_limit
  slots: '3'`
			default:
				return ""
			}
		}

		err := Export("id", "airflow_settings_export.yaml", 2, true, true, true, true)
		assert.NoError(t, err)
	})

	t.Run("variable failure", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case "airflow variables export tmp.var":
				return ""
			default:
				return ""
			}
		}

		err := Export("id", "airflow_settings_export.yaml", 2, false, true, false, true)
		assert.Contains(t, err.Error(), "there was an error during export")
	})

	t.Run("missing id", func(t *testing.T) {
		err := Export("", "", 2, true, true, true, true)
		assert.ErrorIs(t, err, errNoID)
	})

	t.Run("not airflow 2", func(t *testing.T) {
		err := Export("id", "", 1, true, true, true, true)
		assert.Contains(t, err.Error(), "Command must be used with Airflow 2.X")
	})
}
