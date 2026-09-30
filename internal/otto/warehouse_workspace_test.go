package otto

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/internal/emenv"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/secrets"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// linkWorkspace writes a manifest linking a workspace in the v2 project at
// dir and answers its reads with conns, as the Environment Manager does: one
// list per object type, connections under CONNECTION.
func (s *ConfigSuite) linkWorkspace(dir string, fail bool, conns ...astrov1.EnvironmentObject) {
	s.T().Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'cmws'\ndomain = 'localhost'\n"
	s.Require().NoError(os.WriteFile(filepath.Join(dir, project.Marker), []byte(body), 0o600))
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	if fail {
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("dial tcp: no route to host"))
	} else {
		for _, typ := range []astrov1.ListEnvironmentObjectsParamsObjectType{astrov1.ENVIRONMENTVARIABLE, astrov1.AIRFLOWVARIABLE, astrov1.CONNECTION} {
			rows := []astrov1.EnvironmentObject{}
			if typ == astrov1.CONNECTION {
				rows = conns
			}
			mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything,
				mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool { return *p.ObjectType == typ })).
				Return(&astrov1.ListEnvironmentObjectsResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusOK},
					JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: rows, TotalCount: len(rows)},
				}, nil)
		}
	}
	orig := workspaceClients
	workspaceClients = func(emenv.Login) astrov1.APIClient { return mc }
	s.T().Cleanup(func() { workspaceClients = orig })
}

func cloudConn(id, password string) astrov1.EnvironmentObject {
	host, login, schema := "db.cloud", "u", "analytics"
	return astrov1.EnvironmentObject{
		ObjectKey:  id,
		ObjectType: astrov1.EnvironmentObjectObjectTypeCONNECTION,
		Connection: &astrov1.EnvironmentObjectConnection{Type: "postgres", Host: &host, Login: &login, Password: &password, Schema: &schema},
	}
}

// The linked workspace's connections join the warehouses at the lowest
// precedence: one only the workspace holds is written, and one the vault also
// holds keeps the vault's credentials.
func (s *ConfigSuite) TestStartAddsWorkspaceConnectionsBelowTheVault() {
	warehouses, vault := s.prepareLaunch()
	cwd := s.chdirV2Project("workspace-project")
	canon, err := localrt.CanonicalPath(cwd)
	s.Require().NoError(err)
	s.putConn(vault, canon, pg("own", "own-pw"))
	s.linkWorkspace(cwd, false, cloudConn("cloud_only", "cloud-pw"), cloudConn("own", "cloud-own-pw"))

	s.start()

	raw, err := os.ReadFile(filepath.Join(warehouses, "warehouse.yml"))
	s.Require().NoError(err)
	doc := map[string]any{}
	s.Require().NoError(yaml.Unmarshal(raw, &doc))
	var names []string
	for k := range doc {
		names = append(names, k)
	}
	s.ElementsMatch([]string{"airflow_own", "airflow_cloud_only"}, names)
	s.NotContains(string(raw), "-pw", "a secret reached warehouse.yml")

	env, err := os.ReadFile(filepath.Join(warehouses, ".env"))
	s.Require().NoError(err)
	s.Contains(string(env), `AIRFLOW_OWN_PASSWORD="own-pw"`)
	s.Contains(string(env), `AIRFLOW_CLOUD_ONLY_PASSWORD="cloud-pw"`)
	s.NotContains(string(env), "cloud-own-pw", "the workspace beat the vault for a connection both hold")
}

// An unreachable workspace leaves its warehouses out and the launch goes on
// with the vault's.
func (s *ConfigSuite) TestStartWithoutTheWorkspaceWhenItCannotBeRead() {
	warehouses, vault := s.prepareLaunch()
	cwd := s.chdirV2Project("offline-project")
	s.putConn(vault, secrets.GlobalScope, pg("everywhere", "everywhere-pw"))
	s.linkWorkspace(cwd, true)

	s.start()

	raw, err := os.ReadFile(filepath.Join(warehouses, "warehouse.yml"))
	s.Require().NoError(err)
	doc := map[string]any{}
	s.Require().NoError(yaml.Unmarshal(raw, &doc))
	s.Contains(doc, "airflow_everywhere")
	s.Len(doc, 1)
}
