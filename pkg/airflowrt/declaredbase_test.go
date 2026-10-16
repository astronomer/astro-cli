package airflowrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeclaredBaseRuntimeVersion(t *testing.T) {
	tests := []struct {
		name, body string
		known      bool
		want       string
	}{
		{name: "airflow 3 build", body: "FROM astrocrpublic.azurecr.io/runtime:3.3-8\n", known: true, want: "3.3-8"},
		{name: "flavor stripped", body: "FROM astrocrpublic.azurecr.io/runtime:3.3-8-python-3.12\n", known: true, want: "3.3-8"},
		{name: "airflow 2 build", body: "FROM quay.io/astronomer/astro-runtime:13.7.0-slim\n", known: true, want: "13.7.0"},
		{name: "final stage wins", body: "FROM python:3.12 AS b\nFROM astrocrpublic.azurecr.io/runtime:3.2-4\n", known: true, want: "3.2-4"},
		// A mirror keeps the repository's name and its tag scheme, so its
		// version reads the same way.
		{name: "mirrored airflow 3 build", body: "FROM mirror/runtime:3.1-12\n", known: true, want: "3.1-12"},
		{name: "mirrored airflow 2 build", body: "FROM mirror/astronomer/astro-runtime:12.1.0\n", known: true, want: "12.1.0"},
		{name: "mirror behind a port", body: "FROM localhost:5000/runtime:3.1-12-python-3.12\n", known: true, want: "3.1-12"},
		// A build argument in the tag is as unresolved as one in the image.
		{name: "build argument in the tag", body: "ARG V=3.1-12\nFROM astrocrpublic.azurecr.io/runtime:${V}\n", known: false, want: ""},
		{name: "build argument in a mirror's tag", body: "FROM mirror.corp/runtime:${TAG}\n", known: false, want: ""},
		{name: "tag and digest", body: "FROM mirror/runtime:3.1-12@sha256:abc\n", known: true, want: "3.1-12"},
		{name: "mirror pinned by digest alone", body: "FROM mirror/runtime@sha256:abc\n", known: true, want: ""},
		{name: "untagged mirror", body: "FROM mirror/runtime\n", known: true, want: ""},
		{name: "another image called runtime", body: "FROM mcr.microsoft.com/dotnet/runtime:8.0\n", known: true, want: ""},
		{name: "digest pin", body: "FROM astrocrpublic.azurecr.io/runtime@sha256:abc\n", known: true, want: ""},
		{name: "not a runtime image", body: "FROM apache/airflow:3.0.1\n", known: true, want: ""},
		{name: "build argument", body: "ARG BASE=astrocrpublic.azurecr.io/runtime:3.3-8\nFROM ${BASE}\n", known: false, want: ""},
		{name: "no FROM", body: "RUN echo\n", known: false, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Dockerfile")
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o600))
			base := ReadDeclaredBase(path)
			assert.Equal(t, tt.known, base.Known)
			assert.Equal(t, tt.want, base.RuntimeVersion())
		})
	}
	assert.False(t, ReadDeclaredBase("").Known, "no path is no Dockerfile")
	assert.False(t, ReadDeclaredBase(filepath.Join(t.TempDir(), "missing")).Known)
}
