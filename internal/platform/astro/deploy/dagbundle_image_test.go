package deploy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An image deploy checks the bundle in checkDeployment, before the build: a
// Deployment with Dag deploys off, and a bundle that is not there, both end
// it there.
func TestCheckDeploymentRefusesABadDagBundle(t *testing.T) {
	good := bundleOn("b1", "nightly", true, astrov1alpha1.DeploymentBundleTypeDEPLOY)

	for name, tc := range map[string]struct {
		dagOn   bool
		bundles []astrov1alpha1.DeploymentBundle
		want    string
	}{
		"dag deploys off": {false, []astrov1alpha1.DeploymentBundle{good}, "--dag-bundle ships the Dags to a named Dag bundle, but Deployment test-deployment-id has Dag deploys off"},
		"missing bundle":  {true, nil, `no Dag bundle named "nightly"`},
	} {
		t.Run(name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			v1 := new(astrov1_mocks.ClientWithResponsesInterface)
			mockManifestDeployment(v1, tc.dagOn, false)
			alpha, _ := bundleClient(tc.bundles...)
			in := &ManifestImageDeployInput{
				DeploymentID: "test-deployment-id",
				IncludeDags:  true,
				DagBundle:    &DagBundle{Name: "nightly", Client: alpha},
			}
			c := config.Context{Organization: "test-org-id"}
			_, _, err := checkDeployment(context.Background(), &c, in, v1)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
