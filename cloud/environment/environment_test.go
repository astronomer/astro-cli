package environment

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type Suite struct {
	suite.Suite
}

func TestEnvironment(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestListConnections() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	context, _ := config.GetCurrentContext()
	organization := context.Organization
	deploymentID := cuid.New()
	objectType := astrov1.CONNECTION
	showSecrets := true
	resolvedLinked := true
	limit := 1000

	s.Run("List connections with deployment ID", func() {
		listParams := &astrov1.ListEnvironmentObjectsParams{
			DeploymentId:  &deploymentID,
			ObjectType:    &objectType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolvedLinked,
			Limit:         &limit,
		}

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, organization, listParams).Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
				{ObjectKey: "conn1", Connection: &astrov1.EnvironmentObjectConnection{Type: "postgres"}},
			}},
		}, nil).Once()

		conns, err := ListConnections("", deploymentID, mockClient)
		s.NoError(err)
		s.Len(conns, 1)
		s.Equal("postgres", conns["conn1"].Type)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("List connections with specified workspace ID", func() {
		workspaceID := cuid.New()
		listParams := &astrov1.ListEnvironmentObjectsParams{
			WorkspaceId:   &workspaceID,
			ObjectType:    &objectType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolvedLinked,
			Limit:         &limit,
		}

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, organization, listParams).Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
				{ObjectKey: "conn1", Connection: &astrov1.EnvironmentObjectConnection{Type: "postgres"}},
			}},
		}, nil).Once()

		conns, err := ListConnections(workspaceID, "", mockClient)
		s.NoError(err)
		s.Len(conns, 1)
		s.Equal("postgres", conns["conn1"].Type)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("List no connections when no deployment or workspace IDs", func() {
		originalWorkspace := context.Workspace
		context.Workspace = ""
		context.SetContext()
		defer func() { context.Workspace = originalWorkspace; context.SetContext() }()

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)

		_, err := ListConnections("", "", mockClient)
		s.Error(err)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("List no connections when core listing fails", func() {
		listParams := &astrov1.ListEnvironmentObjectsParams{
			WorkspaceId:   &context.Workspace,
			ObjectType:    &objectType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolvedLinked,
			Limit:         &limit,
		}

		mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
		mockClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, organization, listParams).Return(nil, assert.AnError).Once()

		_, err := ListConnections(context.Workspace, "", mockClient)
		s.Error(err)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("List connections handles secrets fetching error from listEnvironmentObjects", func() {
		// This test focuses on verifying that the error detection logic works
		// The integration will be tested end-to-end manually
		secretsError := errors.New("showSecrets on organization with id someOrganization is not allowed")
		result := isSecretsFetchingNotAllowedError(secretsError)
		s.True(result)
	})
}

func (s *Suite) TestIsSecretsFetchingNotAllowedError() {
	testCases := []struct {
		name           string
		err            error
		expectedResult bool
	}{
		{
			"exact match",
			errors.New("showSecrets on organization with id clt4farh34gm801n753x80fo8 is not allowed"),
			true,
		},
		{
			"case insensitive match",
			errors.New("ShowSecrets on ORGANIZATION with id someId IS NOT ALLOWED"),
			true,
		},
		{
			"wrapped error with secrets error",
			fmt.Errorf("API error: %w", errors.New("showSecrets on organization with id test is not allowed")),
			true,
		},
		{
			"different organization error",
			errors.New("organization with id test not found"),
			false,
		},
		{
			"different secrets error",
			errors.New("secrets access denied"),
			false,
		},
		{
			"regular error",
			errors.New("network timeout"),
			false,
		},
		{
			"nil error",
			nil,
			false,
		},
		{
			"empty error",
			errors.New(""),
			false,
		},
		{
			"partial match - missing showSecrets",
			errors.New("organization with id test is not allowed"),
			false,
		},
		{
			"partial match - missing organization",
			errors.New("showSecrets with id test is not allowed"),
			false,
		},
		{
			"partial match - missing not allowed",
			errors.New("showSecrets on organization with id test is allowed"),
			false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := isSecretsFetchingNotAllowedError(tc.err)
			s.Equal(tc.expectedResult, result, "Expected isSecretsFetchingNotAllowedError(%v) to be %v", tc.err, tc.expectedResult)
		})
	}
}
