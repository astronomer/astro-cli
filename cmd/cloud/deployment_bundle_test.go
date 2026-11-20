package cloud

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
)

func TestDeploymentBundleAliasCmd(t *testing.T) {
	t.Run("is hidden from the deployment help", func(t *testing.T) {
		cmd, _, err := newDeploymentRootCmd(new(bytes.Buffer)).Find([]string{"bundle"})
		assert.NoError(t, err)
		assert.Equal(t, "bundle", cmd.Name())
		assert.False(t, cmd.IsAvailableCommand())
	})

	t.Run("lists bundles and says the command moved", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}},
			},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "list", "--deployment-id", "test-id-1", "--json")
		assert.NoError(t, err)
		assert.Contains(t, out, `Command "list" is deprecated, use 'astro bundle list' instead`)
		assert.Contains(t, out, "bundle-1")
		mockAlpha.AssertExpectations(t)
	})

	t.Run("keeps rm as an alias of delete", func(t *testing.T) {
		mockAlpha := setupBundleCmdMocks(t)
		mockAlpha.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		out, err := execDeploymentCmd("bundle", "rm", "bundle-1", "--deployment-id", "test-id-1", "--force")
		assert.NoError(t, err)
		assert.Contains(t, out, "Deleted bundle bundle-1")
		mockAlpha.AssertExpectations(t)
	})
}
