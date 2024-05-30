package version

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/v48/github"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestGithubAPITimeout() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // sleeping and doing nothing
	}))
	defer ts.Close()
	githubURL, err := url.Parse(fmt.Sprintf("%s/", ts.URL))
	s.NoError(err)

	githubClient := github.NewClient(&http.Client{Timeout: 100 * time.Microsecond}) // client side timeout should be less than server side sleep defined above
	githubClient.BaseURL = githubURL

	start := time.Now()
	release, err := getLatestRelease(githubClient, "test", "test")
	elapsed := time.Since(start)
	// assert time to get a response from the function is only slightly greater than client timeout
	s.GreaterOrEqual(elapsed, 100*time.Microsecond)
	s.Less(elapsed, 300*time.Microsecond)
	// assert error returned is related to client timeout
	s.Nil(release)
	s.Error(err)
	s.Contains(err.Error(), "context deadline exceeded")
}
