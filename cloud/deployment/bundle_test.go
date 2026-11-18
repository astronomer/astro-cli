package deployment

import (
	"bytes"
	"net/http"

	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/astro-client-v1alpha1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const testBundleDeploymentID = "test-id-1"

// mockGetDeployment wires the two v1 calls GetDeployment makes to resolve a deployment by ID.
func (s *Suite) mockGetDeployment() {
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
}

func (s *Suite) TestCreateBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("requires exactly one of name or mount-path", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("", "", "deploy", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errCreateBundleTarget)

		err = CreateBundle("my-dags", "/mount", "deploy", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errCreateBundleTarget)

		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

	s.Run("creates a DAG bundle", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.IsDagBundle != nil && *req.IsDagBundle && req.Name != nil && *req.Name == "my-dags" && req.NonDagMountPath == nil
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		err := CreateBundle("my-dags", "", "deploy", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Created bundle bundle-1")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("creates a non-DAG bundle", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(req astrov1alpha1.CreateBundleRequest) bool {
			return req.IsDagBundle != nil && !*req.IsDagBundle &&
				req.NonDagMountPath != nil && *req.NonDagMountPath == "/usr/local/airflow/dbt" &&
				req.NonDagBundleType != nil && *req.NonDagBundleType == "dbt" &&
				req.DagBundleIds != nil && len(*req.DagBundleIds) == 1 && (*req.DagBundleIds)[0] == "dag-1"
		})).Return(&astrov1alpha1.CreateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-2"},
		}, nil).Once()

		err := CreateBundle("", "/usr/local/airflow/dbt", "deploy", "dbt", "my dbt project", []string{"dag-1"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Created bundle bundle-2")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects non-DAG flags on a DAG bundle", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("my-dags", "", "deploy", "dbt", "", []string{"dag-1"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errDagBundleNonDagFlags)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

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

		err := CreateBundle("my-files", "", "file", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Created bundle bundle-3")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects a non-Dag file bundle", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("", "/usr/local/airflow/dbt", "file", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errFileBundleNonDag)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

	s.Run("rejects an unknown type", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("my-dags", "", "git", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, `invalid bundle --type "git"`)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

	s.Run("surfaces an API error", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		err := CreateBundle("my-dags", "", "deploy", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errMock)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdateBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("requires at least one field", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := UpdateBundle("bundle-1", "", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errUpdateBundleNoOp)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "UpdateBundleWithResponse")
	})

	s.Run("updates description and DAG bundle associations", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(req astrov1alpha1.UpdateBundleRequest) bool {
			return req.Description != nil && *req.Description == "new desc" &&
				req.DagBundleIds != nil && len(*req.DagBundleIds) == 1 && (*req.DagBundleIds)[0] == "dag-1"
		})).Return(&astrov1alpha1.UpdateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1"},
		}, nil).Once()

		err := UpdateBundle("bundle-1", "", "", "new desc", []string{"dag-1"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Updated bundle bundle-1")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("requires exactly one identifier", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := UpdateBundle("", "", "", "new desc", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleSelector)

		err = UpdateBundle("bundle-1", "my-dags", "", "new desc", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleSelector)

		mockV1Alpha1Client.AssertNotCalled(s.T(), "UpdateBundleWithResponse")
	})

	s.Run("resolves a DAG bundle by name", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		isDag := true
		name := "my-dags"
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-9", Name: &name, IsDagBundle: &isDag}},
			},
		}, nil).Once()
		mockV1Alpha1Client.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-9", mock.Anything).Return(&astrov1alpha1.UpdateBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-9"},
		}, nil).Once()

		err := UpdateBundle("", "my-dags", "", "new desc", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Updated bundle bundle-9")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListBundlesWithFormat() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	bundleName := "main"
	mountPath := "/usr/local/airflow/dbt"
	isDag := true
	fileBundleName := "my-files"
	fileVersion := "4f2a9c"
	listResponse := &astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1alpha1.BundlesPaginated{
			TotalCount: 3,
			Bundles: []astrov1alpha1.DeploymentBundle{
				{Id: "bundle-1", Type: astrov1alpha1.DeploymentBundleTypeDEPLOY, Name: &bundleName, IsDagBundle: &isDag},
				{Id: "bundle-2", Type: astrov1alpha1.DeploymentBundleTypeDEPLOY, NonDagMountPath: &mountPath},
				{Id: "bundle-3", Type: astrov1alpha1.DeploymentBundleTypeFILE, Name: &fileBundleName, IsDagBundle: &isDag, DesiredVersion: &fileVersion},
			},
		},
	}

	s.Run("table format", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(listResponse, nil).Once()

		err := ListBundlesWithFormat(ws, testBundleDeploymentID, "table", "", out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "BUNDLE ID")
		s.Contains(out.String(), "TYPE")
		s.Contains(out.String(), "bundle-1")
		s.Contains(out.String(), "main")
		s.Contains(out.String(), mountPath)
		s.Contains(out.String(), "deploy")
		s.Contains(out.String(), "file")
		s.Contains(out.String(), fileVersion)
		s.NotContains(out.String(), "CURRENT VERSION")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("json format", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(listResponse, nil).Once()

		err := ListBundlesWithFormat(ws, testBundleDeploymentID, "json", "", out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "bundles")
		s.Contains(out.String(), "bundle-1")
		s.Contains(out.String(), "bundle-2")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("pages through all results", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		page1 := &astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 2,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1"}},
			},
		}
		page2 := &astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 2,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-2"}},
			},
		}
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1alpha1.ListBundlesParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == 0
		})).Return(page1, nil).Once()
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(p *astrov1alpha1.ListBundlesParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == 1
		})).Return(page2, nil).Once()

		err := ListBundlesWithFormat(ws, testBundleDeploymentID, "table", "", out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "bundle-1")
		s.Contains(out.String(), "bundle-2")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDeleteBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("deletes with --force", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		err := DeleteBundle("bundle-1", "", "", ws, testBundleDeploymentID, true, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Deleted bundle bundle-1")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("does not delete when the confirmation is declined", func() {
		defer testUtil.MockUserInput(s.T(), "n")()
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := DeleteBundle("bundle-1", "", "", ws, testBundleDeploymentID, false, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Canceling bundle deletion")
		mockV1Alpha1Client.AssertNotCalled(s.T(), "DeleteBundleWithResponse")
	})

	s.Run("resolves a non-DAG bundle by mount path", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mountPath := "/usr/local/airflow/dbt"
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-7", NonDagMountPath: &mountPath}},
			},
		}, nil).Once()
		mockV1Alpha1Client.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-7").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		}, nil).Once()

		err := DeleteBundle("", "", mountPath, ws, testBundleDeploymentID, true, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Deleted bundle bundle-7")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("errors when the selector matches no bundle", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundlesPaginated{TotalCount: 0},
		}, nil).Once()

		err := DeleteBundle("", "missing", "", ws, testBundleDeploymentID, true, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, `no Dag bundle named "missing"`)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "DeleteBundleWithResponse")
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
		bundle astrov1alpha1.DeploymentBundle
		want   string
	}{
		{"never written", astrov1alpha1.DeploymentBundle{}, notApplicable},
		{"empty desired", astrov1alpha1.DeploymentBundle{DesiredVersion: &empty}, notApplicable},
		{"deletion pending", astrov1alpha1.DeploymentBundle{DeletionIsPending: &pending, DesiredVersion: &fileDigest}, "Deletion pending"},
		{"running version unreported", astrov1alpha1.DeploymentBundle{Type: astrov1alpha1.DeploymentBundleTypeFILE, DesiredVersion: &fileDigest}, fileDigest},
		{"settled", astrov1alpha1.DeploymentBundle{Type: astrov1alpha1.DeploymentBundleTypeFILE, DesiredVersion: &fileDigest, CurrentVersion: &fileDigest}, fileDigest},
		{"file bundle rolling out", astrov1alpha1.DeploymentBundle{Type: astrov1alpha1.DeploymentBundleTypeFILE, DesiredVersion: &fileDigest, CurrentVersion: &olderDigest}, "4cd5070c1600 → bae1a0bfa8d5 (rolling out)"},
		{"deploy bundle rolling out", astrov1alpha1.DeploymentBundle{Type: astrov1alpha1.DeploymentBundleTypeDEPLOY, DesiredVersion: &deployVersion, CurrentVersion: &olderDeployVersion}, olderDeployVersion + " → " + deployVersion + " (rolling out)"},
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

func (s *Suite) TestListBundleVersionsData() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("stops at the end of the history", func() {
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundleVersionsWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.MatchedBy(func(p *astrov1alpha1.ListBundleVersionsParams) bool {
			return *p.Limit == 20 && *p.Offset == 0
		})).Return(bundleVersionsPage(2, "v2", "v1"), nil).Once()

		list, err := ListBundleVersionsData("bundle-1", "", "", 20, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Len(list.Versions, 2)
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

		list, err := ListBundleVersionsData("bundle-1", "", "", bundleVersionListLimit+1, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Len(list.Versions, bundleVersionListLimit+1)
		s.Equal("oldest", list.Versions[bundleVersionListLimit].Version)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects a limit below 1", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		_, err := ListBundleVersionsData("bundle-1", "", "", 0, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleVersionLimit)
	})

	s.Run("requires exactly one identifier", func() {
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		_, err := ListBundleVersionsData("bundle-1", "my-dags", "", 20, ws, testBundleDeploymentID, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleSelector)
	})
}

func (s *Suite) TestRollbackBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("rolls back a non-Dag bundle resolved by mount path", func() {
		out := &bytes.Buffer{}
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
		mockV1Alpha1Client.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-3", mock.MatchedBy(func(req astrov1alpha1.RollbackBundleRequest) bool {
			return req.Version == "old" && req.Description == nil
		})).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-3"},
		}, nil).Once()

		err := RollbackBundle("", "", mountPath, "old", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Rolled back bundle bundle-3 on deployment test-id-1 to version old")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("surfaces an API error", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("RollbackBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(&astrov1alpha1.RollbackBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
			Body:         []byte(`{"message":"version not found"}`),
		}, nil).Once()

		err := RollbackBundle("bundle-1", "", "", "missing", "", ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, "version not found")
		s.NotContains(out.String(), "Rolled back")
	})
}
