package deploy

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/httputil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const blockedByHibernation = `{"message":"Deploy is blocked because Deployment is in HIBERNATING state","statusCode":400}`

const wakeUpHint = "Astro Deployment test-deployment-id (\"test-deployment\") is hibernating, so it cannot take a deploy — wake it with `astro deployment wake-up test-deployment-id`"

func refuse(status int, body string) error {
	return httputil.NormalizeAPIError(&http.Response{StatusCode: status}, []byte(body))
}

func mockCreateDeployRefused(client *astrov1_mocks.ClientWithResponsesInterface, body string) {
	client.On("CreateDeployWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
		Body:         []byte(body),
	}, nil)
}

func TestExplainHibernating(t *testing.T) {
	awake := &astrov1.Deployment{Id: "test-deployment-id", Name: "test-deployment", Status: astrov1.DeploymentStatusHEALTHY}
	asleep := &astrov1.Deployment{Id: "test-deployment-id", Name: "test-deployment", Status: astrov1.DeploymentStatusHIBERNATING}
	cases := []struct {
		name      string
		status    int
		body      string
		dep       *astrov1.Deployment
		explains  bool
		keepsText bool
	}{
		{"the refusal names hibernation", http.StatusBadRequest, blockedByHibernation, awake, true, false},
		{"the Deployment read as hibernating", http.StatusBadRequest, `{"message":"deploy refused"}`, asleep, true, true},
		{"a forbidden deploy to a hibernating Deployment", http.StatusForbidden, `{"message":"no permission to deploy"}`, asleep, true, true},
		{"another refusal from an awake Deployment", http.StatusBadRequest, `{"message":"deploy refused"}`, awake, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refusal := refuse(tc.status, tc.body)
			got := explainHibernating(refusal, tc.dep)
			if !tc.explains {
				assert.Same(t, refusal, got)
				return
			}
			assert.Contains(t, got.Error(), wakeUpHint)
			assert.Equal(t, tc.keepsText, strings.Contains(got.Error(), refusal.Error()), "Astro's own text in %q", got)
			assert.ErrorIs(t, got, refusal)
			assert.True(t, httputil.HasStatus(got, tc.status))
		})
	}
}

func TestExplainHibernatingLeavesANetworkErrorAlone(t *testing.T) {
	offline := errors.New("dial tcp: no such host")
	asleep := &astrov1.Deployment{Id: "test-deployment-id", Status: astrov1.DeploymentStatusHIBERNATING}
	assert.Same(t, offline, explainHibernating(offline, asleep))
}

func TestDeployDagsV2_HibernatingSaysHowToWakeIt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2Deployment(client, true, false)
	mockCreateDeployRefused(client, blockedByHibernation)

	_, err := DeployDagsV2(DagDeployV2Input{
		ProjectDir:   v2ProjectDir(t),
		DeploymentID: "test-deployment-id",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), wakeUpHint)
	client.AssertExpectations(t)
}

func TestDeployBundle_HibernatingSaysHowToWakeIt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	bundlePath := filepath.Join(t.TempDir(), "dbt")
	require.NoError(t, os.Mkdir(bundlePath, 0o755))
	mockV2Deployment(client, true, false)
	mockCreateDeployRefused(client, blockedByHibernation)

	err := DeployBundle(&DeployBundleInput{
		BundlePath:    bundlePath,
		MountPath:     "dbt/project",
		DeploymentID:  "test-deployment-id",
		BundleType:    "dbt",
		AstroV1Client: client,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), wakeUpHint)
	client.AssertExpectations(t)
}

func TestDeployImageV2_HibernatingSaysHowToWakeIt(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockV2DeploymentAt(client, "3.1-2", true, false)
	mockDeploymentOptions(client, "3.1-2")
	mockCreateDeployRefused(client, blockedByHibernation)
	_, handler := withImageSeams(t, "3.1-2")

	_, err := DeployImageV2(ImageDeployV2Input{
		ProjectDir:     v2ProjectDir(t),
		DeploymentID:   "test-deployment-id",
		AirflowVersion: "3.1",
	}, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), wakeUpHint)
	handler.AssertNotCalled(t, "Push", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	client.AssertExpectations(t)
}
