package deploy

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// An interrupt during the Dags upload ends the deploy at once with the
// cancellation, and the finalize request never goes out: the Deployment is
// left as it was.
func TestDeployManifestDagsInterruptedDuringUploadStopsBeforeFinalize(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeployment(client, true, false)
	mockCreateDagDeploy(client, "https://upload-url")

	ctx, cancel := context.WithCancel(context.Background())
	uploading := make(chan struct{})
	orig := azureUploader
	t.Cleanup(func() { azureUploader = orig })
	azureUploader = func(ctx context.Context, _ string, _ io.Reader) (string, error) {
		close(uploading)
		<-ctx.Done()
		return "", ctx.Err()
	}
	go func() { <-uploading; cancel() }()

	done := make(chan error, 1)
	go func() {
		_, err := DeployManifestDags(ManifestDagDeployInput{
			Context:      ctx,
			ProjectDir:   manifestProjectDir(t),
			DeploymentID: "test-deployment-id",
		}, client)
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the deploy kept uploading after the interrupt")
	}
	client.AssertNotCalled(t, "FinalizeDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// The run's context reaches the create and finalize requests, which is what
// lets the shared http layer cancel them and word an interrupted write.
func TestDeployManifestDagsPassesItsContextToTheRequests(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "run")
	carries := mock.MatchedBy(func(c context.Context) bool { return c.Value(key{}) == "run" })

	client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockManifestDeployment(client, true, false)
	client.On("CreateDeployWithResponse", carries, mock.Anything, mock.Anything, mock.Anything).Return(createdDeploy("https://upload-url"), nil)
	client.On("FinalizeDeployWithResponse", carries, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(finalizedDeploy(), nil)

	orig := azureUploader
	t.Cleanup(func() { azureUploader = orig })
	azureUploader = func(c context.Context, _ string, _ io.Reader) (string, error) {
		assert.Equal(t, "run", c.Value(key{}))
		return "tarball-v1", nil
	}

	_, err := DeployManifestDags(ManifestDagDeployInput{
		Context:      ctx,
		ProjectDir:   manifestProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.NoError(t, err)
	client.AssertExpectations(t)
}
