package software

import (
	"bytes"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
)

func (s *Suite) TestVersionMatchCmds() {
	s.Run("0.27.0 platform with teams command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "0.27.0"}, nil)
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
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "0.30.0"}, nil)
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

	s.Run("1.0.1 platform with deployment adopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "1.0.1"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("1.0.1", nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"deployment", "adopt", "--help"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		s.NoError(err)
		io.Copy(b, r)
		s.Contains(b.String(), "unknown command \"adopt\" for \"astro\"")
	})

	s.Run("2.1.0 platform with deployment adopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("2.1.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"deployment", "adopt", "--help"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		s.NoError(err)
		io.Copy(b, r)
		s.Contains(b.String(), "Adopt an existing operator-managed Airflow custom resource")
	})

	s.Run("1.0.1 platform with deployment unadopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "1.0.1"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("1.0.1", nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"deployment", "unadopt", "--help"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		s.NoError(err)
		io.Copy(b, r)
		s.Contains(b.String(), "unknown command \"unadopt\" for \"astro\"")
	})

	s.Run("2.1.0 platform with deployment unadopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("2.1.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"deployment", "unadopt", "--help"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		s.NoError(err)
		io.Copy(b, r)
		s.Contains(b.String(), "Release an adopted Deployment back to operator-only management")
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
