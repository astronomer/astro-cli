package team

import (
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
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
	ErrNoTeamsFoundInOrg        = errors.New("no teams found in your organization")
	ErrNoTeamsFoundInWorkspace  = errors.New("no teams found in your workspace")
	ErrNoTeamsFoundInDeployment = errors.New("no teams found in your deployment")
	ErrNoTeamMembersFoundInTeam = errors.New("no team members found in team")
	ErrNoUsersFoundInOrg        = errors.New("no users found in your organization")
	ErrNoTeamNameProvided       = errors.New("you must give your Team a name")
	teamPagnationLimit          = 100
)

func confirmOperation() bool {
	y, _ := input.Confirm("This is an IDP-managed team. Are you sure you want to continue the operation?")
	return y
}

func CreateTeam(name, description, role string, out io.Writer, client astrocore.CoreClient) error {
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
	teamCreateRequest := astrocore.CreateTeamJSONRequestBody{
		Description:      &description,
		Name:             name,
		OrganizationRole: &role,
	}
	resp, err := client.CreateTeamWithResponse(httpContext.Background(), ctx.Organization, teamCreateRequest)
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
	resp, err := client.GetTeamWithResponse(httpContext.Background(), ctx.Organization, teamID)
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

func UpdateWorkspaceTeamRole(id, role, workspaceID string, out io.Writer, client astrocore.CoreClient) error {
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

	var team astrocore.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspaceID, teamPagnationLimit)
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
	teamID := team.Id

	teamMutateRequest := astrocore.MutateWorkspaceTeamRoleRequest{Role: role}
	resp, err := client.MutateWorkspaceTeamRoleWithResponse(httpContext.Background(), ctx.Organization, workspaceID, teamID, teamMutateRequest)
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

func UpdateTeam(id, name, description, role string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation()
		if !y {
			return nil
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

	resp, err := client.UpdateTeamWithResponse(httpContext.Background(), ctx.Organization, teamID, teamUpdateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully updated\n", team.Name)

	if role != "" {
		err := user.IsOrganizationRoleValid(role)
		if err != nil {
			return err
		}
		teamMutateRoleRequest := astrocore.MutateOrgTeamRoleRequest{Role: role}
		resp, err := client.MutateOrgTeamRoleWithResponse(httpContext.Background(), ctx.Organization, teamID, teamMutateRoleRequest)
		if err != nil {
			return err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Astro Team role %s was successfully updated to %s\n", team.Name, role)
	}
	return nil
}

func RemoveWorkspaceTeam(id, workspaceID string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	var team astrocore.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(client, workspaceID, teamPagnationLimit)
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
	teamID := team.Id
	resp, err := client.DeleteWorkspaceTeamWithResponse(httpContext.Background(), ctx.Organization, ctx.Workspace, teamID)
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

func GetWorkspaceTeams(client astrocore.CoreClient, workspaceID string, limit int) ([]astrocore.Team, error) {
	offset := 0
	var teams []astrocore.Team

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	for {
		resp, err := client.ListWorkspaceTeamsWithResponse(httpContext.Background(), ctx.Organization, workspaceID, &astrocore.ListWorkspaceTeamsParams{
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
func ListWorkspaceTeams(out io.Writer, client astrocore.CoreClient, workspaceID string) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "Role", "Name", "Description", "Create Date"},
	}
	teams, err := GetWorkspaceTeams(client, workspaceID, teamPagnationLimit)
	if err != nil {
		return err
	}

	for i := range teams {
		var teamRole string
		var roles []astrocore.TeamRole
		if teams[i].Roles != nil {
			roles = *teams[i].Roles
		}
		for _, role := range roles {
			if role.EntityType == "WORKSPACE" && role.EntityId == workspaceID {
				teamRole = role.Role
			}
		}
		var teamDescription string
		if teams[i].Description != nil {
			teamDescription = *teams[i].Description
		}
		table.AddRow([]string{
			teams[i].Id,
			teamRole,
			teams[i].Name,
			teamDescription,
			teams[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}

func AddWorkspaceTeam(id, role, workspaceID string, out io.Writer, client astrocore.CoreClient) error {
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}

	teamID := team.Id

	mutateUserInput := astrocore.MutateWorkspaceTeamRoleRequest{
		Role: role,
	}
	resp, err := client.MutateWorkspaceTeamRoleWithResponse(httpContext.Background(), ctx.Organization, workspaceID, teamID, mutateUserInput)
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

	for {
		resp, err := client.ListOrganizationTeamsWithResponse(httpContext.Background(), ctx.Organization, &astrocore.ListOrganizationTeamsParams{
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
		Header:         []string{"ID", "NAME", "DESCRIPTION", "ORG ROLE", "IDP MANAGED", "CREATE DATE"},
	}
	teams, err := GetOrgTeams(client)
	if err != nil {
		return err
	}

	for i := range teams {
		var teamDescription string
		if teams[i].Description != nil {
			teamDescription = *teams[i].Description
		}
		table.AddRow([]string{
			teams[i].Id,
			teams[i].Name,
			teamDescription,
			teams[i].OrganizationRole,
			strconv.FormatBool(teams[i].IsIdpManaged),
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation()
		if !y {
			return nil
		}
	}
	teamID := team.Id
	resp, err := client.DeleteTeamWithResponse(httpContext.Background(), ctx.Organization, teamID)
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation()
		if !y {
			return nil
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

	resp, err := client.RemoveTeamMemberWithResponse(httpContext.Background(), ctx.Organization, teamID, userID)
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	if team.IsIdpManaged {
		y := confirmOperation()
		if !y {
			return nil
		}
	}
	teamID = team.Id

	var userSelection astrocore.User
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
	addTeamMembersRequest := astrocore.AddTeamMembersRequest{
		MemberIds: []string{userID},
	}

	resp, err := client.AddTeamMembersWithResponse(httpContext.Background(), ctx.Organization, teamID, addTeamMembersRequest)
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "FullName", "Email"},
	}
	if team.Members != nil {
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
	fmt.Println("The selected team has no members")
	return nil
}

func GetDeploymentTeams(client astrocore.CoreClient, deploymentID string, limit int) ([]astrocore.Team, error) {
	offset := 0
	var teams []astrocore.Team
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	includeDeploymentRoles := true
	for {
		resp, err := client.ListDeploymentTeamsWithResponse(httpContext.Background(), ctx.Organization, deploymentID, &astrocore.ListDeploymentTeamsParams{
			IncludeDeploymentRoles: &includeDeploymentRoles,
			Offset:                 &offset,
			Limit:                  &limit,
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
func ListDeploymentTeams(out io.Writer, client astrocore.CoreClient, deploymentID string) error {
	table := printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "Role", "Name", "Description", "Create Date"},
	}
	teams, err := GetDeploymentTeams(client, deploymentID, teamPagnationLimit)
	if err != nil {
		return err
	}

	for i := range teams {
		var teamRole string
		var teamDescription string
		if teams[i].Description != nil {
			teamDescription = *teams[i].Description
		}
		var roles []astrocore.TeamRole
		if teams[i].Roles != nil {
			roles = *teams[i].Roles
		}
		for _, role := range roles {
			if role.EntityType == "DEPLOYMENT" && role.EntityId == deploymentID {
				teamRole = role.Role
			}
		}
		table.AddRow([]string{
			teams[i].Id,
			teamRole,
			teams[i].Name,
			teamDescription,
			teams[i].CreatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}

func AddDeploymentTeam(id, role, deploymentID string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
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
		if err != nil {
			return err
		}
		if team.Id == "" {
			return ErrTeamNotFound
		}
	}

	teamID := team.Id

	mutateDeploymentTeamInput := astrocore.MutateDeploymentTeamRoleRequest{
		Role: role,
	}
	resp, err := client.MutateDeploymentTeamRoleWithResponse(httpContext.Background(), ctx.Organization, deploymentID, teamID, mutateDeploymentTeamInput)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The team %s was successfully added to the deployment with the role %s\n", teamID, role)
	return nil
}

func UpdateDeploymentTeamRole(id, role, deploymentID string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrocore.Team
	if id == "" {
		teams, err := GetDeploymentTeams(client, deploymentID, teamPagnationLimit)
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
	teamID := team.Id

	teamMutateRequest := astrocore.MutateDeploymentTeamRoleRequest{Role: role}
	resp, err := client.MutateDeploymentTeamRoleWithResponse(httpContext.Background(), ctx.Organization, deploymentID, teamID, teamMutateRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The deployment team %s role was successfully updated to %s\n", teamID, role)
	return nil
}

func RemoveDeploymentTeam(id, deploymentID string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var team astrocore.Team
	if id == "" {
		teams, err := GetDeploymentTeams(client, deploymentID, teamPagnationLimit)
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
	teamID := team.Id
	resp, err := client.DeleteDeploymentTeamWithResponse(httpContext.Background(), ctx.Organization, deploymentID, teamID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Team %s was successfully removed from deployment %s\n", team.Name, deploymentID)
	return nil
}
