package deploy

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func bundleOn(id, name string, dag bool, typ astrov1alpha1.DeploymentBundleType) astrov1alpha1.DeploymentBundle {
	b := astrov1alpha1.DeploymentBundle{Id: id, IsDagBundle: &dag, Type: typ}
	if dag {
		b.Name = &name
	}
	return b
}

func bundleList(bundles ...astrov1alpha1.DeploymentBundle) *astrov1alpha1.ListBundlesResponse {
	return &astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1alpha1.BundlesPaginated{Bundles: bundles, TotalCount: len(bundles)},
	}
}

// bundleClient is a v1alpha1 mock whose bundle list is bundles and whose
// deploy create succeeds, recording the request it was sent.
func bundleClient(bundles ...astrov1alpha1.DeploymentBundle) (*astrov1alpha1_mocks.ClientWithResponsesInterface, *astrov1alpha1.CreateDeployRequest) {
	client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(bundleList(bundles...), nil)
	sent := &astrov1alpha1.CreateDeployRequest{}
	uploadURL := "https://upload-url"
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { *sent = args.Get(3).(astrov1alpha1.CreateDeployRequest) }).
		Return(&astrov1alpha1.CreateDeployResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.Deploy{Id: "test-deploy-id", DagsUploadUrl: &uploadURL, ImageRepository: "repo", ImageTag: "tag"},
		}, nil)
	return client, sent
}

// A Dags-only deploy to a named bundle creates the deploy through v1alpha1
// with the bundle's name and the DAG type, never through v1, then uploads and
// finalizes on v1 as any Dags deploy does.
func TestDeployManifestDags_NamedDagBundle(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	v1 := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeployment(v1, true, false)
	mockFinalizeDeploy(v1)
	alpha, sent := bundleClient(bundleOn("b1", "nightly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY))

	azureUploader = func(_ context.Context, sasLink string, file io.Reader) (string, error) {
		assert.Equal(t, "https://upload-url", sasLink)
		return "tarball-v1", nil
	}

	res, err := DeployManifestDags(ManifestDagDeployInput{
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
		Description:  "to a bundle",
		DagBundle:    &DagBundle{Name: "nightly", Client: alpha},
	}, v1)
	require.NoError(t, err)

	assert.Equal(t, "tarball-v1", res.DagTarballVersion)
	require.NotNil(t, sent.DagBundleName)
	assert.Equal(t, "nightly", *sent.DagBundleName)
	assert.Equal(t, astrov1alpha1.CreateDeployRequestTypeDAG, sent.Type)
	require.NotNil(t, sent.Description)
	assert.Equal(t, "to a bundle", *sent.Description)
	v1.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	v1.AssertCalled(t, "FinalizeDeployWithResponse", mock.Anything, "test-org-id", "test-deployment-id", "test-deploy-id", mock.Anything)
}

// With no bundle named, the deploy goes through v1 as it always did and
// v1alpha1 is never asked.
func TestDeployManifestDags_NoDagBundleStaysOnV1(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	v1 := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeployment(v1, true, false)
	mockCreateDagDeploy(v1, "https://upload-url")
	mockFinalizeDeploy(v1)
	alpha := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	azureUploader = func(context.Context, string, io.Reader) (string, error) { return "tarball-v1", nil }

	_, err := DeployManifestDags(ManifestDagDeployInput{
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
		DagBundle:    &DagBundle{Client: alpha},
	}, v1)
	require.NoError(t, err)
	alpha.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	alpha.AssertNotCalled(t, "ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The refusals come before the deploy is created, so nothing is started.
func TestDeployManifestDags_DagBundleRefusals(t *testing.T) {
	deleting := true
	pending := bundleOn("old", "nightly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY)
	pending.DeletionIsPending = &deleting

	for name, tc := range map[string]struct {
		bundles []astrov1alpha1.DeploymentBundle
		dagOn   bool
		want    string
	}{
		"no bundle by that name":     {nil, true, `no Dag bundle named "nightly" on Deployment test-deployment-id. Create it with 'astro deployment bundle create --name nightly'`},
		"only another bundle's name": {[]astrov1alpha1.DeploymentBundle{bundleOn("b2", "weekly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY)}, true, `no Dag bundle named "nightly"`},
		"a non-Dag bundle":           {[]astrov1alpha1.DeploymentBundle{bundleOn("b3", "", false, astrov1alpha1.DeploymentBundleTypeDEPLOY)}, true, `no Dag bundle named "nightly"`},
		"one being deleted":          {[]astrov1alpha1.DeploymentBundle{pending}, true, `no Dag bundle named "nightly"`},
		"a file bundle":              {[]astrov1alpha1.DeploymentBundle{bundleOn("b4", "nightly", true, astrov1alpha1.DeploymentBundleTypeFILE)}, true, "is a file bundle, whose files are written by 'astro bundle files'"},
		"dag deploys off":            {[]astrov1alpha1.DeploymentBundle{bundleOn("b1", "nightly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY)}, false, "has Dag deploys off and takes its Dags from the image"},
	} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			v1 := new(astrov1_mocks.ClientWithResponsesInterface)
			mockManifestDeployment(v1, tc.dagOn, false)
			alpha, _ := bundleClient(tc.bundles...)

			_, err := DeployManifestDags(ManifestDagDeployInput{
				ProjectDir:   manifestProjectDir(t),
				DeploymentID: "test-deployment-id",
				DagBundle:    &DagBundle{Name: "nightly", Client: alpha},
			}, v1)
			require.Error(t, err)
			if name == "dag deploys off" {
				// The Dags-only transport names its own enable message first.
				assert.Contains(t, err.Error(), "Dag-only deploys are not enabled")
			} else {
				assert.Contains(t, err.Error(), tc.want)
			}
			alpha.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			v1.AssertNotCalled(t, "CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// The newest of two bundles under one name is the one a create just made.
func TestFindDagBundleViaCheck(t *testing.T) {
	old := bundleOn("old", "nightly", true, astrov1alpha1.DeploymentBundleTypeFILE)
	old.CreatedAt = time.Now().Add(-time.Hour)
	fresh := bundleOn("new", "nightly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY)
	fresh.CreatedAt = time.Now()
	alpha, _ := bundleClient(old, fresh)

	dep := &astrov1.Deployment{Id: "dep", OrganizationId: "org", IsDagDeployEnabled: true}
	require.NoError(t, checkDagBundle(context.Background(), dep, DagBundle{Name: "nightly", Client: alpha}))
}

// An image-and-Dags deploy is a v1alpha1 IMAGE deploy, which the server
// expands to image and Dags.
func TestCreateDeployInDagBundleTypes(t *testing.T) {
	for name, tc := range map[string]struct {
		in   astrov1.CreateDeployRequestType
		want astrov1alpha1.CreateDeployRequestType
	}{
		"dags only":      {astrov1.CreateDeployRequestTypeDAGONLY, astrov1alpha1.CreateDeployRequestTypeDAG},
		"image and Dags": {astrov1.CreateDeployRequestTypeIMAGEANDDAG, astrov1alpha1.CreateDeployRequestTypeIMAGE},
	} {
		t.Run(name, func(t *testing.T) {
			alpha, sent := bundleClient()
			sha := "abc123"
			created, err := createDeployInDagBundle(context.Background(), "org", "dep", astrov1.CreateDeployRequest{
				Type: tc.in,
				Git:  &astrov1.CreateDeployGitRequest{CommitSha: sha, Provider: astrov1.CreateDeployGitRequestProviderGITHUB},
			}, DagBundle{Name: "nightly", Client: alpha})
			require.NoError(t, err)
			assert.Equal(t, tc.want, sent.Type)
			require.NotNil(t, sent.Git)
			assert.Equal(t, sha, sent.Git.CommitSha)
			assert.Equal(t, "test-deploy-id", created.Id)
			assert.Equal(t, "repo", created.ImageRepository)
			assert.Equal(t, "tag", created.ImageTag)
			require.NotNil(t, created.DagsUploadUrl)
		})
	}
}
