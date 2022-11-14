package astro

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestNewgqlClient(t *testing.T) {
	client := NewGQLClient(httputil.NewHTTPClient())
	assert.NotNil(t, client, "Can't create new Astro client")
}

func TestPrepareRESTRequest(t *testing.T) {
	client := NewAstroClient(httputil.NewHTTPClient())
	doOpts := &httputil.DoOptions{
		Path: "/test",
		Headers: map[string]string{
			"test": "test",
		},
	}
	err := client.prepareRESTRequest(doOpts)
	assert.NoError(t, err)
	assert.Equal(t, "test", doOpts.Headers["test"])
	// Test context has no token
	assert.Equal(t, "", doOpts.Headers["Authorization"])
}

func TestDoPublicRESTQuery(t *testing.T) {
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
	assert.NoError(t, err)
	assert.Equal(t, mockResponse, resp.Body)
}
