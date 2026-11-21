package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateFileBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("creates a file bundle", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.Type == astrov1alpha1.CreateBundleRequestTypeFILE && req.IsDagBundle != nil && *req.IsDagBundle && req.Name != nil && *req.Name == "my-files"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-3"},
		}, nil).Once()

		err := CreateBundle("my-files", "", "file", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, nil, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Created bundle bundle-3")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects a non-Dag file bundle", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("", "/usr/local/airflow/dbt", "file", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: &bytes.Buffer{}}, nil, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errFileBundleNonDag)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

	s.Run("rejects an unknown type", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("my-dags", "", "git", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: &bytes.Buffer{}}, nil, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, `invalid bundle --type "git"`)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})
}

func (s *Suite) TestBundleVersionCell() {
	fileDigest := "bae1a0bfa8d594288e0a946ca8a5f7e0a4e7f0de966f71b3d7a803743484a3cc"
	olderDigest := "4cd5070c1600b9ec65e3e244f2a32bd636e0f74357ea48e48cfa8fe559f6277d"
	deployVersion := "2026-10-21T18:11:45.901Z"
	olderDeployVersion := "2026-10-20T09:00:00.000Z"
	empty := ""
	pending := true

	cases := []struct {
		name   string
		bundle BundleInfo
		want   string
	}{
		{"never written", BundleInfo{}, notApplicable},
		{"empty desired", BundleInfo{DesiredVersion: &empty}, notApplicable},
		{"deletion pending", BundleInfo{DeletionIsPending: &pending, DesiredVersion: &fileDigest}, "Deletion pending"},
		{"running version unreported", BundleInfo{DesiredVersion: &fileDigest}, fileDigest},
		{"settled", BundleInfo{DesiredVersion: &fileDigest, CurrentVersion: &fileDigest}, fileDigest},
		{"file bundle rolling out", BundleInfo{DesiredVersion: &fileDigest, CurrentVersion: &olderDigest}, olderDigest + " → " + fileDigest + " (rolling out)"},
		{"deploy bundle rolling out", BundleInfo{DesiredVersion: &deployVersion, CurrentVersion: &olderDeployVersion}, olderDeployVersion + " → " + deployVersion + " (rolling out)"},
	}
	for i := range cases {
		tc := &cases[i]
		s.Run(tc.name, func() {
			s.Equal(tc.want, bundleVersionCell(&tc.bundle))
		})
	}
}

// bundleVersionsPage is one page of a bundle's history, holding the given versions.
func bundleVersionsPage(totalCount int, versions ...string) *astrov1alpha1.ListBundleVersionsResponse {
	page := &astrov1alpha1.BundleVersionsPaginated{TotalCount: totalCount}
	for _, v := range versions {
		page.Versions = append(page.Versions, astrov1alpha1.BundleVersion{Version: v, Action: astrov1alpha1.BundleVersionActionWRITEFILE})
	}
	return &astrov1alpha1.ListBundleVersionsResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: page}
}

func (s *Suite) TestListBundleVersions() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("stops at the end of the history", func() {
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleVersionsParams) bool {
			return *p.Limit == 20 && *p.Offset == 0
		})).Return(bundleVersionsPage(2, "v2", "v1"), nil).Once()

		list, err := listBundleVersionsData(context.Background(), "bundle-1", "", "", 20, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Len(list.Versions, 2)
		s.Equal("WRITE_FILE", list.Versions[0].Action)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("pages past the API's page size up to the limit", func() {
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		firstPage := make([]string, bundleVersionListLimit)
		for i := range firstPage {
			firstPage[i] = "old"
		}
		mockV1Alpha1Client.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleVersionsParams) bool {
			return *p.Limit == bundleVersionListLimit && *p.Offset == 0
		})).Return(bundleVersionsPage(5000, firstPage...), nil).Once()
		mockV1Alpha1Client.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleVersionsParams) bool {
			return *p.Limit == 1 && *p.Offset == bundleVersionListLimit
		})).Return(bundleVersionsPage(5000, "oldest"), nil).Once()

		list, err := listBundleVersionsData(context.Background(), "bundle-1", "", "", bundleVersionListLimit+1, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Len(list.Versions, bundleVersionListLimit+1)
		s.Equal("oldest", list.Versions[bundleVersionListLimit].Version)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects a limit below 1", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		_, err := listBundleVersionsData(context.Background(), "bundle-1", "", "", 0, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleVersionLimit)
	})

	s.Run("requires exactly one identifier", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		_, err := listBundleVersionsData(context.Background(), "bundle-1", "my-dags", "", 20, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleSelector)
	})

	s.Run("stops when the context is canceled", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := listBundleVersionsData(ctx, "bundle-1", "", "", 20, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, context.Canceled)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "ListBundleVersionsWithResponse")
	})

	s.Run("publishes the versions under their snake_case keys", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(bundleVersionsPage(1, "v1"), nil).Once()

		err := ListBundleVersionsWithFormat(context.Background(), "bundle-1", "", "", 20, ws, testBundleDeploymentID, testUtil.Renderer{JSON: true, Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		var got struct {
			Versions []map[string]any `json:"versions"`
		}
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Require().Len(got.Versions, 1)
		s.Equal("v1", got.Versions[0]["version"])
		s.Contains(got.Versions[0], "created_at")
	})
}

func (s *Suite) TestRollbackBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mountPathBundle := func() *astrov1alpha1_mocks.ClientWithResponsesInterface {
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		isDag := false
		mountPath := "/usr/local/airflow/dbt"
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-3", IsDagBundle: &isDag, NonDagMountPath: &mountPath}},
			},
		}, nil).Once()
		return mockV1Alpha1Client
	}

	s.Run("rolls back a non-Dag bundle resolved by mount path", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := mountPathBundle()
		mockV1Alpha1Client.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-3", mock.MatchedBy(func(req astrov1alpha1.RollbackBundleRequest) bool {
			return req.Version == "old" && req.Description == nil
		})).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-3"},
		}, nil).Once()

		err := RollbackBundle(context.Background(), "", "", "/usr/local/airflow/dbt", "old", "", ws, testBundleDeploymentID, true, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Rolled back bundle bundle-3 on Deployment test-id-1 to version old")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("publishes the rollback", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := mountPathBundle()
		mockV1Alpha1Client.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-3", mock.Anything).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		err := RollbackBundle(context.Background(), "", "", "/usr/local/airflow/dbt", "old", "undo", ws, testBundleDeploymentID, true, testUtil.Renderer{JSON: true, Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		var got BundleRollback
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal(BundleRollback{ID: "bundle-3", NonDagMountPath: "/usr/local/airflow/dbt", DeploymentID: testBundleDeploymentID, Version: "old", Action: "rolled_back"}, got)
	})

	s.Run("does not roll back when the confirmation is declined", func() {
		defer testUtil.MockUserInput(s.T(), "n")()
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := RollbackBundle(context.Background(), "bundle-1", "", "", "old", "", ws, testBundleDeploymentID, false, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, input.ErrAborted)
		s.Empty(out.String(), "a decline publishes nothing")
		mockV1Alpha1Client.AssertNotCalled(s.T(), "RollbackBundleWithResponse")
	})

	s.Run("surfaces an API error", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
			Body:         []byte(`{"message":"version not found"}`),
		}, nil).Once()

		err := RollbackBundle(context.Background(), "bundle-1", "", "", "missing", "", ws, testBundleDeploymentID, true, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, "version not found")
		s.NotContains(out.String(), "Rolled back")
	})
}
