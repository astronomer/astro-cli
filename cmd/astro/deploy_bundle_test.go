package astro

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/input"
)

// execBundleRun runs astro deploy bundle with args under a root, as a user
// runs it.
func execBundleRun(t *testing.T, client astrov1.APIClient, answers string, args ...string) tokenRun {
	t.Helper()
	return execAstroCmd(t, client, answers, func(io.Writer) *cobra.Command { return NewDeployCmd() }, append([]string{"deploy", "bundle"}, args...)...)
}

// fakeDeployBundle stands in for the upload, recording what it was asked.
func fakeDeployBundle(t *testing.T) **astrodeploy.DeployBundleInput {
	t.Helper()
	var captured *astrodeploy.DeployBundleInput
	prev := DeployBundle
	t.Cleanup(func() { DeployBundle = prev })
	DeployBundle = func(in *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		captured = in
		return astrodeploy.BundleDeploy{DeploymentID: in.DeploymentID, DeployID: "dep-1", MountPath: in.MountPath, BundleVersion: "v1"}, nil
	}
	return &captured
}

// fakeDeleteBundle stands in for the delete, recording what it was asked.
func fakeDeleteBundle(t *testing.T) **astrodeploy.DeleteBundleInput {
	t.Helper()
	var captured *astrodeploy.DeleteBundleInput
	prev := DeleteBundle
	t.Cleanup(func() { DeleteBundle = prev })
	DeleteBundle = func(in *astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		captured = in
		return astrodeploy.BundleDelete{DeploymentID: in.DeploymentID, DeployID: "dep-1", MountPath: in.MountPath}, nil
	}
	return &captured
}

// noBundleTransport fails the test if the run reaches the upload or the
// delete.
func noBundleTransport(t *testing.T) {
	t.Helper()
	prevDeploy, prevDelete := DeployBundle, DeleteBundle
	t.Cleanup(func() { DeployBundle, DeleteBundle = prevDeploy, prevDelete })
	DeployBundle = func(*astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
		t.Error("nothing is deployed")
		return astrodeploy.BundleDeploy{}, nil
	}
	DeleteBundle = func(*astrodeploy.DeleteBundleInput) (astrodeploy.BundleDelete, error) {
		t.Error("nothing is deleted")
		return astrodeploy.BundleDelete{}, nil
	}
}

// bundleTestID is a Deployment id, which --deployment takes as it is.
const bundleTestID = "clxbundle0000000000000000"

// plainBundle is a directory outside any project that no bundle type
// detects, with the working path a fresh directory beside it.
func plainBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "seeds.csv"), []byte("a,b\n"), 0o600))
	prev := config.WorkingPath
	config.WorkingPath = t.TempDir()
	t.Cleanup(func() { config.WorkingPath = prev })
	return dir
}

// astro deploy bundle is the subcommand, and any other word after deploy is
// still the Deployment the project deploys to.
func TestDeployBundleRouting(t *testing.T) {
	root := deployUnderRoot()
	cmd, rest, err := root.Find([]string{"deploy", "bundle", "some/path"})
	require.NoError(t, err)
	assert.Equal(t, "astro deploy bundle", cmd.CommandPath())
	assert.Equal(t, []string{"some/path"}, rest)

	cmd, rest, err = root.Find([]string{"deploy", "clx-dep"})
	require.NoError(t, err)
	assert.Equal(t, "astro deploy", cmd.CommandPath())
	assert.Equal(t, []string{"clx-dep"}, rest)
}

// The type decides the defaults: a dbt project is detected and mounts below
// /usr/local/airflow/dbt by its name; --type dbt asks for one; any other type
// is a label that needs --mount-path.
func TestDeployBundleTypes(t *testing.T) {
	t.Run("dbt, detected", func(t *testing.T) {
		project := dbtProject(t)
		captured := fakeDeployBundle(t)
		r := execBundleRun(t, nil, "", project, "--deployment", bundleTestID, "--description", "nightly")
		require.NoError(t, r.err, r.stderr)
		in := *captured
		require.NotNil(t, in)
		assert.Equal(t, project, in.BundlePath)
		assert.Equal(t, "/usr/local/airflow/dbt/jaffle_shop", in.MountPath)
		assert.Equal(t, "dbt", in.BundleType)
		assert.Equal(t, bundleTestID, in.DeploymentID)
		assert.Equal(t, "nightly", in.Description)
		assert.Nil(t, in.Deployment, "named by id, so not read")
		assert.Nil(t, in.Login, "the current context")
		assert.Contains(t, r.stderr, "mount path /usr/local/airflow/dbt/jaffle_shop: the default for a dbt bundle")
		assert.Equal(t, "Successfully uploaded bundle with version v1 to Astro.\n", r.stdout)
	})
	t.Run("dbt, the working directory and a mount path given", func(t *testing.T) {
		project := dbtProject(t)
		config.WorkingPath = project
		captured := fakeDeployBundle(t)
		r := execBundleRun(t, nil, "", "--deployment", bundleTestID, "-m", "/usr/local/airflow/dbt/custom")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, project, (*captured).BundlePath)
		assert.Equal(t, "/usr/local/airflow/dbt/custom", (*captured).MountPath)
		assert.Equal(t, "dbt", (*captured).BundleType)
		assert.NotContains(t, r.stderr, "the default for")
	})
	// With no mount path, --type dbt derives one from the project, so it
	// needs one; given a mount path, it labels any directory dbt, as 1.x's
	// --non-dags-bundle-type dbt did.
	t.Run("--type dbt, without a dbt project", func(t *testing.T) {
		bundle := plainBundle(t)
		noBundleTransport(t)
		r := execBundleRun(t, nil, "", bundle, "--type", "dbt", "--deployment", bundleTestID)
		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), "no dbt_project.yml in "+bundle)

		captured := fakeDeployBundle(t)
		r = execBundleRun(t, nil, "", bundle, "--type", "dbt", "--deployment", bundleTestID, "--mount-path", "/usr/local/airflow/dbt/seeds")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, "dbt", (*captured).BundleType)
		assert.Equal(t, "/usr/local/airflow/dbt/seeds", (*captured).MountPath)
	})
	t.Run("--type of another kind needs a mount path", func(t *testing.T) {
		bundle := dbtProject(t)
		noBundleTransport(t)
		r := execBundleRun(t, nil, "", bundle, "--type", "seeds", "--deployment", bundleTestID)
		require.Error(t, r.err)
		assert.True(t, cliout.IsUsage(r.err), "%v", r.err)
		assert.Equal(t, "--mount-path is required for a bundle of type seeds: only a dbt bundle has a default", r.err.Error())
	})
	t.Run("--type of another kind, with a mount path", func(t *testing.T) {
		bundle := dbtProject(t)
		captured := fakeDeployBundle(t)
		r := execBundleRun(t, nil, "", bundle, "--type", "seeds", "--deployment", bundleTestID, "--mount-path", "/usr/local/airflow/seeds")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, "seeds", (*captured).BundleType, "a free-form label")
		assert.Equal(t, "/usr/local/airflow/seeds", (*captured).MountPath)
	})
	t.Run("nothing detected", func(t *testing.T) {
		bundle := plainBundle(t)
		noBundleTransport(t)
		r := execBundleRun(t, nil, "", bundle, "--deployment", bundleTestID)
		require.Error(t, r.err)
		assert.Equal(t, "--mount-path is required for a bundle of type none: only a dbt bundle has a default", r.err.Error())

		captured := fakeDeployBundle(t)
		r = execBundleRun(t, nil, "", bundle, "--deployment", bundleTestID, "--mount-path", "/usr/local/airflow/x")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, "none", (*captured).BundleType)
	})
}

// PATH is a directory of its own: one that does not exist, a file, and one in
// or below a project are refused before anything is read.
func TestDeployBundleRefusesItsPath(t *testing.T) {
	oneX := make1xProject(t)
	manifestDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	nested := filepath.Join(manifestDir, "include", "dbt")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "dbt_project.yml"), []byte("name: x\n"), 0o600))
	file := filepath.Join(t.TempDir(), "a-file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	for path, want := range map[string]string{
		oneX:        "is within an Astro project",
		manifestDir: "is within an Astro project",
		nested:      "is within an Astro project",
		filepath.Join(manifestDir, "..", "absent"): "does not exist",
		file: "is not a directory",
	} {
		noBundleTransport(t)
		r := execBundleRun(t, nil, "", path, "--deployment", bundleTestID, "--mount-path", "/x")
		require.Error(t, r.err, path)
		assert.Contains(t, r.err.Error(), want, path)
	}

	noBundleTransport(t)
	r := execBundleRun(t, nil, "", t.TempDir(), "--deployment", bundleTestID, "--mount-path", "/x", "--wait-time", "1m")
	require.Error(t, r.err)
	assert.True(t, cliout.IsUsage(r.err))
	assert.Contains(t, r.err.Error(), "cannot use --wait-time with --wait=false")
}

// --delete removes the bundle at --mount-path, or at the mount path PATH's
// type gives it.
func TestDeployBundleDelete(t *testing.T) {
	t.Run("the dbt project's default", func(t *testing.T) {
		project := dbtProject(t)
		captured := fakeDeleteBundle(t)
		r := execBundleRun(t, nil, "", "--delete", project, "--deployment", bundleTestID)
		require.NoError(t, r.err, r.stderr)
		in := *captured
		require.NotNil(t, in)
		assert.Equal(t, "/usr/local/airflow/dbt/jaffle_shop", in.MountPath)
		assert.Equal(t, "dbt", in.BundleType)
		assert.Equal(t, bundleTestID, in.DeploymentID)
		assert.Nil(t, in.Login)
		assert.Equal(t, "Successfully requested bundle delete for mount path /usr/local/airflow/dbt/jaffle_shop from Astro.\n", r.stdout)
	})
	t.Run("a mount path needs no bundle here", func(t *testing.T) {
		plainBundle(t)
		captured := fakeDeleteBundle(t)
		r := execBundleRun(t, nil, "", "--delete", "--deployment", bundleTestID, "--mount-path", "/usr/local/airflow/x", "--type", "seeds")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, "/usr/local/airflow/x", (*captured).MountPath)
		assert.Equal(t, "seeds", (*captured).BundleType)
	})
	// From a directory that holds no bundle, a mount path below the dbt
	// default directory is a dbt bundle's.
	t.Run("a dbt mount path from elsewhere", func(t *testing.T) {
		plainBundle(t)
		r := execBundleRun(t, dbtBundleMock(t, ""), "", "--delete", "--deployment", dbtTestDeploymentID, "--mount-path", "/usr/local/airflow/dbt/jaffle_shop", "-o", "json")
		require.NoError(t, r.err, r.stderr)
		var got bundleDeleteJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, "dbt", got.BundleType)
		assert.NotContains(t, fields, "bundle_name", "no bundle here to name")
	})
	t.Run("no mount path and nothing to derive one from", func(t *testing.T) {
		bundle := plainBundle(t)
		noBundleTransport(t)
		r := execBundleRun(t, nil, "", "--delete", bundle, "--deployment", bundleTestID)
		require.Error(t, r.err)
		assert.True(t, cliout.IsUsage(r.err))
		assert.Contains(t, r.err.Error(), "--mount-path is required for a bundle of type none")

		r = execBundleRun(t, nil, "", "--delete", bundle, "--type", "dbt", "--deployment", bundleTestID)
		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), "no dbt_project.yml in")
	})
}

// The target: --deployment, a link of the project here or an id. Naming
// none, a run that cannot be asked is refused as input_required before
// anything is uploaded or deleted.
func TestDeployBundleTarget(t *testing.T) {
	for _, del := range []bool{false, true} {
		name := "deploy"
		if del {
			name = "delete"
		}
		with := func(args ...string) []string {
			if del {
				return append([]string{"--delete"}, args...)
			}
			return args
		}
		t.Run(name+": a link of the project here", func(t *testing.T) {
			bundle := dbtProject(t)
			require.NoError(t, os.WriteFile(filepath.Join(config.WorkingPath, "pyproject.toml"), []byte("[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.deployments.prod]\ntarget = \"astro\"\nworkspace = \"clw-ws\"\ndeployment = \"clx-prod\"\n"), 0o600))
			deployed, deleted := fakeDeployBundle(t), fakeDeleteBundle(t)
			r := execBundleRun(t, nil, "", with(bundle, "--deployment", "prod")...)
			require.NoError(t, r.err, r.stderr)
			if del {
				assert.Equal(t, "clx-prod", (*deleted).DeploymentID)
			} else {
				assert.Equal(t, "clx-prod", (*deployed).DeploymentID)
			}
		})
		t.Run(name+": none named, under json", func(t *testing.T) {
			bundle := dbtProject(t)
			noBundleTransport(t)
			r := execBundleRun(t, nil, "", with(bundle, "-o", "json")...)
			require.Error(t, r.err)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Contains(t, got.Error, "pass --deployment with a Deployment id, a Deployment name, or a link name")
		})
		t.Run(name+": none named, without a terminal", func(t *testing.T) {
			bundle := dbtProject(t)
			noBundleTransport(t)
			r := execBundleRun(t, nil, "", with(bundle)...)
			require.Error(t, r.err)
			assert.True(t, input.IsRequired(r.err), "%v", r.err)
		})
		t.Run(name+": none named, picked", func(t *testing.T) {
			bundle := dbtProject(t)
			interactiveDeploy(t)
			deployed, deleted := fakeDeployBundle(t), fakeDeleteBundle(t)
			r := execBundleRun(t, coreMock(t, dbtTestDeployment()), "1\n", with(bundle, "--workspace", "clxdbtworkspace0000000000")...)
			require.NoError(t, r.err, r.stderr)
			var read *astrov1.Deployment
			if del {
				read = (*deleted).Deployment
			} else {
				read = (*deployed).Deployment
			}
			require.NotNil(t, read, "the picked Deployment is handed on, not read again")
			assert.Equal(t, dbtTestDeploymentID, read.Id)
		})
	}
}

// A deploy and a delete of the same bundle record the same type: --type,
// else what PATH holds, else what the mount path's shape says.
func TestDeployBundleDeployAndDeleteAgreeOnTheType(t *testing.T) {
	for _, tc := range []struct {
		name, mountPath, want string
		args                  []string
		dbtHere               bool
	}{
		{name: "a dbt mount path, no bundle here", mountPath: "/usr/local/airflow/dbt/jaffle_shop", want: "dbt"},
		{name: "a nested mount path", mountPath: "/usr/local/airflow/dbt/a/b", want: "none"},
		{name: "a mount path climbing out", mountPath: "/usr/local/airflow/dbt/../x", want: "none"},
		{name: "a dbt project here, another mount path", mountPath: "/usr/local/airflow/x", want: "dbt", dbtHere: true},
		{name: "--type wins", mountPath: "/usr/local/airflow/dbt/jaffle_shop", want: "seeds", args: []string{"--type", "seeds"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := plainBundle(t)
			if tc.dbtHere {
				bundle = dbtProject(t)
			}
			deployed, deleted := fakeDeployBundle(t), fakeDeleteBundle(t)
			args := append([]string{bundle, "--deployment", bundleTestID, "--mount-path", tc.mountPath}, tc.args...)
			r := execBundleRun(t, nil, "", args...)
			require.NoError(t, r.err, r.stderr)
			r = execBundleRun(t, nil, "", append([]string{"--delete"}, args...)...)
			require.NoError(t, r.err, r.stderr)
			assert.Equal(t, tc.want, (*deployed).BundleType)
			assert.Equal(t, tc.want, (*deleted).BundleType)
		})
	}
}

// bundle_name names the bundle the mount path was derived from: a mount path
// of its own is some other bundle's, or none.
func TestDeployBundleNameOnlyForTheDerivedMountPath(t *testing.T) {
	for mountPath, named := range map[string]bool{
		"":                                   true,
		"/usr/local/airflow/dbt/jaffle_shop": true,
		"/usr/local/airflow/dbt/custom":      false,
	} {
		project := dbtProject(t)
		args := []string{project, "--deployment", dbtTestDeploymentID, "-o", "json"}
		if mountPath != "" {
			args = append(args, "--mount-path", mountPath)
		}
		r := execBundleRun(t, dbtBundleMock(t, fakeBlobStore(t)), "", args...)
		require.NoError(t, r.err, r.stderr)
		var got bundleDeployJSON
		fields := decodeOne(t, r.stdout, &got)
		if named {
			assert.Equal(t, "jaffle_shop", got.BundleName, mountPath)
		} else {
			assert.NotContains(t, fields, "bundle_name", mountPath)
		}
	}
}

// --deployment that is neither a link nor an id is a Deployment's name in
// the workspace: one match is used, as read; none, or several, are errors
// that say so.
func TestDeployBundleDeploymentByName(t *testing.T) {
	other := dbtTestDeployment()
	other.Id = "clxdbtother00000000000000"
	t.Run("one", func(t *testing.T) {
		project := dbtProject(t)
		captured := fakeDeployBundle(t)
		r := execBundleRun(t, coreMock(t, dbtTestDeployment()), "", project, "--deployment", "analytics", "--workspace", "clxdbtworkspace0000000000")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, dbtTestDeploymentID, (*captured).DeploymentID)
		require.NotNil(t, (*captured).Deployment, "the listed Deployment is handed on")
	})
	t.Run("one, deleted", func(t *testing.T) {
		project := dbtProject(t)
		captured := fakeDeleteBundle(t)
		r := execBundleRun(t, coreMock(t, dbtTestDeployment()), "", "--delete", project, "--deployment", "analytics", "--workspace", "clxdbtworkspace0000000000")
		require.NoError(t, r.err, r.stderr)
		assert.Equal(t, dbtTestDeploymentID, (*captured).DeploymentID)
	})
	// None, or several, is a Deployment named wrong: a usage error, under
	// json the usage error object.
	t.Run("none", func(t *testing.T) {
		project := dbtProject(t)
		noBundleTransport(t)
		r := execBundleRun(t, coreMock(t, dbtTestDeployment()), "", project, "--deployment", "reporting", "--workspace", "clxdbtworkspace0000000000", "-o", "json")
		require.Error(t, r.err)
		assert.Equal(t, cliout.ExitUsage, r.code)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, string(cliout.KindUsage), got.Kind)
		assert.Contains(t, got.Error, "no Deployment named reporting in workspace clxdbtworkspace0000000000")
	})
	t.Run("two", func(t *testing.T) {
		project := dbtProject(t)
		noBundleTransport(t)
		r := execBundleRun(t, coreMock(t, dbtTestDeployment(), other), "", project, "--deployment", "analytics", "--workspace", "clxdbtworkspace0000000000", "-o", "json")
		require.Error(t, r.err)
		assert.Equal(t, cliout.ExitUsage, r.code)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, string(cliout.KindUsage), got.Kind)
		assert.Contains(t, got.Error, `2 Deployments are named "analytics" in workspace clxdbtworkspace0000000000: pass --deployment with one's id (`+dbtTestDeploymentID+", clxdbtother00000000000000)")
	})
}

// From a project on another host, a bundle goes under that host's login, by
// link and by id alike, and the picker, which lists only the current
// context's Deployments, is refused.
func TestDeployBundleUsesTheManifestDomainsLogin(t *testing.T) {
	for _, del := range []bool{false, true} {
		for _, target := range []string{"test", bundleTestID} {
			t.Run(fmt.Sprintf("%s, delete %t", target, del), func(t *testing.T) {
				setupDeployOnStage(t, manifestOnProd)
				logInToProd(t)
				deployed, deleted := fakeDeployBundle(t), fakeDeleteBundle(t)
				args := []string{"bundle", t.TempDir(), "--deployment", target, "--mount-path", "/x"}
				if del {
					args = append(args, "--delete")
				}
				_, err := execDeployCapture(args...)
				require.NoError(t, err)
				var login *config.Context
				if del {
					login = (*deleted).Login
				} else {
					login = (*deployed).Login
				}
				require.NotNil(t, login, "under the project's host")
				assert.Equal(t, "astronomer.io", login.Domain)
				assert.Equal(t, "prod-org", login.Organization)
			})
		}
	}

	t.Run("the picker on another host", func(t *testing.T) {
		setupDeployOnStage(t, manifestOnProd)
		logInToProd(t)
		interactiveDeploy(t)
		noBundleTransport(t)
		_, err := execDeployCapture("bundle", t.TempDir(), "--mount-path", "/x")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the deployment picker lists only the current context's Deployments")
		assert.Contains(t, err.Error(), "astro context switch astronomer.io")
	})

	t.Run("outside a project", func(t *testing.T) {
		setupDeployOnStage(t, manifestOnProd)
		require.NoError(t, os.Remove(filepath.Join(config.WorkingPath, "pyproject.toml")))
		captured := fakeDeployBundle(t)
		_, err := execDeployCapture("bundle", t.TempDir(), "--deployment", bundleTestID, "--mount-path", "/x")
		require.NoError(t, err)
		assert.Nil(t, (*captured).Login, "the current context")
	})
}

func TestDeployBundleJSON(t *testing.T) {
	t.Run("deploy", func(t *testing.T) {
		project := dbtProject(t)
		r := execBundleRun(t, dbtBundleMock(t, fakeBlobStore(t)), "", project, "--deployment", dbtTestDeploymentID, "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		var got bundleDeployJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, bundleDeployJSON{
			Deployment:     dbtTestDeploymentID,
			DeploymentName: "analytics",
			Workspace:      "clxdbtworkspace0000000000",
			DeployID:       dbtTestDeployID,
			BundleType:     "dbt",
			BundleName:     "jaffle_shop",
			BundlePath:     project,
			MountPath:      "/usr/local/airflow/dbt/jaffle_shop",
			BundleVersion:  dbtTestVersion,
		}, got)
		assert.NotContains(t, fields, "git", "the project is in no git checkout")
		assert.NotContains(t, fields, "wait_error")
	})
	// Named by id, the Deployment is never read, so its Workspace is not
	// known and not published.
	t.Run("delete", func(t *testing.T) {
		plainBundle(t)
		r := execBundleRun(t, dbtBundleMock(t, ""), "", "--delete", "--deployment", dbtTestDeploymentID, "--mount-path", "/usr/local/airflow/seeds", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		var got bundleDeleteJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, bundleDeleteJSON{
			Action:     "deleted",
			Deployment: dbtTestDeploymentID,
			DeployID:   dbtTestDeployID,
			BundleType: "none",
			MountPath:  "/usr/local/airflow/seeds",
		}, got)
		assert.NotContains(t, fields, "workspace")
	})
}

// A --wait's progress is on stderr in both formats. In text the result comes
// first, so stdout is what it is without --wait; under json the object comes
// after the wait it reports, and says it waited, and why the wait failed.
func TestDeployBundleWait(t *testing.T) {
	for _, del := range []bool{false, true} {
		name := "deploy"
		if del {
			name = "delete"
		}
		run := func(t *testing.T, more ...string) tokenRun {
			project := dbtProject(t)
			args := []string{project, "--deployment", dbtTestDeploymentID}
			var client astrov1.APIClient
			if del {
				client = dbtBundleMock(t, "")
				args = append(args, "--delete")
			} else {
				client = dbtBundleMock(t, fakeBlobStore(t))
			}
			return execBundleRun(t, client, "", append(args, more...)...)
		}
		t.Run(name+" text", func(t *testing.T) {
			without := run(t)
			require.NoError(t, without.err)
			waited := stubBundleWait(t, nil)
			r := run(t, "--wait")
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			assert.Equal(t, []string{dbtTestDeploymentID}, *waited)
			assert.Equal(t, without.stdout, r.stdout)
			assert.Contains(t, r.stderr, "Waiting for the deployment to become healthy…")
		})
		t.Run(name+" json, the wait failing", func(t *testing.T) {
			stubBundleWait(t, errors.New("timed out"))
			r := run(t, "--wait", "-o", "json")
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got struct {
				DeployID  string `json:"deploy_id"`
				Waited    bool   `json:"waited"`
				WaitError string `json:"wait_error"`
			}
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, dbtTestDeployID, got.DeployID)
			assert.True(t, got.Waited)
			assert.Equal(t, "timed out", got.WaitError)
		})
		t.Run(name+" json", func(t *testing.T) {
			waited := stubBundleWait(t, nil)
			r := run(t, "--wait", "-o", "json")
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			assert.Equal(t, []string{dbtTestDeploymentID}, *waited)
			var got struct {
				Waited bool `json:"waited"`
			}
			fields := decodeOne(t, r.stdout, &got)
			assert.True(t, got.Waited)
			assert.NotContains(t, fields, "wait_error")
		})
	}
}

// The guidance is for the subcommand typed, wherever the flags stand, a
// flag's value never taken for it.
func TestDbtRemovedReadsTheSubcommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--workspace-id", "ws1", "deploy"}, errDbtDeployRemoved},
		{[]string{"-n", "prod", "delete"}, errDbtDeleteRemoved},
		{[]string{"--project-path=.", "cleanup"}, errDbtCleanupRemoved},
		{[]string{"-w", "deploy"}, errDbtDeployRemoved},
		{[]string{"-wt", "5m", "deploy"}, errDbtDeployRemoved},
		{[]string{"-nprod", "delete"}, errDbtDeleteRemoved},
		{[]string{"--workspace-id", "deploy"}, errDbtRemoved},
		{[]string{"--", "deploy"}, errDbtRemoved},
	} {
		assert.Equal(t, tc.want, dbtRemovedGuidance(tc.args), "%v", tc.args)
	}
}

// astro dbt is a tombstone: every subcommand, and the group bare, says what
// replaced it, in text and as the one error object under json, and runs
// nothing, whatever 1.x flags it is passed.
func TestDbtRemoved(t *testing.T) {
	cases := map[string]string{
		"":        errDbtRemoved,
		"deploy":  errDbtDeployRemoved,
		"delete":  errDbtDeleteRemoved,
		"cleanup": errDbtCleanupRemoved,
	}
	for sub, want := range cases {
		args := []string{"dbt"}
		if sub != "" {
			args = append(args, sub, "clx-dep", "--project-path", t.TempDir(), "-n", "analytics")
		}
		noBundleTransport(t)
		r := execAstroCmd(t, nil, "", func(io.Writer) *cobra.Command { return newDbtRemovedCmd() }, args...)
		require.Error(t, r.err, sub)
		assert.Equal(t, cliout.ExitUsage, r.code, sub)
		assert.Equal(t, want, r.err.Error(), sub)
		assert.NotContains(t, want, "`", "command names are plain text")
		assert.Empty(t, r.stdout, sub)

		r = execAstroCmd(t, nil, "", func(io.Writer) *cobra.Command { return newDbtRemovedCmd() }, append(args, "-o", "json")...)
		require.Error(t, r.err, sub)
		assert.Equal(t, cliout.ExitUsage, r.code, sub)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, want, got.Error, sub)
	}
}
