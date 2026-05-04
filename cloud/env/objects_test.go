package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestListObjectsPaginates() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	// Two pages: a full page of defaultListLimit followed by a short page,
	// matching how the platform reports paginated results.
	page1 := make([]astrocore.EnvironmentObject, defaultListLimit)
	for i := range page1 {
		page1[i] = astrocore.EnvironmentObject{ObjectKey: "k1"}
	}
	page2 := []astrocore.EnvironmentObject{{ObjectKey: "k2-1"}, {ObjectKey: "k2-2"}}
	totalCount := defaultListLimit + len(page2)

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
		mock.MatchedBy(func(p *astrocore.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == 0
		}),
	).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{
			EnvironmentObjects: page1,
			TotalCount:         totalCount,
		},
	}, nil).Once()
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
		mock.MatchedBy(func(p *astrocore.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == defaultListLimit
		}),
	).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{
			EnvironmentObjects: page2,
			TotalCount:         totalCount,
		},
	}, nil).Once()

	got, err := ListVars(Scope{WorkspaceID: workspaceID}, true, false, mc)
	s.NoError(err)
	s.Len(got, totalCount)
	s.Equal("k2-2", got[len(got)-1].ObjectKey)
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestListObjectsStopsOnShortPage() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	// A server that under-reports rows by returning a short page should still
	// terminate cleanly, even if TotalCount is wildly inflated.
	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{
			EnvironmentObjects: []astrocore.EnvironmentObject{{ObjectKey: "only"}},
			TotalCount:         9999,
		},
	}, nil).Once()

	got, err := ListVars(Scope{WorkspaceID: workspaceID}, true, false, mc)
	s.NoError(err)
	s.Len(got, 1)
	mc.AssertExpectations(s.T())
}
