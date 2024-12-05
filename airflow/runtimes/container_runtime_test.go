package runtimes

import (
	"errors"
	"testing"

	"github.com/astronomer/astro-cli/airflow/runtimes/mocks"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/suite"
)

type ContainerRuntimeSuite struct {
	suite.Suite
}

func TestContainerRuntime(t *testing.T) {
	suite.Run(t, new(ContainerRuntimeSuite))
}

func (s *ContainerRuntimeSuite) TestGetContainerRuntime() {
	s.Run("GetContainerRuntime_Docker", func() {
		mockChecker := new(mocks.RuntimeChecker)
		mockChecker.On("GetContainerRuntimeBinary").Return(docker, nil)

		// Inject the mock and make sure we restore after the test.
		originalGetContainerRuntimeBinary := GetContainerRuntimeBinary
		defer func() { GetContainerRuntimeBinary = originalGetContainerRuntimeBinary }()
		GetContainerRuntimeBinary = mockChecker.GetContainerRuntimeBinary

		runtime, err := GetContainerRuntime()

		assert.Nil(s.T(), err)
		assert.IsType(s.T(), DockerRuntime{}, runtime)
		mockChecker.AssertExpectations(s.T())
	})

	s.Run("GetContainerRuntime_Podman", func() {
		mockChecker := new(mocks.RuntimeChecker)
		mockChecker.On("GetContainerRuntimeBinary").Return(podman, nil)

		// Inject the mock and make sure we restore after the test.
		originalGetContainerRuntimeBinary := GetContainerRuntimeBinary
		defer func() { GetContainerRuntimeBinary = originalGetContainerRuntimeBinary }()
		GetContainerRuntimeBinary = mockChecker.GetContainerRuntimeBinary

		runtime, err := GetContainerRuntime()

		assert.Nil(s.T(), err)
		assert.IsType(s.T(), PodmanRuntime{}, runtime)
		mockChecker.AssertExpectations(s.T())
	})

	s.Run("GetContainerRuntime_Error", func() {
		mockChecker := new(mocks.RuntimeChecker)
		mockChecker.On("GetContainerRuntimeBinary").Return("", errors.New(containerRuntimeNotFoundErrMsg))

		// Inject the mock and make sure we restore after the test.
		originalGetContainerRuntimeBinary := GetContainerRuntimeBinary
		defer func() { GetContainerRuntimeBinary = originalGetContainerRuntimeBinary }()
		GetContainerRuntimeBinary = mockChecker.GetContainerRuntimeBinary

		runtime, err := GetContainerRuntime()

		assert.NotNil(s.T(), err)
		assert.Nil(s.T(), runtime)
		assert.Equal(s.T(), containerRuntimeNotFoundErrMsg, err.Error())
		mockChecker.AssertExpectations(s.T())
	})
}

// TestGetContainerRuntimeBinary runs a suite of tests against GetContainerRuntimeBinary,
// using the MockFileChecker defined above.
func (s *ContainerRuntimeSuite) TestGetContainerRuntimeBinary() {
	tests := []struct {
		name      string
		pathEnv   string
		binary    string
		mockFiles map[string]bool
		expected  bool
	}{
		{
			name:    "Find docker",
			pathEnv: "/usr/local/bin:/usr/bin:/bin",
			binary:  "docker",
			mockFiles: map[string]bool{
				"/usr/local/bin/docker": true,
			},
			expected: true,
		},
		{
			name:      "Find docker - doesn't exist",
			pathEnv:   "/usr/local/bin:/usr/bin:/bin",
			binary:    "docker",
			mockFiles: map[string]bool{},
			expected:  false,
		},
		{
			name:    "Find podman",
			pathEnv: "/usr/local/bin:/usr/bin:/bin",
			binary:  "podman",
			mockFiles: map[string]bool{
				"/usr/local/bin/podman": true,
			},
			expected: true,
		},
		{
			name:      "Find podman - doesn't exist",
			pathEnv:   "/usr/local/bin:/usr/bin:/bin",
			binary:    "podman",
			mockFiles: map[string]bool{},
			expected:  false,
		},
		{
			name:    "Binary not found",
			pathEnv: "/usr/local/bin:/usr/bin:/bin",
			binary:  "notarealbinary",
			mockFiles: map[string]bool{
				"/usr/local/bin/docker": true,
				"/usr/local/bin/podman": true,
			},
			expected: false,
		},
		{
			name:    "Duplicated paths in $PATH, binary exists",
			pathEnv: "/usr/local/bin:/usr/local/bin:/usr/local/bin",
			binary:  "docker",
			mockFiles: map[string]bool{
				"/usr/local/bin/docker": true,
			},
			expected: true,
		},
		{
			name:      "Duplicated paths in $PATH, binary does not exist",
			pathEnv:   "/usr/local/bin:/usr/local/bin:/usr/local/bin",
			binary:    "docker",
			mockFiles: map[string]bool{},
			expected:  false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			mockChecker := mocks.FileChecker{ExistingFiles: tt.mockFiles}
			result := FindBinary(tt.pathEnv, tt.binary, mockChecker, new(DefaultOSChecker))
			s.Equal(tt.expected, result)
		})
	}
}
