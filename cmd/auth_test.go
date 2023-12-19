package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAuthRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	output, err := executeCommand("login", "--help")
	assert.NoError(t, err)
	assert.Contains(t, output, "Authenticate to Astro or Astronomer Software")
}

func TestLogin(t *testing.T) {
	buf := new(bytes.Buffer)
	localDomain := "localhost"
	cloudDomain := "astronomer.io"
	softwareDomain := "astronomer_dev.com"

	cloudLogin = func(domain, token string, client astro.Client, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
		assert.Equal(t, cloudDomain, domain)
		return nil
	}

	softwareLogin = func(domain string, oAuthOnly bool, username, password, houstonVersion string, client houston.ClientInterface, out io.Writer) error {
		assert.Equal(t, softwareDomain, domain)
		return nil
	}

	// cloud login success
	login(&cobra.Command{}, []string{cloudDomain}, nil, nil, nil, buf)

	// software login success
	testUtil.InitTestConfig(testUtil.Initial)
	login(&cobra.Command{}, []string{softwareDomain}, nil, nil, nil, buf)

	// no domain, cloud login
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	login(&cobra.Command{}, []string{}, nil, nil, nil, buf)

	// no domain, software login
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	login(&cobra.Command{}, []string{}, nil, nil, nil, buf)

	// no domain, no current context set
	config.ResetCurrentContext()
	login(&cobra.Command{}, []string{}, nil, nil, nil, buf)

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer testUtil.MockUserInput(t, "n")()
	login(&cobra.Command{}, []string{"fail.astronomer.io"}, nil, nil, nil, buf)
	assert.Contains(t, buf.String(), "fail.astronomer.io is an invalid domain to login into Astro.\n")

	testUtil.InitTestConfig(testUtil.LocalPlatform)
	softwareDomain = "software.astronomer.io"
	buf = new(bytes.Buffer)
	defer testUtil.MockUserInput(t, "y")()
	login(&cobra.Command{}, []string{"software.astronomer.io"}, nil, nil, nil, buf)
	assert.Contains(t, buf.String(), "software.astronomer.io is an invalid domain to login into Astro.\n")
}

func TestLogout(t *testing.T) {
	localDomain := "localhost"
	softwareDomain := "astronomer_dev.com"

	cloudLogout = func(domain string, out io.Writer) {
		assert.Equal(t, localDomain, domain)
	}
	softwareLogout = func(domain string) {
		assert.Equal(t, softwareDomain, domain)
	}

	// cloud logout success
	err := logout(&cobra.Command{}, []string{localDomain}, os.Stdout)
	assert.NoError(t, err)

	// software logout success
	err = logout(&cobra.Command{}, []string{softwareDomain}, os.Stdout)
	assert.NoError(t, err)

	// no domain, cloud logout
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	assert.NoError(t, err)

	// no domain, software logout
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	assert.NoError(t, err)

	// no domain, no current context set
	config.ResetCurrentContext()
	err = logout(&cobra.Command{}, []string{}, os.Stdout)
	assert.EqualError(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
}
