package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/input"
)

// A deploy prompt that ended with no pick (answers that were not choices) is
// not a no to a yes/no question: it is not input.ErrAborted, so it publishes
// no "aborted" kind.
func TestNoDeploymentPickedIsNotADecline(t *testing.T) {
	assert.NotErrorIs(t, ErrAborted, input.ErrAborted)
}
