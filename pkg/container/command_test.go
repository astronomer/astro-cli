package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CommandSuite struct {
	suite.Suite
}

func TestCommand(t *testing.T) {
	suite.Run(t, new(CommandSuite))
}

func (s *CommandSuite) TestCommandExecution() {
	s.Run("Command executes successfully", func() {
		cmd := &command{binary: "echo", args: []string{"hello"}}
		output, err := cmd.execute()
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), "hello\n", output)
	})

	s.Run("Command not found", func() {
		cmd := &command{binary: "nonexistentcommand", args: []string{}}
		output, err := cmd.execute()
		assert.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "executable file not found")
		assert.Empty(s.T(), output)
	})

	s.Run("Command fails", func() {
		cmd := &command{binary: "ls", args: []string{"nonexistentdirectory"}}
		output, err := cmd.execute()
		assert.Error(s.T(), err)
		assert.Contains(s.T(), output, "No such file or directory")
	})

	s.Run("Empty command", func() {
		cmd := &command{binary: "", args: []string{}}
		_, err := cmd.execute()
		assert.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "exec: no command")
	})
}

func (s *CommandSuite) TestErrorFromOutput() {
	s.Run("returns formatted error when error line is present", func() {
		output := "Some output\nError: something went wrong\nMore output"
		err := errorFromOutput("prefix: ", output)
		assert.EqualError(s.T(), err, "prefix: something went wrong")
	})

	s.Run("returns formatted error when output is empty", func() {
		err := errorFromOutput("prefix: ", "")
		assert.EqualError(s.T(), err, "prefix: ")
	})

	s.Run("returns formatted error when output contains only error line", func() {
		err := errorFromOutput("prefix: ", "Error: something went wrong")
		assert.EqualError(s.T(), err, "prefix: something went wrong")
	})
}
