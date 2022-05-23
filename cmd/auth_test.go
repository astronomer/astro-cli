package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
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
	cloudDomain := "astronomer.io"
	softwareDomain := "astronomer_dev.com"

	cloudLogin = func(domain string, client astro.Client, out io.Writer, loginLink bool) error {
		assert.Equal(t, cloudDomain, domain)
		return nil
	}

	softwareLogin = func(domain string, oAuthOnly bool, username, password string, client houston.ClientInterface, out io.Writer) error {
		assert.Equal(t, softwareDomain, domain)
		return nil
	}

	// cloud login success
	login(&cobra.Command{}, []string{cloudDomain}, nil, buf)

	// software login success
	login(&cobra.Command{}, []string{softwareDomain}, nil, buf)

	// no domain, cloud login
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	login(&cobra.Command{}, []string{}, nil, buf)

	// no domain, software login
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	login(&cobra.Command{}, []string{}, nil, buf)

	// no domain, no current context set
	config.ResetCurrentContext()
	login(&cobra.Command{}, []string{}, nil, buf)
}

func TestLogout(t *testing.T) {
	cloudDomain := "astronomer.io"
	softwareDomain := "astronomer_dev.com"

	cloudLogout = func(domain string, out io.Writer) {
		assert.Equal(t, cloudDomain, domain)
	}
	softwareLogout = func(domain string) {
		assert.Equal(t, softwareDomain, domain)
	}

	// cloud logout success
	err := logout(&cobra.Command{}, []string{cloudDomain}, os.Stdout)
	assert.NoError(t, err)

	// software logout success
	err = logout(&cobra.Command{}, []string{softwareDomain}, os.Stdout)
	assert.NoError(t, err)

	// no domain, cloud logout
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
