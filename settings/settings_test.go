package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestSettings(t *testing.T) {
	suite.Run(t, new(Suite))
}

var _ suite.TearDownTestSuite = (*Suite)(nil)

func (s *Suite) TearDownTest() {
	settings = Config{}
}

func (s *Suite) TestConfigSettings() {
	// config settings success
	err := ConfigSettings("container-id", "", nil, 2, false, false, false)
	s.NoError(err)
	// config setttings no id error
	err = ConfigSettings("", "", nil, 2, false, false, false)
	s.ErrorIs(err, errNoID)
	// config settings settings file error
	err = ConfigSettings("container-id", "testfiles/airflow_settings_invalid.yaml", nil, 2, false, false, false)
	s.Contains(err.Error(), "unable to decode file")
}

func (s *Suite) TestAddConnectionsAirflowOne() {
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
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains([]string{expectedAddCmd, expectedListCmd}, airflowCommand)
		return "", nil
	}
	err := AddConnections("test-conn-id", 1, nil)
	s.NoError(err)
}

func (s *Suite) TestAddConnectionsAirflowTwo() {
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
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains([]string{expectedAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'", nil
		}
		return "", nil
	}
	err := AddConnections("test-conn-id", 2, nil)
	s.NoError(err)
}

func ptr[T any](t T) *T {
	return &t
}

func (s *Suite) TestAddConnectionsAirflowTwoWithEnvConns() {
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

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains([]string{expectedAddCmd, expectedEnvAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'", nil
		}
		return "", nil
	}
	err := AddConnections("test-conn-id", 2, envConns)
	s.NoError(err)
}

func (s *Suite) TestAddConnectionsAirflowTwoURI() {
	testConn := Connection{
		ConnURI: "test-uri",
	}
	settings.Airflow.Connections = []Connection{testConn}

	expectedAddCmd := "airflow connections add   'test-id' --conn-uri 'test-uri'"
	expectedDelCmd := "airflow connections delete   \"test-id\""
	expectedListCmd := "airflow connections list -o plain"
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains([]string{expectedAddCmd, expectedListCmd, expectedDelCmd}, airflowCommand)
		if airflowCommand == expectedListCmd {
			return "'test-id' 'test-type' 'test-host' 'test-uri'", nil
		}
		return "", nil
	}
	err := AddConnections("test-conn-id", 2, nil)
	s.NoError(err)
}

func (s *Suite) TestAddConnectionsFailure() {
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
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains([]string{expectedAddCmd, expectedListCmd}, airflowCommand)
		return "", fmt.Errorf("mock error")
	}
	err := AddConnections("test-conn-id", 1, nil)
	s.Contains(err.Error(), "mock error")
}

func (s *Suite) TestAddVariableAirflowOne() {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	expectedAddCmd := "airflow variables -s test-var-name'test-var-val'"
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Equal(expectedAddCmd, airflowCommand)
		return "", nil
	}
	err := AddVariables("test-conn-id", 1)
	s.NoError(err)
}

func (s *Suite) TestAddVariableAirflowTwo() {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	expectedAddCmd := "airflow variables set test-var-name 'test-var-val'"
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Equal(expectedAddCmd, airflowCommand)
		return "", nil
	}
	err := AddVariables("test-conn-id", 2)
	s.NoError(err)
}

func (s *Suite) TestAddPoolsAirflowOne() {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
		},
	}

	expectedAddCmd := "airflow pool -s  test-pool-name 1 'test-pool-description' "
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Equal(expectedAddCmd, airflowCommand)
		return "", nil
	}
	err := AddPools("test-conn-id", 1)
	s.NoError(err)
}

func (s *Suite) TestAddPoolsAirflowTwo() {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
		},
	}

	expectedAddCmd := "airflow pools set  test-pool-name 1 'test-pool-description' "
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Equal(expectedAddCmd, airflowCommand)
		return "", nil
	}
	err := AddPools("test-conn-id", 2)
	s.NoError(err)
}

func (s *Suite) TestAddVariableFailure() {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	expectedAddCmd := "airflow variables -s test-var-name'test-var-val'"
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Equal(expectedAddCmd, airflowCommand)
		return "", fmt.Errorf("mock error")
	}
	err := AddVariables("test-conn-id", 1)
	s.Contains(err.Error(), "mock error")
}

func (s *Suite) TestInitSettingsSuccess() {
	WorkingPath = "./testfiles/"
	ConfigFileName := "airflow_settings.yaml"
	err := InitSettings(ConfigFileName)
	s.NoError(err)
}

func (s *Suite) TestInitSettingsFailure() {
	WorkingPath = "./testfiles/"
	ConfigFileName := "airflow_settings_invalid"
	err := InitSettings(ConfigFileName)
	s.Error(err)
	s.Contains(err.Error(), "unable to decode file")
}

func (s *Suite) TestEnvExport() {
	s.Run("success", func() {
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			switch airflowCommand {
			case airflowVarExport:
				return "1 variables successfully exported to tmp.var", nil
			case catVarFile:
				return `{
					"myvar": "myval"
				}`, nil
			case airflowConnExport:
				return "Connections successfully exported to tmp.json", nil
			case catConnFile:
				return "local_postgres=postgres://username:password@example.db.example.com:5432/schema", nil
			default:
				return "", nil
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, true, true)
		s.NoError(err)
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})
	s.Run("missing id", func() {
		err := EnvExport("", "", 2, true, true)
		s.ErrorIs(err, errNoID)
	})

	s.Run("variable failure", func() {
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			switch airflowCommand {
			case airflowVarExport:
				return "", nil
			default:
				return "", nil
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, false, true)
		s.Contains(err.Error(), "there was an error during env export")
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})

	s.Run("connection failure", func() {
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			switch airflowCommand {
			case airflowConnExport:
				return "", nil
			default:
				return "", nil
			}
		}

		err := EnvExport("id", "testfiles/test.env", 2, true, false)
		s.Contains(err.Error(), "there was an error during env export")
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})

	s.Run("not airflow 2", func() {
		err := EnvExport("id", "", 1, true, true)
		s.Contains(err.Error(), "Command must be used with Airflow 2.X")
	})
}

func (s *Suite) TestExport() {
	s.Run("success", func() {
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
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
  schema: schema`, nil
			case airflowVarExport:
				return "1 variables successfully exported to tmp.var", nil
			case catVarFile:
				return `{
					"myvar": "myval"
				}`, nil
			case ariflowPoolsList:
				return `
- description: Default pool
  pool: default_pool
  slots: '128'
- description: ''
  pool: subdag_limit
  slots: '3'`, nil
			default:
				return "", nil
			}
		}

		err := Export("id", "airflow_settings_export.yaml", 2, true, true, true)
		s.NoError(err)
	})

	s.Run("variable failure", func() {
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			switch airflowCommand {
			case airflowVarExport:
				return "", nil
			default:
				return "", nil
			}
		}

		err := Export("id", "airflow_settings_export.yaml", 2, false, true, false)
		s.Contains(err.Error(), "there was an error during export")
	})

	s.Run("missing id", func() {
		err := Export("", "", 2, true, true, true)
		s.ErrorIs(err, errNoID)
	})

	s.Run("not airflow 2", func() {
		err := Export("id", "", 1, true, true, true)
		s.Contains(err.Error(), "Command must be used with Airflow 2.X")
	})
}

func (s *Suite) TestJsonString() {
	s.Run("basic string", func() {
		conn := Connection{ConnExtra: "test"}
		res := jsonString(&conn)
		s.Equal("test", res)
	})

	s.Run("basic map", func() {
		conn := Connection{ConnExtra: map[interface{}]interface{}{"key1": "value1", "key2": "value2"}}
		res := jsonString(&conn)
		var result map[string]interface{}
		json.Unmarshal([]byte(res), &result)

		s.Equal(result["key1"], "value1")
		s.Equal(result["key2"], "value2")
	})

	s.Run("string-keyed map", func() {
		conn := Connection{ConnExtra: map[string]interface{}{"key1": "value1", "key2": "value2"}}
		res := jsonString(&conn)
		var result map[string]interface{}
		json.Unmarshal([]byte(res), &result)

		s.Equal(result["key1"], "value1")
		s.Equal(result["key2"], "value2")
	})

	s.Run("unexpected type", func() {
		conn := Connection{ConnExtra: []string{"key1", "value1", "key2", "value2"}}
		res := jsonString(&conn)
		s.Equal(res, "")
	})

	s.Run("empty extra", func() {
		conn := Connection{ConnExtra: ""}
		res := jsonString(&conn)
		s.Equal("", res)
	})

	s.Run("nil extra", func() {
		conn := Connection{ConnExtra: nil}
		res := jsonString(&conn)
		s.Equal("", res)
	})
}

func (s *Suite) TestWriteAirflowSettingstoYAML() {
	s.Run("success", func() {
		err := WriteAirflowSettingstoYAML("airflow_settings.yaml")
		s.NoError(err)
		os.Remove("./connections.yaml")
		os.Remove("./variables.yaml")
	})

	s.Run("invalid setttings file", func() {
		err := WriteAirflowSettingstoYAML("airflow_settings_invalid.yaml")
		s.Error(err)
		s.Contains(err.Error(), "unable to decode file")
		os.Remove("./connections.yaml")
		os.Remove("./variables.yaml")
	})
}
