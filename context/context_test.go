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

func TestNewTableOut(t *testing.T) {
	tab := newTableOut()
	assert.NotNil(t, tab)
	assert.Equal(t, []int{36, 36}, tab.Padding)
}

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
	t.Run("validates cloud context based on domains", func(t *testing.T) {
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
			{"prpreview", "pr1234.astronomer-dev.io", config.PrPreview, true},
			{"prpreview", "pr1234.cloud.astronomer-dev.io", config.PrPreview, true},
			{"prpreview", "pr12345.cloud.astronomer-dev.io", config.PrPreview, true},
			{"prpreview", "pr12346.cloud.astronomer-dev.io", config.PrPreview, true},
			{"prpreview", "pr123.cloud.astronomer-dev.io", config.PrPreview, false},
		}

		for _, tt := range tests {
			SetContext(tt.contextDomain)
			Switch(tt.contextDomain)
			config.CFG.LocalPlatform.SetHomeString(tt.localPlatform)
			output := IsCloudContext()
			assert.Equal(t, tt.expectedOutput, output, fmt.Sprintf("input: %+v", tt))
		}
	})
	t.Run("returns true when no current context is set", func(t *testing.T) {
		// Case when no current context is set
		config.ResetCurrentContext()
		output := IsCloudContext()
		assert.Equal(t, true, output)
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := Delete("astronomer.io", true)
	assert.NoError(t, err)

	err = Delete("", false)
	assert.ErrorIs(t, err, config.ErrCtxConfigErr)
}

func TestDeleteContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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

func TestIsCloudDomain(t *testing.T) {
	domainList := []string{
		"astronomer.io",
		"astronomer-dev.io",
		"astronomer-stage.io",
		"astronomer-perf.io",
		"cloud.astronomer.io",
		"cloud.astronomer-dev.io",
		"cloud.astronomer-stage.io",
		"cloud.astronomer-perf.io",
		"https://cloud.astronomer.io",
		"https://cloud.astronomer-dev.io",
		"https://cloud.astronomer-dev.io/",
		"https://cloud.astronomer-stage.io",
		"https://cloud.astronomer-stage.io/",
		"https://cloud.astronomer-perf.io",
		"https://cloud.astronomer-perf.io/",
		"pr1234.cloud.astronomer-dev.io",
		"pr1234.astronomer-dev.io",
		"pr12345.astronomer-dev.io",
		"pr123456.astronomer-dev.io",
		"localhost",
		"localhost123",
	}
	t.Run("returns true for valid domains", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		for _, domain := range domainList {
			actual := IsCloudDomain(domain)
			assert.True(t, actual, domain)
		}
	})
	t.Run("returns false for invalid domains", func(t *testing.T) {
		NotCloudList := []string{
			"cloud.prastronomer-dev.io",
			"pr1234567.astronomer-dev.io",
			"drum.cloud.astronomer-dev.io",
			"localbeast",
		}
		testUtil.InitTestConfig(testUtil.Initial)
		for _, domain := range NotCloudList {
			actual := IsCloudDomain(domain)
			assert.False(t, actual, domain)
		}
	})
}
