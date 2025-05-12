package airflowversions

import (
	"testing"
)

type runtimeVersionsSuite struct {
	t *testing.T
}

func TestRuntimeVersions(t *testing.T) {
	suite := &runtimeVersionsSuite{t: t}
	suite.TestCompareRuntimeVersions()
	suite.TestRuntimeVersionMajor()
	suite.TestRuntimeVersionAirflowMajorVersion()
}

func (s *runtimeVersionsSuite) TestCompareRuntimeVersions() {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{"new format greater than old", "3.0-7", "12.1.1", 1},
		{"old format less than new", "12.1.1", "3.0-7", -1},
		{"same versions", "3.0-7", "3.0-7", 0},
		{"new format comparison", "3.1-1", "3.0-7", 1},
		{"old format comparison", "12.1.1", "11.9.9", 1},
		{"invalid versions equal", "invalid", "invalid", 0},
		{"invalid less than valid", "invalid", "3.0-7", -1},
		{"valid greater than invalid", "3.0-7", "invalid", 1},
	}

	for _, tt := range tests {
		s.t.Run(tt.name, func(t *testing.T) {
			result := CompareRuntimeVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareRuntimeVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func (s *runtimeVersionsSuite) TestRuntimeVersionMajor() {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"new format", "3.0-7", "3"},
		{"old format", "12.1.1", "12"},
		{"invalid version", "invalid", ""},
	}

	for _, tt := range tests {
		s.t.Run(tt.name, func(t *testing.T) {
			result := RuntimeVersionMajor(tt.input)
			if result != tt.expected {
				t.Errorf("RuntimeVersionMajor(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func (s *runtimeVersionsSuite) TestRuntimeVersionMajorMinor() {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"new format", "3.0-7", "3.0"},
		{"old format", "12.1.1", "12.1"},
		{"invalid version", "invalid", ""},
	}

	for _, tt := range tests {
		s.t.Run(tt.name, func(t *testing.T) {
			result := RuntimeVersionMajorMinor(tt.input)
			if result != tt.expected {
				t.Errorf("RuntimeVersionMajorMinor(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func (s *runtimeVersionsSuite) TestRuntimeVersionAirflowMajorVersion() {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"new format", "3.0-7", "3"},
		{"valid old format", "12.1.1", "2"},
		{"invalid old format", "invalid", ""},
	}

	for _, tt := range tests {
		s.t.Run(tt.name, func(t *testing.T) {
			result := AirflowMajorVersionForRuntimeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("RuntimeVersionAirflowMajorVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
