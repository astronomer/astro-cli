package airflowversions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestClientDo() {
	s.Run("success", func() {
		mockResp := Response{RuntimeVersions: map[string]RuntimeVersion{"4.2.5": {RuntimeVersionMetadata{AirflowVersion: "2.2.5", Channel: "stable"}, RuntimeVersionMigrations{}}}}
		jsonResponse, err := json.Marshal(mockResp)
		s.NoError(err)

		httpClient := testUtil.NewTestClient(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
				Header:     make(http.Header),
			}
		})

		client := &Client{HTTPClient: httpClient, useAstronomerCertified: false}
		resp, err := client.Do(&httputil.DoOptions{})
		s.NoError(err)
		s.Equal(mockResp, *resp)
	})
}
