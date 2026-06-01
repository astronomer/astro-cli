package settings

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/pkg/fileutil"
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

// mockAirflowAPI creates an httptest server that accepts POST (create) and PATCH (update)
// requests for Airflow API resources. It returns 201 on POST, 200 on PATCH, and records requests.
type apiRequest struct {
	Method  string
	Path    string
	Body    map[string]interface{}
	RawBody []byte
}

func newMockAirflowAPI(s *Suite) (*httptest.Server, *[]apiRequest) {
	var requests []apiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := apiRequest{Method: r.Method, Path: r.URL.Path, RawBody: body}
		if len(body) > 0 {
			var parsed map[string]interface{}
			if err := json.Unmarshal(body, &parsed); err == nil {
				req.Body = parsed
			}
		}
		requests = append(requests, req)

		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
		}
		fmt.Fprint(w, "{}")
	}))
	origClient := SetHTTPClient(server.Client())
	s.T().Cleanup(func() {
		SetHTTPClient(origClient)
		server.Close()
	})
	return server, &requests
}

// newConflictAirflowAPI creates a server that returns 409 on POST (to test upsert/update path).
func newConflictAirflowAPI(s *Suite) (*httptest.Server, *[]apiRequest) {
	var requests []apiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := apiRequest{Method: r.Method, Path: r.URL.Path, RawBody: body}
		if len(body) > 0 {
			var parsed map[string]interface{}
			if err := json.Unmarshal(body, &parsed); err == nil {
				req.Body = parsed
			}
		}
		requests = append(requests, req)

		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"detail":"already exists"}`)
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "{}")
		}
	}))
	origClient := SetHTTPClient(server.Client())
	s.T().Cleanup(func() {
		SetHTTPClient(origClient)
		server.Close()
	})
	return server, &requests
}

func (s *Suite) TestConfigSettings() {
	server, _ := newMockAirflowAPI(s)
	err := ConfigSettings(server.URL+"/api/v2", "", "", nil, false, false, false)
	s.NoError(err)
	// empty URL error
	err = ConfigSettings("", "", "", nil, false, false, false)
	s.ErrorIs(err, errNoURL)
}

func (s *Suite) TestAddVariables() {
	settings.Airflow.Variables = Variables{
		{VariableName: "test-var-name", VariableValue: "test-var-val"},
	}

	server, requests := newMockAirflowAPI(s)
	err := AddVariables(server.URL+"/api/v2", "")
	s.NoError(err)
	s.Len(*requests, 1)
	s.Equal(http.MethodPost, (*requests)[0].Method)
	s.Equal("/api/v2/variables", (*requests)[0].Path)
	s.Equal("test-var-name", (*requests)[0].Body["key"])
	s.Equal("test-var-val", (*requests)[0].Body["value"])
}

func (s *Suite) TestAddVariablesUpdate() {
	settings.Airflow.Variables = Variables{
		{VariableName: "existing-var", VariableValue: "new-val"},
	}

	server, requests := newConflictAirflowAPI(s)
	err := AddVariables(server.URL+"/api/v2", "")
	s.NoError(err)
	// Should have POST (409) then PATCH
	s.Len(*requests, 2)
	s.Equal(http.MethodPost, (*requests)[0].Method)
	s.Equal(http.MethodPatch, (*requests)[1].Method)
	s.Equal("/api/v2/variables/existing-var", (*requests)[1].Path)
}

func ptr[T any](t T) *T {
	return &t
}

func (s *Suite) TestAddConnections() {
	testConn := Connection{
		ConnID:       "test-id",
		ConnType:     "test-type",
		ConnHost:     "test-host",
		ConnSchema:   "test-schema",
		ConnLogin:    "test-login",
		ConnPassword: "test-password",
		ConnPort:     1,
	}
	settings.Airflow.Connections = []Connection{testConn}

	server, requests := newMockAirflowAPI(s)
	err := AddConnections(server.URL+"/api/v2", "", nil)
	s.NoError(err)
	s.Len(*requests, 1)
	s.Equal(http.MethodPost, (*requests)[0].Method)
	s.Equal("/api/v2/connections", (*requests)[0].Path)
	s.Equal("test-id", (*requests)[0].Body["connection_id"])
	s.Equal("test-type", (*requests)[0].Body["conn_type"])
	s.Equal("test-host", (*requests)[0].Body["host"])
	s.Equal("test-login", (*requests)[0].Body["login"])
	s.Equal("test-password", (*requests)[0].Body["password"])
	s.Equal("test-schema", (*requests)[0].Body["schema"])
	s.Equal(float64(1), (*requests)[0].Body["port"])
}

func (s *Suite) TestAddConnectionsWithEnvConns() {
	testConn := Connection{
		ConnID:   "test-id",
		ConnType: "test-type",
		ConnHost: "test-host",
	}
	settings.Airflow.Connections = []Connection{testConn}

	envConns := map[string]astrov1.EnvironmentObjectConnection{
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

	server, requests := newMockAirflowAPI(s)
	err := AddConnections(server.URL+"/api/v2", "", envConns)
	s.NoError(err)
	// Both settings and env connections should create requests
	s.Len(*requests, 2)

	// Find the env connection request
	var envReq *apiRequest
	for i := range *requests {
		if (*requests)[i].Body["connection_id"] == "test-env-id" {
			envReq = &(*requests)[i]
			break
		}
	}
	s.NotNil(envReq)
	s.Equal("test-env-type", envReq.Body["conn_type"])
	s.Equal("test-env-host", envReq.Body["host"])
}

func (s *Suite) TestAddConnectionsURI() {
	testConn := Connection{
		ConnID:  "test-id",
		ConnURI: "postgres://user:pass@host:5432/db",
	}
	settings.Airflow.Connections = []Connection{testConn}

	server, requests := newMockAirflowAPI(s)
	err := AddConnections(server.URL+"/api/v2", "", nil)
	s.NoError(err)
	s.Len(*requests, 1)
	// URI should be parsed into individual fields
	s.Equal("postgres", (*requests)[0].Body["conn_type"])
	s.Equal("host", (*requests)[0].Body["host"])
	s.Equal("user", (*requests)[0].Body["login"])
	s.Equal("pass", (*requests)[0].Body["password"])
	s.Equal(float64(5432), (*requests)[0].Body["port"])
	s.Equal("db", (*requests)[0].Body["schema"])
}

func (s *Suite) TestAddConnectionsUpdate() {
	settings.Airflow.Connections = []Connection{
		{ConnID: "existing", ConnType: "http"},
	}

	server, requests := newConflictAirflowAPI(s)
	err := AddConnections(server.URL+"/api/v2", "", nil)
	s.NoError(err)
	s.Len(*requests, 2)
	s.Equal(http.MethodPost, (*requests)[0].Method)
	s.Equal(http.MethodPatch, (*requests)[1].Method)
	s.Equal("/api/v2/connections/existing", (*requests)[1].Path)
}

func (s *Suite) TestAddPools() {
	settings.Airflow.Pools = Pools{
		{PoolName: "test-pool", PoolSlot: 5, PoolDescription: "test desc"},
	}

	server, requests := newMockAirflowAPI(s)
	err := AddPools(server.URL+"/api/v2", "")
	s.NoError(err)
	s.Len(*requests, 1)
	s.Equal(http.MethodPost, (*requests)[0].Method)
	s.Equal("/api/v2/pools", (*requests)[0].Path)
	s.Equal("test-pool", (*requests)[0].Body["name"])
	s.Equal(float64(5), (*requests)[0].Body["slots"])
	s.Equal("test desc", (*requests)[0].Body["description"])
}

func (s *Suite) TestAddVariableFailure() {
	settings.Airflow.Variables = Variables{
		{VariableName: "test-var", VariableValue: "val"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"detail":"server error"}`)
	}))
	origClient := SetHTTPClient(server.Client())
	defer func() { SetHTTPClient(origClient); server.Close() }()

	err := AddVariables(server.URL+"/api/v2", "")
	s.Error(err)
	s.Contains(err.Error(), "server error")
}

func (s *Suite) TestAddConnectionsFailure() {
	settings.Airflow.Connections = []Connection{
		{ConnID: "test-id", ConnType: "test-type"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"detail":"server error"}`)
	}))
	origClient := SetHTTPClient(server.Client())
	defer func() { SetHTTPClient(origClient); server.Close() }()

	err := AddConnections(server.URL+"/api/v2", "", nil)
	s.Error(err)
	s.Contains(err.Error(), "server error")
}

func (s *Suite) TestAddPoolsFailure() {
	settings.Airflow.Pools = Pools{
		{PoolName: "test-pool", PoolSlot: 1, PoolDescription: "desc"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"detail":"server error"}`)
	}))
	origClient := SetHTTPClient(server.Client())
	defer func() { SetHTTPClient(origClient); server.Close() }()

	err := AddPools(server.URL+"/api/v2", "")
	s.Error(err)
	s.Contains(err.Error(), "server error")
}

func (s *Suite) TestEmptyLists() {
	settings.Airflow.Variables = Variables{}
	settings.Airflow.Connections = Connections{}
	settings.Airflow.Pools = Pools{}

	server, requests := newMockAirflowAPI(s)
	url := server.URL + "/api/v2"

	s.NoError(AddVariables(url, ""))
	s.NoError(AddConnections(url, "", nil))
	s.NoError(AddPools(url, ""))
	s.Empty(*requests, "no API calls should be made for empty lists")
}

func (s *Suite) TestMultipleVariables() {
	settings.Airflow.Variables = Variables{
		{VariableName: "var1", VariableValue: "val1"},
		{VariableName: "var2", VariableValue: "val2"},
		{VariableName: "var3", VariableValue: "val3"},
	}

	server, requests := newMockAirflowAPI(s)
	err := AddVariables(server.URL+"/api/v2", "")
	s.NoError(err)
	s.Len(*requests, 3)
}

func (s *Suite) TestConnectionExtraTypes() {
	s.Run("map extra", func() {
		settings.Airflow.Connections = []Connection{
			{
				ConnID:   "map-extra-conn",
				ConnType: "http",
				ConnExtra: map[any]any{
					"key1": "val1",
				},
			},
		}
		server, requests := newMockAirflowAPI(s)
		s.NoError(AddConnections(server.URL+"/api/v2", "", nil))
		s.Len(*requests, 1)
		s.Contains((*requests)[0].Body["extra"], "key1")
	})

	s.Run("string json extra", func() {
		settings.Airflow.Connections = []Connection{
			{
				ConnID:    "str-extra-conn",
				ConnType:  "http",
				ConnExtra: `{"key":"val"}`,
			},
		}
		server, requests := newMockAirflowAPI(s)
		s.NoError(AddConnections(server.URL+"/api/v2", "", nil))
		s.Len(*requests, 1)
		s.Equal(`{"key":"val"}`, (*requests)[0].Body["extra"])
	})

	s.Run("nil extra", func() {
		settings.Airflow.Connections = []Connection{
			{
				ConnID:    "nil-extra-conn",
				ConnType:  "http",
				ConnExtra: nil,
			},
		}
		server, requests := newMockAirflowAPI(s)
		s.NoError(AddConnections(server.URL+"/api/v2", "", nil))
		s.Len(*requests, 1)
		_, hasExtra := (*requests)[0].Body["extra"]
		s.False(hasExtra, "nil extra should not appear in JSON")
	})
}

func (s *Suite) TestMixedValidInvalid() {
	settings.Airflow.Variables = Variables{
		{VariableName: "good_var", VariableValue: "good_val"},
		{VariableName: "", VariableValue: "orphan_val"},
		{VariableName: "empty_val_var", VariableValue: ""},
	}

	server, requests := newMockAirflowAPI(s)
	err := AddVariables(server.URL+"/api/v2", "")
	s.NoError(err)
	// Only 1 valid variable should create an API call
	s.Len(*requests, 1)
	s.Equal("good_var", (*requests)[0].Body["key"])
}

func (s *Suite) TestAuthHeaderPassedThrough() {
	settings.Airflow.Variables = Variables{
		{VariableName: "test", VariableValue: "val"},
	}

	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "{}")
	}))
	origClient := SetHTTPClient(server.Client())
	defer func() { SetHTTPClient(origClient); server.Close() }()

	err := AddVariables(server.URL+"/api/v2", "Basic dGVzdDp0ZXN0")
	s.NoError(err)
	s.Equal("Basic dGVzdDp0ZXN0", receivedAuth)
}

func (s *Suite) TestSkipMessages() {
	s.Run("skip pool with zero slots", func() {
		settings.Airflow.Pools = Pools{
			{PoolName: "zero-pool", PoolSlot: 0, PoolDescription: "should skip"},
		}
		server, requests := newMockAirflowAPI(s)
		s.NoError(AddPools(server.URL+"/api/v2", ""))
		s.Empty(*requests)
	})

	s.Run("skip connection without type or uri", func() {
		settings.Airflow.Connections = []Connection{
			{ConnID: "no-type", ConnHost: "host"},
		}
		server, requests := newMockAirflowAPI(s)
		s.NoError(AddConnections(server.URL+"/api/v2", "", nil))
		s.Empty(*requests)
	})
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
			return "", nil
		}

		err := EnvExport("id", "testfiles/test.env", 2, false, true)
		s.Contains(err.Error(), "there was an error during env export")
		_ = fileutil.WriteStringToFile("testfiles/test.env", "")
	})

	s.Run("connection failure", func() {
		execAirflowCommand = func(id, airflowCommand string) (string, error) {
			return "", nil
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
			return "", nil
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

// Ensure unused imports are consumed
var (
	_ = strings.TrimSpace
	_ = fmt.Sprint
)
