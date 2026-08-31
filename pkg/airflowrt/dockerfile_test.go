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
		tag        string
		wantBase   string
		wantPython string
	}{
		{"3.1-12", "3.1-12", ""},
		{"3.1-12-python-3.11", "3.1-12", "3.11"},
		{"3.1-12-python-3.11-base", "3.1-12", "3.11"},
		{"3.1-12-base", "3.1-12", ""},
		{"12.0.0", "12.0.0", ""},
		{"12.0.0-python-3.12", "12.0.0", "3.12"},
		{"13.7.0-slim", "13.7.0", ""},
		{"13.7.0-slim-python-3.12", "13.7.0", "3.12"},
		{"3.1-12-slim", "3.1-12", ""},
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

// The final stage decides the image, and an alias is followed back to it.
//
// The first FROM answers a different question, and got it wrong for the file
// this parsing exists to read: a multi-stage build is the headline reason a
// project declares its own Dockerfile, and those open with a builder stage. A
// caller reading the builder's base saw python where the runtime was, so an
// Airflow 3 project was handed the Airflow 2 compose service set.
func TestParseDockerfileAtResolvesTheFinalStage(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantImage, wantTag string
	}{
		{
			name:      "single stage is unchanged",
			body:      "FROM astrocrpublic.azurecr.io/runtime:3.1-2\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-2",
		},
		{
			name:      "builder first, runtime last",
			body:      "FROM python:3.12-slim AS builder\nRUN pip install poetry\nFROM astrocrpublic.azurecr.io/runtime:3.1-2\nCOPY --from=builder /x /x\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-2",
		},
		{
			name:      "final stage names an earlier alias",
			body:      "FROM astrocrpublic.azurecr.io/runtime:3.1-2 AS base\nFROM python:3.12 AS tools\nFROM base\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-2",
		},
		{
			name:      "alias chain",
			body:      "FROM astrocrpublic.azurecr.io/runtime:3.1-2 AS one\nFROM one AS two\nFROM two\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-2",
		},
		{
			name:      "no tag defaults to latest",
			body:      "FROM python AS builder\nFROM my-own-base\n",
			wantImage: "my-own-base", wantTag: "latest",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "Dockerfile")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			image, tag, err := ParseDockerfileAt(path)
			if err != nil {
				t.Fatal(err)
			}
			if image != tc.wantImage || tag != tc.wantTag {
				t.Errorf("ParseDockerfileAt = %q:%q, want %q:%q", image, tag, tc.wantImage, tc.wantTag)
			}
		})
	}
}
