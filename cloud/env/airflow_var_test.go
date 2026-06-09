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

func (s *Suite) TestCreateAirflowVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	workspaceID := cuid.New()
	createdID := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("CreateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, mock.MatchedBy(func(body astrov1.CreateEnvironmentObjectJSONRequestBody) bool {
		return body.ObjectKey == "MY_VAR" &&
			body.ObjectType == astrov1.CreateEnvironmentObjectRequestObjectTypeAIRFLOWVARIABLE &&
			body.AirflowVariable != nil &&
			body.AirflowVariable.Value != nil &&
			*body.AirflowVariable.Value == "v"
	})).Return(&astrov1.CreateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.CreateEnvironmentObject{Id: createdID},
	}, nil).Once()

	got, err := CreateAirflowVar(Scope{WorkspaceID: workspaceID}, "MY_VAR", "v", false, nil, mc)
	s.NoError(err)
	s.Equal("MY_VAR", got.ObjectKey)
	s.NotNil(got.AirflowVariable)
	mc.AssertExpectations(s.T())
}
