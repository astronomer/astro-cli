package team

import (
	httpContext "context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/picker"
)

var (
	ErrInvalidTeamID            = errors.New("no team found with that ID")
	ErrInvalidTeamKey           = errors.New("invalid team selection")
	ErrInvalidTeamMemberKey     = errors.New("invalid team member selection")
	ErrInvalidName              = errors.New("no name provided for the team")
	ErrTeamNotFound             = errors.New("no team found with that ID")
	ErrNoTeamsFoundInOrg        = errors.New("no teams found in your Organization")
	ErrNoTeamsFoundInWorkspace  = errors.New("no teams found in your Workspace")
	ErrNoTeamsFoundInDeployment = errors.New("no teams found in your Deployment")
	ErrNoTeamMembersFoundInTeam = errors.New("no team members found in team")
	ErrNoUsersFoundInOrg        = errors.New("no users found in your Organization")
	ErrNoTeamNameProvided       = errors.New("a team name is required")
	teamPaginationLimit         = 100
)

// confirmOperation asks before changing an IDP-managed team, unless force. A
// run that may not ask returns the refusal, and one answered no
// input.ErrAborted.
func confirmOperation(force bool) error {
	if force {
		return nil
	}
	return input.ConfirmOrAbort("This is an IDP-managed team. Change it anyway?", input.AnsweredBy("--yes"))
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
		prompt := ansi.ForStderr().Bold("\nTeam name: ")
		if err := input.MayAsk(prompt, input.About("a Team name"), input.AnsweredBy("--name")); err != nil {
			return TeamInfo{}, err
		}
		fmt.Fprintln(os.Stderr, "A team name is required")
		name, err = input.Text(prompt, input.About("a Team name"), input.AnsweredBy("--name"))
		if err != nil {
			return TeamInfo{}, err
		}
		if name == "" {
			return TeamInfo{}, ErrNoTeamNameProvided
		}
	}
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	typedRole := astrov1.CreateTeamRequestOrganizationRole(role)
	teamCreateRequest := astrov1.CreateTeamJSONRequestBody{
		Description:      &description,
		Name:             name,
		OrganizationRole: &typedRole,
	}
	resp, err := client.CreateTeamWithResponse(httpContext.Background(), cfg.Organization, teamCreateRequest)
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
	return getTeam(httpContext.Background(), client, teamID)
}

// getTeam is GetTeam that stops when ctx is canceled.
func getTeam(ctx httpContext.Context, client astrov1.APIClient, teamID string) (team astrov1.Team, err error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return team, err
	}
	resp, err := client.GetTeamWithResponse(pagination.Background(ctx), cfg.Organization, teamID)
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
func UpdateWorkspaceTeamRole(ctx httpContext.Context, id, role, workspaceID string, client astrov1.APIClient) (TeamInfo, error) {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return TeamInfo{}, err
	}
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	workspaceID, err = astrosession.RequireWorkspace(workspaceID)
	if err != nil {
		return TeamInfo{}, err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(ctx, client, workspaceID, teamPaginationLimit)
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
	if err := updateTeamRoles(client, cfg.Organization, team.Id, req); err != nil {
		return TeamInfo{}, err
	}
	info := Info(&team)
	info.WorkspaceRole = role
	return info, nil
}

// orgTeam returns the Organization's team with id, or the one picked when id
// is "".
func orgTeam(ctx httpContext.Context, id string, client astrov1.APIClient) (astrov1.Team, error) {
	if id == "" {
		teams, err := GetOrgTeams(ctx, client)
		if err != nil {
			return astrov1.Team{}, err
		}
		if len(teams) == 0 {
			return astrov1.Team{}, ErrNoTeamsFoundInOrg
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

// UpdateTeam changes the name, description and Organization role of the team
// with id, or of the one picked when id is "": only what is given, so "" keeps
// each as it is. A team that is IdP-managed, where the person declines to
// go on, is input.ErrAborted, and nothing is changed.
//
// A role that is not an Organization role is refused before anything is
// asked or sent. The role is changed before the name and description, so a
// role the API refuses leaves the team as it was. The name and description
// are sent only when one is given; if they fail after the role went through,
// the error says the role changed, and the Update returned with it is the
// team with that role.
func UpdateTeam(ctx httpContext.Context, id, name, description, role string, force bool, client astrov1.APIClient) (*Update, error) {
	if role != "" {
		if err := user.IsOrganizationRoleValid(role); err != nil {
			return nil, err
		}
	}
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	team, err := orgTeam(ctx, id, client)
	if err != nil {
		return nil, err
	}
	if team.IsIdpManaged {
		if err := confirmOperation(force); err != nil {
			return nil, err
		}
	}
	teamID := team.Id

	if role != "" {
		req := astrov1.UpdateTeamRolesRequest{
			OrganizationRole: role,
			WorkspaceRoles:   team.WorkspaceRoles,
			DeploymentRoles:  team.DeploymentRoles,
		}
		if err := updateTeamRoles(client, cfg.Organization, teamID, req); err != nil {
			return nil, err
		}
	}

	// The name and description are sent only when one was given, so a
	// role-only update is one call.
	after := team
	if name != "" || description != "" {
		after, err = renameTeam(&team, name, description, cfg.Organization, client)
		if err != nil {
			if role == "" {
				return nil, err
			}
			// The role went through: say so, with the team as it now is.
			upd := &Update{Team: Info(&team), RoleChanged: true}
			upd.Team.OrgRole = role
			return upd, fmt.Errorf("updated the team's role to %s, but updating its name and description failed: %w", role, err)
		}
	}
	upd := &Update{Team: Info(&after)}
	upd.Team.OrgRole = orDefault(string(after.OrganizationRole), string(team.OrganizationRole))
	if role != "" {
		upd.RoleChanged = true
		upd.Team.OrgRole = role
	}
	return upd, nil
}

// renameTeam sends team's new name and description, keeping its own for
// whichever is "", and returns the team as the update left it: the API's
// answer, or what was asked for when it gave none.
func renameTeam(team *astrov1.Team, name, description, orgID string, client astrov1.APIClient) (astrov1.Team, error) {
	req := astrov1.UpdateTeamJSONRequestBody{Name: team.Name, Description: team.Description}
	if name != "" {
		req.Name = name
	}
	if description != "" {
		req.Description = &description
	}
	resp, err := client.UpdateTeamWithResponse(httpContext.Background(), orgID, team.Id, req)
	if err != nil {
		return astrov1.Team{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return astrov1.Team{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	after := *team
	after.Name, after.Description = req.Name, req.Description
	return after, nil
}

// RemoveWorkspaceTeam removes the role on the Workspace of the team with id,
// or of the Workspace's team picked when id is "", and returns which team it
// removed from which Workspace.
func RemoveWorkspaceTeam(ctx httpContext.Context, id, workspaceID string, client astrov1.APIClient) (WorkspaceRemoval, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return WorkspaceRemoval{}, err
	}
	workspaceID, err = astrosession.RequireWorkspace(workspaceID)
	if err != nil {
		return WorkspaceRemoval{}, err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetWorkspaceTeams(ctx, client, workspaceID, teamPaginationLimit)
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
	if err := updateTeamRoles(client, cfg.Organization, team.Id, req); err != nil {
		return WorkspaceRemoval{}, err
	}
	return WorkspaceRemoval{ID: team.Id, Name: team.Name, WorkspaceID: workspaceID, Action: Removed}, nil
}

func selectTeam(teams []astrov1.Team) (astrov1.Team, error) {
	list := picker.List{
		Title:   "\nSelect a team:",
		Header:  []string{"TEAMNAME", "ID"},
		Ask:     []input.Option{input.About("a team")},
		Invalid: ErrInvalidTeamKey,
	}
	for i := range teams {
		list.AddRow(false, teams[i].Name, teams[i].Id)
	}
	i, err := list.Pick(os.Stderr, os.Stdin)
	if err != nil {
		return astrov1.Team{}, err
	}
	return teams[i], nil
}

// listTeams paginates through GET /teams with optional workspaceId/deploymentId
// filters. It keeps the first limit teams (all when limit is below 1) and stops
// when ctx is canceled. Pages are sorted by name, which is the
// stable order the API offers (its team sorts have no id tiebreaker), so a
// limited list is the first rows of the full one.
func listTeams(ctx httpContext.Context, client astrov1.APIClient, workspaceID, deploymentID *string, limit int) ([]astrov1.Team, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	sorts := []astrov1.ListTeamsParamsSorts{astrov1.ListTeamsParamsSortsNameAsc}
	return pagination.CollectUpTo(ctx, "teams", limit, func(offset int) ([]astrov1.Team, int, error) {
		pageSize := teamPaginationLimit
		if limit > 0 {
			pageSize = min(pageSize, limit-offset)
		}
		params := &astrov1.ListTeamsParams{
			Offset:       &offset,
			Limit:        &pageSize,
			Sorts:        &sorts,
			WorkspaceId:  workspaceID,
			DeploymentId: deploymentID,
		}
		resp, err := client.ListTeamsWithResponse(pagination.Background(ctx), cfg.Organization, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		if resp.JSON200 == nil {
			return nil, 0, errors.New("list teams returned an empty response body")
		}
		return resp.JSON200.Teams, resp.JSON200.TotalCount, nil
	})
}

func GetWorkspaceTeams(ctx httpContext.Context, client astrov1.APIClient, workspaceID string, _ int) ([]astrov1.Team, error) {
	workspaceID, err := astrosession.RequireWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	wsID := workspaceID
	return listTeams(ctx, client, &wsID, nil, 0)
}

// AddWorkspaceTeam gives the Organization's team with id, or the one picked
// when id is "", role on the Workspace (the current one when workspaceID is
// ""), and returns the team with that role.
func AddWorkspaceTeam(ctx httpContext.Context, id, role, workspaceID string, client astrov1.APIClient) (TeamInfo, error) {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return TeamInfo{}, err
	}
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	workspaceID, err = astrosession.RequireWorkspace(workspaceID)
	if err != nil {
		return TeamInfo{}, err
	}
	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(ctx, client)
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
	if err := updateTeamRoles(client, cfg.Organization, team.Id, req); err != nil {
		return TeamInfo{}, err
	}
	info := Info(&team)
	info.WorkspaceRole = role
	return info, nil
}

// pickFailed is the error of a team picker that returned err, or no team:
// ErrInvalidTeamKey, as it has always been, unless the picker refused to ask
// (under --output json), whose refusal names what answers it, or the run was
// interrupted at it, which is the interrupt, not an invalid team.
func pickFailed(err error) error {
	if input.Unanswered(err) {
		return err
	}
	return ErrInvalidTeamKey
}

// GetOrgTeams returns a list of all organization teams.
func GetOrgTeams(ctx httpContext.Context, client astrov1.APIClient) ([]astrov1.Team, error) {
	return listTeams(ctx, client, nil, nil, 0)
}

// Delete deletes the team with id, or the one picked when id is "", and
// returns which team it deleted, asking first unless force. Declined, it is
// input.ErrAborted, and nothing is deleted.
func Delete(ctx httpContext.Context, id string, force bool, client astrov1.APIClient) (*OrganizationRemoval, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	// A run that cannot ask is refused before anything is listed or read:
	// the question it would end on is one only --yes answers.
	if !force {
		if err := input.MayAsk("", input.About("confirmation to delete the team"), input.AnsweredBy("--yes")); err != nil {
			return nil, err
		}
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(ctx, client)
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
	// Every team is asked about, as a Workspace is: a deleted team cannot
	// be brought back, and an IdP-managed one says so too.
	if !force {
		question := fmt.Sprintf("\nDelete team %s? This cannot be undone.", ansi.Bold(team.Name))
		if team.IsIdpManaged {
			question = "\nThis is an IDP-managed team." + question
		}
		if err := input.ConfirmOrAbort(question, input.AnsweredBy("--yes")); err != nil {
			return nil, err
		}
	}
	teamID := team.Id
	resp, err := client.DeleteTeamWithResponse(httpContext.Background(), cfg.Organization, teamID)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return &OrganizationRemoval{ID: team.Id, Name: team.Name, OrganizationID: cfg.Organization, Action: Deleted}, nil
}

// listTeamMembers fetches the members of a team using the paginated v1
// endpoint: the first limit of them (all when limit is below 1), by user name
// then user id so an offset cannot skip or repeat a member where two pages
// meet, stopping when ctx is canceled.
func listTeamMembers(ctx httpContext.Context, client astrov1.APIClient, orgID, teamID string, limit int) ([]astrov1.TeamMember, error) {
	sorts := []astrov1.ListTeamMembersParamsSorts{astrov1.ListTeamMembersParamsSortsUsernameAsc, astrov1.ListTeamMembersParamsSortsUserIdAsc}
	return pagination.CollectUpTo(ctx, "team members", limit, func(offset int) ([]astrov1.TeamMember, int, error) {
		pageSize := teamPaginationLimit
		if limit > 0 {
			pageSize = min(pageSize, limit-offset)
		}
		params := &astrov1.ListTeamMembersParams{
			Offset: &offset,
			Limit:  &pageSize,
			Sorts:  &sorts,
		}
		resp, err := client.ListTeamMembersWithResponse(pagination.Background(ctx), orgID, teamID, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		if resp.JSON200 == nil {
			return nil, 0, errors.New("list team members returned an empty response body")
		}
		return resp.JSON200.TeamMembers, resp.JSON200.TotalCount, nil
	})
}

// RemoveUser removes the member with teamMemberID, or the one picked when it
// is "", from the team with teamID, or the one picked when it is "", and
// returns which user it removed from which team. A team that is IdP-managed,
// where the person declines to go on, is input.ErrAborted.
func RemoveUser(ctx httpContext.Context, teamID, teamMemberID string, force bool, client astrov1.APIClient) (*Membership, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(ctx, client)
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
		if err := confirmOperation(force); err != nil {
			return nil, err
		}
	}
	teamID = team.Id
	teamMembers, err := listTeamMembers(ctx, client, cfg.Organization, teamID, 0)
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
			return nil, fmt.Errorf("user %s is not a member of team %s", teamMemberID, team.Name)
		}
	}
	userID := teamMemberSelection.UserId

	resp, err := client.RemoveTeamMemberWithResponse(httpContext.Background(), cfg.Organization, teamID, userID)
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
// returns which user it added to which team. A team that is IdP-managed,
// where the person declines to go on, is input.ErrAborted.
func AddUser(ctx httpContext.Context, teamID, userID string, force bool, client astrov1.APIClient) (*Membership, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var team astrov1.Team
	if teamID == "" {
		teams, err := GetOrgTeams(ctx, client)
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
		if err := confirmOperation(force); err != nil {
			return nil, err
		}
	}
	teamID = team.Id

	var userSelection astrov1.User
	if userID == "" {
		users, err := user.GetOrgUsers(ctx, client)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			return nil, ErrNoUsersFoundInOrg
		}
		userSelection, err = user.SelectUser(users, "organization", "")
		if err != nil {
			return nil, err
		}
	} else {
		userSelection, err = user.GetUser(ctx, client, userID)
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

	resp, err := client.AddTeamMembersWithResponse(httpContext.Background(), cfg.Organization, teamID, addTeamMembersRequest)
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
		Title:   "\nSelect the team member to remove from the team:",
		Header:  []string{"FULLNAME", "EMAIL", "ID"},
		Ask:     []input.Option{input.About("a team member"), input.AnsweredBy("--user-id")},
		Invalid: ErrInvalidTeamMemberKey,
	}
	for i := range teamMembers {
		var fullName string
		if teamMembers[i].FullName != nil {
			fullName = *teamMembers[i].FullName
		}
		list.AddRow(false, fullName, teamMembers[i].Username, teamMembers[i].UserId)
	}
	i, err := list.Pick(os.Stderr, os.Stdin)
	if err != nil {
		return astrov1.TeamMember{}, err
	}
	return teamMembers[i], nil
}

// ListTeamUsers returns the members of the team with teamID, or of the one
// picked when it is "".
func ListTeamUsers(ctx httpContext.Context, teamID string, limit int, client astrov1.APIClient) (MemberList, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return MemberList{}, err
	}
	var team astrov1.Team
	if teamID == "" {
		teams, err := listTeams(ctx, client, nil, nil, 0)
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
		team, err = getTeam(ctx, client, teamID)
		if err != nil {
			return MemberList{}, err
		}
		if team.Id == "" {
			return MemberList{}, ErrTeamNotFound
		}
	}
	members, err := listTeamMembers(ctx, client, cfg.Organization, team.Id, limit)
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

func GetDeploymentTeams(ctx httpContext.Context, client astrov1.APIClient, deploymentID string, _ int) ([]astrov1.Team, error) {
	dID := deploymentID
	return listTeams(ctx, client, nil, &dID, 0)
}

// AddDeploymentTeam gives the Organization's team with id, or the one picked
// when id is "", role on the Deployment with deploymentID, and returns the
// team with that role.
func AddDeploymentTeam(ctx httpContext.Context, id, role, deploymentID string, client astrov1.APIClient) (TeamInfo, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}

	var team astrov1.Team
	if id == "" {
		teams, err := GetOrgTeams(ctx, client)
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
	return setDeploymentTeamRole(client, cfg.Organization, deploymentID, role, &team)
}

// UpdateDeploymentTeamRole sets the role on the Deployment of the team with
// id, or of the Deployment's team picked when id is "", and returns the team
// with that role.
func UpdateDeploymentTeamRole(ctx httpContext.Context, id, role, deploymentID string, client astrov1.APIClient) (TeamInfo, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return TeamInfo{}, err
	}
	team, err := deploymentTeam(ctx, id, deploymentID, client)
	if err != nil {
		return TeamInfo{}, err
	}
	return setDeploymentTeamRole(client, cfg.Organization, deploymentID, role, &team)
}

// RemoveDeploymentTeam removes the role on the Deployment of the team with
// id, or of the Deployment's team picked when id is "", and returns which
// team it removed from which Deployment.
func RemoveDeploymentTeam(ctx httpContext.Context, id, deploymentID string, client astrov1.APIClient) (DeploymentRemoval, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return DeploymentRemoval{}, err
	}
	team, err := deploymentTeam(ctx, id, deploymentID, client)
	if err != nil {
		return DeploymentRemoval{}, err
	}
	if _, err := setDeploymentTeamRole(client, cfg.Organization, deploymentID, "", &team); err != nil {
		return DeploymentRemoval{}, err
	}
	return DeploymentRemoval{ID: team.Id, Name: team.Name, DeploymentID: deploymentID, Action: Removed}, nil
}

// deploymentTeam returns the team with id, or the Deployment's team picked
// when id is "".
func deploymentTeam(ctx httpContext.Context, id, deploymentID string, client astrov1.APIClient) (astrov1.Team, error) {
	if id == "" {
		teams, err := GetDeploymentTeams(ctx, client, deploymentID, teamPaginationLimit)
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
func ListDeploymentTeamsData(ctx httpContext.Context, client astrov1.APIClient, deploymentID string, limit int) (*TeamList, error) {
	dID := deploymentID
	teams, err := listTeams(ctx, client, nil, &dID, limit)
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

// roleTeamTableConfig is the table of teams with a role on one Deployment or
// Workspace, which role reads.
func roleTeamTableConfig(role func(TeamInfo) string) *output.TableConfig {
	return output.BuildTableConfig(
		[]output.Column[TeamInfo]{
			{Header: "ID", Value: func(t TeamInfo) string { return t.ID }},
			{Header: "ROLE", Value: role},
			{Header: "NAME", Value: func(t TeamInfo) string { return t.Name }},
			{Header: "DESCRIPTION", Value: func(t TeamInfo) string { return t.Description }},
			{Header: "CREATE DATE", Value: func(t TeamInfo) string { return t.CreatedAt.Format(time.RFC3339) }},
		},
		func(d any) []TeamInfo { return d.(*TeamList).Teams },
	)
}

var deploymentTeamTableConfig = roleTeamTableConfig(func(t TeamInfo) string { return t.DeploymentRole })

// ListDeploymentTeamsWithFormat lists deployment teams with the specified output format
func ListDeploymentTeamsWithFormat(ctx httpContext.Context, client astrov1.APIClient, deploymentID string, limit int, r output.Emitter) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListDeploymentTeamsData(ctx, client, deploymentID, limit) },
		deploymentTeamTableConfig, r,
	)
}

// ListWorkspaceTeamsData returns workspace team list data for structured output
func ListWorkspaceTeamsData(ctx httpContext.Context, client astrov1.APIClient, workspaceID string, limit int) (*TeamList, error) {
	// The roles shown are the ones on the Workspace listed, which is the
	// current one when workspaceID is "".
	workspaceID, err := astrosession.RequireWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	wsID := workspaceID
	teams, err := listTeams(ctx, client, &wsID, nil, limit)
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

var workspaceTeamTableConfig = roleTeamTableConfig(func(t TeamInfo) string { return t.WorkspaceRole })

// ListWorkspaceTeamsWithFormat lists workspace teams with the specified output format
func ListWorkspaceTeamsWithFormat(ctx httpContext.Context, client astrov1.APIClient, workspaceID string, limit int, r output.Emitter) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListWorkspaceTeamsData(ctx, client, workspaceID, limit) },
		workspaceTeamTableConfig, r,
	)
}

// ListOrgTeamsData returns organization team list data for structured output
func ListOrgTeamsData(ctx httpContext.Context, client astrov1.APIClient, limit int) (*TeamList, error) {
	teams, err := listTeams(ctx, client, nil, nil, limit)
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
func ListOrgTeamsWithFormat(ctx httpContext.Context, client astrov1.APIClient, limit int, r output.Emitter) error {
	return output.PrintData(
		func() (*TeamList, error) { return ListOrgTeamsData(ctx, client, limit) },
		orgTeamTableConfig, r,
	)
}
