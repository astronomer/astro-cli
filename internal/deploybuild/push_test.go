package deploybuild

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// A push refused on the user's stored credential is told to clear it; one
// refused its own credential is told the registry refused that, in the
// engine's words, and not to log out. Both still match the sentinel and keep
// the engine's failure; anything else passes through.
func TestExplainPush(t *testing.T) {
	cause := errors.New("docker push r/a:t failed: exit status 1: denied: 403 Forbidden")

	stored := ExplainPush(&imagebuild.PushForbiddenError{Err: cause})
	assert.Equal(t, pushForbiddenAdvice, stored.Error())
	assert.ErrorIs(t, stored, imagebuild.ErrPushForbidden)
	assert.ErrorIs(t, stored, cause)

	own := ExplainPush(&imagebuild.PushForbiddenError{WithCredential: true, Err: cause})
	assert.Contains(t, own.Error(), "the registry refused the credential this push was made with")
	assert.Contains(t, own.Error(), "The registry said: docker push r/a:t failed: exit status 1: denied: 403 Forbidden")
	assert.NotContains(t, own.Error(), "docker logout")
	assert.NotContains(t, own.Error(), "snapshotter")
	assert.ErrorIs(t, own, imagebuild.ErrPushForbidden)
	assert.ErrorIs(t, own, cause)

	other := errors.New("manifest blob unknown")
	assert.Equal(t, other, ExplainPush(other))
	assert.NoError(t, ExplainPush(nil))
}
