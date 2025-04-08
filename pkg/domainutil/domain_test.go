package domainutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

var (
	listOfURLs = []string{
		"cloud.astronomer.io",
		"cloud.astronomer-dev.io",
		"cloud.astronomer-stage.io",
		"cloud.astronomer-perf.io",
		"pr1234.fail.astronomer-dev.io",
		"pr1234.astronomer-dev.fail.io",
		"pr1234.astronomer-devpr.io",
		"pr1234.astroprnomer-dev.io",
		"pr123.astroprnomer-dev.io",
		"pr1234.astronomer-dev.io.fail",
		"pr1234.cloud.astronomer-stage.io",
		"pr1234.cloud.astronomer-perf.io",
		"pr12345.cloud.astronomer-perf.io",
		"pr1234.cloud1.astronomer-perf.io",
		"pr1234.cloud.astronomer-stage.io",
		"foo.pr1234.cloud.astronomer-perf.io",
		"pr123.cloud.astronomer-stage.io",
		"pr123.cloud.astronomer-perf.io",
		"pr1234.cloud.astronomer.io",
		"drum.cloud.astronomer-dev.io",
		"drum.cloud.astronomer.io",
	}
	listOfPRURLs = []string{
		"pr1234.astronomer-dev.io",
		"pr12345.astronomer-dev.io",
		"pr12346.astronomer-dev.io",
	}
	listOfPRs = []string{
		"pr1234",
		"pr12345",
		"pr12346",
	}
)

type Suite struct {
	suite.Suite
}

func TestDomainUtil(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestFormatDomain() {
	s.Run("removes cloud from cloud.astronomer.io", func() {
		actual := FormatDomain("cloud.astronomer.io")
		s.Equal("astronomer.io", actual)
	})
	s.Run("removes cloud from cloud.astronomer-dev.io", func() {
		actual := FormatDomain("cloud.astronomer-dev.io")
		s.Equal("astronomer-dev.io", actual)
	})
	s.Run("removes https://cloud from cloud.astronomer-dev.io", func() {
		actual := FormatDomain("https://cloud.astronomer-dev.io")
		s.Equal("astronomer-dev.io", actual)
	})
	s.Run("removes trailing / from cloud.astronomer-dev.io", func() {
		actual := FormatDomain("https://cloud.astronomer-dev.io/")
		s.Equal("astronomer-dev.io", actual)
	})
	s.Run("removes cloud from cloud.astronomer-stage.io", func() {
		actual := FormatDomain("cloud.astronomer-stage.io")
		s.Equal("astronomer-stage.io", actual)
	})
	s.Run("removes https://cloud from cloud.astronomer-stage.io", func() {
		actual := FormatDomain("https://cloud.astronomer-stage.io")
		s.Equal("astronomer-stage.io", actual)
	})
	s.Run("removes cloud from cloud.astronomer-perf.io", func() {
		actual := FormatDomain("cloud.astronomer-perf.io")
		s.Equal("astronomer-perf.io", actual)
	})
	s.Run("removes https://cloud from cloud.astronomer-perf.io", func() {
		actual := FormatDomain("https://cloud.astronomer-perf.io")
		s.Equal("astronomer-perf.io", actual)
	})
	s.Run("removes cloud from pr1234.cloud.astronomer-dev.io", func() {
		actual := FormatDomain("pr1234.cloud.astronomer-dev.io")
		s.Equal("pr1234.astronomer-dev.io", actual)
	})
	s.Run("removes https://cloud from pr1234.cloud.astronomer-dev.io", func() {
		actual := FormatDomain("https://pr1234.cloud.astronomer-dev.io")
		s.Equal("pr1234.astronomer-dev.io", actual)
	})
	s.Run("sets default domain if one was not provided", func() {
		actual := FormatDomain("")
		s.Equal("astronomer.io", actual)
	})
	s.Run("does not mutate domain if cloud is not found in input", func() {
		actual := FormatDomain("fail.astronomer-dev.io")
		s.Equal("fail.astronomer-dev.io", actual)
	})
}

func (s *Suite) TestIsPrPreviewDomain() {
	s.Run("returns true if its pr preview domain", func() {
		for _, urlToCheck := range listOfPRURLs {
			actual := isPrPreviewDomain(urlToCheck)
			s.True(actual, urlToCheck+" should be true")
		}
	})
	s.Run("returns false if its not pr preview domain", func() {
		for _, urlToCheck := range listOfURLs {
			actual := isPrPreviewDomain(urlToCheck)
			s.False(actual, urlToCheck+" should be false")
		}
	})
}

func (s *Suite) TestGetPRSubDomain() {
	s.Run("returns pr subdomain for a valid PR preview domain", func() {
		for i, domainToCheck := range listOfPRURLs {
			actualPR, actualDomain := GetPRSubDomain(domainToCheck)
			s.Equal("astronomer-dev.io", actualDomain)
			s.Equal(listOfPRs[i], actualPR)
		}
	})
	s.Run("returns empty pr subdomain for domains that are not a PR Preview domain", func() {
		for _, domainToCheck := range listOfURLs {
			actualPRDomain, actualDomain := GetPRSubDomain(domainToCheck)
			s.Equal(domainToCheck, actualDomain)
			s.Equal("", actualPRDomain)
		}
	})
}

func (s *Suite) TestGetURLToEndpoint() {
	var prSubDomain, domain, expectedURL, endpoint string
	endpoint = "myendpoint"
	s.Run("returns localhost endpoint", func() {
		domain = "localhost"
		expectedURL = fmt.Sprintf("http://%s:8888/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("http", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns pr preview endpoint", func() {
		prSubDomain = "pr1234"
		domain = "pr1234.astronomer-dev.io"
		expectedURL = fmt.Sprintf("https://%s.api.%s/%s", prSubDomain, "astronomer-dev.io", endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns cloud endpoint for prod", func() {
		domain = "astronomer.io"
		expectedURL = fmt.Sprintf("https://api.%s/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns cloud endpoint for dev", func() {
		domain = "astronomer-dev.io"
		expectedURL = fmt.Sprintf("https://api.%s/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns cloud endpoint for stage", func() {
		domain = "astronomer-stage.io"
		expectedURL = fmt.Sprintf("https://api.%s/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns cloud endpoint for perf", func() {
		domain = "astronomer-perf.io"
		expectedURL = fmt.Sprintf("https://api.%s/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns cloud endpoint for everything else", func() {
		domain = "someotherdomain.io"
		expectedURL = fmt.Sprintf("https://api.%s/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		s.Equal(expectedURL, actualURL)
	})
}
