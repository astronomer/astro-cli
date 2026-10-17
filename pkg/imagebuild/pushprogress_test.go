package imagebuild

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
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

// writes records each Write it is given, as one string.
type writes struct {
	mu  sync.Mutex
	got []string
}

func (w *writes) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.got = append(w.got, string(b))
	return len(b), nil
}

func (w *writes) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.got, "")
}

// Docker's stdout is Out itself, so a terminal there gets docker's live
// progress, and its stderr is kept apart, for the error alone.
func TestPushDockerHandsOutToItsStdout(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { r.Close(); w.Close() })
	var gotOut, gotErr io.Writer
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": func(_ []string, s rt.Stdio) error { fmt.Fprint(s.Out, "Docker version 28.5.1\n"); return nil },
		"push": func(_ []string, s rt.Stdio) error {
			gotOut, gotErr = s.Out, s.Err
			return nil
		},
	}}
	_, err = testEngine(cmd, "docker").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Out: w})
	require.NoError(t, err)
	assert.Same(t, w, gotOut, "docker's stdout is the caller's file, not a pipe")
	assert.NotEqual(t, gotErr, io.Writer(w), "docker's stderr does not reach Out")
}

// A docker push that fails says why in its error, not in Out as well.
func TestPushDockerErrorIsNotRepeatedInOut(t *testing.T) {
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
	assert.True(t, errorChainContains(err, "denied: 403 Forbidden"))
	assert.Equal(t, "The push refers to repository [r.example.com/a]\n", out.String())
}

// Podman's progress is on its stderr, and reaches Out a line at a time, an
// update redrawn with '\r' as its own write.
func TestPushPodmanStreamsStderrToOut(t *testing.T) {
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": podmanVersion,
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, podmanPushProgress[:20])
			fmt.Fprint(s.Err, podmanPushProgress[20:])
			fmt.Fprint(s.Err, "Copying blob 10%\rCopying blob 50%\rCopying blob done\n")
			return nil
		},
	}}
	out := &writes{}
	_, err := testEngine(cmd, "podman").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Out: out})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"Getting image source signatures\n",
		"Copying blob sha256:aaa\n",
		"Copying config sha256:bbb\n",
		"Writing manifest to image destination\n",
		"Copying blob 10%\r",
		"Copying blob 50%\r",
		"Copying blob done\n",
	}, out.got)
}

// A failed podman push still reads the registry's answer off stderr, which
// reaches Out too, and neither carries the credential in any form the
// engine might echo it in.
func TestPushPodmanFailureStreamsAndRedacts(t *testing.T) {
	authEntry := base64.StdEncoding.EncodeToString([]byte("cli:s3cret-token"))
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": podmanVersion,
		"push": func(_ []string, s rt.Stdio) error {
			fmt.Fprint(s.Err, "Getting image source signatures\n")
			fmt.Fprintf(s.Err, "auth %s\n", authEntry)
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
	assert.True(t, errorChainContains(err, "auth [redacted]"), err.Error())
	assert.Equal(t, "Getting image source signatures\n"+
		"auth [redacted]\n"+
		"Error: writing blob: 401 Unauthorized [redacted]", out.String())
	for _, s := range []string{"s3cret", authEntry} {
		assert.NotContains(t, out.String(), s, "the credential never reaches Out")
		assert.NotContains(t, err.Error(), s)
	}
}

// A line too long to hold is written at the cap, but what may be the start
// of a secret is held back, so a secret split across the cut is redacted.
func TestRedactingLinesHoldsASplitSecretAtTheCap(t *testing.T) {
	secrets := secretForms("cli", "s3cret-token")
	var out strings.Builder
	l := newRedactingLines(&out, secrets)
	_, err := l.Write([]byte(strings.Repeat("x", maxProgressLine-4) + "s3cre"))
	require.NoError(t, err)
	assert.NotEmpty(t, out.String(), "the cap forced a write")
	_, err = l.Write([]byte("t-token done\n"))
	require.NoError(t, err)
	l.Flush()
	assert.NotContains(t, out.String(), "s3cre")
	assert.True(t, strings.HasSuffix(out.String(), "x[redacted] done\n"), out.String()[len(out.String())-30:])
	assert.Equal(t, maxProgressLine-4+len("[redacted] done\n"), out.Len())
}

func TestSecretForms(t *testing.T) {
	assert.Nil(t, secretForms("cli", ""))
	assert.ElementsMatch(t, []string{
		base64.StdEncoding.EncodeToString([]byte("cli:Bearer tok")),
		base64.StdEncoding.EncodeToString([]byte("cli:tok")),
		"Bearer tok",
		"tok",
	}, secretForms("cli", "Bearer tok"))
	assert.Equal(t, "a [redacted] b [redacted]", redact("a Bearer tok b "+base64.StdEncoding.EncodeToString([]byte("cli:tok")), secretForms("cli", "Bearer tok")))
}

// Podman's stdout and stderr are copied on separate goroutines into the one
// Out; every stderr line arrives whole, and no write races another.
func TestPushPodmanStdoutAndStderrShareOut(t *testing.T) {
	const n = 200
	cmd := &engineCmd{reply: map[string]func([]string, rt.Stdio) error{
		"--version": podmanVersion,
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
	_, err := testEngine(cmd, "podman").Push(context.Background(), PushRequest{Image: "r.example.com/a:t", Out: &out})
	require.NoError(t, err)
	// Stdout is passed through as podman writes it, so a stderr line may
	// follow a part of one; each stderr line is still one unbroken write.
	got, pos := out.String(), 0
	for i := range n {
		line := fmt.Sprintf("err line %d\n", i)
		idx := strings.Index(got[pos:], line)
		require.GreaterOrEqual(t, idx, 0, "stderr line %d arrives whole, in order", i)
		pos += idx + len(line)
	}
}
