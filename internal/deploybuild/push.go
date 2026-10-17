package deploybuild

import (
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// pushForbiddenAdvice is what the CLI says about a push the registry refused
// as unauthorized (imagebuild.ErrPushForbidden) when the push used the
// credential the user's own engine config holds, whichever control plane the
// image was going to.
const pushForbiddenAdvice = `failed to push image due to authentication error (403 Forbidden).

This commonly occurs due to:
1. Invalid cached Docker credentials
2. Incompatible containerd snapshotter configuration
3. A very long upload: the push credential can expire before a large layer finishes

To resolve:
• Run 'docker logout' for each Astro registry to clear cached credentials
• Ensure containerd snapshotter is disabled (Docker Desktop users)
• If the push ran for many minutes, retry, or make the image smaller
• Try running 'astro deploy' again

For detailed troubleshooting steps, visit:
https://support.astronomer.io/hc/en-us/articles/41427905156243-403-errors-on-image-push`

// credentialRefusedAdvice is what the CLI says when the registry refused the
// credential a push carried itself, the session token: clearing the user's
// cached logins would change nothing, since the push never read them.
const credentialRefusedAdvice = `failed to push image: the registry refused the credential this push was made with (403 Forbidden).

The token may have expired, or it may not have permission to push to this registry. Run 'astro login' (or check the permissions of the API token in use) and try again.

The registry said: %s`

// pushForbiddenError is a refused push, told as the CLI advises on it, over
// the push's own error.
type pushForbiddenError struct {
	text  string
	cause error
}

func (e *pushForbiddenError) Error() string { return e.text }
func (e *pushForbiddenError) Unwrap() error { return e.cause }

// ExplainPush returns a push's error as the CLI tells it: a push the registry
// refused as unauthorized says what to do about it, still matching
// imagebuild.ErrPushForbidden with errors.Is and still wrapping the engine's
// failure. A push on the user's stored credential is advised to clear it; one
// that carried its own (an image deploy's session token) is told the
// registry refused that, with the engine's own words, which carry no secret.
// Any other error, and nil, is returned as it is.
func ExplainPush(err error) error {
	var forbidden *imagebuild.PushForbiddenError
	switch {
	case errors.As(err, &forbidden) && forbidden.WithCredential:
		return &pushForbiddenError{text: fmt.Sprintf(credentialRefusedAdvice, forbidden.Err), cause: err}
	case errors.Is(err, imagebuild.ErrPushForbidden):
		return &pushForbiddenError{text: pushForbiddenAdvice, cause: err}
	}
	return err
}
