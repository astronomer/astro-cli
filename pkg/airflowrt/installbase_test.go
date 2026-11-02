package airflowrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstallBaseRef(t *testing.T) {
	for data, want := range map[string]string{
		"FROM localhost/astro-base\n":                                                   "localhost/astro-base",
		"FROM localhost/astro-base:abc123\nRUN x\n":                                     "localhost/astro-base:abc123",
		"FROM python:3.12 AS builder\nFROM --platform=linux/amd64 localhost/astro-base": "localhost/astro-base",
		"FROM localhost/astro-base AS b\nFROM b\n":                                      "localhost/astro-base",
		"FROM localhost/astro-base\nFROM astrocrpublic.azurecr.io/runtime:3.3-8\n":      "",
		"FROM localhost/my-project-base\n":                                              "",
		"FROM localhost:5000/astro-base\n":                                              "",
		"FROM localhost/astro-base@sha256:abc\n":                                        "",
	} {
		ref, ok := InstallBaseRef([]byte(data))
		assert.Equal(t, want, ref, data)
		assert.Equal(t, want != "", ok, data)
		assert.Equal(t, want != "", UsesInstallBase([]byte(data)), data)
	}
}

func TestReplaceFrom(t *testing.T) {
	in := "# c\nFROM python:3.12 AS builder\nFROM --platform=linux/amd64 localhost/astro-base AS final\nRUN x\n"
	assert.Equal(t, "# c\nFROM python:3.12 AS builder\nFROM --platform=linux/amd64 localhost/astro-base:abc AS final\nRUN x\n",
		string(ReplaceFrom([]byte(in), "localhost/astro-base", "localhost/astro-base:abc")))
}
