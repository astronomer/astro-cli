package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func executeCommandC(root *cobra.Command, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	root.SetArgs(args)
	c, err = root.ExecuteC()
	return c, buf.String(), err
}

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	_, output, err = executeCommandC(root, args...)
	return output, err
}

func TestDevCommand(t *testing.T) {
	output, err := executeCommand(RootCmd, "dev")
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestAirflowCommand(t *testing.T) {
	output, err := executeCommand(RootCmd, "airflow")
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
