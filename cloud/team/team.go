package team

import (
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	ErrInvalidTeamID            = errors.New("team could not be found with selected or passed in id")
	ErrInvalidTeamKey           = errors.New("invalid team selection")
	ErrInvalidTeamMemberKey     = errors.New("invalid team member selection")
	ErrInvalidName              = errors.New("no name provided for the team. Retry with a valid name")
	ErrTeamNotFound             = errors.New("no team was found for the ID you provided")
	ErrWrongEnforceInput        = errors.New("the input to the `--enforce-cicd` flag")
	ErrNoTeamsFoundInOrg        = errors.New("no teams found in your organization")
	ErrNoTeamsFoundInWorkspace  = errors.New("no teams found in your workspace")
	ErrNoTeamsFoundInDeployment = errors.New("no teams found in your deployment")
	ErrNoTeamMembersFoundInTeam = errors.New("no team members found in team")
	ErrNoUsersFoundInOrg        = errors.New("no users found in your organization")
	ErrNoTeamNameProvided       = errors.New("you must give your Team a name")
	teamPaginationLimit         = 100
)

func confirmOperation(force bool) bool {
	if force {
		return true
	}
	y, _ := input.Confirm("This is an IDP-managed team. Are you sure you want to continue the operation?")
	return y
}

func CreateTeam(name, description, role string, out io.Writer, client astrov1.APIClient) error {
	err := user.IsOrganizationRoleValid(role)
	if err != nil {
		return err
	}
	if name == "" {
		fmt.Println("Please specify a name for your Team")
		name = input.Text(ansi.Bold("\nTeam name: "))
		if name == "" {
			return ErrNoTeamNameProvided
		}
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	typedRole := astrov1.CreateTeamRequestOrganizationRole(role)
	teamCreateRequest := astrov1.CreateTeamJSONRequestBody{
		Description:      &description,
		Name:             name,
		OrganizationRole: &typedRole,
	}
	resp, err := client.CreateTeamWithResponse(httpContext.Background(), ctx.Organization, teamCreateRequest)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully created\n", name)
	return nil
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

func UpdateWorkspaceTeamRole(id, role, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	err := user.IsWorkspaceRoleValid(role)
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

	var team astrov1.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspaceID, teamPaginationLimit)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInWorkspace
		}
		team, err = selectTeam(teams)
		if err != nil {
			return err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}

	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   upsertTeamWorkspaceRole(team.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  team.DeploymentRoles,
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return err
	}
	fmt.Fprintf(out, "The workspace team %s role was successfully updated to %s\n", team.Id, role)
	return nil
}

func UpdateTeam(id, name, description, role string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil {
			return err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation(force)
		if !y {
			return nil
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
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully updated\n", team.Name)

	if role != "" {
		if err := user.IsOrganizationRoleValid(role); err != nil {
			return err
		}
		req := astrov1.UpdateTeamRolesRequest{
			OrganizationRole: role,
			WorkspaceRoles:   team.WorkspaceRoles,
			DeploymentRoles:  team.DeploymentRoles,
		}
		if err := updateTeamRoles(client, ctx.Organization, teamID, req); err != nil {
			return err
		}
		fmt.Fprintf(out, "Astro Team role %s was successfully updated to %s\n", team.Name, role)
	}
	return nil
}

func RemoveWorkspaceTeam(id, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspaceID, teamPaginationLimit)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInWorkspace
		}
		team, err = selectTeam(teams)
		if err != nil {
			return err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   upsertTeamWorkspaceRole(team.WorkspaceRoles, workspaceID, ""),
		DeploymentRoles:  team.DeploymentRoles,
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully removed from workspace %s\n", team.Name, workspaceID)
	return nil
}

func selectTeam(teams []astrov1.Team) (astrov1.Team, error) {
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "TEAMNAME", "ID"},
	}

	fmt.Println("\nPlease select a team:")

	teamMap := map[string]astrov1.Team{}
	for i := range teams {
		index := i + 1
		table.AddRow([]string{
			strconv.Itoa(index),
			teams[i].Name,
			teams[i].Id,
		}, false)
		teamMap[strconv.Itoa(index)] = teams[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := teamMap[choice]
	if !ok {
		return astrov1.Team{}, ErrInvalidTeamKey
	}
	return selected, nil
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

func AddWorkspaceTeam(id, role, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	err := user.IsWorkspaceRoleValid(role)
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
	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return ErrInvalidTeamKey
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}

	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   upsertTeamWorkspaceRole(team.WorkspaceRoles, workspaceID, role),
		DeploymentRoles:  team.DeploymentRoles,
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return err
	}
	fmt.Fprintf(out, "The team %s was successfully added to the workspace with the role %s\n", team.Id, role)
	return nil
}

// GetOrgTeams returns a list of all organization teams.
func GetOrgTeams(client astrov1.APIClient) ([]astrov1.Team, error) {
	return listTeams(client, nil, nil)
}

func Delete(id string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return ErrInvalidTeamKey
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation(force)
		if !y {
			return nil
		}
	}
	teamID := team.Id
	resp, err := client.DeleteTeamWithResponse(httpContext.Background(), ctx.Organization, teamID)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully deleted\n", team.Name)
	return nil
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

func RemoveUser(teamID, teamMemberID string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return ErrInvalidTeamKey
		}
	} else {
		team, err = GetTeam(client, teamID)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation(force)
		if !y {
			return nil
		}
	}
	teamID = team.Id
	teamMembers, err := listTeamMembers(client, ctx.Organization, teamID)
	if err != nil {
		return err
	}
	if len(teamMembers) == 0 {
		return ErrNoTeamMembersFoundInTeam
	}

	var teamMemberSelection astrov1.TeamMember
	if teamMemberID == "" {
		teamMemberSelection, err = selectTeamMember(teamMembers)
		if err != nil {
			return err
		}
	} else {
		for i := range teamMembers {
			if teamMembers[i].UserId == teamMemberID {
				teamMemberSelection = teamMembers[i]
			}
		}
		if teamMemberSelection.UserId == "" {
			return ErrTeamNotFound
		}
	}
	userID := teamMemberSelection.UserId

	resp, err := client.RemoveTeamMemberWithResponse(httpContext.Background(), ctx.Organization, teamID, userID)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro User %s was successfully removed from team %s \n", teamMemberSelection.UserId, team.Name)
	return nil
}

func AddUser(teamID, userID string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return err
		}
	} else {
		team, err = GetTeam(client, teamID)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation(force)
		if !y {
			return nil
		}
	}
	teamID = team.Id

	var userSelection astrov1.User
	if userID == "" {
		users, err := user.GetOrgUsers(client)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			return ErrNoUsersFoundInOrg
		}
		userSelection, err = user.SelectUser(users, "organization")
		if err != nil {
			return err
		}
	} else {
		userSelection, err = user.GetUser(client, userID)
		if err != nil {
			return err
		}
		if userSelection.Id == "" {
			return user.ErrUserNotFound
		}
	}

	userID = userSelection.Id
	addTeamMembersRequest := astrov1.AddTeamMembersRequest{
		MemberIds: []string{userID},
	}

	resp, err := client.AddTeamMembersWithResponse(httpContext.Background(), ctx.Organization, teamID, addTeamMembersRequest)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro User %s was successfully added to team %s \n", userSelection.Id, team.Name)
	return nil
}

func selectTeamMember(teamMembers []astrov1.TeamMember) (astrov1.TeamMember, error) {
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "FULLNAME", "EMAIL", "ID"},
	}

	fmt.Println("\nPlease select the teamMember who's membership you'd like to modify:")

	teamMemberMap := map[string]astrov1.TeamMember{}
	for i := range teamMembers {
		index := i + 1
		var fullName string
		if teamMembers[i].FullName != nil {
			fullName = *teamMembers[i].FullName
		}

		table.AddRow([]string{
			strconv.Itoa(index),
			fullName,
			teamMembers[i].Username,
			teamMembers[i].UserId,
		}, false)
		teamMemberMap[strconv.Itoa(index)] = teamMembers[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := teamMemberMap[choice]
	if !ok {
		return astrov1.TeamMember{}, ErrInvalidTeamMemberKey
	}
	return selected, nil
}

func ListTeamUsers(teamID string, out io.Writer, client astrov1.APIClient) (err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return ErrInvalidTeamKey
		}
	} else {
		team, err = GetTeam(client, teamID)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	members, err := listTeamMembers(client, ctx.Organization, team.Id)
	if err != nil {
		return err
	}
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "FullName", "Email"},
	}
	if len(members) == 0 {
		fmt.Println("The selected team has no members")
		return nil
	}
	for i := range members {
		var fullName string
		if members[i].FullName != nil {
			fullName = *members[i].FullName
		}
		table.AddRow([]string{
			members[i].UserId,
			fullName,
			members[i].Username,
		}, false)
	}
	table.Print(out)
	return nil
}

func GetDeploymentTeams(client astrov1.APIClient, deploymentID string, _ int) ([]astrov1.Team, error) {
	dID := deploymentID
	return listTeams(client, nil, &dID)
}

func AddDeploymentTeam(id, role, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(client)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInOrg
		}
		team, err = selectTeam(teams)
		if err != nil || team.Id == "" {
			return ErrInvalidTeamKey
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}

	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   team.WorkspaceRoles,
		DeploymentRoles:  upsertTeamDeploymentRole(team.DeploymentRoles, deploymentID, role),
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return err
	}
	fmt.Fprintf(out, "The team %s was successfully added to the deployment with the role %s\n", team.Id, role)
	return nil
}

func UpdateDeploymentTeamRole(id, role, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetDeploymentTeams(client, deploymentID, teamPaginationLimit)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInDeployment
		}
		team, err = selectTeam(teams)
		if err != nil {
			return err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}

	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   team.WorkspaceRoles,
		DeploymentRoles:  upsertTeamDeploymentRole(team.DeploymentRoles, deploymentID, role),
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return err
	}
	fmt.Fprintf(out, "The deployment team %s role was successfully updated to %s\n", team.Id, role)
	return nil
}

func RemoveDeploymentTeam(id, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetDeploymentTeams(client, deploymentID, teamPaginationLimit)
		if err != nil {
			return err
		}
		if len(teams) == 0 {
			return ErrNoTeamsFoundInDeployment
		}
		team, err = selectTeam(teams)
		if err != nil {
			return err
		}
	} else {
		team, err = GetTeam(client, id)
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	req := astrov1.UpdateTeamRolesRequest{
		OrganizationRole: teamOrgRole(team),
		WorkspaceRoles:   team.WorkspaceRoles,
		DeploymentRoles:  upsertTeamDeploymentRole(team.DeploymentRoles, deploymentID, ""),
	}
	if err := updateTeamRoles(client, ctx.Organization, team.Id, req); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully removed from deployment %s\n", team.Name, deploymentID)
	return nil
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
//nolint:dupl
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
func ListDeploymentTeamsWithFormat(client astrov1.APIClient, deploymentID string, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListDeploymentTeamsData(client, deploymentID) },
		deploymentTeamTableConfig, format, tmpl, out,
	)
}

// ListWorkspaceTeamsData returns workspace team list data for structured output
//
//nolint:dupl
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
func ListWorkspaceTeamsWithFormat(client astrov1.APIClient, workspaceID string, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListWorkspaceTeamsData(client, workspaceID) },
		workspaceTeamTableConfig, format, tmpl, out,
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
func ListOrgTeamsWithFormat(client astrov1.APIClient, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListOrgTeamsData(client) },
		orgTeamTableConfig, format, tmpl, out,
	)
}
