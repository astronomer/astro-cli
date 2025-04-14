package environment

import (
	"net/http"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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
	objectType := astrocore.CONNECTION
	showSecrets := true
	resolvedLinked := true
	limit := 1000

	s.Run("List connections with deployment ID", func() {
		listParams := &astrocore.ListEnvironmentObjectsParams{
			DeploymentId:  &deploymentID,
			ObjectType:    &objectType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolvedLinked,
			Limit:         &limit,
		}

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, organization, listParams).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
				{ObjectKey: "conn1", Connection: &astrocore.EnvironmentObjectConnection{Type: "postgres"}},
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
		listParams := &astrocore.ListEnvironmentObjectsParams{
			WorkspaceId:   &workspaceID,
			ObjectType:    &objectType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolvedLinked,
			Limit:         &limit,
		}

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, organization, listParams).Return(&astrocore.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
				{ObjectKey: "conn1", Connection: &astrocore.EnvironmentObjectConnection{Type: "postgres"}},
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

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		_, err := ListConnections("", "", mockClient)
		s.Error(err)

		mockClient.AssertExpectations(s.T())
	})

	s.Run("List no connections when core listing fails", func() {
		listParams := &astrocore.ListEnvironmentObjectsParams{
			WorkspaceId:   &context.Workspace,
			ObjectType:    &objectType,
			ShowSecrets:   &showSecrets,
			ResolveLinked: &resolvedLinked,
			Limit:         &limit,
		}

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)
		mockClient.On("ListEnvironmentObjectsWithResponse", mock.Anything, organization, listParams).Return(nil, assert.AnError).Once()

		_, err := ListConnections(context.Workspace, "", mockClient)
		s.Error(err)

		mockClient.AssertExpectations(s.T())
	})
}
