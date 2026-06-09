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
