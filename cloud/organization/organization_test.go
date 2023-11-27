package organization

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockOrganizationProduct = astrocore.OrganizationProductHYBRID
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
	mockOKResponse = astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &[]astrocore.Organization{
			{AuthServiceId: "auth-service-id", Id: "org1", Name: "org1", Product: &mockOrganizationProduct},
			{AuthServiceId: "auth-service-id", Id: "org2", Name: "org2", Product: &mockOrganizationProduct},
		},
	}
	errorBody, _ = json.Marshal(astrocore.Error{
		Message: "failed to fetch organizations",
	})
	mockErrorResponse = astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 500,
		},
		Body:    errorBody,
		JSON200: nil,
	}
	errNetwork = errors.New("network error")
)

func TestList(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	t.Run("organization list success", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("organization network error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(nil, errNetwork).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		assert.Contains(t, err.Error(), "network error")
		mockClient.AssertExpectations(t)
	})

	t.Run("organization list error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockErrorResponse, nil).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		assert.Contains(t, err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(t)
	})
}

func TestGetOrganizationSelection(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	t.Run("get organiation selection success", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = getOrganizationSelection(buf, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("get organization selection list error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockErrorResponse, nil).Once()

		buf := new(bytes.Buffer)
		_, err := getOrganizationSelection(buf, mockClient)
		assert.Contains(t, err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(t)
	})

	t.Run("get organization selection select error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

		// mock os.Stdin
		input := []byte("3")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		buf := new(bytes.Buffer)
		_, err = getOrganizationSelection(buf, mockClient)
		assert.ErrorIs(t, err, errInvalidOrganizationKey)
		mockClient.AssertExpectations(t)
	})
}

func TestSwitch(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("successful switch with name", func(t *testing.T) {
		mockGQLClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockGQLClient, mockCoreClient, buf, false)
		assert.NoError(t, err)
		assert.Equal(t, "\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("switching to a current organization", func(t *testing.T) {
		mockGQLClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := Switch("org1", mockGQLClient, mockCoreClient, buf, false)
		assert.NoError(t, err)
		assert.Equal(t, "You selected the same organization as the current one. No switch was made\n", buf.String())
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("successful switch without name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		// mock os.Stdin
		input := []byte("2")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		buf := new(bytes.Buffer)
		err = Switch("", mockClient, mockCoreClient, buf, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failed switch wrong name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("name-wrong", mockClient, mockCoreClient, buf, false)
		assert.ErrorIs(t, err, errInvalidOrganizationName)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failed switch bad selection", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		// mock os.Stdin
		input := []byte("3")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
		buf := new(bytes.Buffer)
		err = Switch("", mockClient, mockCoreClient, buf, false)
		assert.ErrorIs(t, err, errInvalidOrganizationKey)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("successful switch with name and set default product", func(t *testing.T) {
		mockOKResponse = astrocore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.Organization{
				{AuthServiceId: "auth-service-id", Id: "org1", Name: "org1"},
			},
		}
		mockGQLClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockGQLClient, mockCoreClient, buf, false)
		assert.NoError(t, err)
		assert.Equal(t, "\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(t)
	})
}

func TestIsOrgHosted(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	t.Run("org product is hosted", func(t *testing.T) {
		ctx := config.Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "org_short_name_1", "HOSTED")

		isHosted := IsOrgHosted()
		assert.Equal(t, isHosted, true)
	})

	t.Run("org product is hybrid", func(t *testing.T) {
		ctx := config.Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "org_short_name_1", "HYBRID")

		isHosted := IsOrgHosted()
		assert.Equal(t, isHosted, false)
	})
}

func TestListClusters(t *testing.T) {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
	orgShortName := "test-org-name"
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

	t.Run("successful list all clusters", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		clusters, err := ListClusters(orgShortName, mockPlatformCoreClient)
		assert.NoError(t, err)
		assert.Equal(t, len(clusters), 2)
	})

	t.Run("error on listing clusters", func(t *testing.T) {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListClustersResponse{}, errNetwork).Once()
		_, err := ListClusters(orgShortName, mockPlatformCoreClient)
		assert.ErrorIs(t, err, errNetwork)
	})
}
