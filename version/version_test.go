package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	release, err := getLatestRelease(ctx, httpClient, ts.URL, 1)
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
	release, err := getLatestRelease(ctx, httpClient, ts.URL, 1)
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
	release, err := getLatestRelease(ctx, httpClient, ts.URL, 1)
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
	release, err := getLatestRelease(ctx, httpClient, ts.URL, 1)
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
	release, err := getLatestRelease(ctx, httpClient, ts.URL, 1)
	s.Nil(release)
	s.ErrorContains(err, "0 results")
}

func (s *Suite) TestGetLatestReleaseIgnoresOtherMajors() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `
		{
		  "available_releases": [
				{
					"version": "1.45.0"
				},
				{
					"version": "2.0.0"
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
	release, err := getLatestRelease(ctx, httpClient, ts.URL, 1)
	s.Nil(err)
	s.Equal(semver.MustParse("1.45.0"), release)

	release, err = getLatestRelease(ctx, httpClient, ts.URL, 3)
	s.Nil(release)
	s.ErrorContains(err, "0 valid versions for major version 3")
}

// The release list is asked at most once a day: within that, the answer comes
// from the cache, and that includes "this major has no release yet".
func (s *Suite) TestLatestReleaseCachedAsksOnceADay() {
	var asked int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.Write([]byte(`{"available_releases": [{"version": "1.45.0"}]}`))
	}))
	defer ts.Close()

	ctx, client := context.Background(), &http.Client{}
	cachePath := filepath.Join(s.T().TempDir(), "release-check.json")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name      string
		at        time.Time
		major     uint64
		wantAsked int
		want      *semver.Version
	}{
		{name: "nothing cached", at: now, major: 1, wantAsked: 1, want: semver.MustParse("1.45.0")},
		{name: "cached an hour ago", at: now.Add(time.Hour), major: 1, wantAsked: 1, want: semver.MustParse("1.45.0")},
		{name: "cached a day ago", at: now.Add(releaseCheckTTL), major: 1, wantAsked: 2, want: semver.MustParse("1.45.0")},
		{name: "cached in the future", at: now, major: 1, wantAsked: 3, want: semver.MustParse("1.45.0")},
		{name: "cached for another major", at: now, major: 2, wantAsked: 4},
		{name: "no release for the major, cached", at: now.Add(time.Minute), major: 2, wantAsked: 4},
	} {
		latest, err := latestReleaseCached(ctx, client, ts.URL, cachePath, tc.major, tc.at)
		s.NoError(err, tc.name)
		s.Equal(tc.want, latest, tc.name)
		s.Equal(tc.wantAsked, asked, tc.name)
	}
}

// A failure is not an answer: it is not cached, so the next command asks again.
func (s *Suite) TestLatestReleaseCachedKeepsNoFailure() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cachePath := filepath.Join(s.T().TempDir(), "release-check.json")
	_, err := latestReleaseCached(context.Background(), &http.Client{}, ts.URL, cachePath, 1, time.Now())
	s.Error(err)
	s.NoFileExists(cachePath)
}
