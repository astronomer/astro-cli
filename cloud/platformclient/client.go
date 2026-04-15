package platformclient

import (
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NormalizeAPIError is a deliberate re-export of httputil.NormalizeAPIError, allowing
// callers to normalize platform API errors without importing pkg/httputil directly.
var NormalizeAPIError = httputil.NormalizeAPIError

// NewPlatformCoreClient creates an API client for Astro platform core services.
func NewPlatformCoreClient(c *httputil.HTTPClient) *astroplatformcore.ClientWithResponses {
	// we append base url in request editor, so set to an empty string here
	cl, _ := astroplatformcore.NewClientWithResponses("", astroplatformcore.WithHTTPClient(c.HTTPClient), astroplatformcore.WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("platform/v1beta1"), nil
	})))
	return cl
}
