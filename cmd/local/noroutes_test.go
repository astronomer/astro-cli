package local

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// Without a home directory there is nowhere to keep the routes, and every
// command that needs the runtime says so instead of running against a
// relative routes dir.
func TestNoRoutesRuntimeReportsWhyFromEveryCall(t *testing.T) {
	errNoHome := errors.New("no home")
	var rt Runtime = noRoutesRuntime{err: errNoHome}
	ctx := context.Background()

	_, err := rt.Start(ctx, localrt.Plan{}, localrt.Callbacks{})
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.Attach("p")
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.Stopped(localrt.Plan{})
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.LogSource("p")
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.ReadStatus("p")
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.List()
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.PruneStale()
	assert.ErrorIs(t, err, errNoHome)
	_, err = rt.Reset(ctx, "p")
	assert.ErrorIs(t, err, errNoHome)
}
