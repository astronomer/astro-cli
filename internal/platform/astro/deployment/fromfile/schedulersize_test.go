package fromfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
)

// A scheduler_size the request builders do not know used to be dropped: a
// create sent no size and an update sent an empty one, and the command
// reported success. It is refused with the file's other invalid values,
// before any request.
func TestCheckRequiredFieldsSchedulerSize(t *testing.T) {
	file := func(size string) *inspect.FormattedDeployment {
		var f inspect.FormattedDeployment
		f.Deployment.Configuration.Name = "test-deployment"
		f.Deployment.Configuration.Executor = deployment.CeleryExecutor
		f.Deployment.Configuration.SchedulerSize = size
		return &f
	}

	for _, size := range []string{"", "small", "MEDIUM", "Large", "extra_large", "EXTRA_LARGE"} {
		assert.NoError(t, checkRequiredFields(file(size), createAction), "scheduler_size %q", size)
	}

	for _, size := range []string{"extra-large", "xl", "tiny"} {
		err := checkRequiredFields(file(size), updateAction)
		require.ErrorIs(t, err, errInvalidValue, "scheduler_size %q", size)
		assert.ErrorContains(t, err, "scheduler_size "+size)
		assert.ErrorContains(t, err, "small, medium, large, extra_large")
	}
}
