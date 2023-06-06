package team

import (
	httpContext "context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	ErrInvalidTeamID            = errors.New("team could not be found with selected or passed in id")
	ErrInvalidTeamKey           = errors.New("invalid team selection")
	ErrInvalidTeamMemberKey     = errors.New("invalid team member selection")
	ErrInvalidName              = errors.New("no name provided for the team. Retry with a valid name")
	ErrTeamNotFound             = errors.New("no team was found for the ID you provided")
	ErrWrongEnforceInput        = errors.New("the input to the `--enforce-cicd` flag")
	ErrNoShortName              = errors.New("cannot retrieve organization short name from context")
	ErrNoTeamsFoundInOrg        = errors.New("no teams found in your organization")
	ErrNoTeamsFoundInWorkspace  = errors.New("no teams found in your workspace")
	ErrNoTeamMembersFoundInTeam = errors.New("no team members found in team")
	ErrNoUsersFoundInOrg        = errors.New("no users found in your organization")
	teamPagnationLimit          = 100
)

func CreateTeam(name, description string, out io.Writer, client astrocore.CoreClient) error {
	if name == "" {
		return ErrInvalidName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	teamCreateRequest := astrocore.CreateTeamJSONRequestBody{
		Description: &description,
		Name:        name,
	}
	resp, err := client.CreateTeamWithResponse(httpContext.Background(), ctx.OrganizationShortName, teamCreateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully created\n", name)
	return nil
}

func GetTeam(client astrocore.CoreClient, teamID string) (team astrocore.Team, err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return team, err
	}
	if ctx.OrganizationShortName == "" {
		return team, ErrNoShortName
	}

	resp, err := client.GetTeamWithResponse(httpContext.Background(), ctx.OrganizationShortName, teamID)
	if err != nil {
		return team, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return team, err
	}

	team = *resp.JSON200

	return team, nil
}

func UpdateWorkspaceTeamRole(id, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}

	var team astrocore.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspace, teamPagnationLimit)
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	teamID := team.Id

	teamMutateRequest := astrocore.MutateWorkspaceTeamRoleRequest{Role: role}
	resp, err := client.MutateWorkspaceTeamRoleWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, teamID, teamMutateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The workspace team %s role was successfully updated to %s\n", teamID, role)
	return nil
}

func UpdateTeam(id, name, description string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}

	var team astrocore.Team
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	teamID := team.Id

	teamUpdateRequest := astrocore.UpdateTeamJSONRequestBody{}

	if name == "" {
		teamUpdateRequest.Name = team.Name
	} else {
		teamUpdateRequest.Name = name
	}

	if description == "" {
		teamUpdateRequest.Description = *team.Description
	} else {
		teamUpdateRequest.Description = description
	}

	resp, err := client.UpdateTeamWithResponse(httpContext.Background(), ctx.OrganizationShortName, teamID, teamUpdateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully updated\n", team.Name)
	return nil
}

func RemoveWorkspaceTeam(id, workspace string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}

	var team astrocore.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspace, teamPagnationLimit)
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	teamID := team.Id
	resp, err := client.DeleteWorkspaceTeamWithResponse(httpContext.Background(), ctx.OrganizationShortName, ctx.Workspace, teamID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully removed from workspace %s\n", team.Name, ctx.Workspace)
	return nil
}

func selectTeam(teams []astrocore.Team) (astrocore.Team, error) {
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "TEAMNAME", "ID"},
	}

	fmt.Println("\nPlease select a team:")

	teamMap := map[string]astrocore.Team{}
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
		return astrocore.Team{}, ErrInvalidTeamKey
	}
	return selected, nil
}

func GetWorkspaceTeams(client astrocore.CoreClient, workspace string, limit int) ([]astrocore.Team, error) {
	offset := 0
	var teams []astrocore.Team

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if ctx.OrganizationShortName == "" {
		return nil, ErrNoShortName
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	for {
		resp, err := client.ListWorkspaceTeamsWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, &astrocore.ListWorkspaceTeamsParams{
			Offset: &offset,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		teams = append(teams, resp.JSON200.Teams...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += limit
	}

	return teams, nil
}

// Prints a list of all of an organizations teams
func ListWorkspaceTeams(out io.Writer, client astrocore.CoreClient, workspace string) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "Role", "Name", "Description", "Create Date"},
	}
	teams, err := GetWorkspaceTeams(client, workspace, teamPagnationLimit)
	if err != nil {
		return err
	}

	for i := range teams {
		var teamRole string
		for _, role := range *teams[i].Roles {
			if role.EntityType == "WORKSPACE" && role.EntityId == workspace {
				teamRole = role.Role
			}
		}
		table.AddRow([]string{
			teams[i].Id,
			teamRole,
			teams[i].Name,
			*teams[i].Description,
			teams[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}

func AddWorkspaceTeam(id, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsWorkspaceRoleValid(role)
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
	if workspace == "" {
		workspace = ctx.Workspace
	}
	var team astrocore.Team
	if id == "" {
		// Get all org teams. Setting limit to 1000 for now
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}

	teamID := team.Id

	mutateUserInput := astrocore.MutateWorkspaceTeamRoleRequest{
		Role: role,
	}
	resp, err := client.MutateWorkspaceTeamRoleWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, teamID, mutateUserInput)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The team %s was successfully added to the workspace with the role %s\n", teamID, role)
	return nil
}

// Returns a list of all of an organizations teams
func GetOrgTeams(client astrocore.CoreClient) ([]astrocore.Team, error) {
	offset := 0
	var teams []astrocore.Team
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if ctx.OrganizationShortName == "" {
		return nil, ErrNoShortName
	}

	for {
		resp, err := client.ListOrganizationTeamsWithResponse(httpContext.Background(), ctx.OrganizationShortName, &astrocore.ListOrganizationTeamsParams{
			Offset: &offset,
			Limit:  &teamPagnationLimit,
		})
		if err != nil {
			return nil, err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		teams = append(teams, resp.JSON200.Teams...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += teamPagnationLimit
	}

	return teams, nil
}

// Prints a list of all of an organizations users
func ListOrgTeams(out io.Writer, client astrocore.CoreClient) error {
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "Name", "Description", "CREATE DATE"},
	}
	teams, err := GetOrgTeams(client)
	if err != nil {
		return err
	}

	for i := range teams {
		table.AddRow([]string{
			teams[i].Id,
			teams[i].Name,
			*teams[i].Description,
			teams[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}

func Delete(id string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}

	var team astrocore.Team
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	teamID := team.Id
	resp, err := client.DeleteTeamWithResponse(httpContext.Background(), ctx.OrganizationShortName, teamID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully deleted\n", team.Name)
	return nil
}

func RemoveUser(teamID, teamMemberID string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}

	var team astrocore.Team
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	teamID = team.Id
	if team.Members == nil {
		return ErrNoTeamMembersFoundInTeam
	}

	teamMembers := *team.Members

	var teamMemberSelection astrocore.TeamMember
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

	resp, err := client.RemoveTeamMemberWithResponse(httpContext.Background(), ctx.OrganizationShortName, teamID, userID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro User %s was successfully removed from team %s \n", teamMemberSelection.UserId, team.Name)
	return nil
}

func AddUser(teamID, userID string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}

	var team astrocore.Team
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	teamID = team.Id

	users, err := user.GetOrgUsers(client)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return ErrNoUsersFoundInOrg
	}
	var userSelection astrocore.User
	if userID == "" {
		userSelection, err = user.SelectUser(users, false)
		if err != nil {
			return err
		}
	} else {
		for i := range users {
			if users[i].Id == userID {
				userSelection = users[i]
			}
		}
		if userSelection.Id == "" {
			return ErrTeamNotFound
		}
	}

	userID = userSelection.Id
	addTeamMembersRequest := astrocore.AddTeamMembersRequest{
		MemberIds: []string{userID},
	}

	resp, err := client.AddTeamMembersWithResponse(httpContext.Background(), ctx.OrganizationShortName, teamID, addTeamMembersRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro User %s was successfully added to team %s \n", userSelection.Id, team.Name)
	return nil
}

func selectTeamMember(teamMembers []astrocore.TeamMember) (astrocore.TeamMember, error) {
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "FULLNAME", "EMAIL", "ID"},
	}

	fmt.Println("\nPlease select the teamMember who's membership you'd like to modify:")

	teamMemberMap := map[string]astrocore.TeamMember{}
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
		return astrocore.TeamMember{}, ErrInvalidTeamMemberKey
	}
	return selected, nil
}

func ListTeamUsers(teamID string, out io.Writer, client astrocore.CoreClient) (err error) {

	var team astrocore.Team
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

		if err != nil || team.Id == "" {
			return ErrTeamNotFound
		}
	}
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "FullName", "Email"},
	}
	members := *team.Members
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
