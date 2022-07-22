package user

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/pkg/errors"
)

var (
	ErrInvalidRole  = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidEmail = errors.New("no email provided for the invite. Retry with a valid email address")
)

// CreateInvite calls the CreateUserInvite mutation to create a user invite
func CreateInvite(email, role string, out io.Writer, client astro.Client) error {
	var (
		userInviteInput astro.CreateUserInviteInput
		err             error
	)
	if email == "" {
		return ErrInvalidEmail
	}
	err = IsRoleValid(role)
	if err != nil {
		return err
	}
	derivedOrganizationID, err := getOrganizationID(client)
	if err != nil {
		return err
	}
	userInviteInput = astro.CreateUserInviteInput{InviteeEmail: email, Role: role, OrganizationID: derivedOrganizationID}
	_, err = client.CreateUserInvite(userInviteInput)
	if err != nil {
		return fmt.Errorf("failed to create invite: %s", err.Error()) //nolint
	}
	fmt.Fprintf(out, "invite for %s with role %s created\n", email, role)
	return nil
}

// getOrganizationID derives the organizationID of the user creating an invite
// It gets the Invitor's current workspace and returns the workspace.OrganizationID
func getOrganizationID(client astro.Client) (string, error) {
	var (
		currentWorkspaceID string
		invitorWorkspace   astro.Workspace
		err                error
		ctx                config.Context
	)

	// get invitor's current workspace ID
	ctx, err = context.GetCurrentContext()
	if err != nil {
		return "", err
	}

	// get the invitor's workspace
	currentWorkspaceID = ctx.Workspace
	invitorWorkspace, err = client.GetWorkspace(currentWorkspaceID)
	if err != nil {
		return "", err
	}

	// return the invitor's organizationID
	return invitorWorkspace.OrganizationID, nil
}

// IsRoleValid checks if the requested role is valid
// If the role is valid, it returns nil
// error errInvalidRole is returned if the role is not valid
func IsRoleValid(role string) error {
	validRoles := []string{"ORGANIZATION_MEMBER", "ORGANIZATION_BILLING_ADMIN", "ORGANIZATION_OWNER"}
	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return ErrInvalidRole
}
