package env

import (
	"net/http"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// scopedKind drives one object type through the ID-addressed paths.
type scopedKind struct {
	name       string
	objectType astrov1.EnvironmentObjectObjectType
	del        func(idOrKey string, scope Scope, c astrov1.APIClient) (*astrov1.EnvironmentObject, error)
	update     func(idOrKey string, scope Scope, c astrov1.APIClient) error
	get        func(idOrKey string, scope Scope, c astrov1.APIClient) error
}

func scopedKinds() []scopedKind {
	return []scopedKind{
		{
			"variable", astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE, DeleteVar,
			func(id string, sc Scope, c astrov1.APIClient) error {
				_, err := UpdateVar(id, sc, "v", nil, c)
				return err
			},
			func(id string, sc Scope, c astrov1.APIClient) error { _, err := GetVar(id, sc, false, c); return err },
		},
		{
			"connection", astrov1.EnvironmentObjectObjectTypeCONNECTION, DeleteConn,
			func(id string, sc Scope, c astrov1.APIClient) error {
				h := "h"
				_, err := UpdateConn(id, sc, ConnInput{Type: "postgres", Host: &h}, c)
				return err
			},
			func(id string, sc Scope, c astrov1.APIClient) error { _, err := GetConn(id, sc, false, c); return err },
		},
		{
			"airflow-variable", astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE, DeleteAirflowVar,
			func(id string, sc Scope, c astrov1.APIClient) error {
				_, err := UpdateAirflowVar(id, sc, "v", nil, c)
				return err
			},
			func(id string, sc Scope, c astrov1.APIClient) error {
				_, err := GetAirflowVar(id, sc, false, c)
				return err
			},
		},
		{
			"metrics-export", astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT, DeleteMetricsExport,
			func(id string, sc Scope, c astrov1.APIClient) error {
				_, err := UpdateMetricsExport(id, sc, &MetricsInput{Endpoint: "https://p.example.com"}, c)
				return err
			},
			func(id string, sc Scope, c astrov1.APIClient) error {
				_, err := GetMetricsExport(id, sc, false, c)
				return err
			},
		},
	}
}

func mockGetByID(mc *astrov1_mocks.ClientWithResponsesInterface, org string, obj *astrov1.EnvironmentObject) {
	mc.On("GetEnvironmentObjectWithResponse", mock.Anything, org, *obj.Id).Return(&astrov1.GetEnvironmentObjectResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      obj,
	}, nil).Once()
}

// An ID reaches an object anywhere in the organization, so it is held to the
// scope the command named, as a key is by being looked up there.
func (s *Suite) TestDeleteByIDOnlyWithinTheScope() {
	for _, k := range scopedKinds() {
		s.Run(k.name+": deployment object, deployment scope, is deleted", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id, depID := cuid.New(), cuid.New()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "K", ObjectType: k.objectType,
				Scope: astrov1.EnvironmentObjectScopeDEPLOYMENT, ScopeEntityId: depID,
			})
			mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id).Return(&astrov1.DeleteEnvironmentObjectResponse{
				HTTPResponse: &http.Response{StatusCode: 204},
			}, nil).Once()

			s.NoError(errOf(k.del(id, Scope{DeploymentID: depID}, mc)))
			mc.AssertExpectations(s.T())
		})

		s.Run(k.name+": workspace object, workspace scope, is deleted", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id, wsID := cuid.New(), cuid.New()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "K", ObjectType: k.objectType,
				Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wsID,
			})
			mc.On("DeleteEnvironmentObjectWithResponse", mock.Anything, ctx.Organization, id).Return(&astrov1.DeleteEnvironmentObjectResponse{
				HTTPResponse: &http.Response{StatusCode: 204},
			}, nil).Once()

			s.NoError(errOf(k.del(id, Scope{WorkspaceID: wsID}, mc)))
			mc.AssertExpectations(s.T())
		})

		s.Run(k.name+": workspace object linked into the deployment is refused at deployment scope", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id, wsID, depID := cuid.New(), cuid.New(), cuid.New()
			// No delete is mocked: the mock panics if one is attempted.
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "K", ObjectType: k.objectType,
				Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wsID,
				Links: &[]astrov1.EnvironmentObjectLink{{Scope: astrov1.EnvironmentObjectLinkScopeDEPLOYMENT, ScopeEntityId: depID}},
			})

			_, err := k.del(id, Scope{DeploymentID: depID}, mc)
			s.ErrorIs(err, ErrOutOfScope)
			s.Contains(err.Error(), "belongs to workspace "+wsID+", not deployment "+depID)
			s.Contains(err.Error(), "--workspace "+wsID)
			if k.name == "metrics-export" {
				// Metrics exports have no link commands to suggest.
				s.NotContains(err.Error(), "link")
			} else {
				s.Contains(err.Error(), "astro env "+k.name+" link delete")
			}
			mc.AssertExpectations(s.T())
			mc.AssertNotCalled(s.T(), "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})

		s.Run(k.name+": another deployment's object is refused", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id, otherDep, depID := cuid.New(), cuid.New(), cuid.New()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "K", ObjectType: k.objectType,
				Scope: astrov1.EnvironmentObjectScopeDEPLOYMENT, ScopeEntityId: otherDep,
			})

			_, err := k.del(id, Scope{DeploymentID: depID}, mc)
			s.ErrorIs(err, ErrOutOfScope)
			s.Contains(err.Error(), "--deployment "+otherDep)
			mc.AssertNotCalled(s.T(), "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})

		s.Run(k.name+": a deployment object is refused at its workspace's scope", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id, wsID, depID := cuid.New(), cuid.New(), cuid.New()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "K", ObjectType: k.objectType,
				Scope: astrov1.EnvironmentObjectScopeDEPLOYMENT, ScopeEntityId: depID,
			})

			s.ErrorIs(errOf(k.del(id, Scope{WorkspaceID: wsID}, mc)), ErrOutOfScope)
			mc.AssertNotCalled(s.T(), "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})

		s.Run(k.name+": an object of another type is refused", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id, wsID := cuid.New(), cuid.New()
			other := astrov1.EnvironmentObjectObjectTypeCONNECTION
			if k.objectType == other {
				other = astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE
			}
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
				Id: &id, ObjectKey: "K", ObjectType: other,
				Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wsID,
			})

			_, err := k.del(id, Scope{WorkspaceID: wsID}, mc)
			s.ErrorIs(err, ErrOutOfScope)
			s.Contains(err.Error(), id+" is a "+nounForObjectType(other)+", not a "+k.name)
			mc.AssertNotCalled(s.T(), "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})

		s.Run(k.name+": an object that does not say where it lives is refused", func() {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, _ := config.GetCurrentContext()
			id := cuid.New()
			mc := new(astrov1_mocks.ClientWithResponsesInterface)
			mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{Id: &id, ObjectKey: "K", ObjectType: k.objectType})

			_, err := k.del(id, Scope{WorkspaceID: cuid.New()}, mc)
			s.ErrorIs(err, ErrOutOfScope)
			s.Contains(err.Error(), "did not say what kind of object "+id)
			mc.AssertNotCalled(s.T(), "DeleteEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// Update and get resolve an ID through the same lookup, so a workspace object
// cannot be rewritten (or shown as the deployment's) under --deployment-id.
func (s *Suite) TestUpdateAndGetByIDOnlyWithinTheScope() {
	for _, k := range scopedKinds() {
		for verb, call := range map[string]func(string, Scope, astrov1.APIClient) error{"update": k.update, "get": k.get} {
			s.Run(k.name+" "+verb, func() {
				testUtil.InitTestConfig(testUtil.LocalPlatform)
				ctx, _ := config.GetCurrentContext()
				id, wsID, depID := cuid.New(), cuid.New(), cuid.New()
				// No update is mocked: the mock panics if one is attempted.
				mc := new(astrov1_mocks.ClientWithResponsesInterface)
				mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
					Id: &id, ObjectKey: "K", ObjectType: k.objectType,
					Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wsID,
				})

				err := call(id, Scope{DeploymentID: depID}, mc)
				s.ErrorIs(err, ErrOutOfScope)
				s.NotErrorIs(err, ErrNotFound, "a miss would send set on to create")
				mc.AssertNotCalled(s.T(), "UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	}
}

// The scope's kind is compared as well as its ID, so an object is only ever
// the scope's when both match.
func (s *Suite) TestInScopeComparesTheScopeKind() {
	id, entity := cuid.New(), cuid.New()
	obj := &astrov1.EnvironmentObject{
		Id: &id, ObjectKey: "K", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
		Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: entity,
	}
	s.NoError(checkInScope(obj, id, Scope{WorkspaceID: entity}, objectTypeVar))
	s.ErrorIs(checkInScope(obj, id, Scope{DeploymentID: entity}, objectTypeVar), ErrOutOfScope)
}

// Linking only takes workspace objects, so a deployment object's ID gets the
// linking rule, not a hint to pass --deployment (which in a link command
// names the link's target).
func (s *Suite) TestLinkByIDOfADeploymentObjectExplainsLinkingIsWorkspaceOnly() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id, wsID, ownerDep, target := cuid.New(), cuid.New(), cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
		Id: &id, ObjectKey: "K", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
		Scope: astrov1.EnvironmentObjectScopeDEPLOYMENT, ScopeEntityId: ownerDep,
	})

	_, err := Link(LinkVariable, id, Scope{WorkspaceID: wsID}, target, nil, false, mc)
	s.Error(err)
	s.Contains(err.Error(), "only workspace-scoped objects can be linked")
	s.NotContains(err.Error(), "pass --deployment")
	mc.AssertNotCalled(s.T(), "UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// Another workspace's object keeps the scope refusal in a link command.
func (s *Suite) TestLinkByIDOfAnotherWorkspacesObjectIsRefused() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ctx, _ := config.GetCurrentContext()
	id, wsID, otherWS, target := cuid.New(), cuid.New(), cuid.New(), cuid.New()
	mc := new(astrov1_mocks.ClientWithResponsesInterface)
	mockGetByID(mc, ctx.Organization, &astrov1.EnvironmentObject{
		Id: &id, ObjectKey: "K", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
		Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: otherWS,
	})

	_, err := Link(LinkVariable, id, Scope{WorkspaceID: wsID}, target, nil, false, mc)
	s.ErrorIs(err, ErrOutOfScope)
	s.Contains(err.Error(), "--workspace "+otherWS)
	mc.AssertNotCalled(s.T(), "UpdateEnvironmentObjectWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// A missing type is unprovable, not a mismatch to explain.
func (s *Suite) TestOutOfScopeMissingType() {
	id, wsID := cuid.New(), cuid.New()
	obj := &astrov1.EnvironmentObject{Id: &id, ObjectKey: "K", Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wsID}
	err := checkInScope(obj, id, Scope{WorkspaceID: wsID}, objectTypeVar)
	s.ErrorIs(err, ErrOutOfScope)
	s.Contains(err.Error(), "did not say what kind of object "+id)
}
