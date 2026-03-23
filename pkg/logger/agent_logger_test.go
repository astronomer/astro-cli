package logger

import (
	"os"
	"testing"
)

func TestAgentLogger_WriteMessage(t *testing.T) {
	filePath := "/tmp/hello2.txt"

	// Create a new AgentLogger
	agentLogger := NewAgentLogger(filePath)

	// Write the message "hello" to the file
	err := agentLogger.WriteMessage("hello")
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Verify the file was created and contains the correct content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "hello" {
		t.Errorf("Expected file content to be 'hello', got '%s'", string(content))
	}

	// Note: File is left at /tmp/hello2.txt for inspection
}
