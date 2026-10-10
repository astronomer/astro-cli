package apc

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// execUnboundRoot runs args the way production does: the tree is built by
// AddCmds with the process's stdout as it is at construction, under a root
// that binds no writer of its own, and run by cliout.Execute. runAPC binds the
// root's out, which would hide a command whose result leaves by
// cmd.OutOrStdout() on an unbound tree, or whose progress reaches stdout;
// this does not.
func execUnboundRoot(t *testing.T, api houston.ClientInterface, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	houstonVersion = "1.0.0"

	outR, outW, perr := os.Pipe()
	require.NoError(t, perr)
	errR, errW, perr := os.Pipe()
	require.NoError(t, perr)
	prevOut, prevErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	// Put them back however the run ends, so a failure in it cannot leave
	// the rest of the package writing to closed pipes.
	t.Cleanup(func() { os.Stdout, os.Stderr = prevOut, prevErr })
	read := func(r *os.File) <-chan string {
		c := make(chan string)
		go func() {
			b, _ := io.ReadAll(r)
			c <- string(b)
		}()
		return c
	}
	gotOut, gotErr := read(outR), read(errR)

	root := &cobra.Command{Use: "astro", SilenceErrors: true}
	LoadPlatform(api) // as the root does for a line that runs one of these commands
	root.AddCommand(AddCmds(api, os.Stdout)...)
	err = cliout.Execute(context.Background(), root, args, os.Stdout, nil)

	os.Stdout, os.Stderr = prevOut, prevErr
	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	return <-gotOut, <-gotErr, err
}

// Under --output json the result is on stdout and nothing of it on stderr,
// in the tree production builds.
func TestJSONResultReachesStdoutUnderAnUnboundRoot(t *testing.T) {
	t.Run("deployment list", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeployments", mock.Anything).Return([]houston.Deployment{certifiedDep}, nil)

		stdout, stderr, err := execUnboundRoot(t, api, "deployment", "list", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deploymentListJSON
		decodeOne(t, stdout, &got)
		require.Len(t, got.Deployments, 1)
		assert.NotContains(t, stderr, "dep-ac", "the result went to stderr")
	})
	// deploy points os.Stdout at stderr for its progress, so its result is
	// the one that would go astray if it left by anything but the bound out.
	t.Run("deploy", func(t *testing.T) {
		deployMocks(t, deploy.Deployed{DeploymentID: "dep-ac", Image: "registry/rel-ac/airflow:deploy-2"}, nil)

		stdout, stderr, err := execUnboundRoot(t, newAPCClient(), "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got deployJSON
		decodeOne(t, stdout, &got)
		assert.Equal(t, "registry/rel-ac/airflow:deploy-2", got.Image)
		assert.NotContains(t, stderr, "registry/rel-ac/airflow:deploy-2", "the result went to stderr")
		assert.Contains(t, stderr, "Deploying: rel-ac", "the progress goes to stderr")
	})
	t.Run("deployment logs", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeploymentLogs", mock.Anything).Return([]houston.DeploymentLog{{CreatedAt: "t", Log: "a line"}}, nil)

		stdout, stderr, err := execUnboundRoot(t, api, "deployment", "logs", "scheduler", "dep-ac", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		assert.Equal(t, []logEntryJSON{{Component: "scheduler", Timestamp: "t", Message: "a line"}}, decodeLines[logEntryJSON](t, stdout))
		assert.Empty(t, stderr)
	})
}
