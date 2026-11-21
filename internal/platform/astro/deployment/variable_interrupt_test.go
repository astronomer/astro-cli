package deployment

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt that lands while the variables are being saved does not turn a
// saved variable into a failure: the read that reports them is not canceled
// with the run.
func TestVariableModifyReportsWhatWasSavedAfterAnInterrupt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	MockResponseInit()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Maybe()
	client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Maybe()
	client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Maybe()
	updated := astrov1.UpdateDeploymentResponse{JSON200: deploymentResponse.JSON200, HTTPResponse: &http.Response{StatusCode: http.StatusOK}}
	client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(context.Context, string, string, astrov1.UpdateDeploymentJSONRequestBody, ...astrov1.RequestEditorFn) (*astrov1.UpdateDeploymentResponse, error) {
			cancel()
			return &updated, nil
		}).Once()
	// A read made under the interrupted run fails as a real client's does.
	client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(
		func(c context.Context, _, _ string, _ ...astrov1.RequestEditorFn) (*astrov1.GetDeploymentResponse, error) {
			if err := c.Err(); err != nil {
				return nil, err
			}
			return &deploymentResponse, nil
		}).Maybe()

	res, err := VariableModify(ctx, "test-id-1", "test-key-9", "test-value-9", ws, "", "", []string{}, false, false, false, client)
	require.NoError(t, err)
	assert.Equal(t, []VariableOutcome{{Kind: VariableCreated, Key: "test-key-9"}}, res.Outcomes)
}
