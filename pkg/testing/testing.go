package testing

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/spf13/afero"
)

const (
	LocalPlatform         = "local"
	CloudPlatform         = "cloud"
	SoftwarePlatform      = "software"
	Initial               = "initial"
	ErrorReturningContext = "error"
)

var perm os.FileMode = 0o777

// RoundTripFunc
type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// NewTestClient returns *httputil.HTTPClient with Transport replaced to avoid making real calls
func NewTestClient(fn RoundTripFunc) *httputil.HTTPClient {
	testClient := httputil.NewHTTPClient()
	testClient.HTTPClient.Transport = fn
	return testClient
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func NewTestConfig(platform string) []byte {
	testConfig := `cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  host: http://localhost:8871/v1
context: %s
contexts:
  %s:
    domain: %s
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
    organization: test-org-id
`
	switch platform {
	case CloudPlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer.io", strings.Replace("astronomer.io", ".", "_", -1), "astronomer.io")
	case SoftwarePlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer_dev.com", strings.Replace("astronomer_dev.com", ".", "_", -1), "astronomer_dev.com")
	case LocalPlatform:
		testConfig = fmt.Sprintf(testConfig, "localhost", "localhost", "localhost")
	case Initial:
		testConfig = ""
	case ErrorReturningContext:
		// this is an error returning case
		testConfig = fmt.Sprintf(testConfig, "error", "error", "error")
		testConfig = strings.Replace(testConfig, "context: error", "context: ", 1)
	default:
		testConfig = fmt.Sprintf(testConfig, "localhost", "localhost", "localhost")
	}
	cfg := []byte(testConfig)
	return cfg
}

func InitTestConfig(platform string) {
	// fake filesystem
	fs := afero.NewMemMapFs()
	configYaml := NewTestConfig(platform)
	err := afero.WriteFile(fs, config.HomeConfigFile, configYaml, perm)
	config.InitConfig(fs)
	if err != nil {
		panic(err)
	}
}

// StringContains Verify if the string t contains all the substring part of tt
func StringContains(tt []string, t string) bool {
	for _, tUnit := range tt {
		if !strings.Contains(t, tUnit) {
			return false
		}
	}
	return true
}

func MockUserInput(t *testing.T, i string) func() {
	input := []byte(i)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write(input)
	if err != nil {
		t.Error(err)
	}
	w.Close()

	// set os.Stdin = new stdin, and return function to defer in the test
	realStdin := os.Stdin
	os.Stdin = r
	return func() { os.Stdin = realStdin }
}
