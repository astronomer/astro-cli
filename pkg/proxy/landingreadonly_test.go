package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Looking at the proxy's index page must not delete anybody's route.
//
// landingPage listed through ListRoutes, which prunes and then writes the
// pruned list back. The daemon builds its proxy with a store carrying no
// liveness predicate (airflow/proxy's Routes()), so the prune judged a route
// by the pid recorded in it — and that pid is the process that registered the
// route, not the runtime. When the two diverge, which is what happens whenever
// the owner is replaced (Astro Desktop restarting, or a standalone master
// exiting ahead of the group it leads), the row belongs to a project that is
// still serving.
//
// So one GET on http://localhost:6563/ removed it, the hostname stopped
// resolving, and nothing said why. A page that shows things is not allowed to
// delete them.
func TestLandingPageDoesNotDeleteRoutes(t *testing.T) {
	s := testStore(t)

	// Written straight to the file: AddRoute prunes as it writes, so a route
	// this store already considers dead could not be planted through it.
	const ghost = "still-running.localhost"
	require.NoError(t, s.WriteRoutes([]Route{{
		Hostname:   ghost,
		Port:       "8081",
		ProjectDir: t.TempDir(),
		// Nothing owns this. The runtime it fronts is alive; the process that
		// registered the route is not.
		PID: 99999999,
	}}))

	p := NewProxy("6563", s)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", http.NoBody)
	p.handler(httptest.NewRecorder(), req)

	after, err := s.ReadRoutes()
	require.NoError(t, err)

	for _, r := range after {
		if r.Hostname == ghost {
			return
		}
	}
	t.Fatalf("a GET on the landing page deleted %s; routes.json now holds %d row(s)", ghost, len(after))
}

// And it still only shows what is live.
//
// Not writing the prune back is the fix; not pruning at all would be a
// different bug, listing projects that stopped weeks ago as though they were
// up.
func TestLandingPageStillHidesDeadRoutes(t *testing.T) {
	s := testStore(t)
	const ghost = "gone.localhost"
	require.NoError(t, s.WriteRoutes([]Route{{
		Hostname:   ghost,
		Port:       "8081",
		ProjectDir: t.TempDir(),
		PID:        99999999,
	}}))

	p := NewProxy("6563", s)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:6563/", http.NoBody)
	w := httptest.NewRecorder()
	p.handler(w, req)

	if body := w.Body.String(); strings.Contains(body, "gone") {
		t.Errorf("the landing page listed a route whose owner is gone:\n%s", body)
	}
}
