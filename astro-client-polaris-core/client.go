package astropolariscore

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

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/version"
)

var (
	ErrorRequest  = errors.New("failed to perform request")
	ErrorBaseURL  = errors.New("invalid baseurl")
	HTTPStatus200 = 200
	HTTPStatus204 = 204
)

const TrueString = "true"

// a shorter alias
type PolarisClient = ClientWithResponsesInterface

// create api client for astro polaris core services
func NewPolarisCoreClient(c *httputil.HTTPClient) *ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(requestEditor))
	return cl
}

func requestEditor(ctx httpContext.Context, req *http.Request) error {
	currentCtx, err := context.GetCurrentContext()
	if err != nil {
		return nil
	}
	operatingSystem := runtime.GOOS
	arch := runtime.GOARCH
	baseURL := currentCtx.GetPublicRESTAPIURL("private/v1alpha1")
	requestURL, err := url.Parse(baseURL + req.URL.String())
	if err != nil {
		return fmt.Errorf("%w, %s", ErrorBaseURL, baseURL)
	}
	req.URL = requestURL
	req.Header.Add("authorization", currentCtx.Token)
	switch {
	case os.Getenv("DEPLOY_ACTION") == TrueString && os.Getenv("GITHUB_ACTIONS") == TrueString:
		req.Header.Add("x-astro-client-identifier", "deploy-action")
		req.Header.Add("x-astro-client-version", os.Getenv("DEPLOY_ACTION_VERSION"))
	case os.Getenv("GITHUB_ACTIONS") == TrueString:
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

func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != HTTPStatus200 && httpResp.StatusCode != HTTPStatus204 {
		decode := Error{}
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return fmt.Errorf("%w, status %d", ErrorRequest, httpResp.StatusCode)
		}
		return errors.New(decode.Message)
	}
	return nil
}
