package cmd

import (
	"bytes"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *CmdSuite) TestContextSwitch() {
	previous := cloudSwitch
	s.T().Cleanup(func() { cloudSwitch = previous })

	var got string
	cloudSwitch = func(domain string, astroV1Client astrov1.APIClient, out io.Writer) error {
		got = domain
		return nil
	}

	s.Run("an Astro domain or short name goes through the saved login", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		tests := map[string]string{
			"prod":                      "astronomer.io",
			"stage":                     "astronomer-stage.io",
			"pr12345":                   "pr12345.astronomer-dev.io",
			"astronomer-dev.io":         "astronomer-dev.io",
			"pr12345.astronomer-dev.io": "pr12345.astronomer-dev.io",
		}
		for name, want := range tests {
			got = ""
			s.NoError(switchContext(&cobra.Command{}, []string{name}, nil, new(bytes.Buffer)))
			s.Equal(want, got, name)
		}
	})

	s.Run("an APC domain switches as before", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		got = ""
		s.NoError(switchContext(&cobra.Command{}, []string{"software.example.com"}, nil, new(bytes.Buffer)))
		s.Empty(got)
		domain, err := config.GetCurrentDomain()
		s.NoError(err)
		s.Equal("software.example.com", domain)
	})
}
