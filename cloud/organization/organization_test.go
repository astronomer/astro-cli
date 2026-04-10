package organization

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
}

func TestOrganization(t *testing.T) {
	suite.Run(t, new(Suite))
}

var (
	mockOrganizationProduct = astroplatformcore.OrganizationProductHYBRID
	mockGetSelfResponse     = astrocore.GetSelfUserResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.Self{
			Username: "test@astronomer.io",
			FullName: "jane",
			Id:       "user-id",
		},
	}
	mockOKResponse = astroplatformcore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.OrganizationsPaginated{
			Organizations: []astroplatformcore.Organization{
				{Id: "org1", Name: "org1", Product: &mockOrganizationProduct},
				{Id: "org2", Name: "org2", Product: &mockOrganizationProduct},
			},
			TotalCount: 2,
			Limit:      100,
			Offset:     0,
		},
	}
	auditLogsResp          = []int{}
	auditLogsBody          = []byte{}
	mockOKAuditLogResponse = astrocore.GetOrganizationAuditLogsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &auditLogsResp,
		Body:    auditLogsBody,
	}
	errorAuditLogsBody, _ = json.Marshal(astrocore.Error{
		Message: "failed to fetch organizations audit logs",
	})
	mockOKAuditLogResponseError = astrocore.GetOrganizationAuditLogsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		JSON200: nil,
		Body:    errorAuditLogsBody,
	}
	errorBody, _ = json.Marshal(astrocore.Error{
		Message: "failed to fetch organizations",
	})
	mockErrorResponse = astroplatformcore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	errNetwork = errors.New("network error")
)

func (s *Suite) TestList() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("organization list success", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("organization network error", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		s.Contains(err.Error(), "network error")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("organization list error", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockErrorResponse, nil).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		s.Contains(err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListData() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("returns structured organization data", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		data, err := ListData(mockClient)
		s.NoError(err)
		s.Len(data.Organizations, 2)
		s.Equal("org1", data.Organizations[0].Name)
		s.Equal("org1", data.Organizations[0].ID)
		s.Equal("org2", data.Organizations[1].Name)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("returns error on network failure", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()

		_, err := ListData(mockClient)
		s.Error(err)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListWithFormat() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("json output", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListWithFormat(mockClient, "json", "", buf)
		s.NoError(err)

		var result OrganizationList
		s.NoError(json.Unmarshal(buf.Bytes(), &result))
		s.Len(result.Organizations, 2)
		s.Equal("org1", result.Organizations[0].Name)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("table output", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListWithFormat(mockClient, "table", "", buf)
		s.NoError(err)
		s.Contains(buf.String(), "org1")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("template output", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListWithFormat(mockClient, "template", `{{range .Organizations}}{{.Name}}{{"\n"}}{{end}}`, buf)
		s.NoError(err)
		s.Contains(buf.String(), "org1")
		s.Contains(buf.String(), "org2")
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetOrganizationSelection() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("get organiation selection success", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = getOrganizationSelection(buf, mockPlatformCoreClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("get organization selection list error", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockErrorResponse, nil).Once()

		buf := new(bytes.Buffer)
		_, err := getOrganizationSelection(buf, mockClient)
		s.Contains(err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("get organization selection select error", func() {
		mockClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		// mock os.Stdin
		input := []byte("3")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = getOrganizationSelection(buf, mockClient)
		s.ErrorIs(err, errInvalidOrganizationKey)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSwitch() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("successful switch with name", func() {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.NoError(err)
		s.Equal("\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("switching to a current organization", func() {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := Switch("org1", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.NoError(err)
		s.Equal("You selected the same organization as the current one. No switch was made\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("successful switch without name", func() {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		// mock os.Stdin
		input := []byte("2")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		buf := new(bytes.Buffer)
		err = Switch("", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("failed switch wrong name", func() {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("name-wrong", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.ErrorIs(err, errInvalidOrganizationName)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("failed switch bad selection", func() {
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		// mock os.Stdin
		input := []byte("3")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		buf := new(bytes.Buffer)
		err = Switch("", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.ErrorIs(err, errInvalidOrganizationKey)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("successful switch with name and set default product", func() {
		mockOKResponse = astroplatformcore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.OrganizationsPaginated{
				Organizations: []astroplatformcore.Organization{
					{Id: "org1", Name: "org1"},
				},
				TotalCount: 1,
				Limit:      100,
				Offset:     0,
			},
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.NoError(err)
		s.Equal("\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("successful switch with CUID calls GetOrganization directly", func() {
		testCUID := "clh1rai0g000008l50d5hahbc"
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		// Only GetOrganization should be called — no ListOrganizations
		mockPlatformCoreClient.On("GetOrganizationWithResponse", mock.Anything, testCUID, mock.Anything).Return(&astroplatformcore.GetOrganizationResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astroplatformcore.Organization{Id: testCUID, Name: "org3", Product: &mockOrganizationProduct},
		}, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch(testCUID, mockCoreClient, mockPlatformCoreClient, buf, false)
		s.NoError(err)
		s.Equal("\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("switch with CUID not found returns error", func() {
		testCUID := "clh1rai0g000008l50d5hahbc"
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		errorBody, _ := json.Marshal(astroplatformcore.Error{Message: "organization not found"})
		mockPlatformCoreClient.On("GetOrganizationWithResponse", mock.Anything, testCUID, mock.Anything).Return(&astroplatformcore.GetOrganizationResponse{
			HTTPResponse: &http.Response{StatusCode: 404},
			Body:         errorBody,
		}, nil).Once()
		buf := new(bytes.Buffer)
		err := Switch(testCUID, mockCoreClient, mockPlatformCoreClient, buf, false)
		s.Error(err)
		s.Contains(err.Error(), "organization not found")
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("switch with name paginates across pages", func() {
		page1Response := astroplatformcore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astroplatformcore.OrganizationsPaginated{
				Organizations: []astroplatformcore.Organization{
					{Id: "org1", Name: "org1", Product: &mockOrganizationProduct},
				},
				TotalCount: 200,
				Limit:      100,
				Offset:     0,
			},
		}
		page2Response := astroplatformcore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astroplatformcore.OrganizationsPaginated{
				Organizations: []astroplatformcore.Organization{
					{Id: "org-target", Name: "target-org", Product: &mockOrganizationProduct},
				},
				TotalCount: 200,
				Limit:      100,
				Offset:     100,
			},
		}
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		// First call returns page 1 (offset 0), second call returns page 2 (offset 100)
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&page1Response, nil).Once()
		mockPlatformCoreClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&page2Response, nil).Once()
		CheckUserSession = func(c *config.Context, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("target-org", mockCoreClient, mockPlatformCoreClient, buf, false)
		s.NoError(err)
		s.Equal("\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestIsOrgHosted() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("org product is hosted", func() {
		ctx := config.Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "HOSTED")

		isHosted := IsOrgHosted()
		s.Equal(isHosted, true)
	})

	s.Run("org product is hybrid", func() {
		ctx := config.Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "HYBRID")

		isHosted := IsOrgHosted()
		s.Equal(isHosted, false)
	})
}

func (s *Suite) TestListClusters() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	orgID := "test-org-id"
	mockListClustersResponse := astroplatformcore.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.ClustersPaginated{
			Clusters: []astroplatformcore.Cluster{
				{
					Id:   "test-cluster-id",
					Name: "test-cluster",
				},
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
		},
	}

	s.Run("successful list all clusters", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		clusters, err := ListClusters(orgID, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(len(clusters), 2)
	})

	s.Run("error on listing clusters", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListClustersResponse{}, errNetwork).Once()
		_, err := ListClusters(orgID, mockPlatformCoreClient)
		s.ErrorIs(err, errNetwork)
	})
}

func (s *Suite) TestExportAuditLogs() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("export audit logs success", func() {
		mockPlatformClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		mockClient.On("GetOrganizationAuditLogsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockOKAuditLogResponse, nil).Once()
		err := ExportAuditLogs(mockClient, mockPlatformClient, "", "", 1)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockPlatformClient.AssertExpectations(s.T())
	})
	s.Run("export audit logs and select org success", func() {
		mockPlatformClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		mockClient.On("GetOrganizationAuditLogsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockOKAuditLogResponse, nil).Once()
		err := ExportAuditLogs(mockClient, mockPlatformClient, "org1", "", 1)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
		mockPlatformClient.AssertExpectations(s.T())
	})
	s.Run("export failure", func() {
		mockPlatformClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		mockClient.On("GetOrganizationAuditLogsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		err := ExportAuditLogs(mockClient, mockPlatformClient, "", "", 1)
		s.Contains(err.Error(), "network error")
		mockPlatformClient.AssertExpectations(s.T())
		mockClient.AssertExpectations(s.T())
	})
	s.Run("list failure", func() {
		mockPlatformClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(nil, errNetwork).Once()
		err := ExportAuditLogs(mockClient, mockPlatformClient, "org1", "", 1)
		s.Contains(err.Error(), "network error")
		mockPlatformClient.AssertExpectations(s.T())
		mockClient.AssertExpectations(s.T())
	})
	s.Run("organization list error", func() {
		mockPlatformClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockPlatformClient.On("ListOrganizationsWithResponse", mock.Anything, mock.Anything).Return(&mockOKResponse, nil).Once()
		mockClient.On("GetOrganizationAuditLogsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockOKAuditLogResponseError, nil).Once()
		err := ExportAuditLogs(mockClient, mockPlatformClient, "", "", 1)
		s.Contains(err.Error(), "failed to fetch organizations audit logs")
		mockPlatformClient.AssertExpectations(s.T())
		mockClient.AssertExpectations(s.T())
	})
}

func TestIsCUID(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"valid CUID", "clh1rai0g000008l50d5hahbc", true},
		{"valid CUID all zeros", "c000000000000000000000000", true},
		{"too short", "clh1rai0g000008l50d5hahb", false},
		{"too long", "clh1rai0g000008l50d5hahbcc", false},
		{"wrong prefix", "xlh1rai0g000008l50d5hahbc", false},
		{"uppercase chars", "cLH1RAI0G000008L50D5HAHBC", false},
		{"org name", "my-organization", false},
		{"empty string", "", false},
		{"name containing cuid substring", "clh1rai0g000008l50d5hahbc-prod", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, isCUID(tt.input))
		})
	}
}
