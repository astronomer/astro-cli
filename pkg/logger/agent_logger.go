package logger

import (
	"os"
)

// AgentLogger is a simple logger that can write messages to files
type AgentLogger struct {
	filePath string
}

// NewAgentLogger creates a new AgentLogger that writes to the specified file path
func NewAgentLogger(filePath string) *AgentLogger {
	return &AgentLogger{
		filePath: filePath,
	}
}

// WriteMessage writes a message to the configured file
func (al *AgentLogger) WriteMessage(message string) error {
	return os.WriteFile(al.filePath, []byte(message), 0o644)
}
