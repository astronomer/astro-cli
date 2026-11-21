package deploy

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A refusal says where the bundle's name was set: nothing extra for the flag,
// and the link's key in pyproject.toml, with the way out, when the person
// passed no flag.
func TestCheckDagBundleNamesWhereTheNameWasSet(t *testing.T) {
	dep := func(dagOn bool) *astrov1.Deployment {
		return &astrov1.Deployment{Id: "dep", OrganizationId: "org", IsDagDeployEnabled: dagOn}
	}
	deploy := astrov1alpha1.DeploymentBundleTypeDEPLOY
	file := astrov1alpha1.DeploymentBundleTypeFILE
	const key = `dag-bundle = "nightly" under [tool.astro.deployments.prod] in pyproject.toml`

	for name, tc := range map[string]struct {
		dagOn    bool
		bundles  []astrov1alpha1.DeploymentBundle
		link     string
		contains []string
		excludes []string
	}{
		"dag deploys off, from the flag": {false, nil, "", []string{"--dag-bundle ships the Dags"}, []string{"pyproject.toml"}},
		"dag deploys off, from the link": {false, nil, "prod", []string{"pyproject.toml sets " + key, "Remove the key, or enable Dag deploys"}, []string{"--dag-bundle ships"}},
		"no such bundle, from the flag":  {true, nil, "", []string{`no Dag bundle named "nightly"`}, []string{"pyproject.toml"}},
		"no such bundle, from the link":  {true, nil, "prod", []string{`no Dag bundle named "nightly"`, "the bundle comes from " + key, "remove the key"}, nil},
		"a file bundle, from the flag":   {true, []astrov1alpha1.DeploymentBundle{bundleOn("b", "nightly", true, file)}, "", []string{"is a file bundle"}, []string{"pyproject.toml"}},
		"a file bundle, from the link":   {true, []astrov1alpha1.DeploymentBundle{bundleOn("b", "nightly", true, file)}, "prod", []string{"is a file bundle", "the bundle comes from " + key}, nil},
		"a good bundle, from the link":   {true, []astrov1alpha1.DeploymentBundle{bundleOn("b", "nightly", true, deploy)}, "prod", nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			alpha, _ := bundleClient(tc.bundles...)
			err := checkDagBundle(context.Background(), dep(tc.dagOn), DagBundle{Name: "nightly", Client: alpha, Link: tc.link})
			if tc.contains == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tc.contains {
				assert.Contains(t, err.Error(), want)
			}
			for _, not := range tc.excludes {
				assert.NotContains(t, err.Error(), not)
			}
		})
	}
}

// bundleDeployInput is a full deploy to a named bundle, the project's Dags
// uploaded beside the image.
func bundleDeployInput(t *testing.T, alpha astrov1alpha1.APIClient) ManifestImageDeployInput {
	t.Helper()
	return ManifestImageDeployInput{
		Build:        imagebuild.ManifestBuild{ProjectDir: manifestProjectDir(t), AirflowVersion: "3.1", Dependencies: []string{"pandas"}},
		DeploymentID: "test-deployment-id",
		IncludeDags:  true,
		DagBundle:    &DagBundle{Name: "nightly", Client: alpha},
	}
}

func goodBundle() astrov1alpha1.DeploymentBundle {
	return bundleOn("b1", "nightly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY)
}

// A full deploy to a named bundle creates the deploy on v1alpha1 as IMAGE,
// pushes the image, and uploads the Dags to the URL that create returned,
// then finalizes with their version on v1: the Dags reach the bundle.
func TestDeployManifestImage_NamedDagBundleUploadsTheDags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	v1 := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(v1, "3.1-2", true, false)
	mockDeploymentOptions(v1, "3.1-2")
	mockFinalizeDeploy(v1)
	alpha, sent := bundleClient(goodBundle())
	var uploadedTo string
	azureUploader = func(_ context.Context, sasLink string, _ io.Reader) (string, error) {
		uploadedTo = sasLink
		return "tarball-v1", nil
	}
	cmd := withImageSeams(t, "3.1-2")

	res, err := DeployManifestImage(bundleDeployInput(t, alpha), v1)
	require.NoError(t, err)

	assert.Equal(t, "https://upload-url", uploadedTo, "the Dags go to the URL the bundle's create returned")
	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
	assert.Equal(t, "tag", res.ImageTag)
	assert.Equal(t, astrov1alpha1.CreateDeployRequestTypeIMAGE, sent.Type)
	require.NotNil(t, sent.DagBundleName)
	assert.Equal(t, "nightly", *sent.DagBundleName)
	assert.True(t, hasImageCall(cmd.calls, "push"), "the image is pushed, got %v", cmd.calls)
	v1.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	v1.AssertCalled(t, "FinalizeDeployWithResponse", mock.Anything, "test-org-id", "test-deployment-id", "test-deploy-id", mock.Anything)
}

// A created deploy with no Dags upload URL means IMAGE did not expand to
// image and Dags. The deploy fails before the image is pushed or anything is
// finalized, rather than reporting a bundle the Dags never went to.
func TestDeployManifestImage_NamedDagBundleWithNoDagsUploadFails(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	v1 := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeploymentAt(v1, "3.1-2", true, false)
	mockDeploymentOptions(v1, "3.1-2")
	alpha := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	alpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(bundleList(goodBundle()), nil)
	alpha.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.CreateDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1alpha1.Deploy{Id: "test-deploy-id", ImageRepository: "repo", ImageTag: "tag"},
	}, nil)
	uploaded := false
	azureUploader = func(context.Context, string, io.Reader) (string, error) { uploaded = true; return "v", nil }
	cmd := withImageSeams(t, "3.1-2")

	_, err := DeployManifestImage(bundleDeployInput(t, alpha), v1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no Dags upload URL, so the Dags cannot go to Dag bundle "nightly"`)
	assert.False(t, uploaded)
	assert.False(t, hasImageCall(cmd.calls, "push"), "nothing is pushed, got %v", cmd.calls)
	v1.AssertNotCalled(t, "FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestCheckBundleDagsUpload(t *testing.T) {
	url, empty := "https://u", ""
	bundle := &DagBundle{Name: "nightly"}
	assert.Error(t, checkBundleDagsUpload(&astrov1.Deploy{}, bundle, true))
	assert.Error(t, checkBundleDagsUpload(&astrov1.Deploy{DagsUploadUrl: &empty}, bundle, true))
	assert.NoError(t, checkBundleDagsUpload(&astrov1.Deploy{DagsUploadUrl: &url}, bundle, true))
	// No bundle, or a deploy that uploads no Dags, asks nothing of the create.
	assert.NoError(t, checkBundleDagsUpload(&astrov1.Deploy{}, nil, true))
	assert.NoError(t, checkBundleDagsUpload(&astrov1.Deploy{}, &DagBundle{}, true))
	assert.NoError(t, checkBundleDagsUpload(&astrov1.Deploy{}, bundle, false))
}
