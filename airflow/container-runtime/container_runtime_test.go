package container_runtime

import "github.com/astronomer/astro-cli/airflow"

// MockFileChecker is a mock implementation of FileChecker for tests.
type MockFileChecker struct {
	existingFiles map[string]bool
}

// Exists is just a mock for os.Stat(). In our test implementation, we just check
// if the file exists in the list of mocked files for a given test.
func (m MockFileChecker) Exists(path string) bool {
	return m.existingFiles[path]
}

// TestGetContainerRuntimeBinary runs a suite of tests against GetContainerRuntimeBinary,
// using the MockFileChecker defined above.
func (s *airflow.Suite) TestGetContainerRuntimeBinary() {
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
			mockChecker := MockFileChecker{existingFiles: tt.mockFiles}
			result := FindBinary(tt.pathEnv, tt.binary, mockChecker)
			s.Equal(tt.expected, result)
		})
	}
}
