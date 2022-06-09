package houston

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestGetRuntimeReleases(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)

	mockResp := &Response{
		Data: ResponseData{
			RuntimeReleases: RuntimeReleases{
				RuntimeRelease{Version: "4.2.4", AirflowVersion: "2.2.4"},
			},
		},
	}
	jsonResponse, err := json.Marshal(mockResp)
	assert.NoError(t, err)

	t.Run("success without airflow version", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		resp, err := api.GetRuntimeReleases("")
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResp.Data.RuntimeReleases)
	})

	t.Run("success with airflow version", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		resp, err := api.GetRuntimeReleases("2.2.4")
		assert.NoError(t, err)
		assert.Equal(t, resp, mockResp.Data.RuntimeReleases)
	})

	t.Run("error", func(t *testing.T) {
		client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("Internal Server Error")),
				Header:     make(http.Header),
			}
		})
		api := NewClient(client)

		_, err := api.GetRuntimeReleases("")
		assert.Contains(t, err.Error(), "Internal Server Error")
	})
}
