package airflowrt

import (
	"os"
	"path/filepath"
	"strings"
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
		{name: "digest pin", body: "FROM astrocrpublic.azurecr.io/runtime@sha256:abc\n", known: true, want: ""},
		{name: "tag and digest", body: "FROM astrocrpublic.azurecr.io/runtime:3.3-8@sha256:" + strings.Repeat("a", 64) + "\n", known: true, want: "3.3-8"},
		{name: "not a runtime image", body: "FROM apache/airflow:3.0.1\n", known: true, want: ""},
		{name: "build argument", body: "ARG BASE=astrocrpublic.azurecr.io/runtime:3.3-8\nFROM ${BASE}\n", known: true, want: "3.3-8"},
		{name: "build argument with no default", body: "ARG BASE\nFROM ${BASE}\n", known: false, want: ""},
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

// A FROM built from build arguments is read the way astro's own build, which
// passes no --build-arg, reads it: from the defaults of the ARGs before the
// first FROM, each expanded against the ones before it. What cannot be
// expanded is unknown, with the reason, and never some other base.
func TestReadDeclaredBaseExpandsBuildArguments(t *testing.T) {
	const rt = "astrocrpublic.azurecr.io/runtime"
	for _, tc := range []struct {
		name, body string
		// expanded is the reference built from, "" for an unknown base.
		expanded string
		problem  ArgProblem
	}{
		{name: "braced", body: "ARG BASE=" + rt + ":3.1-1\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},
		{name: "bare", body: "ARG BASE=" + rt + ":3.1-1\nFROM $BASE\n", expanded: rt + ":3.1-1"},
		{name: "double-quoted default", body: "ARG BASE=\"" + rt + ":3.1-1\"\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},
		{name: "the tag only", body: "ARG TAG=3.1-1\nFROM " + rt + ":${TAG}\n", expanded: rt + ":3.1-1"},
		{name: "two arguments", body: "ARG REG=astrocrpublic.azurecr.io\nARG TAG=3.1-1\nFROM ${REG}/runtime:$TAG\n", expanded: rt + ":3.1-1"},
		{name: "an inline default", body: "FROM ${BASE:-" + rt + ":3.1-1}\n", expanded: rt + ":3.1-1"},
		{name: "an inline default overridden", body: "ARG BASE=python:3.12\nFROM ${BASE:-" + rt + ":3.1-1}\n", expanded: "python:3.12"},
		{name: "an inline default for the registry", body: "ARG REG\nFROM ${REG:-astrocrpublic.azurecr.io}/runtime:3.1-1\n", expanded: rt + ":3.1-1"},
		{name: ":+ when set", body: "ARG R=1\nFROM ${R:+" + rt + "}:3.1-1\n", expanded: rt + ":3.1-1"},
		{name: "a stage alias after the reference", body: "ARG BASE=" + rt + ":3.1-1\nFROM ${BASE} AS airflow\nFROM airflow\n", expanded: rt + ":3.1-1"},
		{name: "a lower-case arg", body: "arg BASE=" + rt + ":3.1-1\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},
		{name: "two on one line", body: "ARG REG=astrocrpublic.azurecr.io TAG=3.1-1\nFROM ${REG}/runtime:${TAG}\n", expanded: rt + ":3.1-1"},
		{name: "a later default wins", body: "ARG BASE=python:3.12\nARG BASE=" + rt + ":3.1-1\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},

		// An ARG default naming an earlier ARG, in each form.
		{name: "a default naming an earlier argument", body: "ARG REG=astrocrpublic.azurecr.io\nARG BASE=${REG}/runtime:3.1-1\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},
		{name: "a default naming an earlier argument bare", body: "ARG REG=astrocrpublic.azurecr.io\nARG BASE=$REG/runtime:3.1-1\nFROM $BASE\n", expanded: rt + ":3.1-1"},
		{name: "a chain of three", body: "ARG A=astrocrpublic.azurecr.io\nARG B=${A}/runtime\nARG C=${B}:3.1-1\nFROM ${C}\n", expanded: rt + ":3.1-1"},
		{name: "a default whose inline default names another", body: "ARG REG=astrocrpublic.azurecr.io\nARG BASE=${UNSET:-$REG}/runtime:3.1-1\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},

		// Nested defaults: the brace that closes the reference is the matching
		// one, not the first.
		{name: "a nested default", body: "ARG REG=astrocrpublic.azurecr.io\nFROM ${BASE:-${REG}/runtime:3.1-1}\n", expanded: rt + ":3.1-1"},
		{name: "a nested default two deep", body: "FROM ${BASE:-${REG:-astrocrpublic.azurecr.io}/runtime:${TAG:-3.1-1}}\n", expanded: rt + ":3.1-1"},
		{name: "a nested default overridden", body: "ARG BASE=python:3.12\nARG REG=astrocrpublic.azurecr.io\nFROM ${BASE:-${REG}/runtime:3.1-1}\n", expanded: "python:3.12"},

		// Only the arguments before the first FROM reach a FROM.
		{name: "a stage-scoped argument", body: "FROM python:3.12 AS one\nARG BASE=" + rt + ":3.1-1\nFROM ${BASE}\n", problem: ArgUnset},

		// No default: Docker expands it to empty, and astro's build has none.
		{name: "no default", body: "ARG BASE\nFROM ${BASE}\n", problem: ArgUnset},
		{name: "never declared", body: "FROM ${BASE}\n", problem: ArgUnset},
		{name: "part of the reference unset", body: "ARG TAG\nFROM " + rt + ":${TAG}\n", problem: ArgUnset},
		{name: "a default naming an unset argument", body: "ARG REG\nARG BASE=${REG}/runtime:3.1-1\nFROM ${BASE}\n", problem: ArgUnset},
		{name: ":+ when unset", body: "ARG R\nFROM ${R:+" + rt + ":3.1-1}\n", problem: ArgUnset},

		// Forms not expanded here.
		{name: "a required modifier", body: "ARG BASE=" + rt + ":3.1-1\nFROM ${BASE:?required}\n", problem: ArgUnsupported},
		{name: "a required modifier with no default", body: "FROM ${BASE:?required}\n", problem: ArgUnsupported},
		{name: "a pattern modifier", body: "ARG BASE=" + rt + ":3.1-1\nFROM ${BASE%:*}\n", problem: ArgUnsupported},
		{name: "a dash without a colon", body: "FROM ${BASE-" + rt + ":3.1-1}\n", problem: ArgUnsupported},
		{name: "an unbalanced brace", body: "ARG BASE=" + rt + ":3.1-1\nFROM ${BASE\n", problem: ArgUnsupported},
		{name: "an escaped dollar", body: "FROM \\$BASE\n", problem: ArgUnsupported},
		{name: "a lone dollar", body: "FROM " + rt + ":3.1-1$\n", problem: ArgUnsupported},
		{name: "a default holding an unsupported form", body: "ARG BASE=${X:?no}\nFROM ${BASE:-" + rt + ":3.1-1}\n", problem: ArgUnsupported},
		{name: "a single-quoted default keeps its dollar", body: "ARG REG=astrocrpublic.azurecr.io\nARG BASE='${REG}/runtime:3.1-1'\nFROM ${BASE}\n", problem: ArgUnsupported},
		{name: "an unclosed quote", body: "ARG BASE=\"" + rt + ":3.1-1\nFROM ${BASE}\n", problem: ArgUnsupported},

		// Quotes are removed, as Docker's lexer removes them, and never kept in
		// the image.
		{name: "a double-quoted FROM", body: "ARG BASE=" + rt + ":3.1-1\nFROM \"${BASE}\"\n", expanded: rt + ":3.1-1"},
		{name: "a single-quoted FROM keeps its dollar", body: "ARG BASE=" + rt + ":3.1-1\nFROM '${BASE}'\n", problem: ArgUnsupported},
		{name: "an unclosed quote in the FROM", body: "ARG BASE=" + rt + ":3.1-1\nFROM \"${BASE}\n", problem: ArgUnsupported},
		{name: "a quote inside braces", body: "FROM ${BASE:-\"" + rt + ":3.1-1\"}\n", problem: ArgUnsupported},

		// A later ARG with no default keeps the value an earlier one gave.
		{name: "a later declaration with no default", body: "ARG BASE=" + rt + ":3.1-1\nARG BASE\nFROM ${BASE}\n", expanded: rt + ":3.1-1"},

		// The platform arguments BuildKit sets itself are the build's, not the file's.
		{name: "a target platform argument", body: "FROM " + rt + ":3.1-1-${TARGETARCH}\n", problem: ArgUnsupported},
		{name: "a build platform argument", body: "FROM ${BUILDPLATFORM}/runtime:3.1-1\n", problem: ArgUnsupported},
		{name: "a platform argument with a default", body: "FROM " + rt + ":${TARGETOS:-3.1-1}\n", problem: ArgUnsupported},
		{name: "a platform argument declared", body: "ARG TARGETARCH=amd64\nFROM " + rt + ":3.1-1-${TARGETARCH}\n", problem: ArgUnsupported},

		// Only ${ opens a level, as in Docker's lexer: a lone '{' is a literal
		// character, which no image reference holds.
		{name: "a literal brace in a default", body: "FROM ${BASE:-" + rt + "{:3.1-1}\n", problem: ArgUnsupported},
		{name: "a literal brace after a reference", body: "ARG BASE=" + rt + "\nFROM ${BASE}{:3.1-1}\n", problem: ArgUnsupported},

		// Any argument with no value that reaches the FROM makes it unknown,
		// even where the empty expansion still reads as a reference: an empty
		// suffix is harmless, an empty version digit names an image that is
		// not the one meant, and the file cannot tell the two apart.
		{name: "an unset suffix", body: "ARG SUFFIX\nFROM " + rt + ":3.1-12${SUFFIX}\n", problem: ArgUnset},
		{name: "an unset suffix never declared", body: "FROM " + rt + ":3.1-12$SUFFIX\n", problem: ArgUnset},
		{name: "an unset major", body: "ARG MAJOR\nFROM quay.io/astronomer/astro-runtime:1${MAJOR}.0.0\n", problem: ArgUnset},
		{name: "an unset minor", body: "ARG MINOR\nFROM " + rt + ":3.${MINOR}1-12\n", problem: ArgUnset},
		{name: "an unset patch", body: "ARG PATCH\nFROM quay.io/astronomer/astro-runtime:12.1.${PATCH}0\n", problem: ArgUnset},
		{name: "an unset argument through a default", body: "ARG SUFFIX\nARG BASE=" + rt + ":3.1-12${SUFFIX}\nFROM ${BASE}\n", problem: ArgUnset},
		{name: ":+ on an unset argument beside text", body: "ARG R\nFROM " + rt + ":3.1-12${R:+-python-3.12}\n", problem: ArgUnset},
		{name: ":- on an unset argument is the file's answer", body: "ARG SUFFIX\nFROM " + rt + ":3.1-12${SUFFIX:-}\n", expanded: rt + ":3.1-12"},
		{name: "a set argument that does not make a reference", body: "ARG TAG=3.1-12!\nFROM " + rt + ":${TAG}\n", problem: ArgUnsupported},
		{name: "an unset registry leaves no reference", body: "ARG REG\nFROM $REG/runtime:3.1-12\n", problem: ArgUnset},
		{name: "an unset tag leaves no reference", body: "ARG TAG\nFROM " + rt + ":${TAG}\n", problem: ArgUnset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Dockerfile")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			base := ReadDeclaredBase(path)

			assert.Equal(t, tc.problem, base.ArgProblem)
			assert.Equal(t, tc.expanded, base.Expanded)
			assert.Equal(t, tc.expanded != "", base.Known, "known exactly when expanded")
			assert.True(t, IsUnresolvedRef(base.From), "From is as written: %q", base.From)
			if tc.expanded == "" {
				assert.Empty(t, base.Image, "an unknown base names no image")
				return
			}
			assert.Equal(t, tc.expanded, base.Ref())
		})
	}
}

// A stage named through a build argument is followed to the stage it names,
// because every stage is expanded before aliases are followed.
func TestReadDeclaredBaseFollowsAnAliasNamedByABuildArgument(t *testing.T) {
	const rt = "astrocrpublic.azurecr.io/runtime"
	for _, tc := range []struct{ name, body, want string }{
		{
			name: "an alias with a build argument in it",
			body: "ARG ENV=prod\nFROM " + rt + ":3.1-12 AS build-prod\nFROM python:3.12 AS build-dev\nFROM build-${ENV}\n",
			want: rt + ":3.1-12",
		},
		{
			name: "the other alias",
			body: "ARG ENV=dev\nFROM " + rt + ":3.1-12 AS build-prod\nFROM python:3.12 AS build-dev\nFROM build-${ENV}\n",
			want: "python:3.12",
		},
		{name: "an alias that is a build argument", body: "ARG STAGE=base\nFROM " + rt + ":3.1-12 AS base\nFROM ${STAGE}\n", want: rt + ":3.1-12"},
		{name: "through an expanded stage", body: "ARG BASE=" + rt + ":3.1-12\nARG STAGE=one\nFROM ${BASE} AS one\nFROM ${STAGE}\n", want: rt + ":3.1-12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Dockerfile")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			base := ReadDeclaredBase(path)
			require.True(t, base.Known, "%+v", base)
			assert.Equal(t, tc.want, base.Ref())
		})
	}

	t.Run("an alias argument with no default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Dockerfile")
		require.NoError(t, os.WriteFile(path, []byte("ARG ENV\nFROM "+rt+":3.1-12 AS build-prod\nFROM build-${ENV}\n"), 0o600))
		base := ReadDeclaredBase(path)
		assert.False(t, base.Known)
		assert.Equal(t, ArgUnset, base.ArgProblem)
	})
}

// A digest is not part of the image or the tag: a tag before one is still the
// tag, and a digest alone leaves none.
func TestReadDeclaredBaseSplitsADigest(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct{ body, image, tag string }{
		{body: "FROM astrocrpublic.azurecr.io/runtime:3.1-12" + digest + "\n", image: "astrocrpublic.azurecr.io/runtime", tag: "3.1-12"},
		{body: "FROM astrocrpublic.azurecr.io/runtime" + digest + "\n", image: "astrocrpublic.azurecr.io/runtime", tag: ""},
		{body: "FROM localhost:5000/runtime:3.1-12" + digest + "\n", image: "localhost:5000/runtime", tag: "3.1-12"},
		{body: "FROM localhost:5000/runtime" + digest + "\n", image: "localhost:5000/runtime", tag: ""},
	} {
		path := filepath.Join(t.TempDir(), "Dockerfile")
		require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
		base := ReadDeclaredBase(path)
		require.True(t, base.Known, tc.body)
		assert.Equal(t, tc.image, base.Image, tc.body)
		assert.Equal(t, tc.tag, base.Tag, tc.body)
	}
}

// A quoted FROM with no build argument loses its quotes, as Docker reads it.
func TestReadDeclaredBaseRemovesQuotes(t *testing.T) {
	for _, body := range []string{
		"FROM \"astrocrpublic.azurecr.io/runtime:3.1-12\"\n",
		"FROM 'astrocrpublic.azurecr.io/runtime:3.1-12'\n",
	} {
		path := filepath.Join(t.TempDir(), "Dockerfile")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		base := ReadDeclaredBase(path)
		require.True(t, base.Known, body)
		assert.Equal(t, "astrocrpublic.azurecr.io/runtime", base.Image, body)
		assert.Equal(t, "3.1-12", base.Tag, body)
	}
}

// A FROM with no build argument is its own expansion.
func TestReadDeclaredBaseWithoutBuildArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	require.NoError(t, os.WriteFile(path, []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-1 AS a\nFROM a\n"), 0o600))
	assert.Equal(t, DeclaredBase{
		Image: "astrocrpublic.azurecr.io/runtime", Tag: "3.1-1", Known: true,
		From: "astrocrpublic.azurecr.io/runtime:3.1-1", Expanded: "astrocrpublic.azurecr.io/runtime:3.1-1",
	}, ReadDeclaredBase(path))
}
