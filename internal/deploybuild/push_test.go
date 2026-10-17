package deploybuild

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// A refused push is told with the CLI's advice, and still matches the
// sentinel and keeps the engine's failure; anything else passes through.
func TestExplainPush(t *testing.T) {
	cause := errors.New("docker push r/a:t failed: exit status 1: denied: 403 Forbidden")
	refused := fmt.Errorf("%w: %w", imagebuild.ErrPushForbidden, cause)

	err := ExplainPush(refused)
	assert.Equal(t, pushForbiddenAdvice, err.Error())
	assert.ErrorIs(t, err, imagebuild.ErrPushForbidden)
	assert.ErrorIs(t, err, cause)

	other := errors.New("manifest blob unknown")
	assert.Equal(t, other, ExplainPush(other))
	assert.NoError(t, ExplainPush(nil))
}
