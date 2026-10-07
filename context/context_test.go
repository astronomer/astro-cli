package context

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
}

func TestContext(t *testing.T) {
	suite.Run(t, new(Suite))
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	out := new(bytes.Buffer)
	removal, err := Delete("astronomer.io", true, out)
	s.NoError(err)
	s.Equal(&Removal{Domain: "astronomer.io", Action: "deleted"}, removal)
	s.False(Exists("astronomer.io"))
	_, err = config.GetCurrentDomain()
	s.ErrorIs(err, config.ErrGetHomeString, "deleting the current context leaves none current")
	s.Empty(out.String(), "the success line is the command's to print")

	removal, err = Delete("astronomer.io", true, out)
	s.ErrorIs(err, config.ErrContextNotExist)
	s.Nil(removal)

	_, err = Delete("", false, out)
	s.ErrorIs(err, config.ErrCtxConfigErr)
}

// A delete the config refuses to save says which context in the error it
// returns, and prints nothing beside it: under --output json the error is the
// one object on stdout.
func (s *Suite) TestDeleteThatCannotSave() {
	fs := afero.NewMemMapFs()
	s.Require().NoError(afero.WriteFile(fs, config.HomeConfigFile, testUtil.NewTestConfig(testUtil.LocalPlatform), 0o600))
	config.InitConfig(afero.NewReadOnlyFs(fs))
	s.T().Cleanup(func() { testUtil.InitTestConfig(testUtil.LocalPlatform) })

	out := new(bytes.Buffer)
	removal, err := Delete("localhost", true, out)
	s.Nil(removal)
	s.ErrorContains(err, "deleting context localhost: ")
	s.Empty(out.String())
}

func (s *Suite) TestDeleteWithoutConfig() {
	testUtil.InitTestConfig(testUtil.Initial)
	removal, err := Delete("nope.example.com", true, new(bytes.Buffer))
	s.ErrorIs(err, config.ErrContextNotExist)
	s.ErrorContains(err, "nope.example.com")
	s.Nil(removal)
}

func (s *Suite) TestGetContext() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, err := GetContext("localhost")
	s.NoError(err)
	s.Equal(ctx.Domain, "localhost")
}

func (s *Suite) TestSwitchTo() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Require().NoError(SetContext("software.example.com"))

	switched, err := SwitchTo("software.example.com")
	s.NoError(err)
	s.Equal("software.example.com", switched.Domain)
	s.True(switched.IsCurrent)
	domain, err := config.GetCurrentDomain()
	s.NoError(err)
	s.Equal("software.example.com", domain)

	s.Run("a new domain is saved and made current", func() {
		switched, err := SwitchTo("new.example.com")
		s.NoError(err)
		s.Equal(Info{Domain: "new.example.com", IsCurrent: true}, switched)
	})
}

func (s *Suite) TestSavedIgnoresASTRODOMAIN() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Require().NoError(SetContext("astronomer.io"))
	s.T().Setenv("ASTRO_DOMAIN", "astronomer.io")
	saved, err := Saved()
	s.NoError(err)
	s.Equal("localhost", saved.Domain, "the saved current context, not the variable's")
	s.False(saved.IsCurrent, "the variable makes another one current here")
}

func (s *Suite) TestList() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Require().NoError(SetContext("astronomer.io"))
	list, err := List()
	s.NoError(err)
	domains := []string{}
	for _, c := range list.Contexts {
		domains = append(domains, c.Domain)
		s.Equal(c.Domain == "localhost", c.IsCurrent, c.Domain)
	}
	s.Equal([]string{"astronomer.io", "localhost"}, domains, "sorted, with nothing else")
	s.Equal("ck05r3bor07h40d02y2hw4n4v", list.Contexts[1].WorkspaceID)

	s.Run("ASTRO_DOMAIN decides which is current, however it is spelled", func() {
		s.T().Setenv("ASTRO_DOMAIN", "Astronomer.io")
		list, err := List()
		s.NoError(err)
		s.True(list.Contexts[0].IsCurrent)
		s.False(list.Contexts[1].IsCurrent)
	})

	s.Run("fails when the current domain has no saved context", func() {
		s.T().Setenv("ASTRO_DOMAIN", "unsaved.example.com")
		_, err := List()
		s.ErrorIs(err, config.ErrNotConnected)
	})

	s.Run("fails with no current context, as the table did", func() {
		s.Require().NoError(config.ResetCurrentContext())
		_, err := List()
		s.ErrorIs(err, config.ErrGetHomeString)
	})
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
