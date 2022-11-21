package domainutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestFormatDomain(t *testing.T) {
	t.Run("removes cloud from cloud.astronomer.io", func(t *testing.T) {
		actual := FormatDomain("cloud.astronomer.io")
		assert.Equal(t, "astronomer.io", actual)
	})
	t.Run("removes cloud from cloud.astronomer-dev.io", func(t *testing.T) {
		actual := FormatDomain("cloud.astronomer-dev.io")
		assert.Equal(t, "astronomer-dev.io", actual)
	})
	t.Run("removes https://cloud from cloud.astronomer-dev.io", func(t *testing.T) {
		actual := FormatDomain("https://cloud.astronomer-dev.io")
		assert.Equal(t, "astronomer-dev.io", actual)
	})
	t.Run("removes trailing / from cloud.astronomer-dev.io", func(t *testing.T) {
		actual := FormatDomain("https://cloud.astronomer-dev.io/")
		assert.Equal(t, "astronomer-dev.io", actual)
	})
	t.Run("removes cloud from cloud.astronomer-stage.io", func(t *testing.T) {
		actual := FormatDomain("cloud.astronomer-stage.io")
		assert.Equal(t, "astronomer-stage.io", actual)
	})
	t.Run("removes https://cloud from cloud.astronomer-stage.io", func(t *testing.T) {
		actual := FormatDomain("https://cloud.astronomer-stage.io")
		assert.Equal(t, "astronomer-stage.io", actual)
	})
	t.Run("removes cloud from cloud.astronomer-perf.io", func(t *testing.T) {
		actual := FormatDomain("cloud.astronomer-perf.io")
		assert.Equal(t, "astronomer-perf.io", actual)
	})
	t.Run("removes https://cloud from cloud.astronomer-perf.io", func(t *testing.T) {
		actual := FormatDomain("https://cloud.astronomer-perf.io")
		assert.Equal(t, "astronomer-perf.io", actual)
	})
	t.Run("removes cloud from pr1234.cloud.astronomer-dev.io", func(t *testing.T) {
		actual := FormatDomain("pr1234.cloud.astronomer-dev.io")
		assert.Equal(t, "pr1234.astronomer-dev.io", actual)
	})
	t.Run("removes https://cloud from pr1234.cloud.astronomer-dev.io", func(t *testing.T) {
		actual := FormatDomain("https://pr1234.cloud.astronomer-dev.io")
		assert.Equal(t, "pr1234.astronomer-dev.io", actual)
	})
	t.Run("sets default domain if one was not provided", func(t *testing.T) {
		actual := FormatDomain("")
		assert.Equal(t, "astronomer.io", actual)
	})
	t.Run("does not mutate domain if cloud is not found in input", func(t *testing.T) {
		actual := FormatDomain("fail.astronomer-dev.io")
		assert.Equal(t, "fail.astronomer-dev.io", actual)
	})
}

func TestIsPrPreviewDomain(t *testing.T) {
	t.Run("returns true if its pr preview domain", func(t *testing.T) {
		for _, urlToCheck := range listOfPRURLs {
			actual := isPrPreviewDomain(urlToCheck)
			assert.True(t, actual, urlToCheck+" should be true")
		}
	})
	t.Run("returns false if its not pr preview domain", func(t *testing.T) {
		for _, urlToCheck := range listOfURLs {
			actual := isPrPreviewDomain(urlToCheck)
			assert.False(t, actual, urlToCheck+" should be false")
		}
	})
}

func TestGetPRSubDomain(t *testing.T) {
	t.Run("returns pr subdomain for a valid PR preview domain", func(t *testing.T) {
		for i, domainToCheck := range listOfPRURLs {
			actualPR, actualDomain := GetPRSubDomain(domainToCheck)
			assert.Equal(t, "astronomer-dev.io", actualDomain)
			assert.Equal(t, listOfPRs[i], actualPR)
		}
	})
	t.Run("returns empty pr subdomain for domains that are not a PR Preview domain", func(t *testing.T) {
		for _, domainToCheck := range listOfURLs {
			actualPRDomain, actualDomain := GetPRSubDomain(domainToCheck)
			assert.Equal(t, domainToCheck, actualDomain)
			assert.Equal(t, "", actualPRDomain)
		}
	})
}

func TestGetURLToEndpoint(t *testing.T) {
	var prSubDomain, domain, expectedURL, endpoint string
	endpoint = "myendpoint"
	t.Run("returns localhost endpoint", func(t *testing.T) {
		domain = "localhost"
		expectedURL = fmt.Sprintf("http://%s:8871/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns pr preview endpoint", func(t *testing.T) {
		prSubDomain = "pr1234"
		domain = "pr1234.astronomer-dev.io"
		expectedURL = fmt.Sprintf("https://%s.api.%s/hub/%s", prSubDomain, "astronomer-dev.io", endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns cloud endpoint for prod", func(t *testing.T) {
		domain = "astronomer.io"
		expectedURL = fmt.Sprintf("https://api.%s/hub/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns cloud endpoint for dev", func(t *testing.T) {
		domain = "astronomer-dev.io"
		expectedURL = fmt.Sprintf("https://api.%s/hub/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns cloud endpoint for stage", func(t *testing.T) {
		domain = "astronomer-stage.io"
		expectedURL = fmt.Sprintf("https://api.%s/hub/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns cloud endpoint for perf", func(t *testing.T) {
		domain = "astronomer-perf.io"
		expectedURL = fmt.Sprintf("https://api.%s/hub/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
	t.Run("returns cloud endpoint for everything else", func(t *testing.T) {
		domain = "someotherdomain.io"
		expectedURL = fmt.Sprintf("https://api.%s/hub/%s", domain, endpoint)
		actualURL := GetURLToEndpoint("https", domain, endpoint)
		assert.Equal(t, expectedURL, actualURL)
	})
}

func TestTransformToCoreApiEndpoint(t *testing.T) {
	t.Run("transforms non-local url to core api endpoint", func(t *testing.T) {
		actual := TransformToCoreAPIEndpoint("https://somedomain.io/hub/v1alpha1/great-endpoint")
		assert.Equal(t, "https://somedomain.io/v1alpha1/great-endpoint", actual)
	})
	t.Run("transforms local url to core api endpoint", func(t *testing.T) {
		actual := TransformToCoreAPIEndpoint("http://localhost:8871/v1alpha1/great-endpoint")
		assert.Equal(t, "http://localhost:8888/v1alpha1/great-endpoint", actual)
	})
	t.Run("returns without changes if url is not meant for core api", func(t *testing.T) {
		actual := TransformToCoreAPIEndpoint("https://somedomain.io/hub/valpha1/great-enedpoint")
		assert.Equal(t, "https://somedomain.io/hub/valpha1/great-enedpoint", actual)
	})
}
