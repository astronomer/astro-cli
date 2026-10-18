package apc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// deployCalls is what deploy handed its platform calls in one test.
type deployCalls struct {
	// images are the image deploys asked for, in order.
	images []deploy.ManifestImage
	// dagUploads is how many times the DAG deploy was called.
	dagUploads int
	// opts is the options the last DAG deploy was handed.
	opts deploy.Options
}

// deployMocks replaces deploy's platform calls for one test: the image
// deploy answers res, after giving each of warnings, and the DAG deploy
// (--dags) returns dags.
func deployMocks(t *testing.T, res deploy.ManifestImageResult, dags error, warnings ...string) *deployCalls {
	t.Helper()
	prevImage, prevDags := DeployManifestImage, DagsOnlyDeploy
	t.Cleanup(func() { DeployManifestImage, DagsOnlyDeploy = prevImage, prevDags })
	seen := new(deployCalls)
	DeployManifestImage = func(_ houston.ClientInterface, in *deploy.ManifestImage) (deploy.ManifestImageResult, error) {
		seen.images = append(seen.images, *in)
		fmt.Fprintln(in.Progress, "Deploying: rel-ac")
		for _, w := range warnings {
			in.Warn(w)
		}
		return res, nil
	}
	DagsOnlyDeploy = func(_ houston.ClientInterface, _, deploymentID, _ string, _ *string, _ bool, _ string, opts deploy.Options) (string, error) {
		seen.dagUploads++
		seen.opts = opts
		return deploymentID, dags
	}
	return seen
}

// inWorkingDir points the deploy at a fresh directory for one test, with a
// dags directory in it or not.
func inWorkingDir(t *testing.T, withDags bool) string {
	t.Helper()
	dir := t.TempDir()
	if withDags {
		require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o755))
	}
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
	return dir
}

// inProject is inWorkingDir with a pyproject.toml project in the directory,
// the one kind of project whose DAGs --dags uploads.
func inProject(t *testing.T, withDags bool) string {
	t.Helper()
	dir := inWorkingDir(t, withDags)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n"), 0o600))
	return dir
}

// realDagsOnlyDeploy puts the real DAG deploy back for one test, counting
// the times it is asked.
func realDagsOnlyDeploy(t *testing.T) *int {
	t.Helper()
	calls := new(int)
	DagsOnlyDeploy = func(c houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, opts deploy.Options) (string, error) {
		*calls++
		return deploy.DagsOnlyDeploy(c, wsID, deploymentID, dagsParentPath, dagDeployURL, cleanUpFiles, description, opts)
	}
	return calls
}

// realImageDeploy puts the real image deploy back for one test: one that
// needs no container engine, as --remote's does not.
func realImageDeploy(t *testing.T) {
	t.Helper()
	prev := DeployManifestImage
	t.Cleanup(func() { DeployManifestImage = prev })
	DeployManifestImage = deploy.DeployManifestImage
}

// deployWorkspace is the workspace runAPC's config deploys to.
const deployWorkspace = "ck05r3bor07h40d02y2hw4n4v"

// dagsDeployment is the Deployment dagsAPI holds, dep-ac, of DAG deployment
// type typ, read with its type or not (as before 0.29.0).
func dagsDeployment(typ string, read bool) *houston.Deployment {
	return &houston.Deployment{ID: "dep-ac", ReleaseName: "rel-ac", ClusterID: "cl-1", RuntimeVersion: "12.0.0", DagDeployment: houston.DagDeploymentConfig{Type: typ}, DagDeploymentRead: read}
}

// takesDagUploads is the cluster config of a cluster with DAG-only deploys.
var takesDagUploads = &houston.AppConfig{Version: "1.0.0", Flags: houston.FeatureFlags{DagOnlyDeployment: true}}

// dagsAPI is a Houston mock for the real update and DAG deploy: a
// workspace holding the one Deployment dep, whose cluster config each read
// in turn answers with the next of cfgs, the last for any read after.
func dagsAPI(dep *houston.Deployment, cfgs ...func() (*houston.AppConfig, error)) *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	req := houston.GetAppConfigRequest{ClusterID: dep.ClusterID, WorkspaceUUID: deployWorkspace, DeploymentUUID: dep.ID}
	for i, cfg := range cfgs {
		c, err := cfg()
		call := api.On("GetAppConfig", req).Return(c, err)
		if i < len(cfgs)-1 {
			call.Once()
		}
	}
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Maybe()
	api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Maybe()
	api.On("GetWorkspace", deployWorkspace).Return(&houston.Workspace{ID: deployWorkspace}, nil).Maybe()
	api.On("ListDeployments", mock.Anything).Return([]houston.Deployment{*dep}, nil).Maybe()
	api.On("GetDeployment", dep.ID).Return(dep, nil).Maybe()
	api.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{{Version: "12.1.1", AirflowVersion: "2.10.3"}}, nil).Maybe()
	api.On("UpdateDeploymentImage", mock.Anything).Return(&houston.UpdateDeploymentImageResp{}, nil).Maybe()
	return api
}

// cfgOf answers a cluster config read with cfg.
func cfgOf(cfg *houston.AppConfig) func() (*houston.AppConfig, error) {
	return func() (*houston.AppConfig, error) { return cfg, nil }
}

// cfgFails answers a cluster config read with err.
func cfgFails(err error) func() (*houston.AppConfig, error) {
	return func() (*houston.AppConfig, error) { return nil, err }
}

// quietFromTheStart turns apc.show_warnings off once the deploy runs:
// runAPC loads the config afresh before it runs the tree, so it is turned
// off on the deploy's first question to Houston.
func quietFromTheStart(t *testing.T, api *mocks.ClientInterface) {
	t.Helper()
	for _, c := range api.ExpectedCalls {
		if c.Method == "GetWorkspace" {
			c.Run(func(mock.Arguments) { require.NoError(t, config.CFG.ShowWarnings.SetHomeString("false")) })
		}
	}
}

// inImageWarning is the warning for the prebuilt image img:1, which is all
// an image Deployment's DAGs.
const inImageWarning = "this Deployment runs the Dags inside the image img:1; the dags folder is not uploaded. An image astro package built without a dockerfile declared under [tool.astro] contains none."

// deployPushed is what the image deploy reports in these tests.
var deployPushed = deploy.ManifestImageResult{RuntimeVersion: "12.1.1", Image: "registry/rel-ac/airflow:deploy-2", URL: "https://airflow"}

func TestDeployJSON(t *testing.T) {
	pushed := deployPushed

	// From a project's root, --image-name is the project's deploy with a
	// prebuilt image: the image deploy is asked for it, with the DAGs, and
	// the result is in Astro's spellings.
	t.Run("--image-name from a project", func(t *testing.T) {
		inProject(t, true)
		uploaded := pushed
		uploaded.DagsUploaded = true
		seen := deployMocks(t, uploaded, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: deployWorkspace, Type: "image-and-dag", Image: pushed.Image, RuntimeVersion: "12.1.1", URL: pushed.URL}, got)
		assert.Contains(t, run.stderr, "Deploying: rel-ac", "the progress goes to stderr")
		require.Len(t, seen.images, 1)
		in := seen.images[0]
		assert.Equal(t, "img:1", in.ImageName)
		assert.True(t, in.IncludeDags)
		assert.False(t, in.ImageAlone)
		assert.False(t, in.InRegistry)
	})

	// A warning the image deploy gives is on stderr with the progress under
	// json, and in the result; in text, on stdout, once.
	t.Run("--image-name warnings", func(t *testing.T) {
		inProject(t, true)
		inImage := pushed
		inImage.DagsInImage = true
		deployMocks(t, inImage, nil, inImageWarning)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.True(t, got.DagsInImage)
		assert.Equal(t, []string{inImageWarning}, got.Warnings)
		assert.Equal(t, 1, strings.Count(run.stderr, "Warning: "+inImageWarning+"\n"), run.stderr)

		run = runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "Deploying prebuilt image img:1...\nDeploying: rel-ac\nWarning: "+inImageWarning+"\n"+
			"Deployed image registry/rel-ac/airflow:deploy-2 to Deployment dep-ac, with the Dags inside the image.\nDeployment: https://airflow\n", run.stdout)
		assert.NotContains(t, run.stderr, "Warning")
	})

	t.Run("--image --image-name", func(t *testing.T) {
		inProject(t, true)
		seen := deployMocks(t, pushed, errors.New("must not deploy DAGs"))
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image-only", got.Type)
		require.Len(t, seen.images, 1)
		assert.False(t, seen.images[0].IncludeDags)
		assert.Zero(t, seen.dagUploads)
	})

	t.Run("--dags", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		houstonVersion = "1.0.0"
		inProject(t, true)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--dags", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: deployWorkspace, Type: "dag-only"}, got)
	})

	// A project's deploy never picks for you: under json, one that names no
	// Deployment is refused as a question it could not ask, naming the flag
	// that answers it, and nothing is deployed. --image-name and --remote
	// are no different.
	for _, args := range [][]string{
		{"--dags"},
		{"--image-name", "img:1"},
		{"--image-name", "quay.example.com/img:1", "--remote", "--runtime-version", "12.1.1"},
	} {
		t.Run(strings.Join(args, " ")+" with no argument is input_required", func(t *testing.T) {
			seen := deployMocks(t, pushed, nil)
			houstonVersion = "1.0.0"
			linkedProject(t)
			run := runAPC(t, newAPCClient(), "", append(append([]string{"deploy"}, args...), "-o", "json")...)
			require.Equal(t, 1, run.code, "stderr:\n%s", run.stderr)
			var got struct {
				Error string `json:"error"`
				Kind  string `json:"kind"`
			}
			decodeOne(t, run.stdout, &got)
			assert.Equal(t, "input_required", got.Kind)
			assert.Contains(t, got.Error, "--deployment")
			assert.Zero(t, seen.dagUploads)
			assert.Empty(t, seen.images)
		})
	}

	// --yes answered the earlier --image-name route's runtime question and its empty-dags
	// one; the shared rules ask neither, so it is no flag of v2's.
	t.Run("--yes is no flag", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "--yes", "-o", "json")
		assert.Equal(t, 2, run.code)
		assert.Contains(t, run.stdout, "unknown flag: --yes")
	})

	t.Run("--image-name --remote", func(t *testing.T) {
		inProject(t, true)
		seen := deployMocks(t, deploy.ManifestImageResult{Image: "my/image:1", RuntimeVersion: "12.1.1"}, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "my/image:1", "--remote", "--runtime-version", "12.1.1", "--description", "from CI", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: deployWorkspace, Type: "image-and-dag", Image: "my/image:1", RuntimeVersion: "12.1.1"}, got)
		require.Len(t, seen.images, 1)
		in := seen.images[0]
		assert.Equal(t, "my/image:1", in.ImageName)
		assert.True(t, in.InRegistry)
		assert.Equal(t, "12.1.1", in.RuntimeVersion)
		assert.Equal(t, "from CI", in.Description)
	})
}

// --dags asks for nothing but the upload, so with no dags directory the
// deploy fails, having sent nothing. A Deployment or cluster that takes no
// upload is refused as such first, as that is what there is to change.
func TestDeployDagsNoDagsDirectoryJSON(t *testing.T) {
	dir := inProject(t, false)
	for _, args := range [][]string{
		{"deploy", "dep-ac", "--dags", "-o", "json"},
		{"deploy", "dep-ac", "--dags"},
	} {
		deployMocks(t, deployPushed, nil)
		realDagsOnlyDeploy(t)
		run := runAPC(t, dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads)), "", args...)
		assert.NotEqual(t, 0, run.code, "%v", args)
		require.ErrorIs(t, run.err, deploy.ErrNoDagsDirectory, "%v", args)
		assert.ErrorContains(t, run.err, filepath.Join(dir, "dags")+" is not a directory. Nothing was uploaded, and the Deployment keeps the Dags it had", "%v", args)
		assert.NotContains(t, run.stdout, `"deployment"`, "%v: no result for a deploy that did not happen", args)
	}

	for _, tc := range []struct {
		name string
		api  *mocks.ClientInterface
		want error
	}{
		{"a git-sync Deployment", dagsAPI(dagsDeployment(houston.GitSyncDeploymentType, true), cfgOf(takesDagUploads)), deploy.ErrDagOnlyDeployNotEnabledForDeployment},
		{"a cluster without DAG-only deploys", dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(&houston.AppConfig{Version: "2.0.0"})), deploy.ErrDagOnlyDeployDisabledInConfig},
	} {
		deployMocks(t, deployPushed, nil)
		realDagsOnlyDeploy(t)
		run := runAPC(t, tc.api, "", "deploy", "dep-ac", "--dags", "-o", "json")
		assert.NotEqual(t, 0, run.code, tc.name)
		require.ErrorIs(t, run.err, tc.want, tc.name)
		assert.NotErrorIs(t, run.err, deploy.ErrNoDagsDirectory, tc.name)
	}
}

// --image-name --remote end to end through the real update and DAG deploy,
// from a project's root, the Deployment placed from what Houston says of it:
// the mutation is sent, and the DAGs follow as a pushed image's do.
func TestDeployRemoteDagsJSON(t *testing.T) {
	down := errors.New("houston is down")
	undecided := fmt.Sprintf("Dags were NOT updated: whether this Deployment takes Dag uploads could not be read (%s). The image was deployed, and the Deployment keeps the Dags it had. To upload them, run astro deploy dep-ac --dags from the project directory.", "failed to get app config: houston is down")
	noUpload := "Deployment dep-ac takes no Dag upload ("
	for _, tc := range []struct {
		name string
		dep  *houston.Deployment
		cfgs []func() (*houston.AppConfig, error)
		// kind is the result's type.
		kind string
		// warnings are with show_warnings on; quiet those it still gives.
		// Each is the start of the warning given.
		warnings, quiet []string
	}{
		// Houston deploys a Deployment with no type by its image.
		{"no type read warns", dagsDeployment("", true), nil, "image-and-dag", []string{inImageWarning}, nil},
		// Before 0.29.0 GetDeployment reads no type, so no type says
		// nothing of the Deployment: it is uploaded to, and DagsOnlyDeploy
		// refuses it, which is said.
		{"no type before 0.29.0", dagsDeployment("", false), []func() (*houston.AppConfig, error){cfgOf(&houston.AppConfig{Version: "0.25.0"})}, "image-only", []string{noUpload}, []string{noUpload}},
		{"git-sync says where its DAGs come from", dagsDeployment(houston.GitSyncDeploymentType, true), nil, "image-only", []string{"Deployment dep-ac takes its Dags from a git repository (git-sync)"}, []string{"Deployment dep-ac takes its Dags from a git repository (git-sync)"}},
		// A cluster config it cannot read leaves a DAG-only Deployment
		// not placed: DagsOnlyDeploy reads the config again, and decides.
		{"dag_deploy whose cluster config is read the second time skips the upload, and says so", dagsDeployment(houston.DagOnlyDeploymentType, true), []func() (*houston.AppConfig, error){cfgFails(down), cfgOf(takesDagUploads)}, "image-only", []string{"no Dags were uploaded: there is no dags directory in "}, []string{"no Dags were uploaded: there is no dags directory in "}},
		{"dag_deploy whose cluster config is read the second time and refuses", dagsDeployment(houston.DagOnlyDeploymentType, true), []func() (*houston.AppConfig, error){cfgFails(down), cfgOf(&houston.AppConfig{Version: "2.0.0"})}, "image-only", []string{noUpload}, []string{noUpload}},
		// Read neither time: the image update stands, and the deploy says
		// the DAGs were not updated, whatever show_warnings is.
		{"dag_deploy whose cluster config cannot be read says so, and succeeds", dagsDeployment(houston.DagOnlyDeploymentType, true), []func() (*houston.AppConfig, error){cfgFails(down)}, "image-only", []string{undecided}, []string{undecided}},
	} {
		for _, quiet := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s quiet=%v", tc.name, quiet), func(t *testing.T) {
				inProject(t, false)
				realImageDeploy(t)
				cfgs := tc.cfgs
				if cfgs == nil {
					cfgs = []func() (*houston.AppConfig, error){cfgOf(takesDagUploads)}
				}
				api := dagsAPI(tc.dep, cfgs...)
				if quiet {
					quietFromTheStart(t, api)
				}
				run := runAPC(t, api, "", "deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				api.AssertCalled(t, "UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{ReleaseName: "rel-ac", Image: "img:1", RuntimeVersion: "12.1.1", DeployRevisionDescription: "Deployed via <astro deploy>"})
				assert.Contains(t, run.stderr, "Image successfully updated")
				var got deployJSON
				decodeOne(t, run.stdout, &got)
				assert.Equal(t, tc.kind, got.Type)
				assert.Equal(t, "img:1", got.Image)
				assert.Equal(t, "12.1.1", got.RuntimeVersion)
				want := tc.warnings
				if quiet {
					want = tc.quiet
				}
				require.Len(t, got.Warnings, len(want), "%v", got.Warnings)
				for i, w := range want {
					assert.True(t, strings.HasPrefix(got.Warnings[i], w), "%q starts %q", got.Warnings[i], w)
				}
			})
		}
	}
}

// From outside any project --image-name --remote points the Deployment at
// the image, alone: the working directory's dags/ is not uploaded, even to
// a Deployment that takes DAG uploads, and the deploy says the DAGs were not
// updated, whatever show_warnings is.
func TestDeployRemoteOutsideAProjectShipsTheImageAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		dep  *houston.Deployment
		cfg  func() (*houston.AppConfig, error)
		want string
	}{
		{"a Deployment that takes DAG uploads", dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads), "Dags were not updated: this deploy ran outside a pyproject.toml project, so it shipped the image alone, and the Deployment keeps the Dags it had. To upload them, run astro deploy dep-ac --dags from the project directory."},
		{"a Deployment not placed", dagsDeployment(houston.DagOnlyDeploymentType, true), cfgFails(errors.New("houston is down")), "this deploy ran outside a pyproject.toml project, so it shipped the image alone. If this Deployment takes Dag uploads, it keeps the Dags it had: to upload them, run astro deploy dep-ac --dags from the project directory."},
	} {
		for _, quiet := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s quiet=%v", tc.name, quiet), func(t *testing.T) {
				inWorkingDir(t, true)
				realImageDeploy(t)
				uploads := realDagsOnlyDeploy(t)
				api := dagsAPI(tc.dep, tc.cfg)
				if quiet {
					quietFromTheStart(t, api)
				}
				run := runAPC(t, api, "", "deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				assert.Zero(t, *uploads, "no dags/ is uploaded from outside a project")
				var got deployJSON
				decodeOne(t, run.stdout, &got)
				assert.Equal(t, "image-only", got.Type)
				assert.Empty(t, got.Link)
				assert.Equal(t, []string{tc.want}, got.Warnings)
			})
		}
	}
}

// A runtime --runtime-version names that the platform would drop is refused
// before the mutation, in the words a pushed image's label is refused in.
func TestDeployRemoteHoldsItsRuntimeToThePlatformsRules(t *testing.T) {
	inProject(t, true)
	realImageDeploy(t)
	api := dagsAPI(dagsDeployment(houston.ImageDeploymentType, true), cfgOf(takesDagUploads))
	run := runAPC(t, api, "", "deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "11.0.0", "-o", "json")
	require.Equal(t, 1, run.code, "stderr:\n%s", run.stderr)
	assert.Contains(t, run.stdout, "cannot deploy Astro Runtime 11.0.0: it is a downgrade from the deployment's current 12.0.0; to deploy, rebuild img:1 FROM Astro Runtime 12.0.0 or newer")
	api.AssertNotCalled(t, "UpdateDeploymentImage", mock.Anything)
}
