package localdocker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// With no routes dir the start still runs Airflow on its localhost port; only
// the proxy route is missing, and the start says why. The stop has no route to
// remove and still takes the project down.
func TestStartWithoutRoutesRunsAndSaysWhy(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	noHome := errors.New("finding the home directory for the proxy routes: no home")
	e.DisableRoutes(noHome)
	p := testPlan(t)

	var lines []string
	af, err := e.Start(context.Background(), p, rt.Callbacks{
		OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) },
	})
	require.NoError(t, err)

	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, 8081, rec.Port)
	assert.Contains(t, strings.Join(lines, "\n"), "could not register the proxy route for "+rec.Hostname+": "+noHome.Error())

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))
	_, err = localstate.Load(p.ProjectPath)
	assert.ErrorIs(t, err, localstate.ErrNotRunning)
}
