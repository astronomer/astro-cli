package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/pkg/emfetch"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestListObjectsPaginates() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	// Two pages: a full page of emfetch.PageLimit followed by a short page,
	// matching how the platform reports paginated results.
	page1 := make([]astrov1.EnvironmentObject, emfetch.PageLimit)
	for i := range page1 {
		page1[i] = astrov1.EnvironmentObject{ObjectKey: "k1"}
	}
	page2 := []astrov1.EnvironmentObject{{ObjectKey: "k2-1"}, {ObjectKey: "k2-2"}}
	totalCount := emfetch.PageLimit + len(page2)

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
			return p != nil && p.Offset != nil && *p.Offset == emfetch.PageLimit
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

// A short page does not end the read, because a server free to serve fewer rows
// than asked for returns one for a list of any length. An empty page ends it,
// which is how an inflated TotalCount still terminates cleanly.
func (s *Suite) TestListObjectsReadsPastAShortPageAndStopsOnAnEmptyOne() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == 0
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{
			EnvironmentObjects: []astrov1.EnvironmentObject{{ObjectKey: "only"}},
			TotalCount:         9999,
		},
	}, nil).Once()
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
		mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
			return p != nil && p.Offset != nil && *p.Offset == 1
		}),
	).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{TotalCount: 9999},
	}, nil).Once()

	got, err := ListVars(Scope{WorkspaceID: workspaceID}, true, false, mc)
	s.NoError(err)
	s.Len(got, 1)
	mc.AssertExpectations(s.T())
}

// The server may cap its page size below the limit requested. Ending the read
// on that first short page would return a third of this list with no error.
func (s *Suite) TestListObjectsReadsAWholeListWhenThePageSizeIsCapped() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()

	const served, total = 500, 1200
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	for _, offset := range []int{0, served, 2 * served} {
		rows := total - offset
		if rows > served {
			rows = served
		}
		page := make([]astrov1.EnvironmentObject, rows)
		for i := range page {
			page[i] = astrov1.EnvironmentObject{ObjectKey: "k"}
		}
		mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization,
			mock.MatchedBy(func(p *astrov1.ListEnvironmentObjectsParams) bool {
				return p != nil && p.Offset != nil && *p.Offset == offset
			}),
		).Return(&astrov1.ListEnvironmentObjectsResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.EnvironmentObjectsPaginated{
				EnvironmentObjects: page,
				TotalCount:         total,
			},
		}, nil).Once()
	}

	got, err := ListVars(Scope{WorkspaceID: workspaceID}, true, false, mc)
	s.NoError(err)
	s.Len(got, total)
	mc.AssertExpectations(s.T())
}

// The organization's refusal to resolve secrets is recognized from the
// response, so a refusal that does not arrive as the JSON envelope still
// reaches the user as guidance rather than as a bare status.
func (s *Suite) TestListObjectsReportsTheSecretsRefusalWhateverShapeItArrivesIn() {
	bodies := map[string]string{
		"the json envelope": `{"message":"showSecrets is not allowed for this organization"}`,
		"a plain body":      "showSecrets is not allowed for this organization",
		"another key":       `{"error":"showSecrets is not allowed for this organization"}`,
	}
	for name, body := range bodies {
		s.Run(name, func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()

			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).
				Return(&astrov1.ListEnvironmentObjectsResponse{
					HTTPResponse: &http.Response{StatusCode: http.StatusMethodNotAllowed},
					Body:         []byte(body),
				}, nil).Once()

			_, err := ListVars(Scope{WorkspaceID: cuid.New()}, true, true, mc)
			s.Error(err)
			s.Contains(err.Error(), "Environment Secrets Fetching")
			mc.AssertExpectations(s.T())
		})
	}
}
