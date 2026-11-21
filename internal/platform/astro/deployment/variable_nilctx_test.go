package deployment

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A run with no context at all (a command called without ExecuteContext) saves
// and reports its variables: the read after the update does not panic on it.
func TestVariableModifyAcceptsANilContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	MockResponseInit()
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Maybe()
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Maybe()
	client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Maybe()
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Maybe()
	updated := astrov1.UpdateDeploymentResponse{JSON200: deploymentResponse.JSON200, HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
	client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&updated, nil).Once()

	require.NotPanics(t, func() {
		//nolint:staticcheck // a nil context is the case under test
		res, err := VariableModify(nil, "test-id-1", "test-key-9", "test-value-9", ws, "", "", []string{}, false, false, false, client)
		require.NoError(t, err)
		assert.Equal(t, []VariableOutcome{{Kind: VariableCreated, Key: "test-key-9"}}, res.Outcomes)
	})
}
