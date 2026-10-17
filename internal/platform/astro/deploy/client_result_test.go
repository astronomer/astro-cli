package deploy

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// DeployClientImage returns what it pushed and prints only its progress: the
// lines that say where the image went and what to do next are the command's
// to render (cmd/astro's renderRemoteDeploy), so under --output json they
// become the result object instead.
func TestDeployClientImageReturnsWhatItPushed(t *testing.T) {
	cases := []struct {
		name     string
		in       InputClientDeploy
		progress string
		check    func(t *testing.T, got ClientDeploy)
	}{
		{
			name: "a build",
			in:   InputClientDeploy{Platform: "linux/amd64, linux/arm64"},
			progress: "Authenticating with base image registry: images.astronomer.cloud\n" +
				"Building client image for platforms: linux/amd64, linux/arm64\n" +
				"Pushing client image to configured remote registry\n",
			check: func(t *testing.T, got ClientDeploy) {
				assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, got.Platforms)
				assert.Empty(t, got.SourceImage)
			},
		},
		{
			name:     "a prebuilt image",
			in:       InputClientDeploy{ImageName: "local:1"},
			progress: "Using provided image: local:1\nPushing client image to configured remote registry\n",
			check: func(t *testing.T, got ClientDeploy) {
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
			cmd := withClientBuild(t, "docker")
			config.CFG.RemoteClientRegistry.SetHomeString("registry.example.com/agents/client")
			tc.in.Path = dir

			r, w, err := os.Pipe()
			require.NoError(t, err)
			prev := os.Stdout
			os.Stdout = w
			got, err := DeployClientImage(tc.in, nil)
			os.Stdout = prev
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
