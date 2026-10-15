package astro

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// execUnboundRoot runs args the way production does: the tree is built by
// AddCmds with the process's stdout as it is at construction, under a root
// that binds no writer of its own, and run by cliout.Execute. execAstroCmd
// binds the root's out, which hides a command whose result leaves by
// cmd.OutOrStdout() while strayStdoutToStderr has pointed os.Stdout at
// stderr; this does not.
func execUnboundRoot(t *testing.T, client astrov1.APIClient, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return execUnboundRootWith(t, client, nil, args...)
}

// execUnboundRootWith is execUnboundRoot with a v1alpha1 client too, which
// the bundle commands call.
func execUnboundRootWith(t *testing.T, client astrov1.APIClient, alpha astrov1alpha1.APIClient, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	testUtil.SetupOSArgsForGinkgo()
	prevClient, prevAlpha := astroV1Client, astroV1Alpha1Client
	// A bundle deploy may resolve a Workspace into the package's
	// workspaceID, which coalesceWorkspace reads before the context; see
	// execDeployCmd.
	t.Cleanup(func() { astroV1Client, astroV1Alpha1Client, workspaceID = prevClient, prevAlpha, "" })

	outR, outW, perr := os.Pipe()
	require.NoError(t, perr)
	errR, errW, perr := os.Pipe()
	require.NoError(t, perr)
	prevOut, prevErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	// Put them back however the run ends, so a failure in it cannot leave
	// the rest of the package writing to closed pipes. Restoring twice is
	// harmless; the run below restores them itself before it reads.
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
	root.AddCommand(AddCmds(client, alpha, nil, os.Stdout)...)
	err = cliout.Execute(context.Background(), root, args, os.Stdout, nil)

	os.Stdout, os.Stderr = prevOut, prevErr
	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())
	return <-gotOut, <-gotErr, err
}

// Under --output json the result is on stdout and nothing of it on stderr,
// in the tree production builds.
func TestJSONResultReachesStdoutUnderAnUnboundRoot(t *testing.T) {
	t.Run("deploy bundle", func(t *testing.T) {
		project := dbtProject(t)
		stdout, stderr, err := execUnboundRoot(t, dbtBundleMock(t, fakeBlobStore(t)), "deploy", "bundle", project, "--deployment", dbtTestDeploymentID, "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got bundleDeployJSON
		decodeOne(t, stdout, &got)
		assert.Equal(t, dbtTestVersion, got.BundleVersion)
		assert.NotContains(t, stderr, dbtTestVersion, "the result went to stderr")
		assert.Contains(t, stderr, "mount path /usr/local/airflow/dbt/jaffle_shop", "the notes go to stderr")
	})
	// The bundle sits in a git checkout with uncommitted changes, so the
	// platform code prints its note on the way: under json that goes to
	// stderr, and stdout is the one result object.
	t.Run("deploy bundle from a dirty checkout", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("needs git")
		}
		bundle := dbtProject(t)
		gitIn := func(args ...string) {
			t.Helper()
			full := append([]string{"-C", bundle, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)
			out, err := exec.Command("git", full...).CombinedOutput()
			require.NoError(t, err, string(out))
		}
		gitIn("init", "-q")
		gitIn("add", ".")
		gitIn("commit", "-q", "-m", "first")
		require.NoError(t, os.WriteFile(filepath.Join(bundle, "dbt_project.yml"), []byte("name: changed\n"), 0o600))

		stdout, stderr, err := execUnboundRoot(t, dbtBundleMock(t, fakeBlobStore(t)), "deploy", "bundle", bundle, "--deployment", dbtTestDeploymentID, "--mount-path", "/usr/local/airflow/x", "--output", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got bundleDeployJSON
		decodeOne(t, stdout, &got)
		assert.Equal(t, dbtTestVersion, got.BundleVersion)
		assert.Contains(t, stderr, "uncommitted changes", "the platform's note goes to stderr")
	})
	t.Run("remote deploy", func(t *testing.T) {
		stubClientDeploy(t, &astrodeploy.ClientDeploy{Image: "r/c:deploy-x", Registry: "r/c", Tag: "deploy-x"}, nil)
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
		prev := config.WorkingPath
		config.WorkingPath = dir
		t.Cleanup(func() { config.WorkingPath = prev })

		stdout, stderr, err := execUnboundRoot(t, nil, "remote", "deploy", "-o", "json")
		require.NoError(t, err, "stderr:\n%s", stderr)
		var got remoteDeployJSON
		decodeOne(t, stdout, &got)
		assert.Equal(t, "r/c:deploy-x", got.Image)
		assert.NotContains(t, stderr, "r/c:deploy-x", "the result went to stderr")
	})
}
