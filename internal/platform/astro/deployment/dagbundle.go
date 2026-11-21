package deployment

import (
	httpContext "context"
	"fmt"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
)

// FindDagBundle finds the Dag bundle named name among a Deployment's bundles,
// for a deploy to target. A bundle whose deletion is pending is passed over:
// it is on its way out, and a bundle deleted and created again under the same
// name lists both until the first is gone. The API keeps names unique among
// the rest; were there more, the newest is taken, as the one a create just
// made.
//
// The error for a name no Dag bundle has says how to make one.
func FindDagBundle(ctx httpContext.Context, orgID, deploymentID, name string, client astrov1alpha1.APIClient) (*astrov1alpha1.DeploymentBundle, error) {
	bundles, err := listBundlesUpTo(ctx, orgID, deploymentID, 0, client)
	if err != nil {
		return nil, err
	}
	var found *astrov1alpha1.DeploymentBundle
	for i := range bundles {
		b := &bundles[i]
		if b.DeletionIsPending != nil && *b.DeletionIsPending {
			continue
		}
		if b.IsDagBundle == nil || !*b.IsDagBundle || b.Name == nil || *b.Name != name {
			continue
		}
		if found == nil || b.CreatedAt.After(found.CreatedAt) {
			found = b
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no Dag bundle named %q on Deployment %s. Create it with 'astro deployment bundle create --name %s', or list the Deployment's bundles with 'astro deployment bundle list'", name, deploymentID, name)
	}
	return found, nil
}
