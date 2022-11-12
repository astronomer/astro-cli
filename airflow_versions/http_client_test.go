package airflowversions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestClientDo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockResp := Response{RuntimeVersions: map[string]RuntimeVersion{"4.2.5": {RuntimeVersionMetadata{AirflowVersion: "2.2.5", Channel: "stable"}, RuntimeVersionMigrations{}}}}
		jsonResponse, err := json.Marshal(mockResp)
		assert.NoError(t, err)

		httpClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		client := &Client{HTTPClient: httpClient, useAstronomerCertified: false}
		resp, err := client.Do(&httputil.DoOptions{})
		assert.NoError(t, err)
		assert.Equal(t, mockResp, *resp)
	})
}
