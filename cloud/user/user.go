package user

import (
	http_context "context"
	"fmt"
	"io"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"

	"github.com/pkg/errors"
)

var (
	ErrNoShortName  = errors.New("cannot retrieve organization short name from context")
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
		return ErrNoShortName
	}
	userInviteInput = astrocore.CreateUserInviteRequest{
		InviteeEmail: email,
		Role:         role,
	}
	resp, err := client.CreateUserInviteWithResponse(http_context.Background(), ctx.OrganizationShortName, userInviteInput)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "invite for %s with role %s created\n", email, role)
	return nil
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
