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
		// Any order, each once.
		{"12.1.0-python-3.12-slim", "12.1.0", "3.12"},
		{"3.1-12-base-python-3.11", "3.1-12", "3.11"},
		{"13.7.0-slim-python-3.12-base", "13.7.0", "3.12"},
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

// What counts as an Astro Runtime image, and what does not.
//
// The negatives carry the weight. This replaced a `strings.Contains(from,
// "runtime")` check, which accepted three of the cases below — and the compose
// file written for such an image runs Airflow as the `astro` user and reads a
// generation off the tag, neither of which an arbitrary base offers.
func TestIsAstroRuntimeRef(t *testing.T) {
	for _, tc := range []struct {
		image string
		want  bool
	}{
		// Astronomer's published repositories: any tag, or none.
		{"astrocrpublic.azurecr.io/runtime", true},
		{"astrocrpublic.azurecr.io/runtime:3.1-12", true},
		{"astrocrpublic.azurecr.io/runtime:3.1", true},
		{"astrocrpublic.azurecr.io/runtime:latest", true},
		{"astrocrpublic.azurecr.io/runtime:3.1-12-python-3.12", true},
		{"astrocrpublic.azurecr.io/runtime@sha256:abc123", true},
		{"astrocrpublic.azurecr.io/runtime:3.1-12@sha256:abc123", true},
		{"quay.io/astronomer/astro-runtime", true},
		{"quay.io/astronomer/astro-runtime:12.1.0", true},

		// Mirrors: the repository's name on another registry, at any depth or
		// behind a port, with a tag that is an exact Runtime version.
		{"registry.corp.example/astronomer/astro-runtime:12.1.0", true},
		{"registry.corp.example/astronomer/astro-runtime:12.1.0-slim", true},
		{"mirror/runtime:3.1-12", true},
		{"mirror/runtime:3.1-12-python-3.12-base", true},
		// Flavors in any order, as on a published tag.
		{"mirror/astro-runtime:12.1.0-python-3.12-slim", true},
		{"mirror/astro-runtime:12.1.0-slim-python-3.12", true},
		{"mirror/runtime:3.1-12-base-python-3.12", true},
		{"registry.corp.example/a/b/c/runtime:3.1-12", true},
		{"localhost:5000/astro-runtime:3.1-12", true},
		{"quay.io/astronomer-mirror/astro-runtime:13.4.0", true},
		{"mirror/runtime:3.1-12@sha256:abc123", true},
		{"docker.io/myco/astro-runtime:12.1.0", true},

		// The name alone is not enough: other images are called runtime too.
		{"mcr.microsoft.com/dotnet/runtime:8.0", false},
		{"ghcr.io/x/runtime:latest", false},
		{"registry.corp/astro-runtime:stable", false},
		{"mirror/runtime", false},
		{"mirror/runtime:3.1", false},
		{"mirror/astro-runtime:12", false},
		{"mirror/runtime:3.1.0", false},
		{"mirror/runtime@sha256:abc123", false},

		// Docker Hub official images, which no Astronomer image is.
		{"runtime", false},
		{"runtime:3.1-12", false},
		{"astro-runtime:12.1.0", false},
		{"docker.io/runtime:3.1-12", false},
		{"docker.io/library/runtime:3.1-12", false},
		{"library/runtime:3.1-12", false},
		{"index.docker.io/library/astro-runtime:12.1.0", false},

		// A registry or namespace alone is not a repository.
		{"astrocrpublic.azurecr.io", false},
		{"quay.io/astronomer", false},
		{"astrocrpublic.azurecr.io/", false},

		// Other Astronomer images are not Astro Runtime.
		{"quay.io/astronomer/ap-airflow:2.5.1", false},
		{"quay.io/astronomer/astro-runtime-dev:12.1.0", false},

		{"apache/airflow:2.9.3", false},
		{"python", false},
		{"python:3.12-slim", false},
		{"myco/our-runtime:3.1-12", false},
		{"myco/runtime-tools:12.1.0", false},
		{"mirror/Runtime:3.1-12", false},
		{"myco/base:runtime", false},
		{"${REGISTRY}/runtime:3.1-12", false},
		{"mirror/runtime:${TAG}", false},
		{"", false},
	} {
		t.Run(tc.image, func(t *testing.T) {
			image, tag := splitImageRef(tc.image)
			if got := IsAstroRuntimeRef(image, tag); got != tc.want {
				t.Errorf("IsAstroRuntimeRef(%q, %q) = %v, want %v", image, tag, got, tc.want)
			}
		})
	}
}

// The reference shapes a real Dockerfile carries.
//
// Each of these was read wrongly at some point, and each becomes a wrong answer
// rather than a missing one: a caller that refuses a start on what this returns
// turns a misparse into a project that cannot run.
func TestParseDockerfileAtReadsRealReferences(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantImage, wantTag string
	}{
		{
			// The ordinary way to build an amd64 image on Apple silicon.
			name:      "a per-stage platform flag is not the image",
			body:      "FROM --platform=linux/amd64 astrocrpublic.azurecr.io/runtime:3.1-12\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-12",
		},
		{
			name:      "and neither is a templated one",
			body:      "FROM --platform=$BUILDPLATFORM astrocrpublic.azurecr.io/runtime:3.1-12\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-12",
		},
		{
			// docker splits on any run of whitespace; a "FROM " prefix match
			// does not see this line at all.
			name:      "a tab after FROM",
			body:      "FROM\tastrocrpublic.azurecr.io/runtime:3.1-12\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-12",
		},
		{
			// Splitting on the first colon reads the image as "localhost".
			name:      "a registry host carrying a port",
			body:      "FROM localhost:5000/astro-runtime:3.1-12\n",
			wantImage: "localhost:5000/astro-runtime", wantTag: "3.1-12",
		},
		{
			name:      "a port and no tag",
			body:      "FROM localhost:5000/astro-runtime\n",
			wantImage: "localhost:5000/astro-runtime", wantTag: "latest",
		},
		{
			// A digest names an exact image and carries no version. Reporting
			// the hex as a tag reads as Airflow 2 to anything parsing it.
			name:      "a digest reference has no tag",
			body:      "FROM astrocrpublic.azurecr.io/runtime@sha256:abc123\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "",
		},
		{
			// Both: the tag is still what the reference says it is, and the
			// repository is not "runtime:3.1-12".
			name:      "a tag and a digest",
			body:      "FROM localhost:5000/runtime:3.1-12@sha256:abc123\n",
			wantImage: "localhost:5000/runtime", wantTag: "3.1-12",
		},
		{
			name:      "case and alias keywords are not the image",
			body:      "from astrocrpublic.azurecr.io/runtime:3.1-12 as base\n",
			wantImage: "astrocrpublic.azurecr.io/runtime", wantTag: "3.1-12",
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
				t.Fatalf("ParseDockerfileAt: %v", err)
			}
			if image != tc.wantImage || tag != tc.wantTag {
				t.Errorf("ParseDockerfileAt = %q:%q, want %q:%q", image, tag, tc.wantImage, tc.wantTag)
			}
		})
	}
}

// A FROM built from a build argument is no answer at all.
//
// Its value can also arrive from the build command line, so nothing read from
// the file can say what it builds on. A caller that refuses on an unrecognized
// base has to be told that rather than handed "${BASE}".
func TestUnresolvedRefsAreReportedAsSuch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	body := "ARG BASE=astrocrpublic.azurecr.io/runtime:3.1-12\nFROM ${BASE}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	image, _, err := ParseDockerfileAt(path)
	if err != nil {
		t.Fatalf("ParseDockerfileAt: %v", err)
	}
	if !IsUnresolvedRef(image) {
		t.Errorf("IsUnresolvedRef(%q) = false; a caller would judge the literal text", image)
	}
	if IsAstroRuntimeRef(image, "") {
		t.Errorf("%q is not a resolved image reference and must not read as one", image)
	}
	if IsUnresolvedRef("astrocrpublic.azurecr.io/runtime") {
		t.Error("a plain reference must not read as unresolved")
	}
}
