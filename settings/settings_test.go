package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/pkg/fileutil"
)

// extractJSONFromHeredoc extracts the JSON content from a batch import command
// that uses the heredoc pattern: cat > /tmp/... <<'__ASTRO_CLI_EOF__'\n<json>\n__ASTRO_CLI_EOF__\n...
func extractJSONFromHeredoc(command string) string {
	parts := strings.Split(command, "__ASTRO_CLI_EOF__")
	if len(parts) >= 2 {
		s := strings.TrimSpace(parts[1])
		s = strings.TrimPrefix(s, "'")
		return strings.TrimSpace(s)
	}
	return ""
}

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
	err := ConfigSettings("container-id", "", nil, false, false, false)
	s.NoError(err)
	// config setttings no id error
	err = ConfigSettings("", "", nil, false, false, false)
	s.ErrorIs(err, errNoID)
}

func (s *Suite) TestAddConnections() {
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

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains(airflowCommand, "airflow connections import --overwrite")
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var connMap map[string]interface{}
		s.NoError(json.Unmarshal([]byte(jsonStr), &connMap))
		s.Contains(connMap, "test-id")
		connObj, ok := connMap["test-id"].(map[string]interface{})
		s.True(ok)
		s.Equal("test-type", connObj["conn_type"])
		s.Equal("test-host", connObj["host"])
		s.Equal("test-login", connObj["login"])
		s.Equal("test-password", connObj["password"])
		s.Equal("test-schema", connObj["schema"])
		s.Equal(float64(1), connObj["port"])
		return "", nil
	}
	err := AddConnections("test-conn-id", nil)
	s.NoError(err)
}

func ptr[T any](t T) *T {
	return &t
}

func (s *Suite) TestAddConnectionsWithEnvConns() {
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

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains(airflowCommand, "airflow connections import --overwrite")
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var connMap map[string]interface{}
		s.NoError(json.Unmarshal([]byte(jsonStr), &connMap))
		// Both settings and env connections should be present
		s.Contains(connMap, "test-id")
		s.Contains(connMap, "test-env-id")
		// Verify env connection fields
		envConn, ok := connMap["test-env-id"].(map[string]interface{})
		s.True(ok)
		s.Equal("test-env-type", envConn["conn_type"])
		s.Equal("test-env-host", envConn["host"])
		s.Equal(float64(2), envConn["port"])
		extra, ok := envConn["extra"].(map[string]interface{})
		s.True(ok)
		s.Equal("test-extra-value", extra["test-extra-key"])
		return "", nil
	}
	err := AddConnections("test-conn-id", envConns)
	s.NoError(err)
}

func (s *Suite) TestAddConnectionsURI() {
	testConn := Connection{
		ConnID:  "test-id",
		ConnURI: "test-uri",
	}
	settings.Airflow.Connections = []Connection{testConn}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains(airflowCommand, "airflow connections import --overwrite")
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var connMap map[string]interface{}
		s.NoError(json.Unmarshal([]byte(jsonStr), &connMap))
		s.Contains(connMap, "test-id")
		// URI-only connection should be a string value, not an object
		connVal, ok := connMap["test-id"].(string)
		s.True(ok)
		s.Equal("test-uri", connVal)
		return "", nil
	}
	err := AddConnections("test-conn-id", nil)
	s.NoError(err)
}

func (s *Suite) TestAddVariables() {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains(airflowCommand, "airflow variables import")
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var varsMap map[string]string
		s.NoError(json.Unmarshal([]byte(jsonStr), &varsMap))
		s.Equal("test-var-val", varsMap["test-var-name"])
		return "", nil
	}
	err := AddVariables("test-conn-id")
	s.NoError(err)
}

func (s *Suite) TestAddPools() {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool-name",
			PoolSlot:        1,
			PoolDescription: "test-pool-description",
		},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains(airflowCommand, "airflow pools import")
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var poolMap map[string]poolImportEntry
		s.NoError(json.Unmarshal([]byte(jsonStr), &poolMap))
		s.Len(poolMap, 1)
		s.Contains(poolMap, "test-pool-name")
		s.Equal(1, poolMap["test-pool-name"].Slots)
		s.Equal("test-pool-description", poolMap["test-pool-name"].Description)
		return "", nil
	}
	err := AddPools("test-conn-id")
	s.NoError(err)
}

func (s *Suite) TestAddVariableBatchFailure() {
	settings.Airflow.Variables = Variables{
		{
			VariableName:  "test-var-name",
			VariableValue: "test-var-val",
		},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		return "", fmt.Errorf("mock import error")
	}
	err := AddVariables("test-conn-id")
	s.Contains(err.Error(), "mock import error")
}

func (s *Suite) TestAddConnectionsBatchFailure() {
	settings.Airflow.Connections = []Connection{
		{
			ConnID:   "test-id",
			ConnType: "test-type",
		},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		return "", fmt.Errorf("mock import error")
	}
	err := AddConnections("test-conn-id", nil)
	s.Contains(err.Error(), "mock import error")
}

func (s *Suite) TestAddPoolsBatchFailure() {
	settings.Airflow.Pools = Pools{
		{
			PoolName:        "test-pool",
			PoolSlot:        1,
			PoolDescription: "desc",
		},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		return "", fmt.Errorf("mock import error")
	}
	err := AddPools("test-conn-id")
	s.Contains(err.Error(), "mock import error")
}

func (s *Suite) TestBatchImportEmptyLists() {
	settings.Airflow.Variables = Variables{}
	settings.Airflow.Connections = Connections{}
	settings.Airflow.Pools = Pools{}

	called := false
	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		called = true
		return "", nil
	}

	s.NoError(AddVariables("test-id"))
	s.NoError(AddConnections("test-id", nil))
	s.NoError(AddPools("test-id"))
	s.False(called, "execAirflowCommand should not be called for empty lists")
}

func (s *Suite) TestBatchImportMultipleVariables() {
	settings.Airflow.Variables = Variables{
		{VariableName: "var1", VariableValue: "val1"},
		{VariableName: "var2", VariableValue: "val2"},
		{VariableName: "var3", VariableValue: "val3"},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		s.Contains(airflowCommand, "airflow variables import")
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var varsMap map[string]string
		s.NoError(json.Unmarshal([]byte(jsonStr), &varsMap))
		s.Len(varsMap, 3)
		s.Equal("val1", varsMap["var1"])
		s.Equal("val2", varsMap["var2"])
		s.Equal("val3", varsMap["var3"])
		return "", nil
	}
	err := AddVariables("test-id")
	s.NoError(err)
}

func (s *Suite) TestBatchImportConnectionExtraTypes() {
	s.Run("map extra", func() {
		settings.Airflow.Connections = []Connection{
			{
				ConnID:   "map-extra-conn",
				ConnType: "http",
				ConnExtra: map[any]any{
					"key1": "val1",
					"key2": "val2",
				},
			},
		}
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			jsonStr := extractJSONFromHeredoc(airflowCommand)
			var connMap map[string]map[string]interface{}
			s.NoError(json.Unmarshal([]byte(jsonStr), &connMap))
			extra, ok := connMap["map-extra-conn"]["extra"].(map[string]interface{})
			s.True(ok)
			s.Equal("val1", extra["key1"])
			s.Equal("val2", extra["key2"])
			return "", nil
		}
		s.NoError(AddConnections("test-id", nil))
	})

	s.Run("string json extra", func() {
		settings.Airflow.Connections = []Connection{
			{
				ConnID:    "str-extra-conn",
				ConnType:  "http",
				ConnExtra: `{"key":"val"}`,
			},
		}
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			jsonStr := extractJSONFromHeredoc(airflowCommand)
			var connMap map[string]map[string]interface{}
			s.NoError(json.Unmarshal([]byte(jsonStr), &connMap))
			extra, ok := connMap["str-extra-conn"]["extra"].(map[string]interface{})
			s.True(ok)
			s.Equal("val", extra["key"])
			return "", nil
		}
		s.NoError(AddConnections("test-id", nil))
	})

	s.Run("nil extra", func() {
		settings.Airflow.Connections = []Connection{
			{
				ConnID:    "nil-extra-conn",
				ConnType:  "http",
				ConnExtra: nil,
			},
		}
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			jsonStr := extractJSONFromHeredoc(airflowCommand)
			var connMap map[string]map[string]interface{}
			s.NoError(json.Unmarshal([]byte(jsonStr), &connMap))
			_, hasExtra := connMap["nil-extra-conn"]["extra"]
			s.False(hasExtra, "nil extra should not appear in JSON")
			return "", nil
		}
		s.NoError(AddConnections("test-id", nil))
	})
}

func (s *Suite) TestBatchImportMixedValidInvalid() {
	settings.Airflow.Variables = Variables{
		{VariableName: "good_var", VariableValue: "good_val"},
		{VariableName: "", VariableValue: "orphan_val"},
		{VariableName: "empty_val_var", VariableValue: ""},
	}

	execAirflowCommand = func(id, airflowCommand string) (string, error) {
		jsonStr := extractJSONFromHeredoc(airflowCommand)
		var varsMap map[string]string
		s.NoError(json.Unmarshal([]byte(jsonStr), &varsMap))
		s.Len(varsMap, 1)
		s.Equal("good_val", varsMap["good_var"])
		return "", nil
	}
	err := AddVariables("test-id")
	s.NoError(err)
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
}

func (s *Suite) TestExport() {
	s.Run("success", func() {
		WorkingPath = "./testfiles/"
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
		WorkingPath = "./testfiles/"
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
