package software

import (
	"bytes"
	"io"
	"os"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/spf13/cobra"
)

func (s *Suite) TestVersionMatchCmds() {
	s.Run("0.27.0 platform with teams command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", "").Return(&houston.AppConfig{Version: "0.27.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("0.27.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"team", "--help"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		s.NoError(err)
		io.Copy(b, r)
		s.Contains(b.String(), "unknown command \"team\" for \"astro\"")
	})

	s.Run("0.30.0 platform with teams command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", "").Return(&houston.AppConfig{Version: "0.30.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("0.30.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"team", "--help"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		s.NoError(err)
		io.Copy(b, r)
		s.Contains(b.String(), "Teams represents a team or a group from an IDP in the Astronomer Platform")
	})
}

func (s *Suite) TestRemoveCmd() {
	type args struct {
		c *cobra.Command
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			removeCmd(tt.args.c)
		})
	}
}
