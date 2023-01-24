package user

import (
	httpContext "context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/pkg/errors"
)

var (
	ErrNoShortName     = errors.New("cannot retrieve organization short name from context")
	ErrInvalidRole     = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidEmail    = errors.New("no email provided for the invite. Retry with a valid email address")
	ErrInvalidUserKey  = errors.New("invalid User selected")
	userPagnationLimit = 100
	ErrUserNotFound    = errors.New("no user was found for the email you provided")
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
	resp, err := client.CreateUserInviteWithResponse(httpContext.Background(), ctx.OrganizationShortName, userInviteInput)
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

func UpdateUserRole(email, role string, out io.Writer, client astrocore.CoreClient) error {
	var userID string
	err := IsRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return ErrNoShortName
	}
	// Get all org users
	users, err := GetOrgUsers(client)
	if err != nil {
		return err
	}
	if email != "" {
		if err != nil {
			return err
		}

		for i := range users {
			if users[i].Username == email {
				userID = users[i].Id
			}
		}
		if userID == "" {
			return ErrUserNotFound
		}
	} else {
		user, err := selectUser(users)
		userID = user.Id
		email = user.Username
		if err != nil {
			return err
		}
	}
	mutateUserInput := astrocore.MutateOrgUserRoleRequest{
		Role: role,
	}
	resp, err := client.MutateOrgUserRoleWithResponse(httpContext.Background(), ctx.OrganizationShortName, userID, mutateUserInput)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s role was successfully updated to %s\n", email, role)
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

func selectUser(users []astrocore.User) (astrocore.User, error) {
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "FULLNAME", "EMAIL", "ID", "ORGANIZATION ROLE", "CREATE DATE"},
	}

	fmt.Println("\nPlease select the user who's role you would like to update:")

	userMap := map[string]astrocore.User{}
	for i := range users {
		index := i + 1
		table.AddRow([]string{
			strconv.Itoa(index),
			users[i].FullName,
			users[i].Username,
			users[i].Id,
			*users[i].OrgRole,
			users[i].CreatedAt.Format(time.RFC3339),
		}, false)

		userMap[strconv.Itoa(index)] = users[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := userMap[choice]
	if !ok {
		return astrocore.User{}, ErrInvalidUserKey
	}
	return selected, nil
}

// Returns a list of all of an organizations users
func GetOrgUsers(client astrocore.CoreClient) ([]astrocore.User, error) {
	offset := 0
	var users []astrocore.User

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if ctx.OrganizationShortName == "" {
		return nil, ErrNoShortName
	}

	for {
		resp, err := client.ListOrgUsersWithResponse(httpContext.Background(), ctx.OrganizationShortName, &astrocore.ListOrgUsersParams{
			Offset: &offset,
			Limit:  &userPagnationLimit,
		})
		if err != nil {
			return nil, err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		users = append(users, resp.JSON200.Users...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += userPagnationLimit
	}

	return users, nil
}

// Prints a list of all of an organizations users
func ListOrgUsers(out io.Writer, client astrocore.CoreClient) error {
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"FULLNAME", "EMAIL", "ID", "ORGANIZATION ROLE", "CREATE DATE"},
	}
	users, err := GetOrgUsers(client)
	if err != nil {
		return err
	}

	for i := range users {
		table.AddRow([]string{
			users[i].FullName,
			users[i].Username,
			users[i].Id,
			*users[i].OrgRole,
			users[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}
