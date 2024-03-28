package settings

import (
	"encoding/json"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/stretchr/testify/assert"
)

func TestConfigSettings(t *testing.T) {
	// config settings success
	err := ConfigSettings("container-id", "", nil, 2, false, false, false)
	assert.NoError(t, err)
	// config setttings no id error
	err = ConfigSettings("", "", nil, 2, false, false, false)
	assert.ErrorIs(t, err, errNoID)
	// config settings settings file error
	err = ConfigSettings("container-id", "testfiles/airflow_settings_invalid.yaml", nil, 2, false, false, false)
	assert.Contains(t, err.Error(), "unable to decode file")
}

func TestAddConnectionsAirflowOne(t *testing.T) {
	var testExtra map[string]string

	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
		ConnURI:      "test-uri",
		ConnExtra:    testExtra,
	}
	settings.Airflow.Connections = []Connection{testConn}

	expectedAddCmd := "airflow connections -a  --conn_id 'test-id' --conn_type 'test-type' --conn_host 'test-host' --conn_login 'test-login' --conn_password 'test-password' --conn_schema 'test-schema' --conn_port 1"
	expectedListCmd := "airflow connections -l "
	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedListCmd}, airflowCommand)
		return ""
	}
	AddConnections("test-conn-id", 1, nil)
}

func TestAddConnectionsAirflowTwo(t *testing.T) {
	var testExtra map[string]string

	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
		ConnURI:      "test-uri",
		ConnExtra:    testExtra,
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
	AddConnections("test-conn-id", 2, nil)
}

func ptr[T any](t T) *T {
	return &t
}

func TestAddConnectionsAirflowTwoWithEnvConns(t *testing.T) {
	var testExtra map[string]string

	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
		ConnURI:      "test-uri",
		ConnExtra:    testExtra,
	}
	settings.Airflow.Connections = []Connection{testConn}

	envConns := map[string]astrocore.EnvironmentObjectConnection{
		"test-env-id": {
			Type:     "test-env-type",
			Host:     ptr("test-env-host"),
			Port:     ptr(2),
			Login:    ptr("test-env-login"),
			Password: ptr("test-env-password"),
			Schema:   ptr("test-env-schema"),
			Extra: &map[string]any{
				"test-extra-key": "test-extra-value",
			},
		},
	}

	expectedAddCmd := "airflow connections add   'test-id' --conn-type 'test-type' --conn-host 'test-host' --conn-login 'test-login' --conn-password 'test-password' --conn-schema 'test-schema' --conn-port 1"
	expectedDelCmd := "airflow connections delete   \"test-id\""
	expectedListCmd := "airflow connections list -o plain"

	expectedEnvAddCmd := "airflow connections add   'test-env-id' --conn-type 'test-env-type' --conn-extra '{\"test-extra-key\":\"test-extra-value\"}' --conn-host 'test-env-host' --conn-login 'test-env-login' --conn-password 'test-env-password' --conn-schema 'test-env-schema' --conn-port 2"

	execAirflowCommand = func(id, airflowCommand string) string {
		assert.Contains(t, []string{expectedAddCmd, expectedEnvAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'"
		}
		return ""
	}
	AddConnections("test-conn-id", 2, envConns)
}

func TestAddConnectionsAirflowTwoURI(t *testing.T) {
	testConn := Connection{
		ConnURI: "test-uri",
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
	AddConnections("test-conn-id", 2, nil)
}

func TestAddVariableAirflowOne(t *testing.T) {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
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
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
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
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
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
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
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
	assert.Contains(t, err.Error(), "unable to decode file")
}

func TestEnvExport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case airflowVarExport:
				return "1 variables successfully exported to tmp.var"
			case catVarFile:
				return `{
					"myvar": "myval"
				}`
			case airflowConnExport:
				return "Connections successfully exported to tmp.json"
			case catConnFile:
				return "local_postgres=postgres://username:password@example.db.example.com:5432/schema"
			default:
				return ""
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, true, true)
		assert.NoError(t, err)
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})
	t.Run("missing id", func(t *testing.T) {
		err := EnvExport("", "", 2, true, true)
		assert.ErrorIs(t, err, errNoID)
	})

	t.Run("variable failure", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case airflowVarExport:
				return ""
			default:
				return ""
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, false, true)
		assert.Contains(t, err.Error(), "there was an error during env export")
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})

	t.Run("connection failure", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case airflowConnExport:
				return ""
			default:
				return ""
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, true, false)
		assert.Contains(t, err.Error(), "there was an error during env export")
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})

	t.Run("not airflow 2", func(t *testing.T) {
		err := EnvExport("id", "", 1, true, true)
		assert.Contains(t, err.Error(), "Command must be used with Airflow 2.X")
	})
}

func TestExport(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case airflowConnectionList:
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
			case airflowVarExport:
				return "1 variables successfully exported to tmp.var"
			case catVarFile:
				return `{
					"myvar": "myval"
				}`
			case ariflowPoolsList:
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

		err := Export("id", "airflow_settings_export.yaml", 2, true, true, true)
		assert.NoError(t, err)
	})

	t.Run("variable failure", func(t *testing.T) {
		execAirflowCommand = func(id, airflowCommand string) string {
			switch airflowCommand {
			case airflowVarExport:
				return ""
			default:
				return ""
			}
		}

		err := Export("id", "airflow_settings_export.yaml", 2, false, true, false)
		assert.Contains(t, err.Error(), "there was an error during export")
	})

	t.Run("missing id", func(t *testing.T) {
		err := Export("", "", 2, true, true, true)
		assert.ErrorIs(t, err, errNoID)
	})

	t.Run("not airflow 2", func(t *testing.T) {
		err := Export("id", "", 1, true, true, true)
		assert.Contains(t, err.Error(), "Command must be used with Airflow 2.X")
	})
}

func TestJsonString(t *testing.T) {
	t.Run("basic string", func(t *testing.T) {
		conn := Connection{ConnExtra: "test"}
		res := jsonString(&conn)
		assert.Equal(t, "test", res)
	})

	t.Run("basic map", func(t *testing.T) {
		conn := Connection{ConnExtra: map[interface{}]interface{}{"key1": "value1", "key2": "value2"}}
		res := jsonString(&conn)
		var result map[string]interface{}
		json.Unmarshal([]byte(res), &result)

		assert.Equal(t, result["key1"], "value1")
		assert.Equal(t, result["key2"], "value2")
	})

	t.Run("empty extra", func(t *testing.T) {
		conn := Connection{ConnExtra: ""}
		res := jsonString(&conn)
		assert.Equal(t, "", res)
	})
}

func TestWriteAirflowSettingstoYAML(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		err := WriteAirflowSettingstoYAML("airflow_settings.yaml")
		assert.NoError(t, err)
		os.Remove("./connections.yaml")
		os.Remove("./variables.yaml")
	})

	t.Run("invalid setttings file", func(t *testing.T) {
		err := WriteAirflowSettingstoYAML("airflow_settings_invalid.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unable to decode file")
		os.Remove("./connections.yaml")
		os.Remove("./variables.yaml")
	})
}
