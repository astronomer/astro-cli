package deploy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// prebuiltFor is the deploy of the local image named, from a project's root
// (IncludeDags), the way --image-name runs it.
func prebuiltFor(t *testing.T, image string, warnings *[]string) *ManifestImage {
	t.Helper()
	in := manifestImageFor(t, &imagebuild.ManifestBuild{}, warnings)
	in.ImageName = image
	return in
}

// imageAlone is the deploy of the local image named from outside any
// project: no DAGs go with it.
func imageAlone(t *testing.T, image string, warnings *[]string) *ManifestImage {
	t.Helper()
	in := prebuiltFor(t, image, warnings)
	in.IncludeDags, in.ImageAlone = false, true
	return in
}

// withShowWarnings sets apc.show_warnings for one test. It goes after
// apcTarget.client, which loads the test config afresh.
func withShowWarnings(t *testing.T, on bool) {
	t.Helper()
	value := "false"
	if on {
		value = "true"
	}
	require.NoError(t, config.CFG.ShowWarnings.SetHomeString(value))
	t.Cleanup(func() { _ = config.CFG.ShowWarnings.SetHomeString("true") })
}

// A prebuilt image is not built: its runtime is its label's, and it is
// pushed to the APC registry as <registry>/<release>/airflow:<nextTag>,
// whatever its own tag. Houston deploys only a push to <release>/airflow,
// skips a push tagged "latest", and does not roll out a tag it has deployed
// before, so the user's tag would deploy nothing for "latest" or a reused
// one. The push carries the context's token.
func TestDeployPrebuiltImageToTheAPCRegistryTakesTheNextTag(t *testing.T) {
	for _, image := range []string{"my-image:v7", "my-image:latest", "my-image"} {
		t.Run(image, func(t *testing.T) {
			cmd := withManifestEngine(t, "3.1-5")
			client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.0-9", listed: houstonLists, version: "1.0.0", dagsHook: dagServer(t, nil)}).client(t)
			var warnings []string
			res, err := DeployManifestImage(client, prebuiltFor(t, image, &warnings))
			require.NoError(t, err)

			assert.Equal(t, "registry.example.com/my-release/airflow:deploy-7", res.Image)
			assert.Equal(t, "3.1-5", res.RuntimeVersion, "the image's label")
			assert.False(t, cmd.called(" build "), "nothing is built: %v", cmd.calls)
			assert.True(t, cmd.called("docker tag "+image+" registry.example.com/my-release/airflow:deploy-7"), "%v", cmd.calls)
			assert.True(t, cmd.called("docker push registry.example.com/my-release/airflow:deploy-7"), "%v", cmd.calls)
			assert.True(t, cmd.called("docker image inspect --format {{ index .Config.Labels \"io.astronomer.docker.runtime.version\" }} "+image), "the label is read off the image named: %v", cmd.calls)
			assert.NotEmpty(t, cmd.pushAuth, "the push carries the context's token")
			assert.Empty(t, warnings)
		})
	}
}

// dagServer stands in for a Deployment's DAG server, counting the uploads
// it takes into uploads when that is not nil.
func dagServer(t *testing.T, uploads *int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if uploads != nil {
			*uploads++
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// From a project's root, a prebuilt image to a dag_deploy Deployment is
// followed by the project's DAGs, uploaded beside it.
func TestDeployPrebuiltImageUploadsTheProjectsDags(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	var uploads int
	client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: dagServer(t, &uploads)}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, prebuiltFor(t, "my-image:v7", &warnings))
	require.NoError(t, err)
	assert.True(t, res.DagsUploaded)
	assert.Equal(t, 1, uploads)
	assert.Empty(t, warnings)
}

// From a project's root, a prebuilt image to an image Deployment is all the
// DAGs it runs, which the deploy says unless apc.show_warnings is off: the
// image was not built from the project, and may carry none.
func TestDeployPrebuiltImageToAnImageDeploymentWarns(t *testing.T) {
	for _, show := range []bool{true, false} {
		t.Run(map[bool]string{true: "warnings on", false: "warnings off"}[show], func(t *testing.T) {
			withManifestEngine(t, "3.1-5")
			client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
			withShowWarnings(t, show)
			var warnings []string
			res, err := DeployManifestImage(client, prebuiltFor(t, "my-image:v7", &warnings))
			require.NoError(t, err)
			assert.True(t, res.DagsInImage)
			if !show {
				assert.Empty(t, warnings)
				return
			}
			assert.Equal(t, []string{"this Deployment runs the Dags inside the image my-image:v7; the dags folder is not uploaded. An image astro package built without a dockerfile declared under [tool.astro] contains none."}, warnings)
		})
	}
}

// A BYO registry takes a prebuilt image under its own tag, as Astro CLI 1.x
// pushed it, and the image by the mutation, with the runtime and Airflow
// its labels carry and the deploy's description. An image named without a
// tag takes Houston's scheme.
func TestDeployPrebuiltImageToABYORegistry(t *testing.T) {
	for _, tc := range []struct{ image, want string }{
		{"my-image:v7", "byo.example.com/team/repo:v7"},
		{"my-image", "byo.example.com/team/repo:my-release-deploy-"},
	} {
		t.Run(tc.image, func(t *testing.T) {
			cmd := withManifestEngine(t, "3.1-5")
			client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", byo: true}).client(t)
			client.On("UpdateDeploymentImage", mock.MatchedBy(func(r houston.UpdateDeploymentImageRequest) bool {
				return r.ReleaseName == "my-release" && strings.HasPrefix(r.Image, tc.want) &&
					r.RuntimeVersion == "3.1-5" && r.AirflowVersion == "3.1.1" && r.DeployRevisionDescription == "ship it"
			})).Return(nil, nil).Once()
			var warnings []string
			res, err := DeployManifestImage(client, prebuiltFor(t, tc.image, &warnings))
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(res.Image, tc.want), res.Image)
			assert.True(t, cmd.called("docker push "+tc.want), "%v", cmd.calls)
			assert.Empty(t, cmd.pushAuth, "no credential of the deploy's")
			client.AssertExpectations(t)
		})
	}
}

// Below Houston 1.0 the registry is registry.<domain>.
func TestDeployPrebuiltImageBelow1(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "0.37.0"}).client(t)
	var warnings []string
	res, err := DeployManifestImage(client, prebuiltFor(t, "my-image:v7", &warnings))
	require.NoError(t, err)
	c, err := config.GetCurrentContext()
	require.NoError(t, err)
	assert.Equal(t, "registry."+c.Domain+"/my-release/airflow:deploy-7", res.Image)
	assert.True(t, cmd.called("docker push registry."+c.Domain+"/my-release/airflow:deploy-7"), "%v", cmd.calls)
}

// A prebuilt image is held to the rules a built one is, on its label, before
// anything is pushed: a downgrade, Airflow 3 where it is not enabled, and a
// runtime newer than the newest listed are refused, naming the image to
// rebuild; an unlisted runtime no newer than the newest deploys, warned of;
// and an image with no runtime label is not an Astro Runtime image.
func TestDeployPrebuiltImageRuntimeRules(t *testing.T) {
	for _, tc := range []struct {
		name, label, current string
		listed               houston.RuntimeReleases
		wantErr, wantWarning string
	}{
		{name: "a downgrade", label: "3.0-9", current: "3.1-5", listed: houstonLists, wantErr: "cannot deploy Astro Runtime 3.0-9: it is a downgrade from the deployment's current 3.1-5; to deploy, rebuild my-image:v7 FROM Astro Runtime 3.1-5 or newer"},
		{name: "Airflow 3 not enabled", label: "3.1-5", current: "12.1.1", listed: airflow2Only, wantErr: "cannot deploy Astro Runtime 3.1-5: Airflow 3 is not enabled for this Deployment"},
		{name: "newer than listed", label: "3.2-1", current: "3.0-9", listed: houstonLists, wantErr: "cannot deploy Astro Runtime 3.2-1: it is newer than the newest the platform lists for this Deployment, 3.1-5"},
		{name: "no runtime label", label: "", current: "3.0-9", listed: houstonLists, wantErr: `image "my-image:v7" is not based on Astro Runtime`},
		{name: "unlisted", label: "3.1-4", current: "3.0-9", listed: houstonLists, wantWarning: "Astro Runtime 3.1-4 is not one the platform lists for this Deployment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := withManifestEngine(t, tc.label)
			client := (&apcTarget{dagType: houston.ImageDeploymentType, current: tc.current, listed: tc.listed, version: "1.0.0"}).client(t)
			withShowWarnings(t, false) // only the runtime's warnings
			var warnings []string
			_, err := DeployManifestImage(client, prebuiltFor(t, "my-image:v7", &warnings))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.False(t, cmd.called(" push "), "nothing is pushed: %v", cmd.calls)
				return
			}
			require.NoError(t, err)
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0], tc.wantWarning)
			assert.True(t, cmd.called(" push "))
		})
	}
}

// A prebuilt image is never built, so the engine is needed only to read and
// push it: one that cannot be reached fails the deploy before anything is
// pushed.
func TestDeployPrebuiltImageWithoutAnEngine(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := prebuiltFor(t, "my-image:v7", &warnings)
	in.Engine = func() (string, []string, error) { return "", nil, errSomeContainerIssue }
	_, err := DeployManifestImage(client, in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "an image deploy needs Docker")
	assert.Empty(t, cmd.calls)
}

// From outside any project the image ships alone, with no DAGs: the deploy
// says what that leaves each kind of Deployment. An image Deployment runs
// the image's DAGs (said unless apc.show_warnings is off, and not refused,
// as --image is from a project); one that takes uploads keeps the DAGs it
// had, which is said whatever apc.show_warnings is; one whose DAG source
// could not be placed may; and a git-sync one is as it always is.
func TestDeployPrebuiltImageAlone(t *testing.T) {
	for _, tc := range []struct {
		name, dagType string
		show          bool
		want          []string
	}{
		{name: "an image Deployment", dagType: houston.ImageDeploymentType, show: true, want: []string{"this Deployment runs the Dags inside the image my-image:v7; the dags folder is not uploaded. An image astro package built without a dockerfile declared under [tool.astro] contains none."}},
		{name: "an image Deployment, warnings off", dagType: houston.ImageDeploymentType},
		{name: "a dag_deploy Deployment, warnings off", dagType: houston.DagOnlyDeploymentType, want: []string{"Dags were not updated: this deploy ran outside a pyproject.toml project, so it shipped the image alone, and the Deployment keeps the Dags it had. To upload them, run astro deploy dep --dags from the project directory."}},
		{name: "a Deployment not placed", dagType: "nfs", show: true, want: []string{"this deploy ran outside a pyproject.toml project, so it shipped the image alone. If this Deployment takes Dag uploads, it keeps the Dags it had: to upload them, run astro deploy dep --dags from the project directory."}},
		{name: "a git-sync Deployment", dagType: houston.GitSyncDeploymentType, show: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := withManifestEngine(t, "3.1-5")
			var uploads int
			client := (&apcTarget{dagType: tc.dagType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: dagServer(t, &uploads)}).client(t)
			withShowWarnings(t, tc.show)
			var warnings []string
			res, err := DeployManifestImage(client, imageAlone(t, "my-image:v7", &warnings))
			require.NoError(t, err)
			assert.Equal(t, tc.want, warnings)
			assert.Zero(t, uploads, "no DAGs go with an image deployed alone")
			assert.False(t, res.DagsInImage)
			assert.True(t, cmd.called(" push "))
		})
	}
}

// A prebuilt image was not built from the project, so the project's
// .dockerignore says nothing of the DAGs it carries: one that drops every
// DAG file does not refuse it, as it refuses a build to an image Deployment.
func TestDeployPrebuiltImageIsNotHeldToTheProjectsDockerignore(t *testing.T) {
	cmd := withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := prebuiltFor(t, "my-image:v7", &warnings)
	require.NoError(t, os.WriteFile(filepath.Join(in.Build.ProjectDir, ".dockerignore"), []byte("dags/\n"), 0o600))
	_, err := DeployManifestImage(client, in)
	require.NoError(t, err)
	assert.True(t, cmd.called(" push "))
}

// A BYO registry takes every image by updateDeploymentImage, so a platform
// that has not turned it on is told the setting, for a prebuilt image's
// deploy as for --remote.
func TestDeployPrebuiltImageToABYORegistryWithTheMutationOff(t *testing.T) {
	withManifestEngine(t, "3.1-5")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "2.1.0", byo: true}).client(t)
	houstonSays := errors.New("Update deployment image endpoint is disabled. It can be enabled via configuration change.")
	client.On("UpdateDeploymentImage", mock.Anything).Return(nil, houstonSays).Once()
	var warnings []string
	_, err := DeployManifestImage(client, prebuiltFor(t, "my-image:v7", &warnings))
	require.ErrorIs(t, err, houstonSays)
	assert.Contains(t, err.Error(), "setting deployments.deploymentImagesRegistry.updateDeploymentImageEndpoint.enabled to true")
	assert.Contains(t, err.Error(), "a registry of the customer's own takes every image through it")
}
