package deploy

import (
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// registryImage is the deploy of an image already in a registry carrying
// runtime, from a project's root (IncludeDags), the way --image-name
// --remote --runtime-version runs it. It has no container engine: nothing
// may need one.
func registryImage(t *testing.T, runtime string, warnings *[]string) *ManifestImage {
	t.Helper()
	in := manifestImageFor(t, &imagebuild.ManifestBuild{}, warnings)
	in.ImageName, in.InRegistry, in.RuntimeVersion = "quay.example.com/team/airflow:v9", true, runtime
	in.Engine = func() (string, []string, error) { return "", nil, errors.New("no container engine may be asked for") }
	return in
}

// pointsAt expects the updateDeploymentImage mutation for the registry
// image, at runtime, with the deploy's description, answered with err.
func pointsAt(client *mock.Mock, runtime string, err error) {
	client.On("UpdateDeploymentImage", houston.UpdateDeploymentImageRequest{
		ReleaseName: "my-release", Image: "quay.example.com/team/airflow:v9", RuntimeVersion: runtime, DeployRevisionDescription: "ship it",
	}).Return(nil, err).Once()
}

// An image already in a registry is neither pulled, built nor pushed: the
// mutation points the Deployment at it, with the runtime given and the
// deploy's description, and the result names it.
func TestDeployRegistryImage(t *testing.T) {
	cmd := withManifestEngine(t, "never read")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	pointsAt(&client.Mock, "3.1-5", nil)
	withShowWarnings(t, false)
	var warnings []string
	res, err := DeployManifestImage(client, registryImage(t, "3.1-5", &warnings))
	require.NoError(t, err)
	assert.Equal(t, ManifestImageResult{
		WorkspaceID:    "ws",
		RuntimeVersion: "3.1-5",
		Image:          "quay.example.com/team/airflow:v9",
		URL:            "https://deployments.example.com/my-release/airflow",
		DagsInImage:    true,
	}, res)
	assert.Empty(t, cmd.calls, "no container engine is used")
	client.AssertExpectations(t)
}

// --runtime-version is held to the rules a prebuilt image's label is, before
// the mutation: a downgrade is refused, as Houston refuses one, and nothing
// is sent; Airflow 3 where the platform lists none, a runtime newer than the
// newest listed, any other unlisted runtime and an Airflow 2 to 3 upgrade
// deploy, warned of.
func TestDeployRegistryImageRuntimeRules(t *testing.T) {
	for _, tc := range []struct {
		name, runtime, current string
		listed                 houston.RuntimeReleases
		wantErr, wantWarning   string
	}{
		{name: "a downgrade", runtime: "3.0-9", current: "3.1-5", listed: houstonLists, wantErr: "cannot deploy Astro Runtime 3.0-9: it is a downgrade from the deployment's current 3.1-5; to deploy, rebuild quay.example.com/team/airflow:v9 FROM Astro Runtime 3.1-5 or newer"},
		{name: "Airflow 3 to 2", runtime: "12.1.1", current: "3.0-9", listed: houstonLists, wantErr: "cannot deploy Astro Runtime 12.1.1: it is a downgrade from the deployment's current 3.0-9"},
		{name: "Airflow 3 not listed", runtime: "3.1-5", current: "12.1.1", listed: airflow2Only, wantWarning: "Astro Runtime 3.1-5 is Airflow 3, and the platform lists no Airflow 3 build for this Deployment (it lists 12.1.1), so Airflow 3 may not be enabled for it. It deploys anyway"},
		{name: "newer than listed", runtime: "3.2-1", current: "3.0-9", listed: houstonLists, wantWarning: "Astro Runtime 3.2-1 is newer than the newest the platform lists for this Deployment, 3.1-5 (it lists 3.1-5, 3.0-9, 12.1.1). It deploys anyway"},
		{name: "unlisted", runtime: "3.1-4", current: "3.0-9", listed: houstonLists, wantWarning: "Astro Runtime 3.1-4 is not one the platform lists for this Deployment"},
		{name: "Airflow 2 to 3", runtime: "3.1-5", current: "12.1.1", listed: houstonLists, wantWarning: "this deploy upgrades Deployment dep from Airflow 2 to Airflow 3 (Astro Runtime 12.1.1 to 3.1-5)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withManifestEngine(t, "never read")
			client := (&apcTarget{dagType: houston.ImageDeploymentType, current: tc.current, listed: tc.listed, version: "1.0.0"}).client(t)
			withShowWarnings(t, false) // only the runtime's warnings
			var warnings []string
			if tc.wantErr != "" {
				_, err := DeployManifestImage(client, registryImage(t, tc.runtime, &warnings))
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				client.AssertNotCalled(t, "UpdateDeploymentImage", mock.Anything)
				return
			}
			pointsAt(&client.Mock, tc.runtime, nil)
			res, err := DeployManifestImage(client, registryImage(t, tc.runtime, &warnings))
			require.NoError(t, err)
			assert.Equal(t, tc.runtime, res.RuntimeVersion)
			require.NotEmpty(t, warnings)
			assert.Contains(t, warnings[0], tc.wantWarning)
			client.AssertExpectations(t)
		})
	}
}

// Houston takes the image's runtime, and an image in a registry has no label
// here to read it from: with none given, nothing is asked of Houston.
func TestDeployRegistryImageNeedsItsRuntime(t *testing.T) {
	withManifestEngine(t, "never read")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	_, err := DeployManifestImage(client, registryImage(t, "", &warnings))
	require.ErrorIs(t, err, ErrRuntimeVersionNotPassedForRemoteImage)
	assert.Empty(t, client.Calls)
}

// A platform that has not turned updateDeploymentImage on only says it is
// disabled: the deploy names the setting a platform admin turns it on with,
// as the platform's Houston spells it, and the way to deploy meanwhile.
func TestDeployRegistryImageOnAPlatformWithTheMutationOff(t *testing.T) {
	houstonSays := errors.New("Update deployment image endpoint is disabled. It can be enabled via configuration change.")
	for _, tc := range []struct{ version, setting string }{
		{"2.1.0", "deployments.deploymentImagesRegistry.updateDeploymentImageEndpoint.enabled"},
		{"1.0.0", "deployments.enableUpdateDeploymentImageEndpoint"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			withManifestEngine(t, "never read")
			client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: tc.version}).client(t)
			pointsAt(&client.Mock, "3.1-5", houstonSays)
			var warnings []string
			_, err := DeployManifestImage(client, registryImage(t, "3.1-5", &warnings))
			require.ErrorIs(t, err, houstonSays)
			assert.Contains(t, err.Error(), "A platform admin turns it on by setting "+tc.setting+" to true under astronomer.houston.config")
			assert.Contains(t, err.Error(), "astro deploy --image-name <image>, without --remote")
		})
	}
}

// Any other error from the mutation is the deploy's as Houston gave it.
func TestDeployRegistryImageMutationError(t *testing.T) {
	withManifestEngine(t, "never read")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	houstonSays := errors.New("Invalid image param")
	pointsAt(&client.Mock, "3.1-5", houstonSays)
	var warnings []string
	_, err := DeployManifestImage(client, registryImage(t, "3.1-5", &warnings))
	assert.Equal(t, houstonSays, err)
}

// From a project's root, the project's DAGs follow the image to a
// dag_deploy Deployment, as they follow a pushed one.
func TestDeployRegistryImageUploadsTheProjectsDags(t *testing.T) {
	withManifestEngine(t, "never read")
	var uploads int
	client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: dagServer(t, &uploads)}).client(t)
	pointsAt(&client.Mock, "3.1-5", nil)
	var warnings []string
	res, err := DeployManifestImage(client, registryImage(t, "3.1-5", &warnings))
	require.NoError(t, err)
	assert.True(t, res.DagsUploaded)
	assert.Equal(t, 1, uploads)
	assert.Empty(t, warnings)
}

// The cluster config is read only to place the Deployment's DAGs, so one
// that cannot be read does not stop the deploy, as it never did: a
// dag_deploy Deployment is then not placed, and an upload that may have
// been due is said not to have happened, from a project's root and from
// outside any project alike.
func TestDeployRegistryImageWithAnUnreadClusterConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alone bool
		want  string
	}{
		{"from a project's root", false, "Dags were NOT updated: whether this Deployment takes Dag uploads could not be read"},
		{"from outside any project", true, "this deploy ran outside a pyproject.toml project, so it shipped the image alone. If this Deployment takes Dag uploads"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withManifestEngine(t, "never read")
			client := (&apcTarget{dagType: houston.DagOnlyDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0", dagsHook: "https://dags.invalid"}).client(t)
			client.ExpectedCalls = slices.DeleteFunc(client.ExpectedCalls, func(c *mock.Call) bool { return c.Method == "GetAppConfig" })
			client.On("GetAppConfig", mock.Anything).Return(nil, errors.New("app config unreachable"))
			pointsAt(&client.Mock, "3.1-5", nil)
			var warnings []string
			in := registryImage(t, "3.1-5", &warnings)
			if tc.alone {
				in.IncludeDags, in.ImageAlone = false, true
			}
			res, err := DeployManifestImage(client, in)
			require.NoError(t, err)
			assert.Equal(t, "quay.example.com/team/airflow:v9", res.Image)
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0], tc.want)
			client.AssertExpectations(t)
		})
	}
}

// --image to an image Deployment would leave it no DAGs: refused before the
// mutation, as for an image pushed.
func TestDeployRegistryImageRefusesImageOnlyToAnImageDeployment(t *testing.T) {
	withManifestEngine(t, "never read")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := registryImage(t, "3.1-5", &warnings)
	in.IncludeDags = false
	_, err := DeployManifestImage(client, in)
	require.ErrorIs(t, err, ErrDeploymentTypeIncorrectForImageOnly)
	client.AssertNotCalled(t, "UpdateDeploymentImage", mock.Anything)
}

// The image is pointed at whatever registry the platform takes images from:
// a BYO cluster's needs no registry host of the CLI's, which only a push
// would use.
func TestDeployRegistryImageOnABYOClusterWithNoHost(t *testing.T) {
	withManifestEngine(t, "never read")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	client.ExpectedCalls = slices.DeleteFunc(client.ExpectedCalls, func(c *mock.Call) bool { return c.Method == "GetAppConfig" })
	client.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{BYORegistryEnabled: true}}, nil)
	pointsAt(&client.Mock, "3.1-5", nil)
	withShowWarnings(t, false)
	var warnings []string
	_, err := DeployManifestImage(client, registryImage(t, "3.1-5", &warnings))
	require.NoError(t, err)
	client.AssertExpectations(t)
}

// A Deployment that is not in the workspace is refused before anything is
// sent, naming both.
func TestDeployRegistryImageRefusesADeploymentOutsideTheWorkspace(t *testing.T) {
	withManifestEngine(t, "never read")
	client := (&apcTarget{dagType: houston.ImageDeploymentType, current: "3.1-5", listed: houstonLists, version: "1.0.0"}).client(t)
	var warnings []string
	in := registryImage(t, "3.1-5", &warnings)
	in.DeploymentID = "elsewhere"
	_, err := DeployManifestImage(client, in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Deployment elsewhere in workspace ws")
	client.AssertNotCalled(t, "UpdateDeploymentImage", mock.Anything)
}
