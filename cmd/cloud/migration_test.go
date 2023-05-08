package cloud

import (
	"encoding/json"
	"net/http"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
)

func (s *Suite) TestMigrateOrgShortName() {
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

	s.Run("can successfully backfill org shortname", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("organization_short_name", "")
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		err = migrateCloudConfig(mockClient)
		s.NoError(err)
		ctx, err = context.GetCurrentContext()
		s.NoError(err)
		s.Equal("testorg", ctx.OrganizationShortName)
	})

	s.Run("throw error when ListOrganizations failed", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("organization_short_name", "")
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockErrorResponse, nil).Once()
		err = migrateCloudConfig(mockClient)
		s.Contains(err.Error(), "failed to fetch organizations")
	})

	s.Run("throw error when no organization matched", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization", "test-org-id3")
		ctx.SetContextKey("organization_short_name", "")
		mockClient.On("ListOrganizationsWithResponse", mock.Anything, &astrocore.ListOrganizationsParams{}).Return(&mockOKResponse, nil).Once()
		err = migrateCloudConfig(mockClient)
		s.Contains(err.Error(), "cannot find organization shortname")
	})
}
