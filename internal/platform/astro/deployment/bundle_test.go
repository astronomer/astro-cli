package deployment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/stretchr/testify/mock"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
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

		err := CreateBundle("", "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errCreateBundleTarget)

		err = CreateBundle("my-dags", "/mount", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := CreateBundle("my-dags", "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := CreateBundle("", "/usr/local/airflow/dbt", "dbt", "my dbt project", []string{"dag-1"}, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Created bundle bundle-2")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("rejects non-DAG flags on a DAG bundle", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := CreateBundle("my-dags", "", "dbt", "", []string{"dag-1"}, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errDagBundleNonDagFlags)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "CreateBundleWithResponse")
	})

	s.Run("surfaces an API error", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		err := CreateBundle("my-dags", "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errMock)
		mockV1Alpha1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdateBundle() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("requires at least one field", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := UpdateBundle("bundle-1", "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := UpdateBundle("bundle-1", "", "", "new desc", []string{"dag-1"}, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Updated bundle bundle-1")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("requires exactly one identifier", func() {
		out := &bytes.Buffer{}
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := UpdateBundle("", "", "", "new desc", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorIs(err, errBundleSelector)

		err = UpdateBundle("bundle-1", "my-dags", "", "new desc", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := UpdateBundle("", "my-dags", "", "new desc", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := ListBundlesWithFormat(ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := ListBundlesWithFormat(ws, testBundleDeploymentID, testUtil.Renderer{JSON: true, Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := ListBundlesWithFormat(ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := DeleteBundle("bundle-1", "", "", ws, testBundleDeploymentID, true, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Requested deletion of bundle bundle-1")
		mockV1Alpha1Client.AssertExpectations(s.T())
	})

	s.Run("does not delete when the confirmation is declined", func() {
		defer testUtil.MockUserInput(s.T(), "n")()
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)

		err := DeleteBundle("bundle-1", "", "", ws, testBundleDeploymentID, false, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
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

		err := DeleteBundle("", "", mountPath, ws, testBundleDeploymentID, true, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Contains(out.String(), "Requested deletion of bundle bundle-7")
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

		err := DeleteBundle("", "missing", "", ws, testBundleDeploymentID, true, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.ErrorContains(err, `no Dag bundle named "missing"`)
		mockV1Alpha1Client.AssertNotCalled(s.T(), "DeleteBundleWithResponse")
	})

	s.Run("publishes the deletion it requested, with the selector used", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		name := "my-dags"
		isDag := true
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &astrov1alpha1.BundlesPaginated{
				TotalCount: 1,
				Bundles:    []astrov1alpha1.DeploymentBundle{{Id: "bundle-1", Name: &name, IsDagBundle: &isDag}},
			},
		}, nil).Once()
		mockV1Alpha1Client.On("DeleteBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.DeleteBundleResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
		}, nil).Once()

		err := DeleteBundle("", name, "", ws, testBundleDeploymentID, true, testUtil.Renderer{JSON: true, Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		var got BundleRemoval
		s.NoError(json.Unmarshal(out.Bytes(), &got))
		s.Equal(BundleRemoval{ID: "bundle-1", Name: name, DeploymentID: testBundleDeploymentID, Action: "deletion_requested"}, got)
	})
}

// A create or update that succeeds with no bundle in the response (a 204, or
// a body that is not one) reads the bundle back rather than failing on it, and
// says the change was made when it cannot.
func (s *Suite) TestBundleWritesWithNoBundleInTheResponse() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	name := "my-dags"
	isDag := true
	listing := func(bundles ...astrov1alpha1.DeploymentBundle) *astrov1alpha1.ListBundlesResponse {
		return &astrov1alpha1.ListBundlesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1alpha1.BundlesPaginated{TotalCount: len(bundles), Bundles: bundles},
		}
	}
	created := &astrov1alpha1.CreateBundleResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}
	updated := &astrov1alpha1.UpdateBundleResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}

	for _, asJSON := range []bool{false, true} {
		s.Run(fmt.Sprintf("create finds the bundle by its name (json %t)", asJSON), func() {
			out := &bytes.Buffer{}
			s.mockGetDeployment()
			mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
			mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil).Once()
			mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(listing(astrov1alpha1.DeploymentBundle{Id: "bundle-3", Name: &name, IsDagBundle: &isDag}), nil).Once()

			err := CreateBundle(name, "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{JSON: asJSON, Out: out}, mockV1Client, mockV1Alpha1Client)
			s.NoError(err)
			if asJSON {
				var got BundleInfo
				s.NoError(json.Unmarshal(out.Bytes(), &got))
				s.Equal("bundle-3", got.ID)
			} else {
				s.Equal("Created bundle bundle-3 on deployment test-id-1\n", out.String())
			}
		})

		s.Run(fmt.Sprintf("update reads the bundle back (json %t)", asJSON), func() {
			out := &bytes.Buffer{}
			s.mockGetDeployment()
			mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
			mockV1Alpha1Client.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(updated, nil).Once()
			mockV1Alpha1Client.On("GetBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.GetBundleResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusOK},
				JSON200:      &astrov1alpha1.DeploymentBundle{Id: "bundle-1", Name: &name},
			}, nil).Once()

			err := UpdateBundle("bundle-1", "", "", "d", nil, ws, testBundleDeploymentID, testUtil.Renderer{JSON: asJSON, Out: out}, mockV1Client, mockV1Alpha1Client)
			s.NoError(err)
			if asJSON {
				var got BundleInfo
				s.NoError(json.Unmarshal(out.Bytes(), &got))
				s.Equal(&name, got.Name)
			} else {
				s.Equal("Updated bundle bundle-1 on deployment test-id-1\n", out.String())
			}
		})
	}

	// The change happened, so a read-back that fails is a warning: the run
	// succeeds, with what it knows.
	for _, asJSON := range []bool{false, true} {
		s.Run(fmt.Sprintf("create warns when it cannot find the bundle (json %t)", asJSON), func() {
			out := &bytes.Buffer{}
			s.mockGetDeployment()
			mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
			mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil).Once()
			mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(listing(), nil).Once()

			err := CreateBundle(name, "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{JSON: asJSON, Out: out}, mockV1Client, mockV1Alpha1Client)
			s.NoError(err)
			if !asJSON {
				s.Equal("Created bundle my-dags on deployment test-id-1\n"+
					"Warning: the bundle was created, but it could not be read back: no Dag bundle named \"my-dags\" on deployment test-id-1\n", out.String())
				return
			}
			var got map[string]any
			s.NoError(json.Unmarshal(out.Bytes(), &got))
			s.Equal(map[string]any{
				"name":          name,
				"deployment_id": testBundleDeploymentID,
				"warnings":      []any{`the bundle was created, but it could not be read back: no Dag bundle named "my-dags" on deployment test-id-1`},
			}, got)
		})

		s.Run(fmt.Sprintf("update warns when it cannot read the bundle (json %t)", asJSON), func() {
			out := &bytes.Buffer{}
			s.mockGetDeployment()
			mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
			mockV1Alpha1Client.On("UpdateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1", mock.Anything).Return(updated, nil).Once()
			mockV1Alpha1Client.On("GetBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, "bundle-1").Return(&astrov1alpha1.GetBundleResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
			}, nil).Once()

			err := UpdateBundle("bundle-1", "", "", "d", nil, ws, testBundleDeploymentID, testUtil.Renderer{JSON: asJSON, Out: out}, mockV1Client, mockV1Alpha1Client)
			s.NoError(err)
			if !asJSON {
				s.Equal("Updated bundle bundle-1 on deployment test-id-1\n"+
					"Warning: the bundle was updated, but it could not be read back: the API returned no bundle\n", out.String())
				return
			}
			var got map[string]any
			s.NoError(json.Unmarshal(out.Bytes(), &got))
			s.Equal(map[string]any{
				"id":            "bundle-1",
				"deployment_id": testBundleDeploymentID,
				"warnings":      []any{"the bundle was updated, but it could not be read back: the API returned no bundle"},
			}, got)
		})
	}

	// A bundle deleted and created again under its name lists twice until
	// the deletion finishes; the one the create made is the one not pending.
	s.Run("create passes over a bundle whose deletion is pending", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		pending := true
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil).Once()
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(listing(
			astrov1alpha1.DeploymentBundle{Id: "old", Name: &name, IsDagBundle: &isDag, DeletionIsPending: &pending, CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
			astrov1alpha1.DeploymentBundle{Id: "new", Name: &name, IsDagBundle: &isDag, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		), nil).Once()

		err := CreateBundle(name, "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Equal("Created bundle new on deployment test-id-1\n", out.String())
	})

	s.Run("create takes the newest of two that match", func() {
		out := &bytes.Buffer{}
		s.mockGetDeployment()
		mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
		mockV1Alpha1Client.On("CreateBundleWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(created, nil).Once()
		mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(listing(
			astrov1alpha1.DeploymentBundle{Id: "older", Name: &name, IsDagBundle: &isDag, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			astrov1alpha1.DeploymentBundle{Id: "newer", Name: &name, IsDagBundle: &isDag, CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		), nil).Once()

		err := CreateBundle(name, "", "", "", nil, ws, testBundleDeploymentID, testUtil.Renderer{Out: out}, mockV1Client, mockV1Alpha1Client)
		s.NoError(err)
		s.Equal("Created bundle newer on deployment test-id-1\n", out.String())
	})
}

// A list that succeeds with no list in the body is an error, not no bundles,
// and never a panic.
func (s *Suite) TestListBundlesWithNoListInTheResponse() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.mockGetDeployment()
	mockV1Alpha1Client := new(astrov1alpha1_mocks.ClientWithResponsesInterface)
	mockV1Alpha1Client.On("ListBundlesWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1alpha1.ListBundlesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil).Once()

	var err error
	s.NotPanics(func() {
		err = ListBundlesWithFormat(ws, testBundleDeploymentID, testUtil.Renderer{Out: &bytes.Buffer{}}, mockV1Client, mockV1Alpha1Client)
	})
	s.ErrorIs(err, errNoBundleList)
}
