package team

import (
	httpContext "context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	ErrInvalidTeamID            = errors.New("team could not be found with selected or passed in id")
	ErrInvalidTeamKey           = errors.New("invalid team selection")
	ErrInvalidTeamMemberKey     = errors.New("invalid team member selection")
	ErrInvalidName              = errors.New("no name provided for the team. Retry with a valid name")
	ErrTeamNotFound             = errors.New("no team was found for the ID you provided")
	ErrNoTeamsFoundInOrg        = errors.New("no teams found in your organization")
	ErrNoTeamsFoundInWorkspace  = errors.New("no teams found in your workspace")
	ErrNoTeamsFoundInDeployment = errors.New("no teams found in your deployment")
	ErrNoTeamMembersFoundInTeam = errors.New("no team members found in team")
	ErrNoUsersFoundInOrg        = errors.New("no users found in your organization")
	ErrNoTeamNameProvided       = errors.New("you must give your Team a name")
	teamPaginationLimit         = 100
)

// confirmOperation asks before changing an IDP-managed team, unless force. A
// run that may not ask returns the refusal rather than an answer.
func confirmOperation(force bool) (bool, error) {
	if force {
		return true, nil
	}
	return input.Confirm("This is an IDP-managed team. Are you sure you want to continue the operation?", input.AnsweredBy("--yes"))
}

// Info is t as a team command reports it, with no role: the caller sets the
// one on the object the command is about.
func Info(t *astrov1.Team) TeamInfo {
	info := TeamInfo{ID: t.Id, Name: t.Name, CreatedAt: t.CreatedAt, IsIdpManaged: t.IsIdpManaged}
	if t.Description != nil {
		info.Description = *t.Description
	}
	return info
}

// CreateTeam creates a team in the current Organization, asking for its name
// when name is "", and returns the team it created.
func CreateTeam(name, description, role string, client astrov1.APIClient) (TeamInfo, error) {
	err := user.IsOrganizationRoleValid(role)
	if err != nil {
		return TeamInfo{}, err
	}
	if name == "" {
		prompt := ansi.Bold("\nTeam name: ")
		if err := input.MayAsk(prompt, input.About("a Team name"), input.AnsweredBy("--name")); err != nil {
			return TeamInfo{}, err
		}
		fmt.Println("Please specify a name for your Team")
		name, err = input.Text(prompt, input.About("a Team name"), input.AnsweredBy("--name"))
		if err != nil {
			return TeamInfo{}, err
		}
		if name == "" {
			return TeamInfo{}, ErrNoTeamNameProvided
		}
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	typedRole := astrov1.CreateTeamRequestOrganizationRole(role)
	teamCreateRequest := astrov1.CreateTeamJSONRequestBody{
		Description:      &description,
		Name:             name,
		OrganizationRole: &typedRole,
	}
	resp, err := client.CreateTeamWithResponse(httpContext.Background(), ctx.Organization, teamCreateRequest)
	if err != nil {
		return TeamInfo{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return TeamInfo{}, err
	}
	if resp.JSON200 == nil {
		return TeamInfo{Name: name, Description: description, OrgRole: role}, nil
	}
	info := Info(resp.JSON200)
	info.OrgRole = orDefault(string(resp.JSON200.OrganizationRole), role)
	return info, nil
}

// orDefault is s, or def when s is "".
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func GetTeam(client astrov1.APIClient, teamID string) (team astrov1.Team, err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return team, err
	}
	resp, err := client.GetTeamWithResponse(httpContext.Background(), ctx.Organization, teamID)
	if err != nil {
		return team, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return team, err
	}

	team = *resp.JSON200

	return team, nil
}

// teamOrgRole returns the team's current organization role as a string, which is required
// by UpdateTeamRolesRequest whenever any scoped roles are changed.
func teamOrgRole(team astrov1.Team) string { //nolint:gocritic // Team is large; helper returns a short string
	return string(team.OrganizationRole)
}

// upsertTeamWorkspaceRole returns a new workspace-role slice with workspaceID's role set to role
// (added if missing). If role == "", the entry is removed.
func upsertTeamWorkspaceRole(existing *[]astrov1.WorkspaceRole, workspaceID, role string) *[]astrov1.WorkspaceRole {
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

// upsertTeamDeploymentRole mirrors upsertTeamWorkspaceRole for deployment-scoped team roles.
func upsertTeamDeploymentRole(existing *[]astrov1.DeploymentRole, deploymentID, role string) *[]astrov1.DeploymentRole {
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

func updateTeamRoles(client astrov1.APIClient, orgID, teamID string, req astrov1.UpdateTeamRolesRequest) error {
	resp, err := client.UpdateTeamRolesWithResponse(httpContext.Background(), orgID, teamID, req)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// UpdateWorkspaceTeamRole sets the role on the Workspace of the team with
// id, or of the Workspace's team picked when id is "", and returns the team
// with that role.
func UpdateWorkspaceTeamRole(id, role, workspaceID string, client astrov1.APIClient) (TeamInfo, error) {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return TeamInfo{}, err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspaceID, teamPaginationLimit)
		if err != nil {
			return TeamInfo{}, err
		}
		if len(teams) == 0 {
			return TeamInfo{}, ErrNoTeamsFoundInWorkspace
		}
		team, err = selectTeam(teams)
		if err != nil {
			return TeamInfo{}, err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return TeamInfo{}, err
		}
		if team.Id == "" {
			return TeamInfo{}, ErrTeamNotFound
		}
	}

	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   upsertTeamWorkspaceRole(team.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  team.DeploymentRoles,
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return TeamInfo{}, err
	}
	info := Info(&team)
	info.WorkspaceRole = role
	return info, nil
}

// UpdateTeam changes the name, description and Organization role of the team
// with id, or of the one picked when id is "": only what is given, so "" keeps
// each as it is. It returns nil, and changes nothing, when the team is
// IdP-managed and the person declines to go on.
//
// The name and description are sent before the role is checked, so a role
// that is not one, or that the API refuses, fails after they changed; the
// Update returned with that error says so.
func UpdateTeam(id, name, description, role string, force bool, client astrov1.APIClient) (*Update, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return nil, err
		}
		if len(teams) == 0 {
			return nil, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil {
			return nil, err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return nil, err
		}
		if team.Id == "" {
			return nil, ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y, err := confirmOperation(force)
		if err != nil {
			return nil, err
		}
		if !y {
			return nil, nil
		}
	}
	teamID := team.Id
	teamUpdateRequest := astrov1.UpdateTeamJSONRequestBody{}

	if name == "" {
		teamUpdateRequest.Name = team.Name
	} else {
		teamUpdateRequest.Name = name
	}

	if description == "" {
		teamUpdateRequest.Description = team.Description
	} else {
		teamUpdateRequest.Description = &description
	}

	resp, err := client.UpdateTeamWithResponse(httpContext.Background(), ctx.Organization, teamID, teamUpdateRequest)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	after := team
	if resp.JSON200 != nil {
		after = *resp.JSON200
	} else {
		after.Name, after.Description = teamUpdateRequest.Name, teamUpdateRequest.Description
	}
	upd := &Update{Team: Info(&after), PreviousName: team.Name}
	upd.Team.OrgRole = orDefault(string(after.OrganizationRole), string(team.OrganizationRole))

	if role != "" {
		if err := user.IsOrganizationRoleValid(role); err != nil {
			return upd, err
		}
		req := astrov1.UpdateTeamRolesRequest{
			OrganizationRole: role,
			WorkspaceRoles:   team.WorkspaceRoles,
			DeploymentRoles:  team.DeploymentRoles,
		}
		if err := updateTeamRoles(client, ctx.Organization, teamID, req); err != nil {
			return upd, err
		}
		upd.RoleChanged = true
		upd.Team.OrgRole = role
	}
	return upd, nil
}

// RemoveWorkspaceTeam removes the role on the Workspace of the team with id,
// or of the Workspace's team picked when id is "", and returns which team it
// removed from which Workspace.
func RemoveWorkspaceTeam(id, workspaceID string, client astrov1.APIClient) (WorkspaceRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspaceID, teamPaginationLimit)
		if err != nil {
			return WorkspaceRemoval{}, err
		}
		if len(teams) == 0 {
			return WorkspaceRemoval{}, ErrNoTeamsFoundInWorkspace
		}
		team, err = selectTeam(teams)
		if err != nil {
			return WorkspaceRemoval{}, err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return WorkspaceRemoval{}, err
		}
		if team.Id == "" {
			return WorkspaceRemoval{}, ErrTeamNotFound
		}
	}
	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   upsertTeamWorkspaceRole(team.WorkspaceRoles, workspaceID, ""),
		DeploymentRoles:  team.DeploymentRoles,
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return WorkspaceRemoval{}, err
	}
	return WorkspaceRemoval{ID: team.Id, Name: team.Name, WorkspaceID: workspaceID, Action: Removed}, nil
}

func selectTeam(teams []astrov1.Team) (astrov1.Team, error) {
	list := picker.List{
		Title:   "\nPlease select a team:",
		Header:  []string{"TEAMNAME", "ID"},
		Ask:     []input.Option{input.About("a team")},
		Invalid: ErrInvalidTeamKey,
	}
	for i := range teams {
		list.AddRow(false, teams[i].Name, teams[i].Id)
	}
	i, err := list.Pick(os.Stdout, os.Stdin)
	if err != nil {
		return astrov1.Team{}, err
	}
	return teams[i], nil
}

// listTeams paginates through GET /teams with optional workspaceId/deploymentId filters.
func listTeams(client astrov1.APIClient, workspaceID, deploymentID *string) ([]astrov1.Team, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	var teams []astrov1.Team
	offset := 0
	for {
		params := &astrov1.ListTeamsParams{
			Offset:       &offset,
			Limit:        &teamPaginationLimit,
			WorkspaceId:  workspaceID,
			DeploymentId: deploymentID,
		}
		resp, err := client.ListTeamsWithResponse(httpContext.Background(), ctx.Organization, params)
		if err != nil {
			return nil, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		teams = append(teams, resp.JSON200.Teams...)

		if resp.JSON200.TotalCount <= offset+teamPaginationLimit {
			break
		}
		offset += teamPaginationLimit
	}
	return teams, nil
}

func GetWorkspaceTeams(client astrov1.APIClient, workspaceID string, _ int) ([]astrov1.Team, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	wsID := workspaceID
	return listTeams(client, &wsID, nil)
}

// AddWorkspaceTeam gives the Organization's team with id, or the one picked
// when id is "", role on the Workspace (the current one when workspaceID is
// ""), and returns the team with that role.
func AddWorkspaceTeam(id, role, workspaceID string, client astrov1.APIClient) (TeamInfo, error) {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return TeamInfo{}, err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return TeamInfo{}, err
		}
		if len(teams) == 0 {
			return TeamInfo{}, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return TeamInfo{}, pickFailed(err)
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return TeamInfo{}, err
		}
		if team.Id == "" {
			return TeamInfo{}, ErrTeamNotFound
		}
	}

	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   upsertTeamWorkspaceRole(team.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  team.DeploymentRoles,
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return TeamInfo{}, err
	}
	info := Info(&team)
	info.WorkspaceRole = role
	return info, nil
}

// pickFailed is the error of a team picker that returned err, or no team:
// ErrInvalidTeamKey, as it has always been, unless the picker refused to ask
// (under --output json), whose refusal names what answers it.
func pickFailed(err error) error {
	if input.IsRequired(err) {
		return err
	}
	return ErrInvalidTeamKey
}

// GetOrgTeams returns a list of all organization teams.
func GetOrgTeams(client astrov1.APIClient) ([]astrov1.Team, error) {
	return listTeams(client, nil, nil)
}

// Delete deletes the team with id, or the one picked when id is "", and
// returns which team it deleted. It returns nil, and deletes nothing, when
// the team is IdP-managed and the person declines to go on.
func Delete(id string, force bool, client astrov1.APIClient) (*OrganizationRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return nil, err
		}
		if len(teams) == 0 {
			return nil, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return nil, pickFailed(err)
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return nil, err
		}
		if team.Id == "" {
			return nil, ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y, err := confirmOperation(force)
		if err != nil {
			return nil, err
		}
		if !y {
			return nil, nil
		}
	}
	teamID := team.Id
	resp, err := client.DeleteTeamWithResponse(httpContext.Background(), ctx.Organization, teamID)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return &OrganizationRemoval{ID: team.Id, Name: team.Name, OrganizationID: ctx.Organization, Action: Deleted}, nil
}

// listTeamMembers fetches all members for a team using the paginated v1 endpoint.
func listTeamMembers(client astrov1.APIClient, orgID, teamID string) ([]astrov1.TeamMember, error) {
	var members []astrov1.TeamMember
	offset := 0
	for {
		params := &astrov1.ListTeamMembersParams{
			Offset: &offset,
			Limit:  &teamPaginationLimit,
		}
		resp, err := client.ListTeamMembersWithResponse(httpContext.Background(), orgID, teamID, params)
		if err != nil {
			return nil, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		members = append(members, resp.JSON200.TeamMembers...)
		if resp.JSON200.TotalCount <= offset+teamPaginationLimit {
			break
		}
		offset += teamPaginationLimit
	}
	return members, nil
}

// RemoveUser removes the member with teamMemberID, or the one picked when it
// is "", from the team with teamID, or the one picked when it is "", and
// returns which user it removed from which team. It returns nil, and removes
// no one, when the team is IdP-managed and the person declines to go on.
func RemoveUser(teamID, teamMemberID string, force bool, client astrov1.APIClient) (*Membership, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return nil, err
		}
		if len(teams) == 0 {
			return nil, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return nil, pickFailed(err)
		}
	} else {
		team, err = GetTeam(client, teamID)
		if err != nil {
			return nil, err
		}
		if team.Id == "" {
			return nil, ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y, err := confirmOperation(force)
		if err != nil {
			return nil, err
		}
		if !y {
			return nil, nil
		}
	}
	teamID = team.Id
	teamMembers, err := listTeamMembers(client, ctx.Organization, teamID)
	if err != nil {
		return nil, err
	}
	if len(teamMembers) == 0 {
		return nil, ErrNoTeamMembersFoundInTeam
	}

	var teamMemberSelection astrov1.TeamMember
	if teamMemberID == "" {
		teamMemberSelection, err = selectTeamMember(teamMembers)
		if err != nil {
			return nil, err
		}
	} else {
		for i := range teamMembers {
			if teamMembers[i].UserId == teamMemberID {
				teamMemberSelection = teamMembers[i]
			}
		}
		if teamMemberSelection.UserId == "" {
			return nil, ErrTeamNotFound
		}
	}
	userID := teamMemberSelection.UserId

	resp, err := client.RemoveTeamMemberWithResponse(httpContext.Background(), ctx.Organization, teamID, userID)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return &Membership{TeamID: team.Id, TeamName: team.Name, UserID: userID, Email: teamMemberSelection.Username, Action: Removed}, nil
}

// AddUser adds the Organization user with userID, or the one picked when it
// is "", to the team with teamID, or the one picked when it is "", and
// returns which user it added to which team. It returns nil, and adds no one,
// when the team is IdP-managed and the person declines to go on.
func AddUser(teamID, userID string, force bool, client astrov1.APIClient) (*Membership, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return nil, err
		}
		if len(teams) == 0 {
			return nil, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil {
			return nil, err
		}
		// A picked team with no ID used to end the command here with exit
		// 0, having added no one.
		if team.Id == "" {
			return nil, ErrInvalidTeamKey
		}
	} else {
		team, err = GetTeam(client, teamID)
		if err != nil {
			return nil, err
		}
		if team.Id == "" {
			return nil, ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y, err := confirmOperation(force)
		if err != nil {
			return nil, err
		}
		if !y {
			return nil, nil
		}
	}
	teamID = team.Id

	var userSelection astrov1.User
	if userID == "" {
		users, err := user.GetOrgUsers(client)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			return nil, ErrNoUsersFoundInOrg
		}
		userSelection, err = user.SelectUser(users, "organization")
		if err != nil {
			return nil, err
		}
	} else {
		userSelection, err = user.GetUser(client, userID)
		if err != nil {
			return nil, err
		}
		if userSelection.Id == "" {
			return nil, user.ErrUserNotFound
		}
	}

	userID = userSelection.Id
	addTeamMembersRequest := astrov1.AddTeamMembersRequest{
		MemberIds: []string{userID},
	}

	resp, err := client.AddTeamMembersWithResponse(httpContext.Background(), ctx.Organization, teamID, addTeamMembersRequest)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return &Membership{TeamID: team.Id, TeamName: team.Name, UserID: userID, Email: userSelection.Username, Action: Added}, nil
}

func selectTeamMember(teamMembers []astrov1.TeamMember) (astrov1.TeamMember, error) {
	list := picker.List{
		Title:   "\nPlease select the teamMember who's membership you'd like to modify:",
		Header:  []string{"FULLNAME", "EMAIL", "ID"},
		Ask:     []input.Option{input.About("a team member")},
		Invalid: ErrInvalidTeamMemberKey,
	}
	for i := range teamMembers {
		var fullName string
		if teamMembers[i].FullName != nil {
			fullName = *teamMembers[i].FullName
		}
		list.AddRow(false, fullName, teamMembers[i].Username, teamMembers[i].UserId)
	}
	i, err := list.Pick(os.Stdout, os.Stdin)
	if err != nil {
		return astrov1.TeamMember{}, err
	}
	return teamMembers[i], nil
}

// ListTeamUsers returns the members of the team with teamID, or of the one
// picked when it is "".
func ListTeamUsers(teamID string, client astrov1.APIClient) (MemberList, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return MemberList{}, err
	}
	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return MemberList{}, err
		}
		if len(teams) == 0 {
			return MemberList{}, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return MemberList{}, pickFailed(err)
		}
	} else {
		team, err = GetTeam(client, teamID)
		if err != nil {
			return MemberList{}, err
		}
		if team.Id == "" {
			return MemberList{}, ErrTeamNotFound
		}
	}
	members, err := listTeamMembers(client, ctx.Organization, team.Id)
	if err != nil {
		return MemberList{}, err
	}
	list := MemberList{Members: make([]Member, 0, len(members))}
	for i := range members {
		var fullName string
		if members[i].FullName != nil {
			fullName = *members[i].FullName
		}
		list.Members = append(list.Members, Member{ID: members[i].UserId, FullName: fullName, Email: members[i].Username})
	}
	return list, nil
}

func GetDeploymentTeams(client astrov1.APIClient, deploymentID string, _ int) ([]astrov1.Team, error) {
	dID := deploymentID
	return listTeams(client, nil, &dID)
}

// AddDeploymentTeam gives the Organization's team with id, or the one picked
// when id is "", role on the Deployment with deploymentID, and returns the
// team with that role.
func AddDeploymentTeam(id, role, deploymentID string, client astrov1.APIClient) (TeamInfo, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return TeamInfo{}, err
		}
		if len(teams) == 0 {
			return TeamInfo{}, ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return TeamInfo{}, pickFailed(err)
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return TeamInfo{}, err
		}
		if team.Id == "" {
			return TeamInfo{}, ErrTeamNotFound
		}
	}
	return setDeploymentTeamRole(client, ctx.Organization, deploymentID, role, &team)
}

// UpdateDeploymentTeamRole sets the role on the Deployment of the team with
// id, or of the Deployment's team picked when id is "", and returns the team
// with that role.
func UpdateDeploymentTeamRole(id, role, deploymentID string, client astrov1.APIClient) (TeamInfo, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	team, err := deploymentTeam(id, deploymentID, client)
	if err != nil {
		return TeamInfo{}, err
	}
	return setDeploymentTeamRole(client, ctx.Organization, deploymentID, role, &team)
}

// RemoveDeploymentTeam removes the role on the Deployment of the team with
// id, or of the Deployment's team picked when id is "", and returns which
// team it removed from which Deployment.
func RemoveDeploymentTeam(id, deploymentID string, client astrov1.APIClient) (DeploymentRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return DeploymentRemoval{}, err
	}
	team, err := deploymentTeam(id, deploymentID, client)
	if err != nil {
		return DeploymentRemoval{}, err
	}
	if _, err := setDeploymentTeamRole(client, ctx.Organization, deploymentID, "", &team); err != nil {
		return DeploymentRemoval{}, err
	}
	return DeploymentRemoval{ID: team.Id, Name: team.Name, DeploymentID: deploymentID, Action: Removed}, nil
}

// deploymentTeam returns the team with id, or the Deployment's team picked
// when id is "".
func deploymentTeam(id, deploymentID string, client astrov1.APIClient) (astrov1.Team, error) {
	if id == "" {
		teams, err := GetDeploymentTeams(client, deploymentID, teamPaginationLimit)
		if err != nil {
			return astrov1.Team{}, err
		}
		if len(teams) == 0 {
			return astrov1.Team{}, ErrNoTeamsFoundInDeployment
		}
		return selectTeam(teams)
	}
	team, err := GetTeam(client, id)
	if err != nil {
		return astrov1.Team{}, err
	}
	if team.Id == "" {
		return astrov1.Team{}, ErrTeamNotFound
	}
	return team, nil
}

// setDeploymentTeamRole sets team's role on deploymentID to role, or removes
// it when role is "", keeping every other role it holds, and returns the team
// with that role.
func setDeploymentTeamRole(client astrov1.APIClient, orgID, deploymentID, role string, team *astrov1.Team) (TeamInfo, error) {
	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(*team),
		WorkspaceRoles:   team.WorkspaceRoles,
		DeploymentRoles:  upsertTeamDeploymentRole(team.DeploymentRoles, deploymentID, role),
	}
	if err := updateTeamRoles(client, orgID, team.Id, req); err != nil {
		return TeamInfo{}, err
	}
	info := Info(team)
	info.DeploymentRole = role
	return info, nil
}

// roleInDeployment returns the team's deployment role for the given deployment, or "" if absent.
func roleInDeployment(team astrov1.Team, deploymentID string) string { //nolint:gocritic // Team is large; helper returns a short string
	if team.DeploymentRoles == nil {
		return ""
	}
	for _, r := range *team.DeploymentRoles {
		if r.DeploymentId == deploymentID {
			return r.Role
		}
	}
	return ""
}

// roleInWorkspace returns the team's workspace role for the given workspace, or "" if absent.
func roleInWorkspace(team astrov1.Team, workspaceID string) string { //nolint:gocritic // Team is large; helper returns a short string
	if team.WorkspaceRoles == nil {
		return ""
	}
	for _, r := range *team.WorkspaceRoles {
		if r.WorkspaceId == workspaceID {
			return string(r.Role)
		}
	}
	return ""
}

// ListDeploymentTeamsData returns deployment team list data for structured output
//
//nolint:dupl // the duplication is acceptable here
func ListDeploymentTeamsData(client astrov1.APIClient, deploymentID string) (*TeamList, error) {
	teams, err := GetDeploymentTeams(client, deploymentID, teamPaginationLimit)
	if err != nil {
		return nil, err
	}

	teamInfos := make([]TeamInfo, 0, len(teams))
	for i := range teams {
		teamDescription := ""
		if teams[i].Description != nil {
			teamDescription = *teams[i].Description
		}
		teamInfos = append(teamInfos, TeamInfo{
			ID:             teams[i].Id,
			Name:           teams[i].Name,
			Description:    teamDescription,
			DeploymentRole: roleInDeployment(teams[i], deploymentID),
			CreatedAt:      teams[i].CreatedAt,
		})
	}

	return &TeamList{Teams: teamInfos}, nil
}

var deploymentTeamTableConfig = output.BuildTableConfig(
	[]output.Column[TeamInfo]{
		{Header: "ID", Value: func(t TeamInfo) string { return t.ID }},
		{Header: "ROLE", Value: func(t TeamInfo) string { return t.DeploymentRole }},
		{Header: "NAME", Value: func(t TeamInfo) string { return t.Name }},
		{Header: "DESCRIPTION", Value: func(t TeamInfo) string { return t.Description }},
		{Header: "CREATE DATE", Value: func(t TeamInfo) string { return t.CreatedAt.Format(time.RFC3339) }},
	},
	func(d any) []TeamInfo { return d.(*TeamList).Teams },
)

// ListDeploymentTeamsWithFormat lists deployment teams with the specified output format
func ListDeploymentTeamsWithFormat(client astrov1.APIClient, deploymentID string, r output.Emitter) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListDeploymentTeamsData(client, deploymentID) },
		deploymentTeamTableConfig, r,
	)
}

// ListWorkspaceTeamsData returns workspace team list data for structured output
//
//nolint:dupl // the duplication is acceptable here
func ListWorkspaceTeamsData(client astrov1.APIClient, workspaceID string) (*TeamList, error) {
	teams, err := GetWorkspaceTeams(client, workspaceID, teamPaginationLimit)
	if err != nil {
		return nil, err
	}

	teamInfos := make([]TeamInfo, 0, len(teams))
	for i := range teams {
		teamDescription := ""
		if teams[i].Description != nil {
			teamDescription = *teams[i].Description
		}
		teamInfos = append(teamInfos, TeamInfo{
			ID:            teams[i].Id,
			Name:          teams[i].Name,
			Description:   teamDescription,
			WorkspaceRole: roleInWorkspace(teams[i], workspaceID),
			CreatedAt:     teams[i].CreatedAt,
		})
	}

	return &TeamList{Teams: teamInfos}, nil
}

var workspaceTeamTableConfig = output.BuildTableConfig(
	[]output.Column[TeamInfo]{
		{Header: "ID", Value: func(t TeamInfo) string { return t.ID }},
		{Header: "ROLE", Value: func(t TeamInfo) string { return t.WorkspaceRole }},
		{Header: "NAME", Value: func(t TeamInfo) string { return t.Name }},
		{Header: "DESCRIPTION", Value: func(t TeamInfo) string { return t.Description }},
		{Header: "CREATE DATE", Value: func(t TeamInfo) string { return t.CreatedAt.Format(time.RFC3339) }},
	},
	func(d any) []TeamInfo { return d.(*TeamList).Teams },
)

// ListWorkspaceTeamsWithFormat lists workspace teams with the specified output format
func ListWorkspaceTeamsWithFormat(client astrov1.APIClient, workspaceID string, r output.Emitter) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListWorkspaceTeamsData(client, workspaceID) },
		workspaceTeamTableConfig, r,
	)
}

// ListOrgTeamsData returns organization team list data for structured output
func ListOrgTeamsData(client astrov1.APIClient) (*TeamList, error) {
	teams, err := GetOrgTeams(client)
	if err != nil {
		return nil, err
	}

	teamInfos := make([]TeamInfo, 0, len(teams))
	for i := range teams {
		teamDescription := ""
		if teams[i].Description != nil {
			teamDescription = *teams[i].Description
		}
		teamInfos = append(teamInfos, TeamInfo{
			ID:           teams[i].Id,
			Name:         teams[i].Name,
			Description:  teamDescription,
			OrgRole:      string(teams[i].OrganizationRole),
			IsIdpManaged: teams[i].IsIdpManaged,
			CreatedAt:    teams[i].CreatedAt,
		})
	}

	return &TeamList{Teams: teamInfos}, nil
}

var orgTeamTableConfig = output.BuildTableConfig(
	[]output.Column[TeamInfo]{
		{Header: "ID", Value: func(t TeamInfo) string { return t.ID }},
		{Header: "NAME", Value: func(t TeamInfo) string { return t.Name }},
		{Header: "DESCRIPTION", Value: func(t TeamInfo) string { return t.Description }},
		{Header: "ORG ROLE", Value: func(t TeamInfo) string { return t.OrgRole }},
		{Header: "IDP MANAGED", Value: func(t TeamInfo) string { return strconv.FormatBool(t.IsIdpManaged) }},
		{Header: "CREATE DATE", Value: func(t TeamInfo) string { return t.CreatedAt.Format(time.RFC3339) }},
	},
	func(d any) []TeamInfo { return d.(*TeamList).Teams },
)

// ListOrgTeamsWithFormat lists organization teams with the specified output format
func ListOrgTeamsWithFormat(client astrov1.APIClient, r output.Emitter) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListOrgTeamsData(client) },
		orgTeamTableConfig, r,
	)
}
