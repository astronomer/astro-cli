package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestRegistry(t *testing.T) {
	suite.Run(t, new(Suite))
}

// newMockRegistryServer creates an httptest server that mocks the Astronomer Registry API.
func newMockRegistryServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case fmt.Sprintf("/%s/dags/sagemaker-batch-inference/versions/latest", registryAPI):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"githubRawSourceUrl": fmt.Sprintf("http://%s/raw/sagemaker-batch-inference.py", r.Host),
				"filePath":           "dags/sagemaker-batch-inference.py",
			})
		case "/raw/sagemaker-batch-inference.py":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("from airflow import DAG\n# sagemaker-batch-inference DAG\n"))
		case fmt.Sprintf("/%s/providers", registryAPI):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"providers": []interface{}{
					map[string]interface{}{
						"name":    "apache-airflow-providers-snowflake",
						"version": "6.10.0",
					},
				},
			})
		case fmt.Sprintf("/%s/providers/apache-airflow-providers-snowflake/versions/6.10.0", registryAPI):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":    "apache-airflow-providers-snowflake",
				"version": "6.10.0",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// setMockRegistry points the registry client at the given test server and returns a cleanup function.
func setMockRegistry(server *httptest.Server) func() {
	origHost := registryHost
	origScheme := registryScheme
	registryHost = server.Listener.Addr().String()
	registryScheme = "http"
	return func() {
		registryHost = origHost
		registryScheme = origScheme
	}
}

func (s *Suite) TestRegistryCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newRegistryCmd(os.Stdout)
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	s.NoError(err)
	s.Contains(buf.String(), "Astronomer Registry")
}
