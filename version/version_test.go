package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestReleasesEndpointAPITimeout() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // sleeping and doing nothing
	}))
	defer ts.Close()

	httpClient := &http.Client{Timeout: 100 * time.Microsecond} // client side timeout should be less than server side sleep defined above

	ctx := context.Background()
	release, err := getLatestRelease(ctx, httpClient, ts.URL)
	// assert error returned is related to client timeout
	s.Nil(release)
	s.Error(err)
	s.Contains(err.Error(), "context deadline exceeded")
}

func (s *Suite) TestGetLatestRelease() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `
		{
		  "available_releases": [
				{
					"version": "1.20.1"
				},
		    {
					"version": "1.21.0"
				}
			]
		}
		`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
	defer ts.Close()

	httpClient := &http.Client{}

	ctx := context.Background()
	release, err := getLatestRelease(ctx, httpClient, ts.URL)
	s.Nil(err)
	s.Equal(semver.MustParse("1.21.0"), release)
}

func (s *Suite) TestGetLatestReleaseWithErrorCode() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	httpClient := &http.Client{}

	ctx := context.Background()
	release, err := getLatestRelease(ctx, httpClient, ts.URL)
	s.Nil(release)
	s.ErrorContains(err, "request failed")
}

func (s *Suite) TestGetLatestReleaseGetsLatestParsedVersion() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `
		{
		  "available_releases": [
				{
					"version": "1.20.1"
				},
		    {
					"version": "this does not parse"
				}
			]
		}
		`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
	defer ts.Close()

	httpClient := &http.Client{}

	ctx := context.Background()
	release, err := getLatestRelease(ctx, httpClient, ts.URL)
	s.Nil(err)
	s.Equal(semver.MustParse("1.20.1"), release)
}

func (s *Suite) TestGetLatestReleaseErrorsForEmptyReleases() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `
		{
		  "available_releases": []
		}
		`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
	defer ts.Close()

	httpClient := &http.Client{}

	ctx := context.Background()
	release, err := getLatestRelease(ctx, httpClient, ts.URL)
	s.Nil(release)
	s.ErrorContains(err, "0 results")
}
