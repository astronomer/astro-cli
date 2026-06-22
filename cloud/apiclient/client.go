// Package apiclient constructs configured clients for the Astro public APIs.
//
// The generated clients live in their own modules (pkg/astro-client-v1 and
// pkg/astro-client-v1alpha1) so external consumers can import them without
// pulling in the astro-cli dependency graph. The constructors live here, in the
// root module, because they depend on context and pkg/httputil — which a
// standalone module cannot import.
package apiclient

import (
	"github.com/astronomer/astro-cli/context"
	astrov1 "github.com/astronomer/astro-cli/pkg/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/pkg/astro-client-v1alpha1"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// NewV1Client creates an API client for the Astro v1 public API.
func NewV1Client(c *httputil.HTTPClient) *astrov1.ClientWithResponses {
	cl, _ := astrov1.NewClientWithResponses("", astrov1.WithHTTPClient(c.HTTPClient), astrov1.WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("v1"), nil
	})))
	return cl
}

// NewV1Alpha1Client creates an API client for the Astro v1alpha1 public API.
// v1alpha1 is retained only for endpoints that v1 does not yet cover (Astro IDE).
func NewV1Alpha1Client(c *httputil.HTTPClient) *astrov1alpha1.ClientWithResponses {
	cl, _ := astrov1alpha1.NewClientWithResponses("", astrov1alpha1.WithHTTPClient(c.HTTPClient), astrov1alpha1.WithRequestEditorFn(httputil.NewRequestEditorFn(func() (string, string, error) {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return "", "", err
		}
		return ctx.Token, ctx.GetPublicRESTAPIURL("v1alpha1"), nil
	})))
	return cl
}
