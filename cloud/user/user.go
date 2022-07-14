package user

import (
	"fmt"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/software/workspace"
	"github.com/pkg/errors"
)

// CreateInvite calls the CreateUserInvite mutation to create a user invite
func CreateInvite(email, role string, client astro.Client) (astro.UserInvite, error) {
	var userInviteInput astro.CreateUserInviteInput
	userInviteInput.InviteeEmail = email
	userInviteInput.Role = role
	derivedOrganizationID, err := getOrganizationID(client)
	if err != nil {
		return astro.UserInvite{}, err
	}
	userInviteInput.OrganizationID = derivedOrganizationID
	return client.CreateUserInvite(userInviteInput)
}

// getOrganizationID derives the organizationID of the user creating an invite
// It gets the Invitor's current workspace and returns the workspace.OrganizationID
func getOrganizationID(client astro.Client) (string, error) {
	var (
		currentWorkspaceID string
		invitorWorkspace   astro.Workspace
		err                error
	)

	// get invitor's current workspace ID
	currentWorkspaceID, err = workspace.GetCurrentWorkspace()
	if err != nil {
		return "", errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	// get the invitor's workspace

	invitorWorkspace, err = client.GetWorkspace(currentWorkspaceID)
	errMsg := fmt.Sprintf("could not get workspace: %s", currentWorkspaceID)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	// return the invitor's organizationID
	return invitorWorkspace.OrganizationID, nil
}
