package env

import (
	"fmt"

	"github.com/lucsky/cuid"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// The Workspace the run looked in is named with where it came from, for each
// source a person can see; the saved login is the run's own and is not named.
// The owner the object reports is the object's own Workspace, and is never
// labeled, even when it is the Workspace looked in elsewhere.
func (s *Suite) TestOutOfScopeNamesTheWorkspacesSource() {
	for _, tt := range []struct {
		source astrosession.Source
		named  string
	}{
		{astrosession.SourceFlag, "workspace %s (from --workspace)"},
		{astrosession.SourceProject, "workspace %s (from the project link)"},
		{astrosession.SourceToken, "workspace %s (from ASTRO_API_TOKEN)"},
		{astrosession.SourceContext, "workspace %s"},
		{astrosession.SourceNone, "workspace %s"},
	} {
		id, wsID, depID := cuid.New(), cuid.New(), cuid.New()
		obj := &astrov1.EnvironmentObject{
			Id: &id, ObjectKey: "K", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
			Scope: astrov1.EnvironmentObjectScopeDEPLOYMENT, ScopeEntityId: depID,
		}
		err := checkInScope(obj, id, Scope{WorkspaceID: wsID, Source: tt.source}, objectTypeVar)
		s.ErrorIs(err, ErrOutOfScope)
		s.Contains(err.Error(), "belongs to deployment "+depID+", not "+fmt.Sprintf(tt.named, wsID)+".")
	}

	// A workspace-owned object looked up at deployment scope: the owner is not labeled.
	id, wsID, depID := cuid.New(), cuid.New(), cuid.New()
	obj := &astrov1.EnvironmentObject{
		Id: &id, ObjectKey: "K", ObjectType: astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE,
		Scope: astrov1.EnvironmentObjectScopeWORKSPACE, ScopeEntityId: wsID,
	}
	err := checkInScope(obj, id, Scope{DeploymentID: depID, Source: astrosession.SourceFlag}, objectTypeVar)
	s.ErrorIs(err, ErrOutOfScope)
	s.Contains(err.Error(), "belongs to workspace "+wsID+", not deployment "+depID)
	s.NotContains(err.Error(), "(from ")
}
