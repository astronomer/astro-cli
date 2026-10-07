package astro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execOrganizationCmd(args ...string) (string, error) {
	testUtil.SetupOSArgsForGinkgo()
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestOrganizationRootCommand(t *testing.T) {
	testUtil.SetupOSArgsForGinkgo()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	cmd := newOrganizationCmd(os.Stdout)
	cmd.SetOut(buf)
	_, err := cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "organization")
}

func TestOrganizationList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockOrganizationProduct := astrov1.OrganizationProductHYBRID
	mockOrgsResponse := astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{
				{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
			},
		},
	}

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-org")
	mockV1Client.AssertExpectations(t)
}

func TestOrganizationListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockOrganizationProduct := astrov1.OrganizationProductHYBRID
	mockOrgsResponse := astrov1.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.OrganizationsPaginated{
			Organizations: []astrov1.Organization{
				{Name: "test-org", Id: "test-org-id", Product: &mockOrganizationProduct},
			},
		},
	}

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOrgsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list", "-o", "json"}
	resp, err := execOrganizationCmd(cmdArgs...)
	assert.NoError(t, err)

	var result organization.OrganizationList
	assert.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Len(t, result.Organizations, 1)
	assert.Equal(t, "test-org", result.Organizations[0].Name)
	assert.Equal(t, "test-org-id", result.Organizations[0].ID)
	mockV1Client.AssertExpectations(t)
}

func mockClusterListResponse(clusters []astrov1.Cluster) *astrov1.ListClustersResponse {
	return &astrov1.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ClustersPaginated{
			Clusters:   clusters,
			TotalCount: len(clusters),
		},
	}
}

func TestOrganizationClusterList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	clusters := []astrov1.Cluster{
		{
			Name:          "test-cluster",
			Id:            "test-cluster-id",
			CloudProvider: "AWS",
			Region:        "us-east-1",
			Type:          "DEDICATED",
			Status:        "CREATED",
		},
	}

	t.Run("lists the clusters in the current Organization", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(mockClusterListResponse(clusters), nil).Once()
		astroV1Client = mockV1Client

		resp, err := execOrganizationCmd("cluster", "list")
		assert.NoError(t, err)
		assert.Contains(t, resp, "test-cluster")
		assert.Contains(t, resp, "us-east-1")
		assert.Contains(t, resp, "CREATED")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("json output", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(mockClusterListResponse(clusters), nil).Once()
		astroV1Client = mockV1Client

		resp, err := execOrganizationCmd("cluster", "list", "-o", "json")
		assert.NoError(t, err)

		var result organization.ClusterList
		assert.NoError(t, json.Unmarshal([]byte(resp), &result))
		assert.Len(t, result.Clusters, 1)
		assert.Equal(t, "test-cluster", result.Clusters[0].Name)
		assert.Equal(t, "test-cluster-id", result.Clusters[0].ID)
		assert.Equal(t, "DEDICATED", result.Clusters[0].Type)
		mockV1Client.AssertExpectations(t)
	})

	t.Run("empty list", func(t *testing.T) {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(mockClusterListResponse(nil), nil).Once()
		astroV1Client = mockV1Client

		resp, err := execOrganizationCmd("cluster", "ls")
		assert.NoError(t, err)
		assert.Contains(t, resp, "No clusters found in this Organization")
		mockV1Client.AssertExpectations(t)
	})

	t.Run("api error", func(t *testing.T) {
		errorBody, err := json.Marshal(astrov1.Error{Message: "failed to fetch clusters"})
		assert.NoError(t, err)
		errorResponse := &astrov1.ListClustersResponse{
			HTTPResponse: &http.Response{
				StatusCode: 500,
			},
			Body: errorBody,
		}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(errorResponse, nil).Once()
		astroV1Client = mockV1Client

		_, err = execOrganizationCmd("cluster", "list")
		assert.ErrorContains(t, err, "failed to fetch clusters")
		mockV1Client.AssertExpectations(t)
	})
}

func TestOrganizationSwitch(t *testing.T) {
	t.Run("workspace flag triggers wsSwitch with provided id", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		origOrgSwitch := orgSwitch
		orgSwitch = func(orgName string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) (*organization.Switched, error) {
			return &organization.Switched{Changed: true}, nil
		}
		defer func() { orgSwitch = origOrgSwitch }()

		called := false
		gotID := ""
		origWsSwitch := wsSwitch
		wsSwitch = func(id string, client astrov1.APIClient, out io.Writer) (*workspace.WorkspaceInfo, error) {
			called = true
			gotID = id
			return &workspace.WorkspaceInfo{ID: id, IsCurrent: true}, nil
		}
		defer func() { wsSwitch = origWsSwitch }()

		cmdArgs := []string{"switch", "-w", "ws-test-id"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, "ws-test-id", gotID)
	})

	t.Run("orgSwitch error propagates and wsSwitch not called", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)

		expectedErr := fmt.Errorf("org switch failed")

		origOrgSwitch := orgSwitch
		orgSwitch = func(orgName string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) (*organization.Switched, error) {
			return nil, expectedErr
		}
		defer func() { orgSwitch = origOrgSwitch }()

		calledWs := false
		origWsSwitch := wsSwitch
		wsSwitch = func(id string, client astrov1.APIClient, out io.Writer) (*workspace.WorkspaceInfo, error) {
			calledWs = true
			return &workspace.WorkspaceInfo{}, nil
		}
		defer func() { wsSwitch = origWsSwitch }()

		_, err := execOrganizationCmd("switch", "-w", "ws-test-id")
		assert.ErrorIs(t, err, expectedErr)
		assert.False(t, calledWs)
	})
}

func TestOrganizationExportAuditLogs(t *testing.T) {
	// turn on audit logs
	config.CFG.AuditLogs.SetHomeString("true")
	orig := orgExportAuditLogs
	t.Cleanup(func() { orgExportAuditLogs = orig })
	orgExportAuditLogs = func(astroV1Client astrov1.APIClient, orgName, filePath string, earliest int) (*organization.AuditLogExport, error) {
		return &organization.AuditLogExport{OutputFile: filePath, Days: earliest}, nil
	}

	t.Run("Without params", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	t.Run("with auditLogsOutputFilePath param", func(t *testing.T) {
		cmdArgs := []string{"audit-logs", "export", "--organization-name", "Astronomer", "--output-file", "test.json"}
		_, err := execOrganizationCmd(cmdArgs...)
		assert.NoError(t, err)
	})

	// Delete audit logs exports
	currentDir, _ := os.Getwd()
	files, _ := os.ReadDir(currentDir)
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "audit-logs-") {
			os.Remove(file.Name())
		}
	}
	os.Remove("test.json")
}

// auditLogsMock answers an audit-log export of the test config's
// Organization with body.
func auditLogsMock(t *testing.T, body []byte) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	orgs := []astrov1.Organization{{Id: "test-org-id", Name: "Test Org"}}
	m.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&astrov1.ListOrganizationsResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1.OrganizationsPaginated{Organizations: orgs, TotalCount: len(orgs), Limit: 100},
	}, nil).Once()
	m.On("GetOrganizationAuditLogsWithResponse", mock.Anything, "test-org-id", mock.Anything).Return(&astrov1.GetOrganizationAuditLogsResponse{HTTPResponse: ok200(), Body: body}, nil).Once()
	return m
}

// What an audit-log export prints: in text the two lines it always printed;
// under json the file it wrote, with the note on stderr. -o is the output
// format here as everywhere, so it no longer names the file.
func TestOrganizationAuditLogsExportOutput(t *testing.T) {
	body := []byte(`{"action":"login"}` + "\n")
	args := []string{"organization", "audit-logs", "export"}

	t.Run("text", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "audit.gz")
		m := auditLogsMock(t, body)
		r := execAstroCmd(t, m, "", newOrganizationCmd, append(args, "--output-file", path)...)
		require.NoError(t, r.err)
		assert.Equal(t, "This may take some time depending on how many days are being exported.\nFinished exporting logs to local GZIP file\n", r.stdout)
		assert.FileExists(t, path)
		m.AssertExpectations(t)
	})

	t.Run("json with --output-file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "audit.gz")
		m := auditLogsMock(t, body)
		r := execAstroCmd(t, m, "", newOrganizationCmd, append(args, "--output-file", path, "--include", "7", "-o", "json")...)
		require.NoError(t, r.err)
		info, err := os.Stat(path)
		require.NoError(t, err)
		jsonIs(map[string]any{"output_file": path, "organization_id": "test-org-id", "days": float64(7), "bytes": float64(info.Size())})(t, r.stdout)
		assert.Equal(t, "This may take some time depending on how many days are being exported.\n", r.stderr)
		m.AssertExpectations(t)
	})

	// With no --output-file the export names its own file, and the result is
	// how a script learns which.
	t.Run("json naming no file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		m := auditLogsMock(t, body)
		r := execAstroCmd(t, m, "", newOrganizationCmd, append(args, "-o", "json")...)
		require.NoError(t, r.err)
		var got organization.AuditLogExport
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, "testorg-logs-1-day-"+time.Now().Format("20060102")+".ndjson.gz", got.OutputFile)
		assert.FileExists(t, got.OutputFile)
		m.AssertExpectations(t)
	})

	// A script still passing -o <path> gets a usage error that says where
	// the path goes now, and nothing is exported or written.
	t.Run("-o with a path", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// A client that answers nothing: a usage error makes no request.
		r := execAstroCmd(t, new(astrov1_mocks.ClientWithResponsesInterface), "", newOrganizationCmd, append(args, "-o", "audit.gz")...)
		require.Error(t, r.err)
		assert.Equal(t, cliout.ExitUsage, r.code)
		assert.EqualError(t, r.err, `unknown output format "audit.gz" (supported: text, json); -o is the output format, and --output-file takes the path`)
		assert.Empty(t, r.stdout)
		assert.NoFileExists(t, "audit.gz")
	})
}
