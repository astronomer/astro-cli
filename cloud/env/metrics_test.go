package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestCreateMetricsExportRequiresEndpoint() {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	_, err := CreateMetricsExport(Scope{WorkspaceID: cuid.New()}, "k", &MetricsInput{ExporterType: "PROMETHEUS"}, mc)
	s.ErrorContains(err, "endpoint is required")
}

func (s *Suite) TestCreateMetricsExportRequiresExporterType() {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	_, err := CreateMetricsExport(Scope{WorkspaceID: cuid.New()}, "k", &MetricsInput{Endpoint: "https://x"}, mc)
	s.ErrorContains(err, "exporter type is required")
}

func (s *Suite) TestCreateMetricsExport() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	createdID := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "prom_main" &&
			body.ObjectType == astrov1.CreateEnvironmentObjectRequestObjectTypeMETRICSEXPORT &&
			body.MetricsExport != nil &&
			body.MetricsExport.Endpoint == "https://prom" &&
			string(body.MetricsExport.ExporterType) == "PROMETHEUS"
	})).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()

	got, err := CreateMetricsExport(Scope{WorkspaceID: workspaceID}, "prom_main", &MetricsInput{
		Endpoint:     "https://prom",
		ExporterType: "PROMETHEUS",
	}, mc)
	s.NoError(err)
	s.Equal("prom_main", got.ObjectKey)
	mc.AssertExpectations(s.T())
}

// Regression: like the other env-object types, a metrics-export update must
// round-trip the existing Links/ExcludeLinks and auto-link flag or the
// platform drops them (see echoPreservedFields).
func (s *Suite) TestUpdateMetricsExportRoundTripsLinksAndAutoLink() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()
	autoTrue := true
	links := []astrov1.EnvironmentObjectLink{
		{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: depID},
	}
	obj := astrov1.EnvironmentObject{
		Id:                  &id,
		ObjectKey:           "prom_main",
		ObjectType:          astrov1.EnvironmentObjectObjectType(astrov1.METRICSEXPORT),
		Scope:               astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId:       cuid.New(),
		AutoLinkDeployments: &autoTrue,
		Links:               &links,
	}

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{obj}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.AutoLinkDeployments == nil || !*b.AutoLinkDeployments {
				return false
			}
			if b.Links == nil || len(*b.Links) != 1 || (*b.Links)[0].ScopeEntityId != depID {
				return false
			}
			return b.ExcludeLinks != nil && len(*b.ExcludeLinks) == 0 &&
				b.MetricsExport != nil && b.MetricsExport.Endpoint != nil && *b.MetricsExport.Endpoint == "https://prom2"
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "prom_main"},
	}, nil).Once()

	_, err := UpdateMetricsExport("prom_main", Scope{WorkspaceID: cuid.New()}, &MetricsInput{Endpoint: "https://prom2"}, mc)
	s.NoError(err)
	mc.AssertExpectations(s.T())
}
