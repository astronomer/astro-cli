package airflowrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRuntimeTagPython(t *testing.T) {
	tests := []struct {
		tag            string
		wantBase       string
		wantPython     string
	}{
		{"3.1-12", "3.1-12", ""},
		{"3.1-12-python-3.11", "3.1-12", "3.11"},
		{"3.1-12-python-3.11-base", "3.1-12", "3.11"},
		{"3.1-12-base", "3.1-12", ""},
		{"12.0.0", "12.0.0", ""},
		{"12.0.0-python-3.12", "12.0.0", "3.12"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			base, python := ParseRuntimeTagPython(tt.tag)
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantPython, python)
		})
	}
}

func TestIsValidRuntimeTag(t *testing.T) {
	assert.True(t, IsValidRuntimeTag("3.1-12"))
	assert.True(t, IsValidRuntimeTag("3.1-12-python-3.11"))
	assert.False(t, IsValidRuntimeTag("12.0.0"))
	assert.False(t, IsValidRuntimeTag("latest"))
	assert.False(t, IsValidRuntimeTag("3.1"))
}

func TestIsRuntime3(t *testing.T) {
	assert.True(t, IsRuntime3("3.1-12"))
	assert.True(t, IsRuntime3("3.0-1"))
	assert.False(t, IsRuntime3("12.0.0"))
	assert.False(t, IsRuntime3("2.9-1"))
}

func TestParseDockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(dockerfile, []byte("FROM quay.io/astronomer/astro-runtime:3.1-12\n"), 0o644))

	image, tag, err := ParseDockerfile(dir)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime", image)
	assert.Equal(t, "3.1-12", tag)
}

func TestParseDockerfile_WithAlias(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(dockerfile, []byte("FROM astro-runtime:3.1-12 AS stage1\nRUN echo hi\n"), 0o644))

	image, tag, err := ParseDockerfile(dir)
	require.NoError(t, err)
	assert.Equal(t, "astro-runtime", image)
	assert.Equal(t, "3.1-12", tag)
}

func TestParseDockerfile_NoTag(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(dockerfile, []byte("FROM astro-runtime\n"), 0o644))

	image, tag, err := ParseDockerfile(dir)
	require.NoError(t, err)
	assert.Equal(t, "astro-runtime", image)
	assert.Equal(t, "latest", tag)
}

func TestParseDockerfile_NoFrom(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	require.NoError(t, os.WriteFile(dockerfile, []byte("RUN echo hi\n"), 0o644))

	_, _, err := ParseDockerfile(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no FROM instruction")
}

func TestParseDockerfile_NoFile(t *testing.T) {
	_, _, err := ParseDockerfile(t.TempDir())
	assert.Error(t, err)
}
