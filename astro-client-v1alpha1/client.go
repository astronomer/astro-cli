package astrov1alpha1

import (
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NormalizeAPIError is a deliberate re-export of httputil.NormalizeAPIError, allowing
// callers to normalize v1alpha1 API errors without importing pkg/httputil directly.
var NormalizeAPIError = httputil.NormalizeAPIError

// APIClient is the v1alpha1 API client interface.
type APIClient = ClientWithResponsesInterface

// NewV1Alpha1Client creates an API client for the Astro v1alpha1 public API.
// v1alpha1 is retained only for endpoints that v1 does not yet cover (Astro IDE).
func NewV1Alpha1Client(c *httputil.HTTPClient) *ClientWithResponses {
	cl, _ := NewClientWithResponses("", WithHTTPClient(httputil.WithRateLimitRetries(c.HTTPClient)), WithRequestEditorFn(currentContextEditor()))
	return cl
}

// NewV1Alpha1StreamClient creates a v1alpha1 API client whose request and
// response bodies stream. It does not retry, because WithRateLimitRetries reads
// a request body whole; its callers retry with httputil.RetryRateLimited.
func NewV1Alpha1StreamClient(c *httputil.HTTPClient) *Client {
	cl, _ := NewClient("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(currentContextEditor()))
	return cl
}

// currentContextEditor sends a request to the current context's API, with its token.
func currentContextEditor() RequestEditorFn {
	return httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("v1alpha1"), nil
	})
}
