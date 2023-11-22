package astrocore

import (
	"bytes"
	httpContext "context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// a shorter alias
type CoreClient = ClientWithResponsesInterface

// create api client for astro core services
func NewCoreClient(c *httputil.HTTPClient) *ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(CoreRequestEditor))
	return cl
}

func NormalizeAPIError(httpResp *http.Response, body []byte) error {
	if httpResp.StatusCode != HTTPStatus200 && httpResp.StatusCode != HTTPStatus204 {
		decode := Error{}
		err := json.NewDecoder(bytes.NewReader(body)).Decode(&decode)
		if err != nil {
			return fmt.Errorf("%w, status %d", ErrorRequest, httpResp.StatusCode)
		}
		return errors.New(decode.Message) //nolint:goerr113
	}
	return nil
}

func CoreRequestEditor(ctx httpContext.Context, req *http.Request) error {
	currentCtx, err := context.GetCurrentContext()
	if err != nil {
		return nil
	}
	os := runtime.GOOS
	arch := runtime.GOARCH
	baseURL := currentCtx.GetPublicRESTAPIURL("v1alpha1")
	requestURL, err := url.Parse(baseURL + req.URL.String())
	if err != nil {
		return fmt.Errorf("%w, %s", ErrorBaseURL, baseURL)
	}
	req.URL = requestURL
	req.Header.Add("authorization", currentCtx.Token)
	req.Header.Add("x-astro-client-identifier", "cli")
	req.Header.Add("x-astro-client-version", version.CurrVersion)
	req.Header.Add("x-client-os-identifier", os+"-"+arch)
	req.Header.Add("User-Agent", fmt.Sprintf("astro-cli/%s", version.CurrVersion))
	return nil
}
