package organization

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockOKResponse = astrocore.ListOrganizationsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &[]astrocore.Organization{
			{AuthServiceId: "auth-service-id", Id: "org1", Name: "org1"},
			{AuthServiceId: "auth-service-id", Id: "org2", Name: "org2"},
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
		mockClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("organization network error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything).Return(nil, errNetwork).Once()
		buf := new(bytes.Buffer)
		err := List(buf, mockClient)
		assert.Contains(t, err.Error(), "network error")
		mockClient.AssertExpectations(t)
	})

	t.Run("organization list error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockErrorResponse, nil).Once()
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
		mockClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()

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
		mockClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockErrorResponse, nil).Once()

		buf := new(bytes.Buffer)
		_, err := getOrganizationSelection(buf, mockClient)
		assert.Contains(t, err.Error(), "failed to fetch organizations")
		mockClient.AssertExpectations(t)
	})

	t.Run("get organization selection select error", func(t *testing.T) {
		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()

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
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()
		FetchDomainAuthConfig = func(domain string) (astro.AuthConfig, error) {
			return astro.AuthConfig{}, nil
		}
		Login = func(domain, orgID, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockGQLClient, mockCoreClient, buf, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("successful switch without name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()
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
		FetchDomainAuthConfig = func(domain string) (astro.AuthConfig, error) {
			return astro.AuthConfig{}, nil
		}
		Login = func(domain, orgID, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err = Switch("", mockClient, mockCoreClient, buf, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failed switch wrong name", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()
		FetchDomainAuthConfig = func(domain string) (astro.AuthConfig, error) {
			return astro.AuthConfig{}, nil
		}
		Login = func(domain, orgID, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
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
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()

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
		FetchDomainAuthConfig = func(domain string) (astro.AuthConfig, error) {
			return astro.AuthConfig{}, nil
		}
		Login = func(domain, orgID, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return nil
		}
		buf := new(bytes.Buffer)
		err = Switch("", mockClient, mockCoreClient, buf, false)
		assert.ErrorIs(t, err, errInvalidOrganizationKey)
		mockCoreClient.AssertExpectations(t)
	})

	t.Run("failed switch bad login", func(t *testing.T) {
		mockError := errors.New("mock error")
		mockClient := new(astro_mocks.Client)
		mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockCoreClient.On("ListOrganizationsWithResponse", mock.Anything).Return(&mockOKResponse, nil).Once()
		FetchDomainAuthConfig = func(domain string) (astro.AuthConfig, error) {
			return astro.AuthConfig{}, nil
		}
		Login = func(domain, orgID, token string, client astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
			return mockError
		}
		buf := new(bytes.Buffer)
		err := Switch("org1", mockClient, mockCoreClient, buf, false)
		assert.ErrorIs(t, err, mockError)
		mockCoreClient.AssertExpectations(t)
	})
}
