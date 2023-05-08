package astro

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestAstroClientSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestNewAstroClient() {
	client := NewAstroClient(httputil.NewHTTPClient())
	s.NotNil(client, "Can't create new Astro client")
}

func (s *Suite) TestPrepareRESTRequest() {
	client := NewAstroClient(httputil.NewHTTPClient())
	doOpts := &httputil.DoOptions{
		Path: "/test",
		Headers: map[string]string{
			"test": "test",
		},
	}
	err := client.prepareRESTRequest(doOpts)
	s.NoError(err)
	s.Equal("test", doOpts.Headers["test"])
	// Test context has no token
	s.Equal("", doOpts.Headers["Authorization"])
}

func (s *Suite) TestDoPublicRESTQuery() {
	mockResponse := "A REST query response"
	client := testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer([]byte(mockResponse))),
			Header:     make(http.Header),
		}
	})
	astroClient := NewAstroClient(client)
	doOpts := &httputil.DoOptions{
		Path: "/test",
		Headers: map[string]string{
			"test": "test",
		},
	}
	resp, err := astroClient.DoPublicRESTQuery(doOpts)
	s.NoError(err)
	s.Equal(mockResponse, resp.Body)
}
