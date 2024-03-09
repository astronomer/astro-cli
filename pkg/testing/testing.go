package testing

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/spf13/afero"
)

const (
	LocalPlatform         = "local"
	CloudPlatform         = "cloud"
	CloudDevPlatform      = "dev"
	CloudPerfPlatform     = "perf"
	CloudStagePlatform    = "stage"
	CloudPrPreview        = "prpreview"
	SoftwarePlatform      = "software"
	Initial               = "initial"
	ErrorReturningContext = "error"
	userInputSleep        = 100
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
duplicate_volumes: true
context: %s
contexts:
  %s:
    domain: %s
    token: token
    last_used_workspace: ck05r3bor07h40d02y2hw4n4v
    workspace: ck05r3bor07h40d02y2hw4n4v
    organization: test-org-id
    organization_short_name: test-org-short-name
disable_env_objects: false
`
	switch platform {
	case CloudPlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer.io", strings.Replace("astronomer.io", ".", "_", -1), "astronomer.io")
	case CloudDevPlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer-dev.io", strings.Replace("astronomer-dev.io", ".", "_", -1), "astronomer-dev.io")
	case CloudStagePlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer-stage.io", strings.Replace("astronomer-stage.io", ".", "_", -1), "astronomer-stage.io")
	case CloudPerfPlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer-perf.io", strings.Replace("astronomer-perf.io", ".", "_", -1), "astronomer-perf.io")
	case SoftwarePlatform:
		testConfig = fmt.Sprintf(testConfig, "astronomer_dev.com", strings.Replace("astronomer_dev.com", ".", "_", -1), "astronomer_dev.com")
	case LocalPlatform:
		testConfig = fmt.Sprintf(testConfig, "localhost", "localhost", "localhost")
	case Initial:
		testConfig = ""

	case CloudPrPreview:
		testConfig = fmt.Sprintf(testConfig, "pr1234.astronomer-dev.io", strings.Replace("pr1234.astronomer-dev.io", ".", "_", -1), "pr1234.astronomer-dev.io")
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

func MockUserInputs(t *testing.T, inputs []string) func() {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	realStdin := os.Stdin
	os.Stdin = r

	// Start a goroutine to write inputs to the pipe sequentially
	go func() {
		defer w.Close() // Ensure the write end is closed after writing all inputs
		for _, input := range inputs {
			_, err := w.WriteString(input + "\n") // Simulate pressing Enter after each input
			if err != nil {
				t.Error(err)
				return
			}
			time.Sleep(userInputSleep * time.Millisecond) // Slight delay to simulate user typing
		}
	}()

	return func() { os.Stdin = realStdin }
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

// This handles a bug in ginkgo when testing cobra commands. Essentially when no arguments are supplied
// Cobra uses os.Args[1:] as the arguments to the command. This causes a panic when os.Args is empty or
// if it contains a ginkgo argument.
func SetupOSArgsForGinkgo() func() {
	origArgs := os.Args
	newArgs := []string{}
	for i := range origArgs {
		if !(strings.Contains(origArgs[i], "ginkgo") || strings.Contains(origArgs[i], "test")) {
			newArgs = append(newArgs, origArgs[i])
		}
	}
	if len(os.Args) > 1 {
		os.Args = newArgs
	} else {
		os.Args = []string{"single-arg-for-cobra"}
	}
	return func() {
		os.Args = origArgs
	}
}
