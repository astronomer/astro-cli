package deploy

import (
	"context"
	"errors"
	"fmt"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// dagBundleNeedsDagDeployMsg refuses a named Dag bundle on a Deployment with
// Dag deploys off, which takes its Dags from the image and has no bundles to
// target.
const dagBundleNeedsDagDeployMsg = "--dag-bundle ships the Dags to a named Dag bundle, but Deployment %[1]s has Dag deploys off and takes its Dags from the image. Enable Dag deploys with 'astro deployment update %[1]s --dag-deploy enable', or deploy without --dag-bundle"

// DagBundle names the Dag bundle a deploy ships its Dags to instead of the
// Deployment's default one, with the v1alpha1 client that reaches it: only
// that API takes a named bundle on a deploy.
type DagBundle struct {
	Name   string
	Client astrov1alpha1.APIClient
	// Link is the manifest link whose dag-bundle named it, "" when
	// --dag-bundle did, so a refusal can say where the name was set.
	Link string
}

// setting is where the name was set: the flag, or the link's key in
// pyproject.toml.
func (b DagBundle) setting() string {
	if b.Link == "" {
		return "--dag-bundle"
	}
	return fmt.Sprintf("dag-bundle = %q under [tool.astro.deployments.%s] in pyproject.toml", b.Name, b.Link)
}

// explain adds, to a refusal about the bundle, where a link's setting named
// it, since the person passed no flag. A flag-named bundle needs no more.
func (b DagBundle) explain(err error) error {
	if b.Link == "" {
		return err
	}
	return fmt.Errorf("%w (the bundle comes from %s: fix the name, remove the key, or pass --dag-bundle to name another)", err, b.setting())
}

// checkDagBundle holds a deploy to the named Dag bundle to what it needs
// before anything is built or uploaded: a Deployment that takes Dag deploys,
// and a bundle on it that is a Dag bundle written by deploys (not by
// 'astro bundle files').
func checkDagBundle(ctx context.Context, dep *astrov1.Deployment, bundle DagBundle) error {
	if !dep.IsDagDeployEnabled {
		if bundle.Link != "" {
			return fmt.Errorf("Deployment %[1]s has Dag deploys off and takes its Dags from the image, but pyproject.toml sets %[2]s, so this deploy would ship its Dags to a named Dag bundle. Remove the key, or enable Dag deploys with 'astro deployment update %[1]s --dag-deploy enable'", dep.Id, bundle.setting())
		}
		return fmt.Errorf(dagBundleNeedsDagDeployMsg, dep.Id)
	}
	if bundle.Client == nil {
		return errors.New("no client to check the Dag bundle with")
	}
	found, err := deployment.FindDagBundle(ctx, dep.OrganizationId, dep.Id, bundle.Name, bundle.Client)
	if err != nil {
		return bundle.explain(err)
	}
	if found.Type == astrov1alpha1.DeploymentBundleTypeFILE {
		return bundle.explain(fmt.Errorf("bundle %q on Deployment %s is a file bundle, whose files are written by 'astro bundle files'. A deploy cannot write to it: name a Dag bundle created for deploys, or create one with 'astro deployment bundle create --name'", bundle.Name, dep.Id))
	}
	return nil
}

// createDeployInDagBundle creates a deploy that ships its Dags to the named
// Dag bundle, through the v1alpha1 deploy API, the only one that takes
// dagBundleName. The rest of the deploy (the upload and the finalize) stays on
// v1 and works on the same deploy, so the created deploy comes back in v1's
// shape, with the fields those steps read.
//
// The v1alpha1 type vocabulary is simpler than v1's: the server expands IMAGE
// to IMAGE_AND_DAG, so a deploy that ships an image and Dags is IMAGE, and a
// Dags-only deploy is DAG.
func createDeployInDagBundle(ctx context.Context, organizationID, deploymentID string, request astrov1.CreateDeployRequest, bundle DagBundle) (*astrov1.Deploy, error) {
	deployType := astrov1alpha1.CreateDeployRequestTypeIMAGE
	if request.Type == astrov1.CreateDeployRequestTypeDAGONLY {
		deployType = astrov1alpha1.CreateDeployRequestTypeDAG
	}
	name := bundle.Name
	resp, err := bundle.Client.CreateDeployWithResponse(ctx, organizationID, deploymentID, astrov1alpha1.CreateDeployRequest{
		Type:          deployType,
		Description:   request.Description,
		DagBundleName: &name,
		Git:           toV1Alpha1GitRequest(request.Git),
	})
	if err != nil {
		return nil, err
	}
	if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errors.New("no deploy received from Astro")
	}
	created := resp.JSON200
	return &astrov1.Deploy{
		Id:              created.Id,
		ImageRepository: created.ImageRepository,
		ImageTag:        created.ImageTag,
		DagsUploadUrl:   created.DagsUploadUrl,
	}, nil
}

// checkCreatedImageDeploy holds the deploy an image deploy just created to
// what the rest of it needs: the repository and tag to push to, and, for a
// deploy that ships Dags to a named bundle, a Dags upload URL.
func checkCreatedImageDeploy(created *astrov1.Deploy, bundle *DagBundle, uploadsDags bool) error {
	if created.ImageRepository == "" || created.ImageTag == "" {
		return errors.New("no image repository or tag received from Astro")
	}
	return checkBundleDagsUpload(created, bundle, uploadsDags)
}

// checkBundleDagsUpload holds an image deploy that ships its Dags to a named
// bundle to getting a Dags upload URL back. The v1alpha1 IMAGE type is
// expected to expand to image and Dags, so a created deploy with no URL means
// the Dags would not reach the bundle: it fails here, before the image is
// pushed, rather than finishing and reporting a bundle the Dags never went to.
func checkBundleDagsUpload(created *astrov1.Deploy, bundle *DagBundle, uploadsDags bool) error {
	if bundle == nil || bundle.Name == "" || !uploadsDags {
		return nil
	}
	if created.DagsUploadUrl == nil || *created.DagsUploadUrl == "" {
		return fmt.Errorf("Astro created the deploy with no Dags upload URL, so the Dags cannot go to Dag bundle %q: nothing was pushed. Try the deploy again, or deploy the Dags alone with --dags", bundle.Name)
	}
	return nil
}

// createDeployFor creates the deploy the request describes: in the named Dag
// bundle when bundle is set, else on the Deployment's default one.
func createDeployFor(ctx context.Context, organizationID, deploymentID string, request astrov1.CreateDeployRequest, bundle *DagBundle, astroV1Client astrov1.APIClient) (*astrov1.Deploy, error) {
	if bundle != nil && bundle.Name != "" {
		return createDeployInDagBundle(ctx, organizationID, deploymentID, request, *bundle)
	}
	return createDeploy(ctx, organizationID, deploymentID, request, astroV1Client)
}

func toV1Alpha1GitRequest(git *astrov1.CreateDeployGitRequest) *astrov1alpha1.CreateDeployGitRequest {
	if git == nil {
		return nil
	}
	return &astrov1alpha1.CreateDeployGitRequest{
		Account:         git.Account,
		AuthorName:      git.AuthorName,
		AuthorUrl:       git.AuthorUrl,
		AuthorUsername:  git.AuthorUsername,
		BeforeCommitSha: git.BeforeCommitSha,
		Branch:          git.Branch,
		CommitSha:       git.CommitSha,
		CommitUrl:       git.CommitUrl,
		Path:            git.Path,
		Provider:        astrov1alpha1.CreateDeployGitRequestProvider(git.Provider),
		RemoteUrl:       git.RemoteUrl,
		Repo:            git.Repo,
	}
}
