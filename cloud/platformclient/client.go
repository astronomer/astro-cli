package platformclient

import (
	httpContext "context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/version"
)

// NewPlatformCoreClient creates an API client for Astro platform core services.
func NewPlatformCoreClient(c *httputil.HTTPClient) *astroplatformcore.ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := astroplatformcore.NewClientWithResponses("", astroplatformcore.WithHTTPClient(c.HTTPClient), astroplatformcore.WithRequestEditorFn(requestEditor))
	return cl
}

func requestEditor(ctx httpContext.Context, req *http.Request) error {
	currentCtx, err := context.GetCurrentContext()
	if err != nil {
		return nil
	}
	operatingSystem := runtime.GOOS
	arch := runtime.GOARCH
	baseURL := currentCtx.GetPublicRESTAPIURL("platform/v1beta1")
	requestURL, err := url.Parse(baseURL + req.URL.String())
	if err != nil {
		return fmt.Errorf("%w, %s", astroplatformcore.ErrorBaseURL, baseURL)
	}
	req.URL = requestURL
	req.Header.Add("authorization", currentCtx.Token)
	switch {
	case os.Getenv("DEPLOY_ACTION") == astroplatformcore.TrueString && os.Getenv("GITHUB_ACTIONS") == astroplatformcore.TrueString:
		req.Header.Add("x-astro-client-identifier", "deploy-action")
		req.Header.Add("x-astro-client-version", os.Getenv("DEPLOY_ACTION_VERSION"))
	case os.Getenv("GITHUB_ACTIONS") == astroplatformcore.TrueString:
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
