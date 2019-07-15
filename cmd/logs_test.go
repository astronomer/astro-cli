package cmd

import (
	"testing"
)


func TestLogsCommand(t *testing.T) {
	output, err := executeCommand(RootCmd, "logs")
	if output != "" {
		t.Errorf("Unexpected output: %v", output)
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
