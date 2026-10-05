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
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) { //nolint:errcheck // error deliberately ignored in this shell code
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("v1"), nil
	})))
	return cl
}

// NewV1ClientForLogin creates a v1 API client bound to one stored login — its
// token and its host's v1 base URL — rather than to whichever login the
// current-context pointer names at request time. A project's workspace link
// names its own host, and its values are read with the login for that host even
// while the CLI is switched to another — see docs/workspace-link.md.
func NewV1ClientForLogin(c *httputil.HTTPClient, token, baseURL string) *ClientWithResponses {
	cl, _ := NewClientWithResponses("", WithHTTPClient(c.HTTPClient), WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) { //nolint:errcheck // same construction as NewV1Client, which cannot fail with a base URL supplied per request
		return token, baseURL, nil
	})))
	return cl
}
