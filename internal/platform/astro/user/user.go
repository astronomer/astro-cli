package user

import (
	httpContext "context"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/picker"
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

// CreateInvite invites email to the current Organization with role, and
// returns the invitation it sent.
func CreateInvite(email, role string, client astrov1.APIClient) (Invite, error) {
	var (
		userInviteInput astrov1.CreateUserInviteRequest
		err             error
		ctx             config.Context
	)
	if email == "" {
		return Invite{}, ErrInvalidEmail
	}
	err = IsRoleValid(role)
	if err != nil {
		return Invite{}, err
	}
	ctx, err = context.GetCurrentContext()
	if err != nil {
		return Invite{}, err
	}
	userInviteInput = astrov1.CreateUserInviteRequest{
		InviteeEmail: email,
		Role:         astrov1.CreateUserInviteRequestRole(role),
	}
	resp, err := client.CreateUserInviteWithResponse(httpContext.Background(), ctx.Organization, userInviteInput)
	if err != nil {
		return Invite{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return Invite{}, err
	}
	inv := Invite{Email: email, Role: role, OrganizationID: ctx.Organization}
	if got := resp.JSON200; got != nil {
		inv.InviteID = got.InviteId
		if got.OrganizationId != "" {
			inv.OrganizationID = got.OrganizationId
		}
		if got.UserId != nil {
			inv.UserID = *got.UserId
		}
		if !got.ExpiresAt.IsZero() {
			expires := got.ExpiresAt
			inv.ExpiresAt = &expires
		}
	}
	return inv, nil
}

// Info is u as a user command reports it, with no role: the caller sets the
// one on the object the command is about.
func Info(u *astrov1.User) UserInfo {
	return UserInfo{FullName: u.FullName, Email: u.Username, ID: u.Id, CreatedAt: u.CreatedAt}
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

// UpdateUserRole sets the Organization role of the user with email, or of
// the one picked when email is "", and returns the user with that role.
func UpdateUserRole(email, role string, client astrov1.APIClient) (UserInfo, error) {
	err := IsRoleValid(role)
	if err != nil {
		return UserInfo{}, err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return UserInfo{}, err
	}
	// Get all org users
	users, err := GetOrgUsers(client)
	if err != nil {
		return UserInfo{}, err
	}
	user, err := findUser(email, users, "organization")
	if err != nil {
		return UserInfo{}, err
	}
	mutateUserInput := astrov1.UpdateUserRolesRequest{
		OrganizationRole: &role,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, user.Id, mutateUserInput)
	if err != nil {
		return UserInfo{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return UserInfo{}, err
	}
	info := Info(&user)
	info.OrgRole = role
	return info, nil
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

	list := picker.List{
		Title:   "\nPlease select the user:",
		Header:  []string{"FULLNAME", "EMAIL", "ID", roleColumn, "CREATE DATE"},
		Ask:     []input.Option{input.About("a user")},
		Invalid: ErrInvalidUserKey,
	}
	for i := range users {
		list.AddRow(false,
			users[i].FullName,
			users[i].Username,
			users[i].Id,
			userRoleForScope(users[i], roleEntity, ""),
			users[i].CreatedAt.Format(time.RFC3339),
		)
	}
	i, err := list.Pick(os.Stdout, os.Stdin)
	if err != nil {
		return astrov1.User{}, err
	}
	return users[i], nil
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

// AddWorkspaceUser gives the Organization user with email, or the one picked
// when email is "", role on the Workspace (the current one when workspaceID
// is ""), and returns the user with that role.
func AddWorkspaceUser(email, role, workspaceID string, client astrov1.APIClient) (UserInfo, error) {
	err := IsWorkspaceRoleValid(role)
	if err != nil {
		return UserInfo{}, err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return UserInfo{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	users, err := GetOrgUsers(client)
	if err != nil {
		return UserInfo{}, err
	}
	return setWorkspaceRole(client, ctx.Organization, workspaceID, email, role, users, "organization")
}

// UpdateWorkspaceUserRole sets the role on the Workspace of its user with
// email, or of the one picked when email is "", and returns the user with
// that role.
func UpdateWorkspaceUserRole(email, role, workspaceID string, client astrov1.APIClient) (UserInfo, error) {
	err := IsWorkspaceRoleValid(role)
	if err != nil {
		return UserInfo{}, err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return UserInfo{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	users, err := GetWorkspaceUsers(client, workspaceID, userPaginationLimit)
	if err != nil {
		return UserInfo{}, err
	}
	return setWorkspaceRole(client, ctx.Organization, workspaceID, email, role, users, "workspace")
}

// setWorkspaceRole finds the user among users, by email or through the
// picker, and sets its role on workspaceID, keeping every other role it holds.
func setWorkspaceRole(client astrov1.APIClient, orgID, workspaceID, email, role string, users []astrov1.User, roleEntity string) (UserInfo, error) {
	found, err := findUser(email, users, roleEntity)
	if err != nil {
		return UserInfo{}, err
	}
	current, err := GetUser(client, found.Id)
	if err != nil {
		return UserInfo{}, err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   upsertWorkspaceRole(current.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  current.DeploymentRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), orgID, found.Id, req)
	if err != nil {
		return UserInfo{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return UserInfo{}, err
	}
	// The user as listed, which is where the email the command was given
	// matched.
	info := Info(&found)
	info.WorkspaceRole = role
	return info, nil
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

// RemoveWorkspaceUser removes the role on the Workspace of its user with
// email, or of the one picked when email is "", and returns which user it
// removed from which Workspace.
func RemoveWorkspaceUser(email, workspaceID string, client astrov1.APIClient) (WorkspaceRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	users, err := GetWorkspaceUsers(client, workspaceID, userPaginationLimit)
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	found, err := findUser(email, users, "workspace")
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	current, err := GetUser(client, found.Id)
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   upsertWorkspaceRole(current.WorkspaceRoles, workspaceID, ""),
		DeploymentRoles:  current.DeploymentRoles,
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), ctx.Organization, found.Id, req)
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return WorkspaceRemoval{}, err
	}
	return WorkspaceRemoval{ID: found.Id, Email: found.Username, WorkspaceID: workspaceID, Action: Removed}, nil
}

// findUser returns the user among users whose email is email, or the one
// picked when email is "".
func findUser(email string, users []astrov1.User, roleEntity string) (astrov1.User, error) {
	if email == "" {
		return SelectUser(users, roleEntity)
	}
	var found astrov1.User
	for i := range users {
		if users[i].Username == email {
			found = users[i]
		}
	}
	if found.Id == "" {
		return astrov1.User{}, ErrUserNotFound
	}
	return found, nil
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

// AddDeploymentUser gives the Organization user with email, or the one
// picked when email is "", role on the Deployment with deploymentID, and
// returns the user with that role.
func AddDeploymentUser(email, role, deploymentID string, client astrov1.APIClient) (UserInfo, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return UserInfo{}, err
	}
	users, err := GetOrgUsers(client)
	if err != nil {
		return UserInfo{}, err
	}
	return setDeploymentRole(client, ctx.Organization, deploymentID, email, role, users, "organization")
}

// UpdateDeploymentUserRole sets the role on the Deployment of its user with
// email, or of the one picked when email is "", and returns the user with
// that role.
func UpdateDeploymentUserRole(email, role, deploymentID string, client astrov1.APIClient) (UserInfo, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return UserInfo{}, err
	}
	users, err := GetDeploymentUsers(client, deploymentID, userPaginationLimit)
	if err != nil {
		return UserInfo{}, err
	}
	return setDeploymentRole(client, ctx.Organization, deploymentID, email, role, users, "deployment")
}

// RemoveDeploymentUser removes the role on the Deployment of its user with
// email, or of the one picked when email is "", and returns which user it
// removed from which Deployment.
func RemoveDeploymentUser(email, deploymentID string, client astrov1.APIClient) (DeploymentRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return DeploymentRemoval{}, err
	}
	users, err := GetDeploymentUsers(client, deploymentID, userPaginationLimit)
	if err != nil {
		return DeploymentRemoval{}, err
	}
	found, err := putDeploymentRole(client, ctx.Organization, deploymentID, email, "", users, "deployment")
	if err != nil {
		return DeploymentRemoval{}, err
	}
	return DeploymentRemoval{ID: found.Id, Email: found.Username, DeploymentID: deploymentID, Action: Removed}, nil
}

// setDeploymentRole finds the user among users, by email or through the
// picker, sets its role on deploymentID, and returns it with that role.
func setDeploymentRole(client astrov1.APIClient, orgID, deploymentID, email, role string, users []astrov1.User, roleEntity string) (UserInfo, error) {
	found, err := putDeploymentRole(client, orgID, deploymentID, email, role, users, roleEntity)
	if err != nil {
		return UserInfo{}, err
	}
	// The user as listed, which is where the email the command was given
	// matched.
	info := Info(&found)
	info.DeploymentRole = role
	return info, nil
}

// putDeploymentRole finds the user among users, by email or through the
// picker, and sets its role on deploymentID to role, or removes it when role
// is "", keeping every other role it holds. It returns the user found.
func putDeploymentRole(client astrov1.APIClient, orgID, deploymentID, email, role string, users []astrov1.User, roleEntity string) (astrov1.User, error) {
	found, err := findUser(email, users, roleEntity)
	if err != nil {
		return astrov1.User{}, err
	}
	current, err := GetUser(client, found.Id)
	if err != nil {
		return astrov1.User{}, err
	}
	req := astrov1.UpdateUserRolesRequest{
		OrganizationRole: orgRolePtr(current),
		WorkspaceRoles:   current.WorkspaceRoles,
		DeploymentRoles:  upsertDeploymentRole(current.DeploymentRoles, deploymentID, role),
	}
	resp, err := client.UpdateUserRolesWithResponse(httpContext.Background(), orgID, found.Id, req)
	if err != nil {
		return astrov1.User{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return astrov1.User{}, err
	}
	return found, nil
}

// GetDeploymentUsers returns users with a role in the given deployment.
func GetDeploymentUsers(client astrov1.APIClient, deploymentID string, _ int) ([]astrov1.User, error) {
	dID := deploymentID
	return listUsers(client, nil, &dID)
}

// ListDeploymentUsersData returns deployment user list data for structured output
//
//nolint:dupl // the duplication is acceptable here
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
func ListDeploymentUsersWithFormat(client astrov1.APIClient, deploymentID string, r output.Emitter) error {
	return output.PrintData(
		func() (*UserList, error) { return ListDeploymentUsersData(client, deploymentID) },
		deploymentUserTableConfig, r,
	)
}

// ListWorkspaceUsersData returns workspace user list data for structured output
//
//nolint:dupl // the duplication is acceptable here
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
func ListWorkspaceUsersWithFormat(client astrov1.APIClient, workspaceID string, r output.Emitter) error {
	return output.PrintData(
		func() (*UserList, error) { return ListWorkspaceUsersData(client, workspaceID) },
		workspaceUserTableConfig, r,
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
func ListOrgUsersWithFormat(client astrov1.APIClient, r output.Emitter) error {
	return output.PrintData(
		func() (*UserList, error) { return ListOrgUsersData(client) },
		orgUserTableConfig, r,
	)
}
