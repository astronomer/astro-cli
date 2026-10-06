package deployment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
)

// A run that may not ask (any command under -o json) is refused before the
// picker draws, and the refusal names the flag that answers it, rather than
// only "pass the answer as a flag".
func TestSelectDeploymentRefusalNamesTheFlag(t *testing.T) {
	t.Cleanup(input.SetGuard(func() string { return "with --output json it cannot" }))

	_, err := SelectDeployment([]astrov1.Deployment{{Id: "a", Name: "one"}, {Id: "b", Name: "two"}}, "Select a Deployment")
	require.Error(t, err)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "pass --deployment")
}
