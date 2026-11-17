package otto

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/emenv"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/project"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// readLog records what the fake Environment Manager was asked.
type readLog struct {
	mu         sync.Mutex
	workspaces []string
	domains    []string
}

// fakeWorkspaceReads stands in for the Environment Manager and records the
// workspace each read asks for and the host of the login it is made with.
func fakeWorkspaceReads(t *testing.T) *readLog {
	t.Helper()
	log := &readLog{}
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			log.mu.Lock()
			defer log.mu.Unlock()
			if p, ok := args.Get(2).(*astrov1.ListEnvironmentObjectsParams); ok && p.WorkspaceId != nil {
				log.workspaces = append(log.workspaces, *p.WorkspaceId)
			}
		}).
		Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{}},
		}, nil).Maybe()
	orig := workspaceClients
	workspaceClients = func(l emenv.Login) astrov1.APIClient {
		log.mu.Lock()
		defer log.mu.Unlock()
		log.domains = append(log.domains, l.Domain)
		return mc
	}
	t.Cleanup(func() { workspaceClients = orig })
	return log
}

// Otto's warehouses read the workspace a start reads (emenv.WorkspaceOf): the
// manifest's, else a Workspace or Deployment token's own, on the token run's
// host. A manifest with no workspace must not hide the token's from Otto.
func TestWorkspaceConnectionsUnderAWorkspaceTokenWithNoManifestWorkspace(t *testing.T) {
	const manifestBody = "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
	tests := []struct {
		name        string
		permissions []string
		wantRead    string
	}{
		{"workspace token", []string{"organizationId:org", "workspaceId:ws-from-token"}, "ws-from-token"},
		{"deployment token", []string{"organizationId:org", "workspaceId:ws-from-token", "deploymentId:dep"}, "ws-from-token"},
		{"organization token reads nothing", []string{"organizationId:org"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			t.Cleanup(keyring.MockInit)
			t.Setenv("ASTRO_DOMAIN", "")
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			testUtil.UseAPIToken(t, tt.permissions...)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifestBody), 0o600); err != nil {
				t.Fatal(err)
			}
			log := fakeWorkspaceReads(t)

			if _, err := workspaceConnections(dir); err != nil {
				t.Fatal(err)
			}

			log.mu.Lock()
			defer log.mu.Unlock()
			assertReads(t, log, tt.wantRead)
		})
	}
}

// assertReads checks every read asked for want, on the token run's default
// host, and that there was none when want is empty.
func assertReads(t *testing.T, log *readLog, want string) {
	t.Helper()
	if want == "" {
		if len(log.workspaces) != 0 {
			t.Fatalf("read %v, want no read", log.workspaces)
		}
		return
	}
	if len(log.workspaces) == 0 {
		t.Fatal("Otto skipped the token's workspace")
	}
	for _, ws := range log.workspaces {
		if ws != want {
			t.Errorf("read workspace %q, want %q", ws, want)
		}
	}
	for _, d := range log.domains {
		if d != "astronomer.io" {
			t.Errorf("read on %q, want the token run's default host astronomer.io", d)
		}
	}
}
