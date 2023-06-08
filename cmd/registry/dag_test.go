package registry

import (
	"bytes"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func execDagCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newRegistryDagCmd()
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestDagAdd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	_ = os.Remove("dags/sagemaker-batch-inference.py")

	//mockClient := new(astro_mocks.Client)
	//mockClient.On("ListDeployments", mock.Anything, "").Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
	//astroClient = mockClient

	cmdArgs := []string{"add", "sagemaker-batch-inference"}
	resp, err := execDagCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-id-2")
	//mockClient.AssertExpectations(t)
}

func TestDagList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	assert.Equal(t, 1, 0)
}
