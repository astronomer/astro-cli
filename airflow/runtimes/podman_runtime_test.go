package runtimes

import (
	"os"

	"github.com/stretchr/testify/assert"
)

func (s *ContainerRuntimeSuite) TestIsDockerHostSet() {
	s.Run("DOCKER_HOST is set and returns true", func() {
		os.Setenv("DOCKER_HOST", "some_value")
		defer os.Unsetenv("DOCKER_HOST")

		result := IsDockerHostSet()
		assert.True(s.T(), result)
	})

	s.Run("DOCKER_HOST is set and returns true", func() {
		os.Unsetenv("DOCKER_HOST")

		result := IsDockerHostSet()
		assert.False(s.T(), result)
	})
}

func (s *ContainerRuntimeSuite) TestIsDockerHostSetToAstroMachine() {
	s.Run("DOCKER_HOST is set to astro-machine and returns true", func() {
		os.Setenv("DOCKER_HOST", "unix:///path/to/astro-machine.sock")
		defer os.Unsetenv("DOCKER_HOST")

		result := IsDockerHostSetToAstroMachine()
		assert.True(s.T(), result)
	})

	s.Run("DOCKER_HOST is set to other-machine and returns false", func() {
		os.Setenv("DOCKER_HOST", "unix:///path/to/other-machine.sock")
		defer os.Unsetenv("DOCKER_HOST")

		result := IsDockerHostSetToAstroMachine()
		assert.False(s.T(), result)
	})

	s.Run("DOCKER_HOST is not set and returns false", func() {
		os.Unsetenv("DOCKER_HOST")

		result := IsDockerHostSetToAstroMachine()
		assert.False(s.T(), result)
	})
}
