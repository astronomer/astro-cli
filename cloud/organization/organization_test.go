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
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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

type Suite struct {
	suite.Suite
}

func TestCloudOrganizationSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestList() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	s.Run("organization list success", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("organization network error", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(nil, errNetwork).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		s.Contains(err.Error(), "network error")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("organization list error", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockErrorResponse, nil).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		s.Contains(err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetOrganizationSelection() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	s.Run("get organiation selection success", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

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
		_, err = getOrganizationSelection(buf, mockClient)
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("get organization selection list error", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockErrorResponse, nil).Once()

		buf := new(bytes.Buffer)
		_, err := getOrganizationSelection(buf, mockClient)
		s.Contains(err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(s.T())
	})

	s.Run("get organization selection select error", func() {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()

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
		mockGQLClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockGQLClient, mockCoreClient, buf, false)
		s.NoError(err)
		s.Equal("\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("successful switch without name", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
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
		err = Switch("", mockClient, mockCoreClient, buf, false)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failed switch wrong name", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("name-wrong", mockClient, mockCoreClient, buf, false)
		s.ErrorIs(err, errInvalidOrganizationName)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failed switch bad selection", func() {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		CheckUserSession = func(c *config.Context, client astro.Client, coreClient astrocore.CoreClient, out io.Writer) error {
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
		err = Switch("", mockClient, mockCoreClient, buf, false)
		s.ErrorIs(err, errInvalidOrganizationKey)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("successful switch with name and set default product", func() {
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
		s.NoError(err)
		s.Equal("\nSuccessfully switched organization\n", buf.String())
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestIsOrgHosted() {
	// initialize empty config
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("org product is hosted", func() {
		ctx := config.Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "org_short_name_1", "HOSTED")

		isHosted := IsOrgHosted()
		s.Equal(isHosted, true)
	})

	s.Run("org product is hybrid", func() {
		ctx := config.Context{Domain: "localhost"}
		ctx.SetOrganizationContext("org1", "org_short_name_1", "HYBRID")

		isHosted := IsOrgHosted()
		s.Equal(isHosted, false)
	})
}
