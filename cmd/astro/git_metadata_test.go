package astro

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The global deploy.git_metadata only turns the commit a deploy records off:
// false wins over a project's git-metadata = true, and says so; it never
// turns it on for a project that set it false.
func TestGitMetadataAllowed(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Cleanup(func() { _ = config.CFG.DeployGitMetadata.SetHomeString("true") })

	for _, tc := range []struct {
		global     string
		wanted     bool
		want, note bool
	}{
		{"true", true, true, false},
		{"true", false, false, false},
		{"false", true, false, true},
		{"false", false, false, false},
	} {
		require.NoError(t, config.CFG.DeployGitMetadata.SetHomeString(tc.global))
		var errOut bytes.Buffer
		assert.Equal(t, tc.want, gitMetadataAllowed(&errOut, tc.wanted), "global %s, project %v", tc.global, tc.wanted)
		if tc.note {
			assert.Contains(t, errOut.String(), "deploy.git_metadata is false in your global config")
		} else {
			assert.Empty(t, errOut.String(), "global %s, project %v", tc.global, tc.wanted)
		}
	}
}
