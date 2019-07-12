package cmd

import (
	"github.com/spf13/cobra"
	"testing"
)


func TestDeploymentLogsCommand(t *testing.T) {
	output, err := executeCommand(deploymentRootCmd, "logs")
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestDeploymentCreateCommand(t *testing.T) {
	var rootCmdArgs []string
	rootCmd := &cobra.Command{
		Use: "root",
		Run: func(_ *cobra.Command, args []string) { rootCmdArgs = args },
	}
	output, err := executeCommand(rootCmd)
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}