package cmd

import (
	"bytes"
	"io"
	"os"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
)

func (s *CmdSuite) TestAuthRootCommand() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("login", "--help")
	s.NoError(err)
	s.Contains(output, "Authenticate to Astro or Astronomer Software")
}

func setupTokenFile(token string) string {
	tmpFile, err := os.CreateTemp("", "testfile_")
	if err != nil {
		return ""
	}
	defer tmpFile.Close()

	_, err = tmpFile.Write([]byte(token))
	if err != nil {
		return ""
	}
	return tmpFile.Name()
}

func withTokenFlag(cmd *cobra.Command, token string) *cobra.Command {
	cmd.Flags().Set("token-login", token)
	return cmd
}

func (s *CmdSuite) TestLogin() {
	buf := new(bytes.Buffer)
	cloudDomain := "astronomer.io"
	softwareDomain := "astronomer_dev.com"
	token = "fake.jwt.claims"
	tokenFilePath := setupTokenFile(token)

	cloudLogin = func(domain, tok string, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
		s.Equal(cloudDomain, domain)
		s.Equal(tok, token)
		return nil
	}

	softwareLogin = func(domain string, oAuthOnly bool, username, password, houstonVersion string, client houston.ClientInterface, out io.Writer) error {
		s.Equal(softwareDomain, domain)
		return nil
	}

	// cloud login success
	login(&cobra.Command{}, []string{cloudDomain}, nil, nil, buf)

	// cloud login, token in flag value
	login(withTokenFlag(&cobra.Command{}, token), []string{cloudDomain}, nil, nil, buf)

	// cloud login, token in file
	login(withTokenFlag(&cobra.Command{}, tokenFilePath), []string{cloudDomain}, nil, nil, buf)

	// software login success
	testUtil.InitTestConfig(testUtil.Initial)
	login(&cobra.Command{}, []string{softwareDomain}, nil, nil, buf)

	// no domain, cloud login
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	login(&cobra.Command{}, []string{}, nil, nil, buf)

	// no domain, cloud login, token in flag value
	login(withTokenFlag(&cobra.Command{}, token), []string{}, nil, nil, buf)

	// no domain, cloud login, token in file
	login(withTokenFlag(&cobra.Command{}, tokenFilePath), []string{}, nil, nil, buf)

	// no domain, software login
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	login(&cobra.Command{}, []string{}, nil, nil, buf)

	// no domain, no current context set
	config.ResetCurrentContext()
	login(&cobra.Command{}, []string{}, nil, nil, buf)

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	softwareDomain = "software.astronomer.io"
	login(&cobra.Command{}, []string{softwareDomain}, nil, nil, buf)
	s.Contains(buf.String(), "To login to Astronomer Software follow the instructions below. If you are attempting to login in to Astro cancel the login and run 'astro login'.\n\n")
}

func (s *CmdSuite) TestLogout() {
	localDomain := "localhost"
	softwareDomain := "astronomer_dev.com"

	cloudLogout = func(domain string, out io.Writer) {
		s.Equal(localDomain, domain)
	}
	softwareLogout = func(domain string) {
		s.Equal(softwareDomain, domain)
	}

	// cloud logout success
	err := logout(&cobra.Command{}, []string{localDomain}, os.Stdout)
	s.NoError(err)

	// software logout success
	err = logout(&cobra.Command{}, []string{softwareDomain}, os.Stdout)
	s.NoError(err)

	// no domain, cloud logout
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	s.NoError(err)

	// no domain, software logout
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	s.NoError(err)

	// no domain, no current context set
	config.ResetCurrentContext()
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	s.EqualError(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
}
