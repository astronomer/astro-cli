package astro

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

// A Deployment given by ID is one GET on the wire: the resolver reads it to
// learn its Workspace, and the command goes on with that Deployment, so the
// Workspace is not listed and the Deployment is not read a second time. The
// fake server counts what a real client sends.
func TestADeploymentGivenByIDIsOneRequest(t *testing.T) {
	d := wqDeployment()
	body, err := json.Marshal(d)
	require.NoError(t, err)

	var (
		mu       sync.Mutex
		requests []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/organizations/env-org/deployments/"+wqDeploymentID {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	t.Cleanup(srv.Close)

	for name, args := range map[string][]string{
		"open":    {"open", wqDeploymentID, "-o", "json"},
		"inspect": {"inspect", wqDeploymentID, "-o", "json"},
	} {
		t.Run(name, func(t *testing.T) {
			orgTokenNoWorkspace(t)
			astroV1Client = astrov1.NewV1ClientForLogin(httputil.NewHTTPClient(), "Bearer token", srv.URL+"/v1")
			mu.Lock()
			requests = nil
			mu.Unlock()

			_, err := execDeploymentCmd(args...)
			if name == "open" {
				require.NoError(t, err)
			}

			mu.Lock()
			defer mu.Unlock()
			gets, lists := 0, 0
			for _, r := range requests {
				switch r {
				case "GET /v1/organizations/env-org/deployments/" + wqDeploymentID:
					gets++
				case "GET /v1/organizations/env-org/deployments":
					lists++
				}
			}
			assert.Equal(t, 1, gets, "fetches of the Deployment: %v", requests)
			assert.Zero(t, lists, "lists of its Workspace: %v", requests)
		})
	}
}
