package apc

import (
	"bytes"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

func (s *Suite) TestVersionMatchCmds() {
	s.Run("0.27.0 platform with teams command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "0.27.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("0.27.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		LoadPlatform(mockAPI)
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
		io.Copy(b, r)
		s.Error(err)
		s.Contains(err.Error(), "astro team needs Astro Private Cloud 0.28.0 or newer")
		s.Contains(err.Error(), "this platform reports 0.27.0")
	})

	s.Run("0.29.0 platform with team update command and no TEAM ID arg", func() {
		// "astro team" is gated at 0.28.0 (so it stays visible here), but "astro team
		// update" specifically is gated at 0.29.2, so it should be the one removed. Its
		// Args: cobra.ExactArgs(1) validator must not run before removeCmd's handler,
		// even though no TEAM ID positional arg is supplied.
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "0.29.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("0.29.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		LoadPlatform(mockAPI)
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"team", "update"})

		r, w, err := os.Pipe()
		s.NoError(err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		io.Copy(b, r)
		s.Error(err)
		s.Contains(err.Error(), "astro team update needs Astro Private Cloud 0.29.2 or newer")
		s.Contains(err.Error(), "this platform reports 0.29.0")
	})

	s.Run("0.30.0 platform with teams command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "0.30.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("0.30.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		LoadPlatform(mockAPI)
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
		s.Contains(b.String(), "A team represents a group of users from an IDP in the APC platform")
	})

	s.Run("1.0.1 platform with deployment adopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "1.0.1"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("1.0.1", nil)
		cmd := &cobra.Command{Use: "astro"}
		LoadPlatform(mockAPI)
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
		io.Copy(b, r)
		s.Error(err)
		s.Contains(err.Error(), "astro deployment adopt needs Astro Private Cloud 2.1.0 or newer")
		s.Contains(err.Error(), "this platform reports 1.0.1")
	})

	s.Run("2.1.0 platform with deployment adopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("2.1.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		LoadPlatform(mockAPI)
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
		LoadPlatform(mockAPI)
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
		io.Copy(b, r)
		s.Error(err)
		s.Contains(err.Error(), "astro deployment unadopt needs Astro Private Cloud 2.1.0 or newer")
		s.Contains(err.Error(), "this platform reports 1.0.1")
	})

	s.Run("2.1.0 platform with deployment unadopt command", func() {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.0"}, nil)
		mockAPI.On("GetPlatformVersion", nil).Return("2.1.0", nil)
		cmd := &cobra.Command{Use: "astro"}
		LoadPlatform(mockAPI)
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

func (s *Suite) TestVersionNeeded() {
	tests := []struct {
		name string
		r    houston.VersionRestrictions
		want string
	}{
		{"lower bound only", houston.VersionRestrictions{GTE: "2.1.0"}, "2.1.0 or newer"},
		{"both bounds", houston.VersionRestrictions{GTE: "0.28.0", LT: "1.0.0"}, "0.28.0 or newer, below 1.0.0"},
		{"upper bound only", houston.VersionRestrictions{LT: "1.0.0"}, "older than 1.0.0"},
		{"exact versions", houston.VersionRestrictions{EQ: []string{"0.30.0", "0.31.0"}}, "0.30.0 or 0.31.0"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.want, versionNeeded(tt.r))
		})
	}
}

// A gated command must fail, not print an error and exit 0. A script that runs
// `astro deployment adopt` against an older platform has to be able to tell.
func (s *Suite) TestRemovedCmdReturnsAnError() {
	buf := new(bytes.Buffer)
	mockAPI := new(houston_mocks.ClientInterface)
	mockAPI.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "1.0.1"}, nil)
	mockAPI.On("GetPlatformVersion", nil).Return("1.0.1", nil)
	cmd := &cobra.Command{Use: "astro"}
	LoadPlatform(mockAPI)
	cmd.AddCommand(AddCmds(mockAPI, buf)...)
	VersionMatchCmds(cmd, []string{"astro"})

	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"deployment", "adopt"})

	_, err := cmd.ExecuteC()
	s.Error(err, "a version-gated command must return an error so the exit code is non-zero")
	s.NotContains(err.Error(), "unknown command", "the command is known; it is the platform that is too old")
}
