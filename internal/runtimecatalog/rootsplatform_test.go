package runtimecatalog

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Astronomer's platform packages are resolved with the candidates, so what
// they alone pull in is theirs, and no root: kubernetes, here, which only the
// Kubernetes executor needs. The platform package itself is no root either.
func TestRuntimeRootsLeavesOutWhatOnlyAPlatformPackagePullsIn(t *testing.T) {
	rootsServer(t, map[string]string{})
	freeze := append(slices.Clone(freeze339), "kubernetes==31.0.0")
	var compiled []string
	f := &fakeUV{answer: func(input []string, python, platform string, annotated bool, held []string) (string, error) {
		if annotated {
			compiled = slices.Clone(input)
			return "statsd==4.0.1                # via -c constraints.txt\n" +
				"kubernetes==31.0.0           # via astronomer-kubernetes-executor, -c constraints.txt\n" +
				"astronomer-kubernetes-executor==10.22.0+astro.1  # via -c constraints.txt\n", nil
		}
		return answers339(input, python, platform, false, held)
	}}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze)
	require.NoError(t, err)
	assert.Contains(t, compiled, "astronomer-kubernetes-executor", "the platform package is resolved")
	assert.NotContains(t, got.Names, "kubernetes")
	assert.NotContains(t, got.Names, "astronomer-kubernetes-executor")
	assert.Contains(t, got.Names, "statsd")
}

// A platform package no index serves leaves the resolution without them,
// rather than failing it.
func TestRuntimeRootsResolvesWithoutAPlatformPackageItCannotFind(t *testing.T) {
	rootsServer(t, map[string]string{})
	compiles := 0
	f := &fakeUV{answer: func(input []string, python, platform string, annotated bool, held []string) (string, error) {
		if annotated {
			compiles++
			if slices.Contains(input, "astronomer-kubernetes-executor") {
				return "", noWheel("astronomer-kubernetes-executor")
			}
			return "statsd==4.0.1                # via -c constraints.txt\n", nil
		}
		return answers339(input, python, platform, false, held)
	}}
	f.install(t)
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze339)
	require.NoError(t, err)
	assert.Equal(t, 2, compiles)
	assert.Contains(t, got.Names, "statsd")
	assert.NotContains(t, got.Names, "astronomer-kubernetes-executor")
}

// Running out of retries says nothing for good: a root that keeps pulling in
// another source-only package is kept out only for a while.
func TestRuntimeRootsKeepsARunOutOfRetriesForAWhile(t *testing.T) {
	rootsServer(t, map[string]string{published339: "manysources\n"})
	f := &fakeUV{answer: func(input []string, _, _ string, _ bool, held []string) (string, error) {
		if slices.Contains(input, "apache-airflow") {
			return "", nil
		}
		if input[0] != "manysources" {
			// Each source-only package has no wheel for any Python or platform.
			return "", noWheel(input[0])
		}
		for i := range rootsRetries + 1 {
			name := "src" + string(rune('a'+i))
			if slices.Contains(held, name) {
				return "", noWheel(name)
			}
		}
		return "manysources==1.0\n", nil
	}}
	f.install(t)
	freeze := []string{"apache-airflow==3.3.2+astro.2", "manysources==1.0"}
	for i := range rootsRetries + 1 {
		freeze = append(freeze, "src"+string(rune('a'+i))+"==1.0")
	}
	got, err := RuntimeRoots(context.Background(), "3.3-9", "3.14", freeze)
	require.NoError(t, err)
	require.Len(t, got.LeftOut, 1)
	answer := checkAnswer(t, "manysources@"+rootsPlatforms[0].uv)
	assert.True(t, strings.Contains(answer, "expires"), "an answer of run-out retries is kept for good: %q", answer)
}
