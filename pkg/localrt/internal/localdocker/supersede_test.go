package localdocker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// builtImageIDs answers the project's `image inspect` with before for the
// look a start takes before its build, and after for every later one.
func builtImageIDs(before, after string) func(string) ([]byte, error) {
	looks := 0
	return func(call string) ([]byte, error) {
		if !strings.Contains(call, "image inspect") {
			return nil, nil
		}
		looks++
		if looks == 1 {
			return []byte(before + "\n"), nil
		}
		return []byte(after + "\n"), nil
	}
}

func imageRemovals(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.Contains(c, "image rm") {
			out = append(out, c)
		}
	}
	return out
}

// A start whose build moved the project's tag (its dependencies changed)
// removes the image it replaced, once Airflow runs on the new one: without
// it each such start leaves the last image behind untagged.
func TestStartRemovesTheImageItsBuildReplaced(t *testing.T) {
	cmd := &fakeCmd{output: builtImageIDs("sha256:old", "sha256:new")}
	e := testEngine(t, cmd)
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)

	require.Equal(t, []string{"docker image rm sha256:old"}, imageRemovals(cmd.calls),
		"no --force: an image still in use, or shared with another project's tag, stays")
	rm := slices.Index(cmd.calls, "docker image rm sha256:old")
	up := slices.IndexFunc(cmd.calls, func(c string) bool { return strings.Contains(c, "up --detach") })
	assert.Greater(t, rm, up, "the old image goes only after the containers moved off it")
}

// A build the layer cache answered (nothing the image holds changed: a .env
// edit) leaves the tag where it was, and nothing is removed.
func TestStartKeepsAnImageItsBuildDidNotReplace(t *testing.T) {
	cmd := &fakeCmd{output: builtImageIDs("sha256:same", "sha256:same")}
	e := testEngine(t, cmd)
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, imageRemovals(cmd.calls))
}

// A start that never becomes healthy keeps the old image: it is what a
// rolled-back or retried start may need again.
func TestAFailedStartKeepsTheImageItsBuildReplaced(t *testing.T) {
	cmd := &fakeCmd{output: builtImageIDs("sha256:old", "sha256:new")}
	e := testEngine(t, cmd)
	e.health = func(context.Context, []string, time.Duration) error { return errors.New("never healthy") }
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	assert.Empty(t, imageRemovals(cmd.calls))
}
