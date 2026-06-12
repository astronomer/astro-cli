package cloud

import (
	"net/http"
	"testing"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// linkedEnvVarObj returns a workspace-scoped env-var EnvironmentObject
// sufficient for the link commands' resolve step.
func linkedEnvVarObj(id, key, value string, links *[]astrov1.EnvironmentObjectLink) astrov1.EnvironmentObject {
	return astrov1.EnvironmentObject{
		Id:                  &id,
		ObjectKey:           key,
		ObjectType:          astrov1.EnvironmentObjectObjectType(astrov1.ENVIRONMENTVARIABLE),
		Scope:               astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId:       cuid.New(),
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: value},
		Links:               links,
	}
}

func TestEnvVarLinkCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	depID := cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			linkedEnvVarObj(id, "FOO", "ws-default", nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			return b.Links != nil && len(*b.Links) == 1 && (*b.Links)[0].ScopeEntityId == depID
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "link", "create", "--variable-key", "FOO", "--workspace-id", cuid.New(), "--deployment-id", depID)
	assert.NoError(t, err)
	assert.Contains(t, out, "Linked FOO to deployment "+depID)
	mc.AssertExpectations(t)
}

func TestEnvVarLinkList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	defer resetEnvFlags()

	id := cuid.New()
	depID := cuid.New()
	override := "dep-override"
	links := []astrov1.EnvironmentObjectLink{{
		ScopeEntityId:                depID,
		EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: override},
	}}
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			linkedEnvVarObj(id, "FOO", "ws-default", &links),
		}},
	}, nil).Once()
	astroV1Client = mc

	out, err := execEnvCmd("var", "link", "list", "--variable-key", "FOO", "--workspace-id", cuid.New())
	assert.NoError(t, err)
	assert.Contains(t, out, "FOO")
	assert.Contains(t, out, depID)
	assert.Contains(t, out, override)
	mc.AssertExpectations(t)
}

func TestEnvVarLinkVariableFlagValidation(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// neither --variable-id nor --variable-key
	resetEnvFlags()
	_, err := execEnvCmd("var", "link", "list", "--workspace-id", cuid.New())
	assert.ErrorContains(t, err, "at least one of the flags in the group [variable-id variable-key] is required")

	// both --variable-id and --variable-key
	resetEnvFlags()
	_, err = execEnvCmd("var", "link", "list", "--variable-id", cuid.New(), "--variable-key", "FOO", "--workspace-id", cuid.New())
	assert.ErrorContains(t, err, "none of the others can be")

	// create requires --deployment-id
	resetEnvFlags()
	_, err = execEnvCmd("var", "link", "create", "--variable-key", "FOO", "--workspace-id", cuid.New())
	assert.ErrorContains(t, err, `required flag(s) "deployment-id" not set`)

	// --value and --exclude are mutually exclusive
	resetEnvFlags()
	_, err = execEnvCmd("var", "link", "create", "--variable-key", "FOO", "--workspace-id", cuid.New(), "--deployment-id", cuid.New(), "--value", "x", "--exclude")
	assert.ErrorContains(t, err, "none of the others can be")

	// empty identifier values satisfy cobra's one-required group but are rejected
	resetEnvFlags()
	_, err = execEnvCmd("var", "link", "list", "--variable-id", "", "--workspace-id", cuid.New())
	assert.ErrorContains(t, err, "--variable-id or --variable-key cannot be empty")
	resetEnvFlags()
	_, err = execEnvCmd("var", "link", "create", "--variable-key", "", "--workspace-id", cuid.New(), "--deployment-id", cuid.New())
	assert.ErrorContains(t, err, "--variable-id or --variable-key cannot be empty")
	resetEnvFlags()
}
