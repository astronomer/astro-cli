package cloud

import (
	"encoding/json"
	"net/http"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMigrateOrgShortName(t *testing.T) {
	var (
		mockOKResponse = astrocore.ListOrganizationsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.Organization{
				{AuthServiceId: "auth-service-id", Id: "test-org-id", Name: "test org", ShortName: "testorg"},
				{AuthServiceId: "auth-service-id-2", Id: "test-org-id-2", Name: "test org 2", ShortName: "testorg2"},
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
	)
	mockClient := new(astrocore_mocks.ClientWithResponsesInterface)

	t.Run("can successfully backfill org shortname", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("organization_short_name", "")
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		err = migrateCloudConfig(mockClient)
		assert.NoError(t, err)
		ctx, err = context.GetCurrentContext()
		assert.NoError(t, err)
		assert.Equal(t, "testorg", ctx.OrganizationShortName)
	})

	t.Run("throw error when ListOrganizations failed", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("organization_short_name", "")
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockErrorResponse, nil).Once()
		err = migrateCloudConfig(mockClient)
		assert.Contains(t, err.Error(), "failed to fetch organizations")
	})

	t.Run("throw error when no organization matched", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization", "test-org-id3")
		ctx.SetContextKey("organization_short_name", "")
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		err = migrateCloudConfig(mockClient)
		assert.Contains(t, err.Error(), "cannot find organization shortname")
	})
}
