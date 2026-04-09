package httputil

import (
	"bytes"
	httpContext "context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"

	"github.com/astronomer/astro-cli/version"
)

var ErrorRequest = errors.New("failed to perform request")

type apiError struct {
	Message string `json:"message"`
}

func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusNoContent {
		var decode apiError
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return fmt.Errorf("%w, status %d", ErrorRequest, httpResp.StatusCode)
		}
		return errors.New(decode.Message)
	}
	return nil
}

// NewRequestEditorFn returns a request editor that sets auth headers and the
// base URL. getTokenAndURL is called per-request and should return the bearer
// token and the resolved base URL for the given API path (e.g. from
// context.GetCurrentContext).
func NewRequestEditorFn(getTokenAndURL func() (token, baseURL string, err error)) func(httpContext.Context, *http.Request) error {
	return func(ctx httpContext.Context, req *http.Request) error {
		token, baseURL, err := getTokenAndURL()
		if err != nil {
			return nil
		}
		operatingSystem := runtime.GOOS
		arch := runtime.GOARCH
		requestURL, err := url.Parse(baseURL + req.URL.String())
		if err != nil {
			return fmt.Errorf("%w, %s", ErrorBaseURL, baseURL)
		}
		req.URL = requestURL
		req.Header.Add("authorization", token)
		switch {
		case os.Getenv("DEPLOY_ACTION") == "true" && os.Getenv("GITHUB_ACTIONS") == "true":
			req.Header.Add("x-astro-client-identifier", "deploy-action")
			req.Header.Add("x-astro-client-version", os.Getenv("DEPLOY_ACTION_VERSION"))
		case os.Getenv("GITHUB_ACTIONS") == "true":
			req.Header.Add("x-astro-client-identifier", "github-action")
			req.Header.Add("x-astro-client-version", version.CurrVersion)
		default:
			req.Header.Add("x-astro-client-identifier", "cli")
			req.Header.Add("x-astro-client-version", version.CurrVersion)
		}
		req.Header.Add("x-client-os-identifier", operatingSystem+"-"+arch)
		req.Header.Add("User-Agent", fmt.Sprintf("astro-cli/%s", version.CurrVersion))
		return nil
	}
}
