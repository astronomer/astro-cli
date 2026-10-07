package astro

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
)

// Fixtures for the `astro ide project` tests: two Astro IDE projects in the
// test config's Workspace, and a client answering what list, import and
// export ask of the API.

const (
	ideSessionRW = "sess-rw"
	ideSessionRO = "sess-ro"
)

var ideCreated = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func ideProjects() (etl, ml astrov1alpha1.AstroIdeProject) {
	desc := "Nightly loads"
	etl = astrov1alpha1.AstroIdeProject{
		Id: "proj-etl", Name: "ETL", Description: &desc,
		OrganizationId: "test-org-id", WorkspaceId: curWorkspaceID,
		CreatedAt: ideCreated, UpdatedAt: ideCreated.Add(time.Hour),
	}
	ml = astrov1alpha1.AstroIdeProject{
		Id: "proj-ml", Name: "ML",
		OrganizationId: "test-org-id", WorkspaceId: curWorkspaceID,
		CreatedAt: ideCreated, UpdatedAt: ideCreated,
	}
	return etl, ml
}

// ideArchive is the tar.gz an IDE session exports: a DAG and a requirements
// file, 20 bytes between them.
func ideArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "dags", Typeflag: tar.TypeDir, Mode: 0o755}))
	for name, body := range map[string]string{"dags/etl.py": "print('etl')\n", "requirements.txt": "pandas\n"} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// ideMock is the alpha client: projects listed and each one got, an export
// of any session answering ideArchive, and whatever more adds. A session
// created read-write is ideSessionRW, read-only ideSessionRO.
func ideMock(t *testing.T, projects []astrov1alpha1.AstroIdeProject, more ...func(m *astrov1alpha1_mocks.ClientWithResponsesInterface)) *astrov1alpha1_mocks.ClientWithResponsesInterface {
	t.Helper()
	if projects == nil {
		projects = []astrov1alpha1.AstroIdeProject{}
	}
	m := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	m.On("ListAstroIdeProjectsWithResponse", mock.Anything, "test-org-id", curWorkspaceID, mock.Anything).Return(&astrov1alpha1.ListAstroIdeProjectsResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1alpha1.AstroIdeProjectsPaginated{Projects: projects, TotalCount: len(projects), Limit: 1000},
	}, nil).Maybe()
	for i := range projects {
		p := projects[i]
		m.On("GetAstroIdeProjectWithResponse", mock.Anything, "test-org-id", curWorkspaceID, p.Id).Return(&astrov1alpha1.GetAstroIdeProjectResponse{HTTPResponse: ok200(), JSON200: &p}, nil).Maybe()
	}
	m.On("ExportAstroIdeSessionTar", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(http.StatusOK, ideArchive(t)).Maybe()
	for _, f := range more {
		f(m)
	}
	return m
}

// ideExporter is the session export the import streams, answered by the
// mock's "ExportAstroIdeSessionTar" expectation with a status and a body: the
// generated mock has only the *WithResponse calls.
type ideExporter struct {
	*astrov1alpha1_mocks.ClientWithResponsesInterface
}

func (c ideExporter) ExportAstroIdeSessionTar(ctx context.Context, org, ws, project, session string, params *astrov1alpha1.ExportAstroIdeSessionTarParams, _ ...astrov1alpha1.RequestEditorFn) (*http.Response, error) {
	ret := c.Called(ctx, org, ws, project, session, params)
	return &http.Response{StatusCode: ret.Int(0), Body: io.NopCloser(bytes.NewReader(ret.Get(1).([]byte)))}, nil
}

// opensSession answers a session created on any project with perm, and an
// upgrade to read-write with ideSessionRW.
func opensSession(perm astrov1alpha1.CreateAstroIdeSessionPermission) func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
		isRW := func(b astrov1alpha1.CreateAstroIdeSessionJSONRequestBody) bool {
			return b.Permission != nil && *b.Permission == astrov1alpha1.CreateAstroIdeSessionRequestPermissionREADWRITE
		}
		isRO := func(b astrov1alpha1.CreateAstroIdeSessionJSONRequestBody) bool {
			return b.Permission != nil && *b.Permission == astrov1alpha1.CreateAstroIdeSessionRequestPermissionREADONLY
		}
		m.On("CreateAstroIdeSessionWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(isRW)).Return(&astrov1alpha1.CreateAstroIdeSessionResponse{
			HTTPResponse: ok200(), JSON200: &astrov1alpha1.CreateAstroIdeSession{Id: ideSessionRW, Permission: astrov1alpha1.CreateAstroIdeSessionPermissionREADWRITE},
		}, nil).Maybe()
		m.On("CreateAstroIdeSessionWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(isRO)).Return(&astrov1alpha1.CreateAstroIdeSessionResponse{
			HTTPResponse: ok200(), JSON200: &astrov1alpha1.CreateAstroIdeSession{Id: ideSessionRO, Permission: astrov1alpha1.CreateAstroIdeSessionPermissionREADONLY},
		}, nil).Maybe()
		id := ideSessionRW
		if perm == astrov1alpha1.CreateAstroIdeSessionPermissionREADONLY {
			id = ideSessionRO
		}
		unset := func(b astrov1alpha1.CreateAstroIdeSessionJSONRequestBody) bool { return b.Permission == nil }
		m.On("CreateAstroIdeSessionWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(unset)).Return(&astrov1alpha1.CreateAstroIdeSessionResponse{
			HTTPResponse: ok200(), JSON200: &astrov1alpha1.CreateAstroIdeSession{Id: id, Permission: perm},
		}, nil).Maybe()
	}
}

// acceptsUpload answers the upload, the save and the session's downgrade.
func acceptsUpload(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	m.On("ImportAstroIdeSessionTarWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, "application/gzip", mock.Anything).Return(&astrov1alpha1.ImportAstroIdeSessionTarResponse{HTTPResponse: ok200()}, nil).Maybe()
	m.On("SaveAstroIdeSessionWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.SaveAstroIdeSessionResponse{HTTPResponse: ok200()}, nil).Maybe()
	m.On("UpdateAstroIdeSessionWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.UpdateAstroIdeSessionResponse{HTTPResponse: ok200()}, nil).Maybe()
}

// createsProject answers a project create with made.
func createsProject(made astrov1alpha1.AstroIdeProject) func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) { //nolint:gocritic // a test fixture
	return func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
		m.On("CreateAstroIdeProjectWithResponse", mock.Anything, "test-org-id", curWorkspaceID, mock.Anything).Return(&astrov1alpha1.CreateAstroIdeProjectResponse{HTTPResponse: ok200(), JSON200: &made}, nil).Once()
		m.On("GetAstroIdeProjectWithResponse", mock.Anything, "test-org-id", curWorkspaceID, made.Id).Return(&astrov1alpha1.GetAstroIdeProjectResponse{HTTPResponse: ok200(), JSON200: &made}, nil).Maybe()
	}
}

// failsList answers the project list with status.
func failsList(status int) func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
		m.ExpectedCalls = nil
		m.On("ListAstroIdeProjectsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListAstroIdeProjectsResponse{
			HTTPResponse: &http.Response{StatusCode: status},
			Body:         []byte(`{"message":"no IDE for you"}`),
		}, nil)
	}
}

// failsDownload answers a session's export with a 500.
func failsDownload(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	kept := m.ExpectedCalls[:0]
	for _, c := range m.ExpectedCalls {
		if c.Method != "ExportAstroIdeSessionTar" {
			kept = append(kept, c)
		}
	}
	m.ExpectedCalls = kept
	m.On("ExportAstroIdeSessionTar", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(http.StatusInternalServerError, []byte(`{"message":"export broke"}`))
}

// createsNothing answers a project create with a 200 and no project.
func createsNothing(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	m.On("CreateAstroIdeProjectWithResponse", mock.Anything, "test-org-id", curWorkspaceID, mock.Anything).Return(&astrov1alpha1.CreateAstroIdeProjectResponse{HTTPResponse: ok200()}, nil).Once()
}

// failsUpload answers the archive's upload with a 500.
func failsUpload(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	m.On("ImportAstroIdeSessionTarWithBodyWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, "application/gzip", mock.Anything).Return(&astrov1alpha1.ImportAstroIdeSessionTarResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
		Body:         []byte(`{"message":"upload broke"}`),
	}, nil)
}

// cannotRead answers a read of project id with a 404, as for a project
// deleted while the run worked on it.
func cannotRead(id string) func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
	return func(m *astrov1alpha1_mocks.ClientWithResponsesInterface) {
		m.On("GetAstroIdeProjectWithResponse", mock.Anything, mock.Anything, mock.Anything, id).Return(&astrov1alpha1.GetAstroIdeProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
			Body:         []byte(`{"message":"no such project"}`),
		}, nil)
	}
}

// ideWorkspaceMock is the v1 client: the Workspace a create names.
func ideWorkspaceMock(t *testing.T) astrov1.APIClient {
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("GetWorkspaceWithResponse", mock.Anything, "test-org-id", curWorkspaceID).Return(&astrov1.GetWorkspaceResponse{
		HTTPResponse: ok200(), JSON200: &astrov1.Workspace{Id: curWorkspaceID, Name: "Development"},
	}, nil).Maybe()
	return m
}

// ideDir is the directory a run imports into or exports from: a new
// temporary one, made current for the test, holding files (path to body).
func ideDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	t.Chdir(dir)
	return dir
}

// execIDECmd runs `astro ide <args>` in dir, with alpha as the IDE client.
func execIDECmd(t *testing.T, alpha *astrov1alpha1_mocks.ClientWithResponsesInterface, answers string, args ...string) tokenRun {
	t.Helper()
	prev := astroV1Alpha1Client
	prevExporter := astroIDEExporter
	astroV1Alpha1Client, astroIDEExporter = alpha, ideExporter{alpha}
	t.Cleanup(func() { astroIDEExporter = prevExporter })
	t.Cleanup(func() { astroV1Alpha1Client = prev })
	return execAstroCmd(t, ideWorkspaceMock(t), answers, newIDECommand, append([]string{"ide"}, args...)...)
}
