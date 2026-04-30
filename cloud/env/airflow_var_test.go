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

func (s *Suite) TestCreateAirflowVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	createdID := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, mock.MatchedBy(func(body astrocore.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "MY_VAR" &&
			body.ObjectType == astrocore.CreateEnvironmentObjectRequestObjectTypeAIRFLOWVARIABLE &&
			body.AirflowVariable != nil &&
			body.AirflowVariable.Value != nil &&
			*body.AirflowVariable.Value == "v"
	})).Return(&astrocore.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()

	got, err := CreateAirflowVar(Scope{WorkspaceID: workspaceID}, "MY_VAR", "v", false, mc)
	s.NoError(err)
	s.Equal("MY_VAR", got.ObjectKey)
	s.NotNil(got.AirflowVariable)
	mc.AssertExpectations(s.T())
}
