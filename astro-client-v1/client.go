package astrov1

import (
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NormalizeAPIError is a deliberate re-export of httputil.NormalizeAPIError, allowing
// callers to normalize v1 API errors without importing pkg/httputil directly.
var NormalizeAPIError = httputil.NormalizeAPIError

// APIClient is the v1 API client interface.
type APIClient = ClientWithResponsesInterface

// NewV1Client creates an API client for the Astro v1 public API.
func NewV1Client(c *httputil.HTTPClient) *ClientWithResponses {
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("v1"), nil
	})))
	return cl
}
