package organization

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
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	ErrInvalidName                 = errors.New("no name provided for the organization token. Retry with a valid name")
	errInvalidOrganizationTokenKey = errors.New("invalid Organization API token selection")
	errOrganizationTokenNotFound   = errors.New("organization token specified was not found")
	errOrgTokenInWorkspace         = errors.New("this Organization API token has already been added to the Workspace with that role")
	errWrongTokenTypeSelected      = errors.New("the token selected is not of the type you are trying to modify")
)

const (
	deploymentEntity   = "DEPLOYMENT"
	workspaceEntity    = "WORKSPACE"
	organizationEntity = "ORGANIZATION"
	tokenPaginationLim = 100
)

func newTokenTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "NAME", "DESCRIPTION", "SCOPE", "ORGANIZATION ROLE", "CREATED", "CREATED BY"},
	}
}

func newTokenRolesTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ENTITY_TYPE", "ENTITY_ID", "ROLE"},
	}
}

func newTokenSelectionTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "DESCRIPTION", "ROLE", "EXPIRES"},
	}
}

// tokenRoles returns the token's roles as a flat slice, handling the nil pointer case.
func tokenRoles(token astrov1.ApiToken) []astrov1.ApiTokenRole { //nolint:gocritic // ApiToken is large; helper returns a slice
	if token.Roles == nil {
		return nil
	}
	return *token.Roles
}

// orgRoleOf returns the token's Organization role, if present.
func orgRoleOf(token astrov1.ApiToken) string { //nolint:gocritic // ApiToken is large; helper returns a short string
	for _, r := range tokenRoles(token) {
		if r.EntityType == astrov1.ApiTokenRoleEntityTypeORGANIZATION {
			return r.Role
		}
	}
	return ""
}

// upsertRole returns a new roles slice with the (entityType, entityId) entry set to role
// (added if missing). If role == "", the matching entry is removed.
func upsertRole(existing []astrov1.ApiTokenRole, entityType astrov1.ApiTokenRoleEntityType, entityID, role string) []astrov1.ApiTokenRole {
	out := []astrov1.ApiTokenRole{}
	for _, r := range existing {
		if r.EntityType == entityType && r.EntityId == entityID {
			continue
		}
		out = append(out, r)
	}
	if role != "" {
		out = append(out, astrov1.ApiTokenRole{
			EntityType: entityType,
			EntityId:   entityID,
			Role:       role,
		})
	}
	return out
}

// replaceOrgRole returns roles with the ORGANIZATION entry replaced by role.
// Non-org roles are preserved.
func replaceOrgRole(existing []astrov1.ApiTokenRole, orgID, role string) []astrov1.ApiTokenRole {
	return upsertRole(existing, astrov1.ApiTokenRoleEntityTypeORGANIZATION, orgID, role)
}

func AddOrgTokenToWorkspace(id, name, role, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	if err := user.IsWorkspaceRoleValid(role); err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	token, err := GetTokenFromInputOrUser(id, name, ctx.Organization, client)
	if err != nil {
		return err
	}

	// Short-circuit: already has this role on the workspace.
	for _, r := range tokenRoles(token) {
		if r.EntityType == astrov1.ApiTokenRoleEntityTypeWORKSPACE && r.EntityId == workspaceID && r.Role == role {
			return errOrgTokenInWorkspace
		}
	}

	newRoles := upsertRole(tokenRoles(token), astrov1.ApiTokenRoleEntityTypeWORKSPACE, workspaceID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{
		Roles: newRoles,
	})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully added to the Workspace\n", token.Name)
	return nil
}

func selectTokens(apiTokens []astrov1.ApiToken) (astrov1.ApiToken, error) {
	apiTokensMap := map[string]astrov1.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		orgRole := orgRoleOf(apiTokens[i])
		expires := apiTokens[i].ExpiryPeriodInDays

		index := i + 1
		expiresStr := ""
		if expires != nil {
			expiresStr = fmt.Sprint(*expires)
		}
		tab.AddRow([]string{
			strconv.Itoa(index),
			name,
			description,
			orgRole,
			expiresStr,
		}, false)
		apiTokensMap[strconv.Itoa(index)] = apiTokens[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := apiTokensMap[choice]
	if !ok {
		return astrov1.ApiToken{}, errInvalidOrganizationTokenKey
	}
	return selected, nil
}

// listOrgScopedAPITokens returns only ORGANIZATION-scoped tokens (paginated).
func listOrgScopedAPITokens(client astrov1.APIClient) ([]astrov1.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	only := true
	limit := tokenPaginationLim
	var tokens []astrov1.ApiToken
	offset := 0
	for {
		params := &astrov1.ListApiTokensParams{
			IncludeOnlyOrganizationTokens: &only,
			Offset:                        &offset,
			Limit:                         &limit,
		}
		resp, err := client.ListApiTokensWithResponse(httpContext.Background(), ctx.Organization, params)
		if err != nil {
			return nil, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		tokens = append(tokens, resp.JSON200.Tokens...)
		if resp.JSON200.TotalCount <= offset+limit {
			break
		}
		offset += limit
	}
	return tokens, nil
}

func getOrganizationToken(id, name, message string, tokens []astrov1.ApiToken) (token astrov1.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(tokens)
		if err != nil {
			return astrov1.ApiToken{}, err
		}
	case name == "" && id != "":
		for i := range tokens {
			if tokens[i].Id == id {
				token = tokens[i]
			}
		}
		if token.Id == "" {
			return astrov1.ApiToken{}, errOrganizationTokenNotFound
		}
	case name != "" && id == "":
		var matchedTokens []astrov1.ApiToken
		for i := range tokens {
			if tokens[i].Name == name {
				matchedTokens = append(matchedTokens, tokens[i])
			}
		}
		if len(matchedTokens) == 1 {
			token = matchedTokens[0]
		} else if len(matchedTokens) > 1 {
			fmt.Printf("\nThere are more than one API tokens with name %s. Please select an API token:\n", name)
			token, err = selectTokens(matchedTokens)
			if err != nil {
				return astrov1.ApiToken{}, err
			}
		}
	}
	if token.Id == "" {
		return astrov1.ApiToken{}, errOrganizationTokenNotFound
	}
	return token, nil
}

// ListTokens lists all Organization-scoped API tokens.
func ListTokens(client astrov1.APIClient, out io.Writer) error {
	apiTokens, err := listOrgScopedAPITokens(client)
	if err != nil {
		return err
	}

	tab := newTokenTableOut()
	for i := range apiTokens {
		created := TimeAgo(apiTokens[i].CreatedAt)
		var createdBy string
		if apiTokens[i].CreatedBy != nil {
			switch {
			case apiTokens[i].CreatedBy.FullName != nil:
				createdBy = *apiTokens[i].CreatedBy.FullName
			case apiTokens[i].CreatedBy.ApiTokenName != nil:
				createdBy = *apiTokens[i].CreatedBy.ApiTokenName
			}
		}
		tab.AddRow([]string{
			apiTokens[i].Id,
			apiTokens[i].Name,
			apiTokens[i].Description,
			string(apiTokens[i].Scope),
			orgRoleOf(apiTokens[i]),
			created,
			createdBy,
		}, false)
	}
	tab.Print(out)

	return nil
}

func getTokenByID(id, orgID string, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	resp, err := client.GetApiTokenWithResponse(httpContext.Background(), orgID, id)
	if err != nil {
		return astrov1.ApiToken{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.ApiToken{}, err
	}
	return *resp.JSON200, nil
}

func GetTokenFromInputOrUser(id, name, organization string, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	if id == "" {
		tokens, err := listOrgScopedAPITokens(client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getOrganizationToken(id, name, "\nPlease select the Organization API token you would like to update:", tokens)
		if err != nil {
			return token, err
		}
		token, err = getTokenByID(tokenFromList.Id, organization, client)
		if err != nil {
			return token, err
		}
	} else {
		token, err = getTokenByID(id, organization, client)
		if err != nil {
			return token, err
		}
	}
	if string(token.Scope) != organizationEntity {
		return token, errWrongTokenTypeSelected
	}
	return token, err
}

// ListTokenRoles lists all roles for a given Organization token.
func ListTokenRoles(id string, client astrov1.APIClient, out io.Writer) (err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	apiToken, err := GetTokenFromInputOrUser(id, "", ctx.Organization, client)
	if err != nil {
		return err
	}

	tab := newTokenRolesTableOut()
	for _, r := range tokenRoles(apiToken) {
		tab.AddRow([]string{string(r.EntityType), r.EntityId, r.Role}, false)
	}
	tab.Print(out)

	return nil
}

// CreateToken creates an Organization-scoped API token.
func CreateToken(name, description, role string, expiration int, cleanOutput bool, out io.Writer, client astrov1.APIClient) error {
	if err := user.IsOrganizationRoleValid(role); err != nil {
		return err
	}
	if name == "" {
		return ErrInvalidName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	req := astrov1.CreateApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
		Scope:       astrov1.CreateApiTokenRequestScopeORGANIZATION,
	}
	if expiration != 0 {
		req.TokenExpiryPeriodInDays = &expiration
	}
	resp, err := client.CreateApiTokenWithResponse(httpContext.Background(), ctx.Organization, req)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	token := resp.JSON200
	if cleanOutput {
		if token.Token != nil {
			fmt.Println(*token.Token)
		}
	} else {
		fmt.Fprintf(out, "\nAstro Organization API token %s was successfully created\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		if token.Token != nil {
			fmt.Println("\n" + *token.Token)
		}
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// UpdateToken updates an Organization-scoped API token. Name/description are updated via
// UpdateApiToken; a role change is written via UpdateApiTokenRoles (full role-set replacement).
func UpdateToken(id, name, newName, description, role string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	token, err := GetTokenFromInputOrUser(id, name, ctx.Organization, client)
	if err != nil {
		return err
	}

	updateReq := astrov1.UpdateApiTokenJSONRequestBody{}
	if newName == "" {
		updateReq.Name = token.Name
	} else {
		updateReq.Name = newName
	}
	if description == "" {
		d := token.Description
		updateReq.Description = &d
	} else {
		d := description
		updateReq.Description = &d
	}
	resp, err := client.UpdateApiTokenWithResponse(httpContext.Background(), ctx.Organization, token.Id, updateReq)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}

	if role != "" {
		if err := user.IsOrganizationRoleValid(role); err != nil {
			return err
		}
		newRoles := replaceOrgRole(tokenRoles(token), ctx.Organization, role)
		rolesResp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
		if err != nil {
			return err
		}
		if err := astrov1.NormalizeAPIError(rolesResp.HTTPResponse, rolesResp.Body); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Astro Organization API token %s was successfully updated\n", token.Name)
	return nil
}

// RotateToken rotates the secret for an Organization API token.
func RotateToken(id, name string, cleanOutput, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	token, err := GetTokenFromInputOrUser(id, name, ctx.Organization, client)
	if err != nil {
		return err
	}

	if !force {
		fmt.Println("WARNING: API Token rotation will invalidate the current token and cannot be undone.")
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to rotate the %s API token?", ansi.Bold(token.Name)))

		if !i {
			fmt.Println("Canceling token rotation")
			return nil
		}
	}
	resp, err := client.RotateApiTokenWithResponse(httpContext.Background(), ctx.Organization, token.Id)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	rotated := resp.JSON200
	if cleanOutput {
		if rotated.Token != nil {
			fmt.Println(*rotated.Token)
		}
	} else {
		fmt.Fprintf(out, "\nAstro Organization API token %s was successfully rotated\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		if rotated.Token != nil {
			fmt.Println("\n" + *rotated.Token)
		}
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// DeleteToken deletes an Organization-scoped API token or removes a non-org token's association
// from the Organization.
func DeleteToken(id, name string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	token, err := GetTokenFromInputOrUser(id, name, ctx.Organization, client)
	if err != nil {
		return err
	}
	isOrg := string(token.Scope) == organizationEntity
	if !force {
		var msg string
		if isOrg {
			fmt.Println("WARNING: API token deletion cannot be undone.")
			msg = fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name))
		} else {
			msg = fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Organization?", ansi.Bold(token.Name))
		}
		i, _ := input.Confirm(msg)
		if !i {
			if isOrg {
				fmt.Println("Canceling API Token deletion")
			} else {
				fmt.Println("Canceling API Token removal")
			}
			return nil
		}
	}

	resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), ctx.Organization, token.Id)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	if isOrg {
		fmt.Fprintf(out, "Astro Organization API token %s was successfully deleted\n", token.Name)
	} else {
		fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Organization\n", token.Name)
	}
	return nil
}

func TimeAgo(date time.Time) string {
	duration := time.Since(date)
	days := int(duration.Hours() / 24) //nolint:mnd
	hours := int(duration.Hours())
	minutes := int(duration.Minutes())

	switch {
	case days > 0:
		return fmt.Sprintf("%d days ago", days)
	case hours > 0:
		return fmt.Sprintf("%d hours ago", hours)
	case minutes > 0:
		return fmt.Sprintf("%d minutes ago", minutes)
	default:
		return "Just now"
	}
}
