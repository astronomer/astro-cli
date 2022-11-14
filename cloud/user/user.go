package user

import (
	"bytes"
	http_context "context"
	"encoding/json"
	"fmt"
	"io"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"

	"github.com/pkg/errors"
)

var (
	ErrInvalidRole  = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidEmail = errors.New("no email provided for the invite. Retry with a valid email address")
)

// CreateInvite calls the CreateUserInvite mutation to create a user invite
func CreateInvite(email, role string, out io.Writer, client astrocore.CoreClient) error {
	var (
		userInviteInput astrocore.CreateUserInviteRequest
		err             error
		ctx             config.Context
	)
	if email == "" {
		return ErrInvalidEmail
	}
	err = IsRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err = context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return fmt.Errorf("cannot retrieve organization short name from context")
	}
	userInviteInput = astrocore.CreateUserInviteRequest{
		InviteeEmail: email,
		Role:         role,
	}
	// fixme, this is a core api bug that openapi spec does not specify content-type
	buf, err := json.Marshal(userInviteInput)
	if err != nil {
		return err
	}
	resp, err := client.CreateUserInviteWithBodyWithResponse(http_context.Background(), ctx.OrganizationShortName, "application/json", bytes.NewReader(buf))
	err = astrocore.NormalizeApiError(resp.HTTPResponse, resp.Body, err)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "invite for %s with role %s created\n", email, role)
	return nil
}

// // getOrganizationID derives the organizationID of the user creating an invite
// // It gets the Invitor's current workspace and returns the workspace.OrganizationID
// func getOrganizationID(client astro.Client) (string, error) {
// 	var (
// 		currentWorkspaceID string
// 		invitorWorkspace   astro.Workspace
// 		err                error
// 		ctx                config.Context
// 	)

// 	// get invitor's current workspace ID
// 	ctx, err = context.GetCurrentContext()
// 	if err != nil {
// 		return "", err
// 	}

// 	// get the invitor's workspace
// 	currentWorkspaceID = ctx.Workspace
// 	invitorWorkspace, err = client.GetWorkspace(currentWorkspaceID)
// 	if err != nil {
// 		return "", err
// 	}

// 	// return the invitor's organizationID
// 	return invitorWorkspace.OrganizationID, nil
// }

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
