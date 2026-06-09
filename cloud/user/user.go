package user

import (
	httpContext "context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	ErrInvalidRole             = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidWorkspaceRole    = errors.New("requested role is invalid. Possible values are WORKSPACE_MEMBER, WORKSPACE_AUTHOR, WORKSPACE_OPERATOR and WORKSPACE_OWNER ")
	ErrInvalidOrganizationRole = errors.New("requested role is invalid. Possible values are ORGANIZATION_MEMBER, ORGANIZATION_BILLING_ADMIN and ORGANIZATION_OWNER ")
	ErrInvalidEmail            = errors.New("no email provided for the invite. Retry with a valid email address")
	ErrInvalidUserKey          = errors.New("invalid User selected")
	userPaginationLimit        = 100
	ErrUserNotFound            = errors.New("no user was found for the email you provided")
)

func CreateInvite(email, role string, out io.Writer, client astrov1.APIClient) error {
	var (
		userInviteInput astrov1.CreateUserInviteRequest
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
	userInviteInput = astrov1.CreateUserInviteRequest{
		InviteeEmail: email,
		Role:         astrov1.CreateUserInviteRequestRole(role),
	}
	resp, err := client.CreateUserInviteWithResponse(httpContext.Background(), ctx.Organization, userInviteInput)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "invite for %s with role %s created\n", email, role)
	return nil
}

// orgRolePtr returns a pointer to the user's current organization role (as a plain string),
// or nil if it is unset. v1's UpdateUserRoles requires specifying the Organization role whenever
// Workspace or Deployment roles are updated, so we round-trip it from GetUser.
func orgRolePtr(user astrov1.User) *string { //nolint:gocritic // User is large; helper returns a short pointer
	if user.OrganizationRole == nil {
		return nil
	}
	s := string(*user.OrganizationRole)
	return &s
}

func UpdateUserRole(email, role string, out io.Writer, client astrov1.APIClient) error {
	var userID string
	err := IsRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
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
			}
		}
		if userID == "" {
			return ErrUserNotFound
		}
	} else {
		user, err := SelectUser(users, "organization")
		if err != nil {
			return err
		}
		userID = user.Id
		email = user.Username
	}
	mutateUserInput := astrov1.UpdateUserRolesRequest{
		OrganizationRole: &role,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, mutateUserInput)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
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

// userRoleForScope returns the string role the user has in the given scope (org/workspace/deployment),
// resolved against the supplied workspace/deployment ID. Returns "" if the user has no role for that scope.
func userRoleForScope(user astrov1.User, roleEntity, scopeID string) string { //nolint:gocritic // User is large; helper returns a short string
	switch roleEntity {
	case "workspace":
		if user.WorkspaceRoles == nil {
			return ""
		}
		for _, r := range *user.WorkspaceRoles {
			if r.WorkspaceId == scopeID {
				return string(r.Role)
			}
		}
		return ""
	case "deployment":
		if user.DeploymentRoles == nil {
			return ""
		}
		for _, r := range *user.DeploymentRoles {
			if r.DeploymentId == scopeID {
				return r.Role
			}
		}
		return ""
	default:
		if user.OrganizationRole == nil {
			return ""
		}
		return string(*user.OrganizationRole)
	}
}

func SelectUser(users []astrov1.User, roleEntity string) (astrov1.User, error) {
	roleColumn := "ORGANIZATION ROLE"
	switch roleEntity {
	case "workspace":
		roleColumn = "WORKSPACE ROLE"
	case "deployment":
		roleColumn = "DEPLOYMENT ROLE"
	}

	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "FULLNAME", "EMAIL", "ID", roleColumn, "CREATE DATE"},
	}

	fmt.Println("\nPlease select the user:")

	userMap := map[string]astrov1.User{}
	for i := range users {
		index := i + 1
		table.AddRow([]string{
			strconv.Itoa(index),
			users[i].FullName,
			users[i].Username,
			users[i].Id,
			userRoleForScope(users[i], roleEntity, ""),
			users[i].CreatedAt.Format(time.RFC3339),
		}, false)

		userMap[strconv.Itoa(index)] = users[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := userMap[choice]
	if !ok {
		return astrov1.User{}, ErrInvalidUserKey
	}
	return selected, nil
}

// GetOrgUsers returns a list of all organization users.
func GetOrgUsers(client astrov1.APIClient) ([]astrov1.User, error) {
	return listUsers(client, nil, nil)
}

// listUsers paginates through GET /users with optional workspaceId/deploymentId filters.
func listUsers(client astrov1.APIClient, workspaceID, deploymentID *string) ([]astrov1.User, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var users []astrov1.User
	offset := 0
	for {
		params := &astrov1.ListUsersParams{
			Offset:       &offset,
			Limit:        &userPaginationLimit,
			WorkspaceId:  workspaceID,
			DeploymentId: deploymentID,
		}
		resp, err := client.ListUsersWithResponse(httpContext.Background(), ctx.Organization, params)
		if err != nil {
			return nil, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		users = append(users, resp.JSON200.Users...)

		if resp.JSON200.TotalCount <= offset+userPaginationLimit {
			break
		}
		offset += userPaginationLimit
	}
	return users, nil
}

// upsertWorkspaceRole returns a new workspace-role slice with the role for workspaceID
// set to role (added if missing). If role == "", the entry is removed.
func upsertWorkspaceRole(existing *[]astrov1.WorkspaceRole, workspaceID, role string) *[]astrov1.WorkspaceRole {
	out := []astrov1.WorkspaceRole{}
	if existing != nil {
		for _, r := range *existing {
			if r.WorkspaceId == workspaceID {
				continue
			}
			out = append(out, r)
		}
	}
	if role != "" {
		out = append(out, astrov1.WorkspaceRole{
			WorkspaceId: workspaceID,
			Role:        astrov1.WorkspaceRoleRole(role),
		})
	}
	return &out
}

// upsertDeploymentRole mirrors upsertWorkspaceRole for deployment-scoped roles.
func upsertDeploymentRole(existing *[]astrov1.DeploymentRole, deploymentID, role string) *[]astrov1.DeploymentRole {
	out := []astrov1.DeploymentRole{}
	if existing != nil {
		for _, r := range *existing {
			if r.DeploymentId == deploymentID {
				continue
			}
			out = append(out, r)
		}
	}
	if role != "" {
		out = append(out, astrov1.DeploymentRole{
			DeploymentId: deploymentID,
			Role:         role,
		})
	}
	return &out
}

func AddWorkspaceUser(email, role, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	err := IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	users, err := GetOrgUsers(client)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, "organization")
	if err != nil {
		return err
	}
	current, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   upsertWorkspaceRole(current.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  current.DeploymentRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s was successfully added to the workspace with the role %s\n", email, role)
	return nil
}

func UpdateWorkspaceUserRole(email, role, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	err := IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	users, err := GetWorkspaceUsers(client, workspaceID, userPaginationLimit)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, "workspace")
	if err != nil {
		return err
	}
	current, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   upsertWorkspaceRole(current.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  current.DeploymentRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
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

// IsOrganizationRoleValid checks if the requested role is valid
// If the role is valid, it returns nil
// error ErrInvalidOrganizationRole is returned if the role is not valid
func IsOrganizationRoleValid(role string) error {
	validRoles := []string{"ORGANIZATION_MEMBER", "ORGANIZATION_BILLING_ADMIN", "ORGANIZATION_OWNER"}
	for _, validRole := range validRoles {
		if role == validRole {
			return nil
		}
	}
	return ErrInvalidOrganizationRole
}

// GetWorkspaceUsers returns users with a role in the given workspace.
func GetWorkspaceUsers(client astrov1.APIClient, workspaceID string, _ int) ([]astrov1.User, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	wsID := workspaceID
	return listUsers(client, &wsID, nil)
}

func RemoveWorkspaceUser(email, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	users, err := GetWorkspaceUsers(client, workspaceID, userPaginationLimit)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, "workspace")
	if err != nil {
		return err
	}
	current, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   upsertWorkspaceRole(current.WorkspaceRoles, workspaceID, ""),
		DeploymentRoles:  current.DeploymentRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s was successfully removed from the workspace\n", email)
	return nil
}

func getUserID(email string, users []astrov1.User, roleEntity string) (userID, newEmail string, err error) {
	if email == "" {
		user, err := SelectUser(users, roleEntity)
		if err != nil {
			return "", user.Username, err
		}
		return user.Id, user.Username, nil
	}
	for i := range users {
		if users[i].Username == email {
			userID = users[i].Id
		}
	}
	if userID == "" {
		return userID, email, ErrUserNotFound
	}
	return userID, email, nil
}

func GetUser(client astrov1.APIClient, userID string) (user astrov1.User, err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return user, err
	}

	resp, err := client.GetUserWithResponse(httpContext.Background(), ctx.Organization, userID)
	if err != nil {
		return user, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return user, err
	}

	return *resp.JSON200, nil
}

func AddDeploymentUser(email, role, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	users, err := GetOrgUsers(client)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, "organization")
	if err != nil {
		return err
	}
	current, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   current.WorkspaceRoles,
		DeploymentRoles:  upsertDeploymentRole(current.DeploymentRoles, deploymentID, role),
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s was successfully added to the deployment with the role %s\n", email, role)
	return nil
}

func UpdateDeploymentUserRole(email, role, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	users, err := GetDeploymentUsers(client, deploymentID, userPaginationLimit)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, "deployment")
	if err != nil {
		return err
	}
	current, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   current.WorkspaceRoles,
		DeploymentRoles:  upsertDeploymentRole(current.DeploymentRoles, deploymentID, role),
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "The deployment user %s role was successfully updated to %s\n", email, role)
	return nil
}

func RemoveDeploymentUser(email, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	users, err := GetDeploymentUsers(client, deploymentID, userPaginationLimit)
	if err != nil {
		return err
	}
	userID, email, err := getUserID(email, users, "deployment")
	if err != nil {
		return err
	}
	current, err := GetUser(client, userID)
	if err != nil {
		return err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   current.WorkspaceRoles,
		DeploymentRoles:  upsertDeploymentRole(current.DeploymentRoles, deploymentID, ""),
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, userID, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "The user %s was successfully removed from the deployment\n", email)
	return nil
}

// GetDeploymentUsers returns users with a role in the given deployment.
func GetDeploymentUsers(client astrov1.APIClient, deploymentID string, _ int) ([]astrov1.User, error) {
	dID := deploymentID
	return listUsers(client, nil, &dID)
}

// ListDeploymentUsersData returns deployment user list data for structured output
//
//nolint:dupl
func ListDeploymentUsersData(client astrov1.APIClient, deploymentID string) (*UserList, error) {
	users, err := GetDeploymentUsers(client, deploymentID, userPaginationLimit)
	if err != nil {
		return nil, err
	}

	userInfos := make([]UserInfo, 0, len(users))
	for i := range users {
		userInfos = append(userInfos, UserInfo{
			FullName:       users[i].FullName,
			Email:          users[i].Username,
			ID:             users[i].Id,
			DeploymentRole: userRoleForScope(users[i], "deployment", deploymentID),
			CreatedAt:      users[i].CreatedAt,
		})
	}

	return &UserList{Users: userInfos}, nil
}

// userTableConfigWithRoleColumn builds a UserList table with a role column whose header
// and value function vary per scope (deployment / workspace / org).
func userTableConfigWithRoleColumn(roleHeader string, role func(UserInfo) string) *output.TableConfig {
	return output.BuildTableConfig(
		[]output.Column[UserInfo]{
			{Header: "FULLNAME", Value: func(u UserInfo) string { return u.FullName }},
			{Header: "EMAIL", Value: func(u UserInfo) string { return u.Email }},
			{Header: "ID", Value: func(u UserInfo) string { return u.ID }},
			{Header: roleHeader, Value: role},
			{Header: "CREATE DATE", Value: func(u UserInfo) string { return u.CreatedAt.Format(time.RFC3339) }},
		},
		func(d any) []UserInfo { return d.(*UserList).Users },
		output.WithPadding([]int{30, 50, 10, 50, 10, 10, 10}),
	)
}

var deploymentUserTableConfig = userTableConfigWithRoleColumn("DEPLOYMENT ROLE", func(u UserInfo) string { return u.DeploymentRole })

// ListDeploymentUsersWithFormat lists deployment users with the specified output format
func ListDeploymentUsersWithFormat(client astrov1.APIClient, deploymentID string, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*UserList, error) { return ListDeploymentUsersData(client, deploymentID) },
		deploymentUserTableConfig, format, tmpl, out,
	)
}

// ListWorkspaceUsersData returns workspace user list data for structured output
//
//nolint:dupl
func ListWorkspaceUsersData(client astrov1.APIClient, workspaceID string) (*UserList, error) {
	users, err := GetWorkspaceUsers(client, workspaceID, userPaginationLimit)
	if err != nil {
		return nil, err
	}

	userInfos := make([]UserInfo, 0, len(users))
	for i := range users {
		userInfos = append(userInfos, UserInfo{
			FullName:      users[i].FullName,
			Email:         users[i].Username,
			ID:            users[i].Id,
			WorkspaceRole: userRoleForScope(users[i], "workspace", workspaceID),
			CreatedAt:     users[i].CreatedAt,
		})
	}

	return &UserList{Users: userInfos}, nil
}

var workspaceUserTableConfig = userTableConfigWithRoleColumn("WORKSPACE ROLE", func(u UserInfo) string { return u.WorkspaceRole })

// ListWorkspaceUsersWithFormat lists workspace users with the specified output format
func ListWorkspaceUsersWithFormat(client astrov1.APIClient, workspaceID string, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*UserList, error) { return ListWorkspaceUsersData(client, workspaceID) },
		workspaceUserTableConfig, format, tmpl, out,
	)
}

// ListOrgUsersData returns organization user list data for structured output
func ListOrgUsersData(client astrov1.APIClient) (*UserList, error) {
	users, err := GetOrgUsers(client)
	if err != nil {
		return nil, err
	}

	userInfos := make([]UserInfo, 0, len(users))
	for i := range users {
		orgRole := ""
		if users[i].OrganizationRole != nil {
			orgRole = string(*users[i].OrganizationRole)
		}
		userInfos = append(userInfos, UserInfo{
			FullName:  users[i].FullName,
			Email:     users[i].Username,
			ID:        users[i].Id,
			OrgRole:   orgRole,
			CreatedAt: users[i].CreatedAt,
		})
	}

	return &UserList{Users: userInfos}, nil
}

var orgUserTableConfig = userTableConfigWithRoleColumn("ORGANIZATION ROLE", func(u UserInfo) string { return u.OrgRole })

// ListOrgUsersWithFormat lists organization users with the specified output format
func ListOrgUsersWithFormat(client astrov1.APIClient, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*UserList, error) { return ListOrgUsersData(client) },
		orgUserTableConfig, format, tmpl, out,
	)
}
