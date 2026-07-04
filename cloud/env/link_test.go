package env

import (
	"encoding/json"
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// envVarObj returns a workspace-scoped env-var EnvironmentObject with the given
// links / excludes / autoLink, sufficient for resolveWorkspaceVar to succeed.
func envVarObj(id, value string, autoLink *bool, links []astrov1.EnvironmentObjectLink, excludes []astrov1.EnvironmentObjectExcludeLink) astrov1.EnvironmentObject {
	o := astrov1.EnvironmentObject{
		Id:                  &id,
		ObjectKey:           "FOO",
		ObjectType:          astrov1.EnvironmentObjectObjectType(astrov1.ENVIRONMENTVARIABLE),
		Scope:               astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
		ScopeEntityId:       cuid.New(),
		AutoLinkDeployments: autoLink,
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: value},
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

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	// resolveWorkspaceVar -> getObject -> List by key
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "ws-default", &autoTrue, nil, nil),
		}},
	}, nil).Once()

	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
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
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
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

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "ws-default", nil, nil, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
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
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, &override, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarUpsertsExistingLink() {
	// Re-linking an existing link should replace its override, not error.
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()
	oldOverride := "old"
	newOverride := "new"

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", nil, []astrov1.EnvironmentObjectLink{
				{
					Scope:                        astrov1.EnvironmentObjectLinkScopeDEPLOYMENT,
					ScopeEntityId:                depID,
					EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: oldOverride},
				},
			}, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			l := (*b.Links)[0]
			return l.ScopeEntityId == depID &&
				l.Overrides != nil &&
				l.Overrides.EnvironmentVariable != nil &&
				l.Overrides.EnvironmentVariable.Value != nil &&
				*l.Overrides.EnvironmentVariable.Value == newOverride
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, &newOverride, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarUpsertOmitsOverrideWhenNotProvided() {
	// Re-linking without --value sends Overrides=nil for that entry, which the
	// platform's per-entry PATCH merge interprets as "leave existing override
	// alone." This is the no-op case we rely on for safe re-runs.
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()
	oldOverride := "old"

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", nil, []astrov1.EnvironmentObjectLink{
				{
					Scope:                        astrov1.EnvironmentObjectLinkScopeDEPLOYMENT,
					ScopeEntityId:                depID,
					EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: oldOverride},
				},
			}, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			// Overrides is nil → field omitted → platform preserves existing override.
			return (*b.Links)[0].ScopeEntityId == depID && (*b.Links)[0].Overrides == nil
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, nil, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarUpsertPreservesOtherLinks() {
	// Upserting one link must not mangle other deployments' links/overrides.
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	keepDep := cuid.New()
	keepOverride := "keep"
	upsertDep := cuid.New()
	newOverride := "upsert"

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", nil, []astrov1.EnvironmentObjectLink{
				{
					Scope:                        astrov1.EnvironmentObjectLinkScopeDEPLOYMENT,
					ScopeEntityId:                keepDep,
					EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: keepOverride},
				},
			}, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Links == nil || len(*b.Links) != 2 {
				return false
			}
			var sawKeep, sawUpsert bool
			for _, l := range *b.Links {
				switch l.ScopeEntityId {
				case keepDep:
					sawKeep = l.Overrides != nil &&
						l.Overrides.EnvironmentVariable != nil &&
						l.Overrides.EnvironmentVariable.Value != nil &&
						*l.Overrides.EnvironmentVariable.Value == keepOverride
				case upsertDep:
					sawUpsert = l.Overrides != nil &&
						l.Overrides.EnvironmentVariable != nil &&
						l.Overrides.EnvironmentVariable.Value != nil &&
						*l.Overrides.EnvironmentVariable.Value == newOverride
				}
			}
			return sawKeep && sawUpsert
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, upsertDep, &newOverride, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestExcludeVarIdempotent() {
	// Excluding an already-excluded deployment should succeed without a PATCH.
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", nil, nil, []astrov1.EnvironmentObjectExcludeLink{
				{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: depID},
			}),
		}},
	}, nil).Once()
	// Note: no ExcludeLinkingEnvironmentObjectWithResponse expectation - idempotent path skips the call entirely.

	s.NoError(ExcludeVar("FOO", Scope{WorkspaceID: cuid.New()}, depID, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestLinkVarRejectsDeploymentScope() {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	err := LinkVar("FOO", Scope{DeploymentID: cuid.New()}, cuid.New(), nil, mc)
	s.ErrorContains(err, "workspace-id")
}

func (s *Suite) TestLinkVarRejectsBadDeploymentID() {
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	s.ErrorContains(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, "", nil, mc), "cannot be empty")
	s.ErrorContains(LinkVar("FOO", Scope{WorkspaceID: cuid.New()}, "not-a-cuid", nil, mc), "valid deployment ID")
}

func (s *Suite) TestUnlinkVar() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	keepDep := cuid.New()
	dropDep := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", nil, []astrov1.EnvironmentObjectLink{
				{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: keepDep},
				{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: dropDep},
			}, nil),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.Links == nil || len(*b.Links) != 1 {
				return false
			}
			return (*b.Links)[0].ScopeEntityId == keepDep
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
	}, nil).Once()

	s.NoError(UnlinkVar("FOO", Scope{WorkspaceID: cuid.New()}, dropDep, mc))
	mc.AssertExpectations(s.T())
}

func (s *Suite) TestExcludeVarUsesDedicatedEndpoint() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()
	depID := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", nil, nil, nil),
		}},
	}, nil).Once()
	mc.On("ExcludeLinkingEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		astrov1.ExcludeLinkingEnvironmentObjectJSONRequestBody{
			Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScopeDEPLOYMENT,
			ScopeEntityId: depID,
		},
	).Return(&astrov1.ExcludeLinkingEnvironmentObjectResponse{
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

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "v", &autoTrue, nil, []astrov1.EnvironmentObjectExcludeLink{
				{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: keepDep},
				{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: dropDep},
			}),
		}},
	}, nil).Once()
	mc.On("UpdateEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id,
		mock.MatchedBy(func(b astrov1.UpdateEnvironmentObjectJSONRequestBody) bool {
			if b.AutoLinkDeployments == nil || !*b.AutoLinkDeployments {
				return false
			}
			if b.ExcludeLinks == nil || len(*b.ExcludeLinks) != 1 {
				return false
			}
			return (*b.ExcludeLinks)[0].ScopeEntityId == keepDep
		}),
	).Return(&astrov1.UpdateEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &astrov1.EnvironmentObject{Id: &id, ObjectKey: "FOO"},
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

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			{
				Id:                  &id,
				ObjectKey:           "FOO",
				ObjectType:          astrov1.EnvironmentObjectObjectType(astrov1.ENVIRONMENTVARIABLE),
				Scope:               astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE),
				AutoLinkDeployments: &autoTrue,
				EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{Value: "ws-default"},
				Links: &[]astrov1.EnvironmentObjectLink{
					{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: dep1},
					{
						Scope:                        astrov1.EnvironmentObjectLinkScopeDEPLOYMENT,
						ScopeEntityId:                dep2,
						EnvironmentVariableOverrides: &astrov1.EnvironmentObjectEnvironmentVariableOverrides{Value: overrideVal},
					},
				},
				ExcludeLinks: &[]astrov1.EnvironmentObjectExcludeLink{
					{Scope: astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT, ScopeEntityId: dep3},
				},
			},
		}},
	}, nil).Once()

	report, err := ListVarLinks("FOO", Scope{WorkspaceID: cuid.New()}, false, mc)
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

// The report's Links/ExcludeLinks must marshal as [] rather than null so
// scripted consumers can iterate .links[] without null guards.
func (s *Suite) TestListVarLinksEmptyMarshalsAsArrays() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id := cuid.New()

	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mc.On("ListEnvironmentObjectsWithResponse", mock.Anything, ctx.Organization, mock.Anything).Return(&astrov1.ListEnvironmentObjectsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.EnvironmentObjectsPaginated{EnvironmentObjects: []astrov1.EnvironmentObject{
			envVarObj(id, "ws-default", nil, nil, nil),
		}},
	}, nil).Once()

	report, err := ListVarLinks("FOO", Scope{WorkspaceID: cuid.New()}, false, mc)
	s.NoError(err)
	b, err := json.Marshal(report)
	s.NoError(err)
	s.Contains(string(b), `"links":[]`)
	s.Contains(string(b), `"excludeLinks":[]`)
	mc.AssertExpectations(s.T())
}
