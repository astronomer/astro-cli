package imagebuild

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// podmanPushProgress is what podman writes to stderr as it pushes: its
// progress, where docker writes its own to stdout.
const podmanPushProgress = "Getting image source signatures\n" +
	"Copying blob sha256:aaa\n" +
	"Copying config sha256:bbb\n" +
	"Writing manifest to image destination\n"

func podmanVersion(_ []string, s rt.Stdio) error {
	fmt.Fprint(s.Out, "podman version 5.6.0\n")
	return nil
}

// Podman's progress is on its stderr, and reaches Out as docker's does.
func TestPushStreamsTheEnginesStderrToOut(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": podmanVersion,
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, podmanPushProgress)
			return nil
		},
	}}
	var out strings.Builder
	_, err := testEngine(cmd, "podman").Push(context.Background(), PushRequest{
		Image:    "images.example.com/org/dep:deploy-1",
		Username: "cli",
		Secret:   "Bearer s3cret-token",
		Out:      &out,
	})
	require.NoError(t, err)
	assert.Equal(t, podmanPushProgress, out.String())
}

// A failed push still reads the registry's answer off stderr, which reaches
// Out too, and neither carries the secret, even when the engine echoes it
// across two writes.
func TestPushFailureStreamsAndKeepsStderr(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": podmanVersion,
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "Getting image source signatures\n")
			fmt.Fprint(s.Out, "echoed s3cret-token\n")
			fmt.Fprint(s.Err, "Error: writing blob: 401 Unauthorized s3cret-to")
			fmt.Fprint(s.Err, "ken")
			return errors.New("exit status 125")
		},
	}}
	var out strings.Builder
	_, err := testEngine(cmd, "podman").Push(context.Background(), PushRequest{
		Image:    "images.example.com/org/dep:deploy-1",
		Username: "cli",
		Secret:   "Bearer s3cret-token",
		Out:      &out,
	})
	var forbidden *PushForbiddenError
	require.ErrorAs(t, err, &forbidden)
	assert.True(t, forbidden.WithCredential)
	assert.True(t, errorChainContains(err, "401 Unauthorized [redacted]"), err.Error())
	assert.NotContains(t, err.Error(), "s3cret")
	assert.Equal(t, "Getting image source signatures\n"+
		"echoed [redacted]\n"+
		"Error: writing blob: 401 Unauthorized [redacted]", out.String())
	assert.NotContains(t, out.String(), "s3cret", "the secret never reaches Out")
}

// Docker's 403 on stderr is still a *PushForbiddenError on the stored
// credential, and its stdout progress reaches Out as before.
func TestPushDockerProgressAndForbiddenFromStderr(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Out, "The push refers to repository [r.example.com/a]\n")
			fmt.Fprint(s.Err, "denied: 403 Forbidden\n")
			return errors.New("exit status 1")
		},
	}}
	var out strings.Builder
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Out: &out})
	var forbidden *PushForbiddenError
	require.ErrorAs(t, err, &forbidden)
	assert.False(t, forbidden.WithCredential)
	assert.Equal(t, "The push refers to repository [r.example.com/a]\ndenied: 403 Forbidden\n", out.String())
}

// The engine's stdout and stderr are written from separate goroutines, as
// exec does; every line reaches Out whole, never spliced into another.
func TestPushInterleavesWholeLines(t *testing.T) {
	const n = 200
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"push": func(_ []string, s rt.Stdio) error {
			var wg sync.WaitGroup
			for name, w := range map[string]io.Writer{"out": s.Out, "err": s.Err} {
				wg.Go(func() {
					for i := range n {
						// Each line in two writes, so a splice would show.
						fmt.Fprintf(w, "%s line ", name)
						fmt.Fprintf(w, "%d\n", i)
					}
				})
			}
			wg.Wait()
			return nil
		},
	}}
	var out strings.Builder
	_, err := testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Out: &out})
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	require.Len(t, lines, 2*n)
	next := map[string]int{}
	for _, l := range lines {
		var name string
		var i int
		_, err := fmt.Sscanf(l, "%s line %d", &name, &i)
		require.NoError(t, err, l)
		assert.Equal(t, next[name], i, l)
		next[name] = i + 1
	}
}
