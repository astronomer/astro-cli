package software

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/houston"
	houston_mocks "github.com/astronomer/astro-cli/houston/mocks"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestVersionMatchCmds(t *testing.T) {
	t.Run("0.29.0 platform with teams command", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", nil).Return(&houston.AppConfig{Version: "0.29.0"}, nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"team", "--help"})

		r, w, err := os.Pipe()
		assert.NoError(t, err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		assert.NoError(t, err)
		io.Copy(b, r)
		assert.Contains(t, b.String(), "unknown command \"team\" for \"astro\"")
	})

	t.Run("0.30.0 platform with teams command", func(t *testing.T) {
		buf := new(bytes.Buffer)
		mockAPI := new(houston_mocks.ClientInterface)
		mockAPI.On("GetAppConfig", nil).Return(&houston.AppConfig{Version: "0.30.0"}, nil)
		cmd := &cobra.Command{Use: "astro"}
		childCMDs := AddCmds(mockAPI, buf)
		cmd.AddCommand(childCMDs...)

		VersionMatchCmds(cmd, []string{"astro"})
		buf.Reset()
		b := new(bytes.Buffer)
		cmd.SetArgs([]string{"team", "--help"})

		r, w, err := os.Pipe()
		assert.NoError(t, err)

		realStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = realStdout }()

		_, err = cmd.ExecuteC()
		w.Close()
		assert.NoError(t, err)
		io.Copy(b, r)
		assert.Contains(t, b.String(), "Teams represents a team or a group from an IDP in the Astronomer Platform")
	})
}

func TestRemoveCmd(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			removeCmd(tt.args.c)
		})
	}
}
