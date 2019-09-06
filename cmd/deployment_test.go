package cmd

import (
	"testing"
)

func TestDeploymentRootCommand(t *testing.T) {
	output, err := executeCommand(deploymentRootCmd)
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestDeploymentCreateCommand(t *testing.T) {
	output, err := executeCommand(deploymentCreateCmd)
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestDeploymentListCommand(t *testing.T) {
	output, err := executeCommand(deploymentListCmd)
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
