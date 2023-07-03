package version

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/v48/github"
	"github.com/stretchr/testify/assert"
)

func TestGithubAPITimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // sleeping and doing nothing
	}))
	defer ts.Close()
	githubURL, err := url.Parse(fmt.Sprintf("%s/", ts.URL))
	assert.NoError(t, err)

	githubClient := github.NewClient(&http.Client{Timeout: 1 * time.Second}) // client side timeout should be less than server side sleep defined above
	githubClient.BaseURL = githubURL

	start := time.Now()
	release, err := getLatestRelease(githubClient, "test", "test")
	elapsed := time.Since(start)
	// assert time to get a response from the function is only slightly greater than client timeout
	assert.GreaterOrEqual(t, elapsed, 1*time.Second)
	assert.Less(t, elapsed, 2*time.Second)
	// assert error returned is related to client timeout
	assert.Nil(t, release)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}
