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
)

func TestListConnections(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	context, _ := config.GetCurrentContext()
	organization := context.Organization
	deploymentID := cuid.New()
	objectType := astrocore.ListEnvironmentObjectsParamsObjectTypeCONNECTION
	showSecrets := true
	resolvedLinked := true
	limit := 1000

	t.Run("List connections with deployment ID", func(t *testing.T) {
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
		assert.NoError(t, err)
		assert.Len(t, conns, 1)
		assert.Equal(t, "postgres", conns["conn1"].Type)

		mockClient.AssertExpectations(t)
	})

	t.Run("List connections with specified workspace ID", func(t *testing.T) {
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
		assert.NoError(t, err)
		assert.Len(t, conns, 1)
		assert.Equal(t, "postgres", conns["conn1"].Type)

		mockClient.AssertExpectations(t)
	})

	t.Run("List no connections when no deployment or workspace IDs", func(t *testing.T) {
		originalWorkspace := context.Workspace
		context.Workspace = ""
		context.SetContext()
		defer func() { context.Workspace = originalWorkspace; context.SetContext() }()

		mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

		_, err := ListConnections("", "", mockClient)
		assert.Error(t, err)

		mockClient.AssertExpectations(t)
	})

	t.Run("List no connections when core listing fails", func(t *testing.T) {
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
		assert.Error(t, err)

		mockClient.AssertExpectations(t)
	})
}
