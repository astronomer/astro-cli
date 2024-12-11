package runtimes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PodmanEngineSuite struct {
	suite.Suite
}

func TestPodmanEngine(t *testing.T) {
	suite.Run(t, new(PodmanEngineSuite))
}

func (s *PodmanRuntimeSuite) TestPodmanEngineErrorFromOutput() {
	s.Run("returns formatted error when error line is present", func() {
		output := "Some output\nError: something went wrong\nMore output"
		err := ErrorFromOutput("prefix: %s", output)
		assert.EqualError(s.T(), err, "prefix: something went wrong")
	})

	s.Run("returns formatted error when output is empty", func() {
		output := ""
		err := ErrorFromOutput("prefix: %s", output)
		assert.EqualError(s.T(), err, "prefix: ")
	})

	s.Run("returns formatted error when output contains only error line", func() {
		output := "Error: something went wrong"
		err := ErrorFromOutput("prefix: %s", output)
		assert.EqualError(s.T(), err, "prefix: something went wrong")
	})
}
