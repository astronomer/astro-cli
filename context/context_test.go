package context

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestContext(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestNewTableOut() {
	tab := newTableOut()
	s.NotNil(tab)
	s.Equal([]int{36, 36}, tab.Padding)
}

func (s *Suite) TestExists() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// Check that we don't have localhost123 in test config from testUtils.NewTestConfig()
	s.False(Exists("localhost123"))
}

func (s *Suite) TestGetCurrentContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cluster, err := GetCurrentContext()
	s.NoError(err)
	s.Equal(cluster.Domain, testUtil.GetEnv("HOST", "localhost"))
	s.Equal(cluster.Workspace, "ck05r3bor07h40d02y2hw4n4v")
	s.Equal(cluster.LastUsedWorkspace, "ck05r3bor07h40d02y2hw4n4v")
	s.Equal(cluster.Token, "token")
}

func (s *Suite) TestGetContextKeyValidContextConfig() {
	c := config.Context{Domain: "test.com"}
	key, err := c.GetContextKey()
	s.NoError(err)
	s.Equal(key, "test_com")
}

func (s *Suite) TestGetContextKeyInvalidContextConfig() {
	c := config.Context{}
	_, err := c.GetContextKey()
	s.EqualError(err, "context config invalid, no domain specified")
}

func (s *Suite) TestIsCloudContext() {
	s.Run("validates cloud context based on domains", func() {
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
			s.Equal(tt.expectedOutput, output, fmt.Sprintf("input: %+v", tt))
		}
	})
	s.Run("returns true when no current context is set", func() {
		// Case when no current context is set
		config.ResetCurrentContext()
		output := IsCloudContext()
		s.Equal(true, output)
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := Delete("astronomer.io", true)
	s.NoError(err)

	err = Delete("", false)
	s.ErrorIs(err, config.ErrCtxConfigErr)
}

func (s *Suite) TestDeleteContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := DeleteContext(&cobra.Command{}, []string{"astronomer.io"}, false)
	s.NoError(err)
}

func (s *Suite) TestGetContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, err := GetContext("localhost")
	s.NoError(err)
	s.Equal(ctx.Domain, "localhost")
}

func (s *Suite) TestSwitchContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err := SwitchContext(&cobra.Command{}, []string{"localhost"})
	s.NoError(err)
}

func (s *Suite) TestListContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	SetContext("astronomer.io")
	buf := new(bytes.Buffer)
	err := ListContext(&cobra.Command{}, []string{}, buf)
	s.NoError(err)
	s.Contains(buf.String(), "localhost")
}

func (s *Suite) TestIsCloudDomain() {
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
	s.Run("returns true for valid domains", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		for _, domain := range domainList {
			actual := IsCloudDomain(domain)
			s.True(actual, domain)
		}
	})
	s.Run("returns false for invalid domains", func() {
		NotCloudList := []string{
			"cloud.prastronomer-dev.io",
			"pr1234567.astronomer-dev.io",
			"drum.cloud.astronomer-dev.io",
			"localbeast",
		}
		testUtil.InitTestConfig(testUtil.Initial)
		for _, domain := range NotCloudList {
			actual := IsCloudDomain(domain)
			s.False(actual, domain)
		}
	})
}
