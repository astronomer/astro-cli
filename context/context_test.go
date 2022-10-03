package context

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestExists(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// Check that we don't have localhost123 in test config from testUtils.NewTestConfig()
	assert.False(t, Exists("localhost123"))
}

func TestGetCurrentContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cluster, err := GetCurrentContext()
	assert.NoError(t, err)
	assert.Equal(t, cluster.Domain, testUtil.GetEnv("HOST", "localhost"))
	assert.Equal(t, cluster.Workspace, "ck05r3bor07h40d02y2hw4n4v")
	assert.Equal(t, cluster.LastUsedWorkspace, "ck05r3bor07h40d02y2hw4n4v")
	assert.Equal(t, cluster.Token, "token")
}

func TestGetContextKeyValidContextConfig(t *testing.T) {
	c := config.Context{Domain: "test.com"}
	key, err := c.GetContextKey()
	assert.NoError(t, err)
	assert.Equal(t, key, "test_com")
}

func TestGetContextKeyInvalidContextConfig(t *testing.T) {
	c := config.Context{}
	_, err := c.GetContextKey()
	assert.EqualError(t, err, "context config invalid, no domain specified")
}

func TestIsCloudContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	tests := []struct {
		name           string
		contextDomain  string
		localPlatform  string
		expectedOutput bool
	}{
		{"cloud-domain", "cloud.astronomer.io", config.CloudPlatform, true},
		{"cloud-domain", "astronomer.io", config.CloudPlatform, true},
		{"cloud-dev-domain", "astronomer-dev.io", config.CloudPlatform, true},
		{"cloud-dev-domain", "cloud.astronomer-dev.io", config.CloudPlatform, true},
		{"cloud-stage-domain", "astronomer-stage.io", config.CloudPlatform, true},
		{"cloud-stage-domain", "cloud.astronomer-stage.io", config.CloudPlatform, true},
		{"cloud-perf-domain", "astronomer-perf.io", config.CloudPlatform, true},
		{"cloud-perf-domain", "cloud.astronomer-perf.io", config.CloudPlatform, true},
		{"local-cloud", "localhost", config.CloudPlatform, true},
		{"software-domain", "dev.astrodev.com", config.SoftwarePlatform, false},
		{"software-domain", "software.astronomer-test.io", config.SoftwarePlatform, false},
		{"local-software", "localhost", config.SoftwarePlatform, false},
	}

	for _, tt := range tests {
		SetContext(tt.contextDomain)
		Switch(tt.contextDomain)
		config.CFG.LocalPlatform.SetHomeString(tt.localPlatform)
		output := IsCloudContext()
		assert.Equal(t, tt.expectedOutput, output, fmt.Sprintf("input: %+v", tt))
	}

	// Case when no current context is set
	config.ResetCurrentContext()
	output := IsCloudContext()
	assert.Equal(t, true, output)
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	err := Delete("astronomer.io", true)
	assert.NoError(t, err)

	err = Delete("", false)
	assert.ErrorIs(t, err, config.ErrCtxConfigErr)
}

func TestDeleteContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	err := DeleteContext(&cobra.Command{}, []string{"astronomer.io"}, false)
	assert.NoError(t, err)
}

func TestGetContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, err := GetContext("localhost")
	assert.NoError(t, err)
	assert.Equal(t, ctx.Domain, "localhost")
}

func TestSwitchContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := SwitchContext(&cobra.Command{}, []string{"localhost"})
	assert.NoError(t, err)
}

func TestListContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	SetContext("astronomer.io")
	buf := new(bytes.Buffer)
	err := ListContext(&cobra.Command{}, []string{}, buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "localhost")
}
