package user

import (
	httpContext "context"
	"fmt"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	"github.com/samber/lo"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/pkg/errors"
)

var (
	ErrNoShortName              = errors.New("cannot retrieve organization short name from context")
	ErrNoOrganizationID         = errors.New("cannot retrieve organizationId from context")
	ErrNoExistingWorkspaceRole  = errors.New("user does not have an existing role in the workspace")
	ErrHasExistingWorkspaceRole = errors.New("user has an existing role in the workspace")
	ErrInvalidRole              = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidWorkspaceRole     = errors.New("requested role is invalid. Possible values are WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
	ErrInvalidOrganizationRole  = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidEmail             = errors.New("no email provided for the invite. Retry with a valid email address")
	ErrInvalidUserKey           = errors.New("invalid User selected")
	userPaginationLimit         = 500
	ErrUserNotFound             = errors.New("no user was found for the email you provided")
)

func CreateInvite(email, role string, out io.Writer, client astroiamcore.CoreClient) error {
	var (
		userInviteInput astroiamcore.CreateUserInviteJSONRequestBody
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
	if ctx.Organization == "" {
		return ErrNoOrganizationID
	}
	userInviteInput = astroiamcore.CreateUserInviteJSONRequestBody{
		InviteeEmail: email,
		Role:         astroiamcore.CreateUserInviteRequestRole(role),
	}
	resp, err := client.CreateUserInviteWithResponse(httpContext.Background(), ctx.Organization, userInviteInput)
	if err != nil {
		return err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "invite for %s with role %s created\n", email, role)
	return nil
}

func UpdateOrgRole(email, role string, out io.Writer, client astroiamcore.CoreClient) error {
	var userID string
	var workspaceRoles *[]astroiamcore.WorkspaceRole
	err := IsRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.Organization == "" {
		return ErrNoOrganizationID
	}
	// Get all org users
	users, err := GetOrgUsers(client)
	if err != nil {
		return err
	}
	if email != "" {
		for i := range users {
			if users[i].Username == email {
				userID = users[i].Id
				break
			}
		}
		if userID == "" {
			return ErrUserNotFound
		}
	} else {
		user, err := SelectUser(users, false)
		userID = user.Id
		email = user.Username
		if user.WorkspaceRoles != nil {
			workspaceRoles = user.WorkspaceRoles
		}
		if err != nil {
			return err
		}
	}
	orgRole := astroiamcore.UpdateUserRolesRequestOrganizationRole(role)
	updateUserRolesInput := astroiamcore.UpdateUserRolesRequest{
		OrganizationRole: &orgRole,
		WorkspaceRoles:   workspaceRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, updateUserRolesInput)
	if err != nil {
		return err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
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

func SelectUser(users []astroiamcore.User, workspace bool) (astroiamcore.User, error) {
	roleColumn := "ORGANIZATION ROLE"
	if workspace {
		roleColumn = "WORKSPACE ROLE"
	}
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "FULLNAME", "EMAIL", "ID", roleColumn, "CREATE DATE"},
	}

	fmt.Println("\nPlease select the user:")

	userMap := map[string]astroiamcore.User{}
	for i := range users {
		index := i + 1
		if workspace {
			table.AddRow([]string{
				strconv.Itoa(index),
				users[i].FullName,
				users[i].Username,
				users[i].Id,
				string((*users[i].WorkspaceRoles)[0].Role),
				users[i].CreatedAt.Format(time.RFC3339),
			}, false)
		} else {
			table.AddRow([]string{
				strconv.Itoa(index),
				users[i].FullName,
				users[i].Username,
				users[i].Id,
				string(*users[i].OrganizationRole),
				users[i].CreatedAt.Format(time.RFC3339),
			}, false)
		}
		userMap[strconv.Itoa(index)] = users[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := userMap[choice]
	if !ok {
		return astroiamcore.User{}, ErrInvalidUserKey
	}
	return selected, nil
}

// Returns a list of all of an organizations users
func GetOrgUsers(client astroiamcore.CoreClient) ([]astroiamcore.User, error) {
	offset := 0
	var users []astroiamcore.User

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if ctx.Organization == "" {
		return nil, ErrNoOrganizationID
	}

	for {
		resp, err := client.ListUsersWithResponse(httpContext.Background(), ctx.Organization, &astroiamcore.ListUsersParams{
			Offset: &offset,
			Limit:  &userPaginationLimit,
		})
		if err != nil {
			return nil, err
		}
		err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		users = append(users, resp.JSON200.Users...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += userPaginationLimit
	}

	return users, nil
}

// Prints a list of all of an organizations users
func ListOrgUsers(out io.Writer, client astroiamcore.CoreClient) error {
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
			string(*users[i].OrganizationRole),
			users[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}

func AddWorkspaceUser(email, role, workspace string, out io.Writer, client astroiamcore.CoreClient) error {
	var workspaceRoles []astroiamcore.WorkspaceRole
	err := IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.Organization == "" {
		return ErrNoOrganizationID
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	// Get all org users. Setting limit to 1000 for now
	users, err := GetOrgUsers(client)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, false)
	if err != nil {
		return err
	}
	user, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	orgRole := astroiamcore.UpdateUserRolesRequestOrganizationRole(*user.OrganizationRole)
	if user.WorkspaceRoles != nil {
		_, _, found := lo.FindIndexOf(*user.WorkspaceRoles, func(role astroiamcore.WorkspaceRole) bool {
			return role.WorkspaceId == workspace
		})
		if found {
			return ErrHasExistingWorkspaceRole
		}
		workspaceRoles = append(*user.WorkspaceRoles, astroiamcore.WorkspaceRole{
			Role:        astroiamcore.WorkspaceRoleRole(role),
			WorkspaceId: workspace,
		})
	} else {
		workspaceRoles = []astroiamcore.WorkspaceRole{
			{
				Role:        astroiamcore.WorkspaceRoleRole(role),
				WorkspaceId: workspace,
			},
		}
	}

	updateUserRoles := astroiamcore.UpdateUserRolesRequest{
		OrganizationRole: &orgRole,
		WorkspaceRoles:   &workspaceRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, updateUserRoles)
	if err != nil {
		return err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s was successfully added to the workspace with the role %s\n", email, role)
	return nil
}

func UpdateWorkspaceUserRole(email, role, workspace string, out io.Writer, client astroiamcore.CoreClient) error {
	err := IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.Organization == "" {
		return ErrNoOrganizationID
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	// Get all workspace users. Setting limit to 1000 for now
	users, err := GetWorkspaceUsers(client, workspace, userPaginationLimit)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, true)
	if err != nil {
		return err
	}
	user, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	orgRole := astroiamcore.UpdateUserRolesRequestOrganizationRole(*user.OrganizationRole)
	if user.WorkspaceRoles == nil {
		return ErrNoExistingWorkspaceRole
	}
	workspaceRoles := *user.WorkspaceRoles
	_, idx, found := lo.FindIndexOf(*user.WorkspaceRoles, func(role astroiamcore.WorkspaceRole) bool {
		return role.WorkspaceId == workspace
	})
	if !found {
		return ErrNoExistingWorkspaceRole
	}
	workspaceRoles[idx].Role = astroiamcore.WorkspaceRoleRole(role)

	updateUserRoles := astroiamcore.UpdateUserRolesRequest{
		OrganizationRole: &orgRole,
		WorkspaceRoles:   &workspaceRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, updateUserRoles)
	if err != nil {
		fmt.Println("error in UpdateUserRolesWithResponse")
		return err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		fmt.Println("error in NormalizeAPIError")
		return err
	}
	fmt.Fprintf(out, "The workspace user %s role was successfully updated to %s\n", email, role)
	return nil
}

// IsWorkspaceRoleValid checks if the requested role is valid
// If the role is valid, it returns nil
// error ErrInvalidWorkspaceRole is returned if the role is not valid
func IsWorkspaceRoleValid(role string) error {
	validRoles := []string{"WORKSPACE_MEMBER", "WORKSPACE_AUTHOR", "WORKSPACE_OPERATOR", "WORKSPACE_OWNER"}
	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return ErrInvalidWorkspaceRole
}

// IsOrgnaizationRoleValid checks if the requested role is valid
// If the role is valid, it returns nil
// error ErrInvalidOrgnaizationRole is returned if the role is not valid
func IsOrganizationRoleValid(role string) error {
	validRoles := []string{"ORGANIZATION_MEMBER", "ORGANIZATION_BILLING_ADMIN", "ORGANIZATION_OWNER"}
	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return ErrInvalidOrganizationRole
}

// Returns a list of all of a workspace's users
func GetWorkspaceUsers(client astroiamcore.CoreClient, workspace string, limit int) ([]astroiamcore.User, error) {
	offset := 0
	var users []astroiamcore.User

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if ctx.Organization == "" {
		return nil, ErrNoOrganizationID
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	for {
		resp, err := client.ListUsersWithResponse(httpContext.Background(), ctx.Organization, &astroiamcore.ListUsersParams{
			Offset:      &offset,
			Limit:       &limit,
			WorkspaceId: &workspace,
		})
		if err != nil {
			return nil, err
		}
		err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		users = append(users, resp.JSON200.Users...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += limit
	}

	return users, nil
}

// Prints a list of all of a workspace's users
func ListWorkspaceUsers(out io.Writer, client astroiamcore.CoreClient, workspace string) error {
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"FULLNAME", "EMAIL", "ID", "WORKSPACE ROLE", "CREATE DATE"},
	}
	users, err := GetWorkspaceUsers(client, workspace, userPaginationLimit)
	if err != nil {
		return err
	}

	for i := range users {
		table.AddRow([]string{
			users[i].FullName,
			users[i].Username,
			users[i].Id,
			string((*users[i].WorkspaceRoles)[0].Role),
			users[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}

func RemoveWorkspaceUser(email, workspace string, out io.Writer, client astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.Organization == "" {
		return ErrNoOrganizationID
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	// Get all workspace users. Setting limit to 1000 for now
	users, err := GetWorkspaceUsers(client, workspace, userPaginationLimit)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, true)
	if err != nil {
		return err
	}
	user, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	orgRole := astroiamcore.UpdateUserRolesRequestOrganizationRole(*user.OrganizationRole)
	if user.WorkspaceRoles == nil {
		return ErrNoExistingWorkspaceRole
	}
	workspaceRoles := lo.Filter(*user.WorkspaceRoles, func(i astroiamcore.WorkspaceRole, _ int) bool {
		return i.WorkspaceId != workspace
	})
	if len(workspaceRoles) == len(*user.WorkspaceRoles) {
		return ErrNoExistingWorkspaceRole
	}

	updateUserRoles := astroiamcore.UpdateUserRolesRequest{
		OrganizationRole: &orgRole,
		WorkspaceRoles:   &workspaceRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, updateUserRoles)
	if err != nil {
		fmt.Println("error in UpdateUserRolesWithResponse")
		return err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s was successfully removed from the workspace\n", email)
	return nil
}

func getUserID(email string, users []astroiamcore.User, workspace bool) (userID, newEmail string, err error) {
	if email == "" {
		user, err := SelectUser(users, workspace)
		userID = user.Id
		email = user.Username
		if err != nil {
			return "", email, err
		}
	} else {
		for i := range users {
			if users[i].Username == email {
				userID = users[i].Id
				break
			}
		}
	}
	return userID, email, nil
}

func GetUser(client astroiamcore.CoreClient, userID string) (user astroiamcore.User, err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return user, err
	}
	if ctx.Organization == "" {
		return user, ErrNoOrganizationID
	}

	resp, err := client.GetUserWithResponse(httpContext.Background(), ctx.Organization, userID)
	if err != nil {
		return user, err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return user, err
	}

	user = *resp.JSON200

	return user, nil
}
