package cmd

import (
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
