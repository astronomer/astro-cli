package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The global deploy.git_metadata only turns the commit a deploy records off:
// false wins over a project's git-metadata = true; it never turns it on for a
// project that set it false.
func TestGitMetadataAllowed(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Cleanup(func() { _ = config.CFG.DeployGitMetadata.SetHomeString("true") })

	for _, tc := range []struct {
		global       string
		wanted, want bool
	}{
		{"true", true, true},
		{"true", false, false},
		{"false", true, false},
		{"false", false, false},
	} {
		require.NoError(t, config.CFG.DeployGitMetadata.SetHomeString(tc.global))
		assert.Equal(t, tc.want, gitMetadataAllowed(tc.wanted), "global %s, project %v", tc.global, tc.wanted)
	}
}

// The note is for a commit the off switch kept out: none outside a git
// checkout, none when the project did not want one, none when nothing is off.
func TestGitMetadataOffNoteOnlyWhenACommitWasKeptOut(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Cleanup(func() { _ = config.CFG.DeployGitMetadata.SetHomeString("true") })
	notACheckout := t.TempDir()

	require.NoError(t, config.CFG.DeployGitMetadata.SetHomeString("false"))
	assert.Empty(t, gitMetadataOffNote(notACheckout, true), "no checkout, no commit to keep out")
	assert.Empty(t, gitMetadataOffNote(".", false), "the project did not want one")

	require.NoError(t, config.CFG.DeployGitMetadata.SetHomeString("true"))
	assert.Empty(t, gitMetadataOffNote(".", true), "nothing is off")
}
