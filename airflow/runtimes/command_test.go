package runtimes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ContainerRuntimeCommandSuite struct {
	suite.Suite
}

func TestContainerRuntimeCommand(t *testing.T) {
	suite.Run(t, new(ContainerRuntimeCommandSuite))
}

func (s *ContainerRuntimeCommandSuite) TestCommandExecution() {
	s.Run("Command executes successfully", func() {
		cmd := &Command{
			Command: "echo",
			Args:    []string{"hello"},
		}

		output, err := cmd.Execute()

		assert.NoError(s.T(), err)
		assert.Equal(s.T(), "hello\n", output)
	})

	s.Run("Command not found", func() {
		cmd := &Command{
			Command: "nonexistentcommand",
			Args:    []string{},
		}

		output, err := cmd.Execute()

		assert.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "executable file not found")
		assert.Empty(s.T(), output)
	})

	s.Run("Command fails", func() {
		cmd := &Command{
			Command: "ls",
			Args:    []string{"nonexistentdirectory"},
		}

		output, err := cmd.Execute()

		assert.Error(s.T(), err)
		assert.Contains(s.T(), output, "No such file or directory")
	})

	s.Run("Empty command", func() {
		cmd := &Command{
			Command: "",
			Args:    []string{},
		}

		_, err := cmd.Execute()

		assert.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "exec: no command")
	})
}
