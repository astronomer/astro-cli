package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestGetRuntimeReleases() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResp := &Response{
		Data: ResponseData{
			RuntimeReleases: RuntimeReleases{
				RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResp)
	s.NoError(err)

	s.Run("success without airflow version", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		vars := make(map[string]interface{})
		resp, err := api.GetRuntimeReleases(vars)
		s.NoError(err)
		s.Equal(resp, mockResp.Data.RuntimeReleases)
	})

	s.Run("success without airflow version for 1.0.0", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		vars := make(map[string]interface{})
		vars["clusterId"] = "test-cluster-id"
		resp, err := api.GetRuntimeReleases(vars)
		s.NoError(err)
		s.Equal(resp, mockResp.Data.RuntimeReleases)
	})

	s.Run("success with airflow version", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		vars := make(map[string]interface{})
		vars["airflowVersion"] = "2.2.4"
		resp, err := api.GetRuntimeReleases(vars)
		s.NoError(err)
		s.Equal(resp, mockResp.Data.RuntimeReleases)
	})

	s.Run("error", func() {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		vars := make(map[string]interface{})
		_, err := api.GetRuntimeReleases(vars)
		s.Contains(err.Error(), "Internal Server Error")
	})
}

// From Houston 2.0.0 the query takes the workspace and the Deployment, so
// their overrides decide what it lists; before it, neither is sent, nor
// asked for in the query, which an older schema would refuse.
func (s *Suite) TestGetRuntimeReleasesScopedFrom200() {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	jsonResponse, err := json.Marshal(&Response{Data: ResponseData{RuntimeReleases: RuntimeReleases{}}})
	s.NoError(err)
	for _, tc := range []struct {
		version string
		scoped  bool
	}{{"2.0.0", true}, {"2.2.1", true}, {"1.0.43", false}, {"0.37.0", false}} {
		old := version
		version = tc.version
		var sent struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			body, _ := io.ReadAll(req.Body)
			s.NoError(json.Unmarshal(body, &sent))
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBuffer(jsonResponse)), Header: make(http.Header)}
		})
		_, err := NewClient(client).GetRuntimeReleases(map[string]interface{}{"clusterId": "cl", "workspaceUuid": "ws", "deploymentUuid": "dep"})
		version = old
		s.NoError(err, tc.version)
		_, hasWS := sent.Variables["workspaceUuid"]
		_, hasDep := sent.Variables["deploymentUuid"]
		s.Equal(tc.scoped, hasWS && hasDep, tc.version)
		s.Equal(tc.scoped, strings.Contains(sent.Query, "deploymentUuid: $deploymentUuid"), tc.version)
		s.Equal("cl", sent.Variables["clusterId"], tc.version)
	}
}
