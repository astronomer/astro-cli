package testing

import (
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/spf13/afero"
	"net/http"
	"os"
	"strings"
)

// RoundTripFunc
type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// NewTestClient returns *httputil.HTTPClient with Transport replaced to avoid making real calls
func NewTestClient(fn RoundTripFunc) *httputil.HTTPClient {
	testClient := httputil.NewHTTPClient()
	testClient.HTTPClient.Transport = RoundTripFunc(fn)
	return testClient
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func NewTestConfig() []byte {
	houstonHost := GetEnv("HOUSTON_HOST", "localhost")
	configRaw := []byte(`cloud:
  api:
    port: "443"
    protocol: https
    ws_protocol: wss
local:
  enabled: true
  houston: http://HOUSTON_HOST:8871/v1
context: HOUSTON_HOST
contexts:
  HOUSTON_HOST:
    domain: HOUSTON_HOST
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
`)
	config := strings.ReplaceAll(string(configRaw), "HOUSTON_HOST", houstonHost)
	return []byte(config)
}

func InitTestConfig() {
	// fake filesystem
	fs := afero.NewMemMapFs()
	configYaml := NewTestConfig()
	err := afero.WriteFile(fs, config.HomeConfigFile, []byte(configYaml), 0777)
	config.InitConfig(fs)
	if err != nil {
		panic(err)
	}
}
