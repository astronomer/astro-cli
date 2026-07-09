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

		err := CreateBundle("", "", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errCreateBundleTarget)

		err = CreateBundle("my-dags", "/mount", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
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

		err := CreateBundle("my-dags", "", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
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

		err := CreateBundle("", "/usr/local/airflow/dbt", "dbt", "my dbt project", []string{"dag-1"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Created bundle bundle-2")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects non-DAG flags on a DAG bundle", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("my-dags", "", "dbt", "", []string{"dag-1"}, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errDagBundleNonDagFlags)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

	s.Run("surfaces an API error", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		err := CreateBundle("my-dags", "", "", "", nil, ws, testBundleDeploymentID, out, mockV1Client, mockV1Alpha1Client)
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
	listResponse := &astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1alpha1.BundlesPaginated{
			TotalCount: 2,
			Bundles: []astrov1alpha1.DeploymentBundle{
				{Id: "bundle-1", Name: &bundleName, IsDagBundle: &isDag},
				{Id: "bundle-2", NonDagMountPath: &mountPath},
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
		s.Contains(out.String(), "bundle-1")
		s.Contains(out.String(), "main")
		s.Contains(out.String(), mountPath)
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
		s.ErrorContains(err, `no DAG bundle named "missing"`)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "DeleteBundleWithResponse")
	})
}
