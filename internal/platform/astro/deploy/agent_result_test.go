package deploy

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// DeployAgentImage returns what it pushed and prints only its progress: the
// lines that say where the image went and what to do next are the command's
// to render (cmd/astro's renderRemoteDeploy), so under --output json they
// become the result object instead.
func TestDeployAgentImageReturnsWhatItPushed(t *testing.T) {
	cases := []struct {
		name     string
		in       InputAgentDeploy
		progress string
		check    func(t *testing.T, got AgentDeploy)
	}{
		{
			name: "a build",
			in:   InputAgentDeploy{Platform: "linux/amd64, linux/arm64"},
			progress: "Authenticating with base image registry: images.astronomer.cloud\n" +
				"Building agent image for platforms: linux/amd64, linux/arm64\n" +
				"Pushing agent image to configured remote registry\n",
			check: func(t *testing.T, got AgentDeploy) {
				assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, got.Platforms)
				assert.Empty(t, got.SourceImage)
			},
		},
		{
			name:     "a prebuilt image",
			in:       InputAgentDeploy{ImageName: "local:1"},
			progress: "Using provided image: local:1\nPushing agent image to configured remote registry\n",
			check: func(t *testing.T, got AgentDeploy) {
				assert.Equal(t, "local:1", got.SourceImage)
				assert.Empty(t, got.Platforms)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile.client"), []byte("FROM images.astronomer.cloud/baseimages/astro-remote-execution-agent:3.1-1-python-3.12-astro-agent-1.1.0"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements-client.txt"), nil, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "packages-client.txt"), nil, 0o600))
			testUtil.InitTestConfig(testUtil.CloudPlatform)
			cmd := withAgentBuild(t, "docker")
			tc.in.Registry, tc.in.RegistrySource = "registry.example.com/agents/client", "--registry"
			tc.in.Path = dir

			r, w, err := os.Pipe()
			require.NoError(t, err)
			prevOut, prevErr := os.Stdout, os.Stderr
			os.Stdout, os.Stderr = w, w
			got, err := DeployAgentImage(tc.in, nil)
			os.Stdout, os.Stderr = prevOut, prevErr
			require.NoError(t, w.Close())
			out, readErr := io.ReadAll(r)
			require.NoError(t, readErr)

			require.NoError(t, err)
			assert.Equal(t, tc.progress, string(out))
			assert.Equal(t, "registry.example.com/agents/client", got.Registry)
			assert.Contains(t, cmd.calls, "docker push "+got.Image, "the image is the reference pushed")
			assert.Equal(t, got.Registry+":"+got.Tag, got.Image)
			assert.Nil(t, got.RuntimeCheck, "no Deployment was named")
			tc.check(t, got)
		})
	}
}
