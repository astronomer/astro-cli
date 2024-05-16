package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

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

		resp, err := api.GetRuntimeReleases("")
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

		resp, err := api.GetRuntimeReleases("2.2.4")
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

		_, err := api.GetRuntimeReleases("")
		s.Contains(err.Error(), "Internal Server Error")
	})
}
