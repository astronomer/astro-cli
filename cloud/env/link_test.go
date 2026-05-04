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

// envVarObj returns a workspace-scoped env-var EnvironmentObject with the given
// links / excludes / autoLink, sufficient for resolveWorkspaceVar to succeed.
func envVarObj(id, value string, autoLink *bool, links []astrocore.EnvironmentObjectLink, excludes []astrocore.EnvironmentObjectExcludeLink) astrocore.EnvironmentObject {
	o := astrocore.EnvironmentObject{
		Id:                  &id,
		ObjectKey:           "FOO",
		ObjectType:          astrocore.EnvironmentObjectObjectType(astrocore.ENVIRONMENTVARIABLE),
		Scope:               astrocore.EnvironmentObjectScope(astrocore.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId:       cuid.New(),
		AutoLinkDeployments: autoLink,
		EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: value},
	}
	if links != nil {
		o.Links = &links
	}
	if excludes != nil {
		o.ExcludeLinks = &excludes
	}
	return o
}

func (s *Suite) TestLinkVarPreservesAutoLink() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()
	autoTrue := true

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	// resolveWorkspaceVar -> getObject -> List by key
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			envVarObj(id, "ws-default", &autoTrue, nil, nil),
		}},
	}, nil).Once()

	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrocore.UpdateEnvironmentObjectJSONRequestBody) bool {
			// AINF-1792: must echo autoLinkDeployments back as true.
			if b.AutoLinkDeployments == nil || !*b.AutoLinkDeployments {
				return false
			}
			// Links should contain the new entry with the new dep ID.
			if b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			return (*b.Links)[0].ScopeEntityId == depID
		}),
	).Return(&astrocore.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, nil, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarWithOverride() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()
	override := "deployment-override"

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			envVarObj(id, "ws-default", nil, nil, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrocore.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			l := (*b.Links)[0]
			return l.ScopeEntityId == depID &&
				l.Overrides != nil &&
				l.Overrides.EnvironmentVariable != nil &&
				l.Overrides.EnvironmentVariable.Value != nil &&
				*l.Overrides.EnvironmentVariable.Value == override
		}),
	).Return(&astrocore.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, &override, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarRejectsDuplicate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			envVarObj(id, "v", nil, []astrocore.EnvironmentObjectLink{
				{Scope: astrocore.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: depID},
			}, nil),
		}},
	}, nil).Once()

	err := LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, nil, mc)
	s.ErrorContains(err, "already linked")
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarRejectsDeploymentScope() {
	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	err := LinkVar("FOO", Scope{DeploymentID: cuid.New()}, "dep", nil, mc)
	s.ErrorContains(err, "workspace-id")
}

func (s *Suite) TestUnlinkVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	keepDep := cuid.New()
	dropDep := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			envVarObj(id, "v", nil, []astrocore.EnvironmentObjectLink{
				{Scope: astrocore.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: keepDep},
				{Scope: astrocore.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: dropDep},
			}, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrocore.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			return (*b.Links)[0].ScopeEntityId == keepDep
		}),
	).Return(&astrocore.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(UnlinkVar("FOO", Scope{WorkspaceID: cuid.New()}, dropDep, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestExcludeVarUsesDedicatedEndpoint() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			envVarObj(id, "v", nil, nil, nil),
		}},
	}, nil).Once()
	mc.On("ExcludeLinkingEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		astrocore.ExcludeLinkingEnvironmentObjectJSONRequestBody{
			Scope:         astrocore.ExcludeLinkEnvironmentObjectRequestScopeDEPLOYMENT,
			ScopeEntityId: depID,
		},
	).Return(&astrocore.ExcludeLinkingEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
	}, nil).Once()

	s.NoError(ExcludeVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestUnexcludeVarPreservesAutoLink() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	keepDep := cuid.New()
	dropDep := cuid.New()
	autoTrue := true

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			envVarObj(id, "v", &autoTrue, nil, []astrocore.EnvironmentObjectExcludeLink{
				{Scope: astrocore.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: keepDep},
				{Scope: astrocore.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: dropDep},
			}),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrocore.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.AutoLinkDeployments == nil || !*b.AutoLinkDeployments {
				return false
			}
			if b.ExcludeLinks == nil || len(*b.ExcludeLinks) != 1 {
				return false
			}
			return (*b.ExcludeLinks)[0].ScopeEntityId == keepDep
		}),
	).Return(&astrocore.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrocore.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(UnexcludeVar("FOO", Scope{WorkspaceID: cuid.New()}, dropDep, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestListVarLinksReportsAll() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	dep1 := cuid.New()
	dep2 := cuid.New()
	dep3 := cuid.New()
	autoTrue := true
	overrideVal := "override-for-dep2"

	mc := new(astrocore_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrocore.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrocore.EnvironmentObjectsPaginated{EnvironmentObjects: []astrocore.EnvironmentObject{
			{
				Id:                  &id,
				ObjectKey:           "FOO",
				ObjectType:          astrocore.EnvironmentObjectObjectType(astrocore.ENVIRONMENTVARIABLE),
				Scope:               astrocore.EnvironmentObjectScope(astrocore.CreateEnvironmentObjectRequestScopeWORKSPACE),
				AutoLinkDeployments: &autoTrue,
				EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{Value: "ws-default"},
				Links: &[]astrocore.EnvironmentObjectLink{
					{Scope: astrocore.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: dep1},
					{
						Scope:                        astrocore.EnvironmentObjectLinkScopeDEPLOYMENT,
						ScopeEntityId:                dep2,
						EnvironmentVariableOverrides: &astrocore.EnvironmentObjectEnvironmentVariableOverrides{Value: overrideVal},
					},
				},
				ExcludeLinks: &[]astrocore.EnvironmentObjectExcludeLink{
					{Scope: astrocore.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: dep3},
				},
			},
		}},
	}, nil).Once()

	report, err := ListVarLinks("FOO", Scope{WorkspaceID: cuid.New()}, mc)
	s.NoError(err)
	s.Equal("FOO", report.ObjectKey)
	s.Equal("ws-default", report.WorkspaceValue)
	s.True(report.AutoLinkDeployments)
	s.Len(report.Links, 2)
	s.Nil(report.Links[0].OverrideValue) // dep1 has no override
	s.Equal(overrideVal, *report.Links[1].OverrideValue)
	s.Equal([]string{dep3}, report.ExcludeLinks)
	mc.AssertExpectations(s.T())
}
