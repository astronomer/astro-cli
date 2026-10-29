package imagebuild

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uvImagePinRe is the one shape uv-image.txt may take: the release tag and the
// multi-arch index digest, which scripts/bump-uv-image.sh writes and a build
// pulls by.
var uvImagePinRe = regexp.MustCompile(`^ghcr\.io/astral-sh/uv:\d+\.\d+\.\d+@sha256:[0-9a-f]{64}$`)

func TestUVImageIsPinnedByTagAndDigest(t *testing.T) {
	raw, err := os.ReadFile("uv-image.txt")
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), "\n"), "uv-image.txt is one line")
	assert.Regexp(t, uvImagePinRe, strings.TrimSpace(string(raw)))
	assert.Equal(t, strings.TrimSpace(string(raw)), UVImage)
}

func TestTheInstallStagesDefaultIsTheUVImagePin(t *testing.T) {
	t.Setenv(uvImageEnv, "")
	assert.Contains(t, InstallDockerfile("runtime:3.3-11-base"), "ARG ASTRO_UV_IMAGE="+UVImage+"\n")
	assert.Equal(t, UVImage, uvImage())
}
