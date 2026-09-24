package httputil

import (
	"bytes"
	"cmp"
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

// StatusError is the error NormalizeAPIError returns. Its text is the API's
// message, or ErrorRequest and the status when the body carries none, and the
// status travels with it, so a caller can tell a refused login (401) from the
// message by errors.As rather than by reading the text.
type StatusError struct {
	StatusCode int
	msg        string
	wrapped    error
}

func (e *StatusError) Error() string { return e.msg }

func (e *StatusError) Unwrap() error { return e.wrapped }

// HasStatus reports that err is, or wraps, a StatusError with the given status.
func HasStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == status
}

func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusNoContent {
		var decode apiError
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return &StatusError{
				StatusCode: httpResp.StatusCode,
				msg:        fmt.Sprintf("%s, status %d", ErrorRequest, httpResp.StatusCode),
				wrapped:    ErrorRequest,
			}
		}
		return &StatusError{StatusCode: httpResp.StatusCode, msg: decode.Message}
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
		// The client version is this CLI's own, except under the deploy action,
		// which ships its own and reports that instead. Its version comes from
		// the environment, so fall back to ours when it exported none: the
		// header has to name something, or the API cannot tell what called it.
		identifier, clientVersion := "cli", version.Current()
		switch {
		case os.Getenv("DEPLOY_ACTION") == "true" && os.Getenv("GITHUB_ACTIONS") == "true":
			identifier = "deploy-action"
			clientVersion = cmp.Or(os.Getenv("DEPLOY_ACTION_VERSION"), clientVersion)
		case os.Getenv("GITHUB_ACTIONS") == "true":
			identifier = "github-action"
		}
		req.Header.Add("x-astro-client-identifier", identifier)
		req.Header.Add("x-astro-client-version", clientVersion)
		req.Header.Add("x-client-os-identifier", operatingSystem+"-"+arch)
		req.Header.Add("User-Agent", fmt.Sprintf("astro-cli/%s", version.Current()))
		return nil
	}
}
