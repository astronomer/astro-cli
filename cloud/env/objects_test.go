package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestListObjectsPaginates() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	// Two pages: a full page of defaultListLimit followed by a short page,
	// matching how the platform reports paginated results.
	page1 := make([]astrov1.EnvironmentObject, defaultListLimit)
	for i := range page1 {
		page1[i] = astrov1.EnvironmentObject{ObjectKey: "k1"}
	}
	page2 := []astrov1.EnvironmentObject{{ObjectKey: "k2-1"}, {ObjectKey: "k2-2"}}
	totalCount := defaultListLimit + len(page2)

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == 0
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{
			EnvironmentObjects: page1,
			TotalCount:         totalCount,
		},
	}, nil).Once()
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == defaultListLimit
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{
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
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{
			EnvironmentObjects: []astrov1.EnvironmentObject{{ObjectKey: "only"}},
			TotalCount:         9999,
		},
	}, nil).Once()

	got, err := ListVars(Scope{WorkspaceID: workspaceID}, true, false, mc)
	s.NoError(err)
	s.Len(got, 1)
	mc.AssertExpectations(s.T())
}
