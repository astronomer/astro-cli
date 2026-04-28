package workspacetoken

import (
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/user"
	workspaceService "github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// TokenType is a scope filter used by ListTokens to narrow results to tokens whose
// Scope matches one of the supplied values. v1's /tokens endpoint does not expose a
// per-scope filter flag, so the filtering is applied client-side on the ApiToken.Scope field.
type TokenType string

const (
	TokenTypeWORKSPACE    TokenType = "WORKSPACE"
	TokenTypeORGANIZATION TokenType = "ORGANIZATION"
	tokenPaginationLim              = 100
)

func newTokenTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "NAME", "DESCRIPTION", "SCOPE", "WORKSPACE ROLE", "CREATED", "CREATED BY"},
	}
}

func newTokenSelectionTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "ID", "NAME", "DESCRIPTION", "SCOPE", "WORKSPACE ROLE", "CREATED", "CREATED BY"},
	}
}

var (
	errInvalidWorkspaceTokenKey   = errors.New("invalid Workspace API token selection")
	errWorkspaceTokenInDeployment = errors.New("this Workspace API token has already been added to the Deployment with that role")
	ErrWorkspaceTokenNotFound     = errors.New("no Workspace API token was found for the API token name you provided")
	errOrgTokenInWorkspace        = errors.New("this Organization API token has already been added to the Workspace with that role")
	errWrongTokenTypeSelected     = errors.New("the token selected is not of the type you are trying to modify")
)

const (
	workspaceEntity    = "WORKSPACE"
	deploymentEntity   = "DEPLOYMENT"
	organizationEntity = "ORGANIZATION"
)

// tokenRoles flattens a token's roles pointer to a usable slice.
func tokenRoles(t astrov1.ApiToken) []astrov1.ApiTokenRole { //nolint:gocritic // ApiToken is large; helper returns a slice
	if t.Roles == nil {
		return nil
	}
	return *t.Roles
}

// roleForEntity returns the first role whose (entityType, entityId) matches, or "".
func workspaceRoleOf(t astrov1.ApiToken, workspaceID string) string { //nolint:gocritic // ApiToken is large; helper returns a short string
	const entityType = astrov1.ApiTokenRoleEntityTypeWORKSPACE
	entityID := workspaceID
	for _, r := range tokenRoles(t) {
		if r.EntityType == entityType && r.EntityId == entityID {
			return r.Role
		}
	}
	return ""
}

// upsertWorkspaceRole replaces (or inserts) the WORKSPACE-scoped entry for workspaceID with role.
// If role == "", the matching entry is removed.
func upsertWorkspaceRole(existing []astrov1.ApiTokenRole, workspaceID, role string) []astrov1.ApiTokenRole {
	const entityType = astrov1.ApiTokenRoleEntityTypeWORKSPACE
	out := []astrov1.ApiTokenRole{}
	for _, r := range existing {
		if r.EntityType == entityType && r.EntityId == workspaceID {
			continue
		}
		out = append(out, r)
	}
	if role != "" {
		out = append(out, astrov1.ApiTokenRole{
			EntityType: entityType,
			EntityId:   workspaceID,
			Role:       role,
		})
	}
	return out
}

// ListTokens lists tokens with a workspace role. tokenTypes (if non-nil) filters by token Scope.
func ListTokens(client astrov1.APIClient, workspaceID string, tokenTypes *[]TokenType, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	apiTokens, err := getWorkspaceTokens(workspaceID, tokenTypes, client)
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
			workspaceRoleOf(apiTokens[i], workspaceID),
			created,
			createdBy,
		}, false)
	}
	tab.Print(out)

	return nil
}

// CreateToken creates a Workspace-scoped API token.
func CreateToken(name, description, role, workspaceID string, expiration int, cleanOutput bool, out io.Writer, client astrov1.APIClient) error {
	if err := user.IsWorkspaceRoleValid(role); err != nil {
		return err
	}
	if name == "" {
		return workspaceService.ErrInvalidTokenName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	wsID := workspaceID
	req := astrov1.CreateApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
		Scope:       astrov1.CreateApiTokenRequestScopeWORKSPACE,
		EntityId:    &wsID,
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
		fmt.Fprintf(out, "\nAstro Workspace API token %s was successfully created\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		if token.Token != nil {
			fmt.Println("\n" + *token.Token)
		}
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// UpdateToken updates a Workspace-scoped API token's name/description and optionally its workspace role.
func UpdateToken(id, name, newName, description, role, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	organizationID := ctx.Organization

	tokenTypes := []TokenType{TokenTypeWORKSPACE}

	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client)
	if err != nil {
		return err
	}

	// Name/description updates
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
	resp, err := client.UpdateApiTokenWithResponse(httpContext.Background(), organizationID, token.Id, updateReq)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}

	// Determine the new/effective workspace role.
	newRole := role
	if newRole == "" {
		newRole = workspaceRoleOf(token, workspaceID)
	}
	if newRole == "" {
		// No existing role and none requested — nothing to mutate on the roles side.
		fmt.Fprintf(out, "Astro Workspace API token %s was successfully updated\n", token.Name)
		return nil
	}
	if err := user.IsWorkspaceRoleValid(newRole); err != nil {
		return err
	}
	// Short-circuit: requested role is already set.
	if role != "" && workspaceRoleOf(token, workspaceID) == role {
		return errWorkspaceTokenInDeployment
	}
	newRoles := upsertWorkspaceRole(tokenRoles(token), workspaceID, newRole)
	rolesResp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(rolesResp.HTTPResponse, rolesResp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully updated\n", token.Name)
	return nil
}

// RotateToken rotates the secret for a Workspace-scoped token.
func RotateToken(id, name, workspaceID string, cleanOutput, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	organizationID := ctx.Organization
	tokenTypes := []TokenType{TokenTypeWORKSPACE}

	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client)
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
	resp, err := client.RotateApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
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
		fmt.Fprintf(out, "\nAstro Workspace API token %s was successfully rotated\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		if rotated.Token != nil {
			fmt.Println("\n" + *rotated.Token)
		}
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// DeleteToken deletes a Workspace-scoped token, or (for an Organization token referenced via
// a workspace role) removes the workspace role to "detach" it from the workspace.
func DeleteToken(id, name, workspaceID string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	organizationID := ctx.Organization
	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, nil, client)
	if err != nil {
		return err
	}
	isWS := string(token.Scope) == workspaceEntity
	if !force {
		var msg string
		if isWS {
			fmt.Println("WARNING: API token deletion cannot be undone.")
			msg = fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name))
		} else {
			msg = fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Workspace?", ansi.Bold(token.Name))
		}
		i, _ := input.Confirm(msg)
		if !i {
			if isWS {
				fmt.Println("Canceling API Token deletion")
			} else {
				fmt.Println("Canceling API Token removal")
			}
			return nil
		}
	}

	if isWS {
		resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
		if err != nil {
			return err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return err
		}
		fmt.Fprintf(out, "Astro Workspace API token %s was successfully deleted\n", token.Name)
		return nil
	}
	// Detach by removing the workspace role.
	newRoles := upsertWorkspaceRole(tokenRoles(token), workspaceID, "")
	rolesResp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(rolesResp.HTTPResponse, rolesResp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Workspace\n", token.Name)
	return nil
}

func selectTokens(workspaceID string, apiTokens []astrov1.ApiToken) (astrov1.ApiToken, error) {
	apiTokensMap := map[string]astrov1.ApiToken{}
	tab := newTokenSelectionTableOut()
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

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			apiTokens[i].Id,
			apiTokens[i].Name,
			apiTokens[i].Description,
			string(apiTokens[i].Scope),
			workspaceRoleOf(apiTokens[i], workspaceID),
			created,
			createdBy,
		}, false)
		apiTokensMap[strconv.Itoa(index)] = apiTokens[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")

	selected, ok := apiTokensMap[choice]
	if !ok {
		return astrov1.ApiToken{}, errInvalidWorkspaceTokenKey
	}
	return selected, nil
}

// getWorkspaceTokens lists tokens with a role in the given workspace, then filters client-side
// by token scope if tokenTypes is non-nil.
func getWorkspaceTokens(workspaceID string, tokenTypes *[]TokenType, client astrov1.APIClient) ([]astrov1.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	wsID := workspaceID
	limit := tokenPaginationLim
	var tokens []astrov1.ApiToken
	offset := 0
	for {
		params := &astrov1.ListApiTokensParams{
			WorkspaceId: &wsID,
			Offset:      &offset,
			Limit:       &limit,
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
	if tokenTypes == nil || len(*tokenTypes) == 0 {
		return tokens, nil
	}
	allowed := map[string]struct{}{}
	for _, t := range *tokenTypes {
		allowed[string(t)] = struct{}{}
	}
	filtered := tokens[:0]
	for i := range tokens {
		if _, ok := allowed[string(tokens[i].Scope)]; ok {
			filtered = append(filtered, tokens[i])
		}
	}
	return filtered, nil
}

func getWorkspaceToken(id, name, workspaceID, message string, tokens []astrov1.ApiToken) (token astrov1.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(workspaceID, tokens)
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
			return astrov1.ApiToken{}, ErrWorkspaceTokenNotFound
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
			token, err = selectTokens(workspaceID, matchedTokens)
			if err != nil {
				return astrov1.ApiToken{}, err
			}
		}
	}
	if token.Id == "" {
		return astrov1.ApiToken{}, ErrWorkspaceTokenNotFound
	}
	return token, nil
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

func getTokenByID(id, orgID string, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	resp, err := client.GetApiTokenWithResponse(httpContext.Background(), orgID, id)
	if err != nil {
		return astrov1.ApiToken{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return astrov1.ApiToken{}, err
	}
	return *resp.JSON200, nil
}

func GetTokenFromInputOrUser(id, name, workspaceID, organizationID string, tokenTypes *[]TokenType, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	if id == "" {
		tokens, err := getWorkspaceTokens(workspaceID, tokenTypes, client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getWorkspaceToken(id, name, workspaceID, "\nPlease select the Workspace API token you would like to add to the Deployment:", tokens)
		if err != nil {
			return token, err
		}
		token, err = getTokenByID(tokenFromList.Id, organizationID, client)
		if err != nil {
			return token, err
		}
	} else {
		token, err = getTokenByID(id, organizationID, client)
		if err != nil {
			return token, err
		}
	}
	if tokenTypes != nil && len(*tokenTypes) > 0 {
		stringTokenTypes := []string{}
		for _, tokenType := range *tokenTypes {
			stringTokenTypes = append(stringTokenTypes, string(tokenType))
		}
		if !slices.Contains(stringTokenTypes, string(token.Scope)) {
			return token, errWrongTokenTypeSelected
		}
	}
	return token, err
}

// RemoveOrgTokenWorkspaceRole removes the workspace-scope role from an Organization token.
func RemoveOrgTokenWorkspaceRole(id, name, workspaceID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	organizationID := ctx.Organization
	tokenTypes := []TokenType{TokenTypeORGANIZATION}
	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client)
	if err != nil {
		return err
	}
	newRoles := upsertWorkspaceRole(tokenRoles(token), workspaceID, "")
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Workspace\n", token.Name)
	return nil
}

// UpsertOrgTokenWorkspaceRole adds or updates a workspace-scope role on an Organization token.
// operation=="create" looks up the token via the Organization token catalog; otherwise via the
// workspace token catalog restricted to ORGANIZATION-scoped tokens.
func UpsertOrgTokenWorkspaceRole(id, name, role, workspaceID, operation string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	var token astrov1.ApiToken
	if operation == "create" {
		token, err = organization.GetTokenFromInputOrUser(id, name, ctx.Organization, client)
		if err != nil {
			return err
		}
	} else {
		tokenTypes := []TokenType{TokenTypeORGANIZATION}
		token, err = GetTokenFromInputOrUser(id, name, workspaceID, ctx.Organization, &tokenTypes, client)
		if err != nil {
			return err
		}
	}

	// Short-circuit: already has this role on the workspace.
	if workspaceRoleOf(token, workspaceID) == role {
		return errOrgTokenInWorkspace
	}

	newRoles := upsertWorkspaceRole(tokenRoles(token), workspaceID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully added/updated to the Workspace\n", token.Name)
	return nil
}
