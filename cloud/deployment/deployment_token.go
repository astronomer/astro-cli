package deployment

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
	workspaceService "github.com/astronomer/astro-cli/cloud/workspace-token"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// DeploymentTokenType narrows ListTokens results by token Scope (client-side filter,
// since v1's /tokens endpoint does not expose a per-scope filter flag).
type DeploymentTokenType string

const (
	DeploymentTokenTypeDEPLOYMENT   DeploymentTokenType = "DEPLOYMENT"
	DeploymentTokenTypeWORKSPACE    DeploymentTokenType = "WORKSPACE"
	DeploymentTokenTypeORGANIZATION DeploymentTokenType = "ORGANIZATION"
	deploymentTokenPaginationLim                        = 100
)

func newTokenTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"ID", "NAME", "DESCRIPTION", "SCOPE", "DEPLOYMENT ROLE", "CREATED", "CREATED BY"},
	}
}

func newTokenSelectionTableOut() *printutil.Table {
	return &printutil.Table{
		DynamicPadding: true,
		Header:         []string{"#", "ID", "NAME", "DESCRIPTION", "SCOPE", "DEPLOYMENT ROLE", "CREATED", "CREATED BY"},
	}
}

var (
	errInvalidDeploymentTokenKey  = errors.New("invalid Deployment API token selection")
	ErrDeploymentTokenNotFound    = errors.New("no Deployment API token was found for the API token name you provided")
	errWorkspaceTokenInDeployment = errors.New("this Workspace API token has already been added to the Deployment with that role")
	errOrgTokenInDeployment       = errors.New("this Organization API token has already been added to the Deployment with that role")
	errWrongTokenTypeSelected     = errors.New("the token selected is not of the type you are trying to modify")
)

const (
	deploymentEntity   = "DEPLOYMENT"
	workspaceEntity    = "WORKSPACE"
	organizationEntity = "ORGANIZATION"
)

// tokenRoles flattens a token's roles pointer to a usable slice.
func tokenRoles(t astrov1.ApiToken) []astrov1.ApiTokenRole { //nolint:gocritic // ApiToken is large; helper returns a slice and isn't hot
	if t.Roles == nil {
		return nil
	}
	return *t.Roles
}

// roleForEntity returns the first role whose (entityType, entityId) matches, or "".
func deploymentRoleOf(t astrov1.ApiToken, deploymentID string) string { //nolint:gocritic // ApiToken is large; helper returns a short string
	const entityType = astrov1.ApiTokenRoleEntityTypeDEPLOYMENT
	entityID := deploymentID
	for _, r := range tokenRoles(t) {
		if r.EntityType == entityType && r.EntityId == entityID {
			return r.Role
		}
	}
	return ""
}

// upsertDeploymentRole replaces (or inserts) the DEPLOYMENT-scoped entry for deploymentID with role.
// If role == "", the matching entry is removed.
func upsertDeploymentRole(existing []astrov1.ApiTokenRole, deploymentID, role string) []astrov1.ApiTokenRole {
	const entityType = astrov1.ApiTokenRoleEntityTypeDEPLOYMENT
	out := []astrov1.ApiTokenRole{}
	for _, r := range existing {
		if r.EntityType == entityType && r.EntityId == deploymentID {
			continue
		}
		out = append(out, r)
	}
	if role != "" {
		out = append(out, astrov1.ApiTokenRole{
			EntityType: entityType,
			EntityId:   deploymentID,
			Role:       role,
		})
	}
	return out
}

// ListTokens lists tokens with a role in the given deployment. tokenTypes (if non-nil) filters by Scope.
func ListTokens(client astrov1.APIClient, deploymentID string, tokenTypes *[]DeploymentTokenType, out io.Writer) error {
	apiTokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
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
			deploymentRoleOf(apiTokens[i], deploymentID),
			created,
			createdBy,
		}, false)
	}
	tab.Print(out)

	return nil
}

// CreateToken creates a Deployment-scoped API token.
func CreateToken(name, description, role, deploymentID string, expiration int, cleanOutput bool, out io.Writer, client astrov1.APIClient) error {
	if name == "" {
		return ErrInvalidTokenName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	dID := deploymentID
	req := astrov1.CreateApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
		Scope:       astrov1.CreateApiTokenRequestScopeDEPLOYMENT,
		EntityId:    &dID,
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
		fmt.Fprintf(out, "\nAstro Deployment API token %s was successfully created\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		if token.Token != nil {
			fmt.Println("\n" + *token.Token)
		}
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// UpdateToken updates a Deployment-scoped API token's name/description and optionally its deployment role.
func UpdateToken(id, name, newName, description, role, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organizationID := ctx.Organization
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeDEPLOYMENT}

	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client)
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
	resp, err := client.UpdateApiTokenWithResponse(httpContext.Background(), organizationID, token.Id, updateReq)
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}

	newRole := role
	if newRole == "" {
		newRole = deploymentRoleOf(token, deploymentID)
	}
	if newRole == "" {
		fmt.Fprintf(out, "Astro Deployment API token %s was successfully updated\n", token.Name)
		return nil
	}
	// Short-circuit: requested role is already set.
	if role != "" && deploymentRoleOf(token, deploymentID) == role {
		return errWorkspaceTokenInDeployment
	}
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, newRole)
	rolesResp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(rolesResp.HTTPResponse, rolesResp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Deployment API token %s was successfully updated\n", token.Name)
	return nil
}

// RotateToken rotates the secret for a Deployment-scoped API token.
func RotateToken(id, name, deploymentID string, cleanOutput, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organizationID := ctx.Organization
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeDEPLOYMENT}

	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client)
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
		fmt.Fprintf(out, "\nAstro Deployment API token %s was successfully rotated\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		if rotated.Token != nil {
			fmt.Println("\n" + *rotated.Token)
		}
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// DeleteToken deletes a Deployment-scoped token or detaches a non-deployment token from the deployment.
func DeleteToken(id, name, deploymentID string, force bool, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organizationID := ctx.Organization
	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, nil, client)
	if err != nil {
		return err
	}
	isDep := string(token.Scope) == deploymentEntity
	if !force {
		var msg string
		if isDep {
			fmt.Println("WARNING: API token deletion cannot be undone.")
			msg = fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name))
		} else {
			msg = fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Deployment?", ansi.Bold(token.Name))
		}
		i, _ := input.Confirm(msg)
		if !i {
			if isDep {
				fmt.Println("Canceling API Token deletion")
			} else {
				fmt.Println("Canceling API Token removal")
			}
			return nil
		}
	}

	if isDep {
		resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
		if err != nil {
			return err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return err
		}
		fmt.Fprintf(out, "Astro Deployment API token %s was successfully deleted\n", token.Name)
		return nil
	}
	// Detach by removing the deployment role.
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, "")
	rolesResp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(rolesResp.HTTPResponse, rolesResp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro API token %s was successfully removed from the Deployment\n", token.Name)
	return nil
}

func selectTokens(deploymentID string, apiTokens []astrov1.ApiToken) (astrov1.ApiToken, error) {
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
			deploymentRoleOf(apiTokens[i], deploymentID),
			created,
			createdBy,
		}, false)
		apiTokensMap[strconv.Itoa(index)] = apiTokens[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")

	selected, ok := apiTokensMap[choice]
	if !ok {
		return astrov1.ApiToken{}, errInvalidDeploymentTokenKey
	}
	return selected, nil
}

// getDeploymentTokens lists tokens with a role in the given deployment, filtered client-side by scope.
func getDeploymentTokens(deploymentID string, tokenTypes *[]DeploymentTokenType, client astrov1.APIClient) ([]astrov1.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	depID := deploymentID
	limit := deploymentTokenPaginationLim
	var tokens []astrov1.ApiToken
	offset := 0
	for {
		params := &astrov1.ListApiTokensParams{
			DeploymentId: &depID,
			Offset:       &offset,
			Limit:        &limit,
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

func getDeploymentToken(id, name, deploymentID, message string, tokens []astrov1.ApiToken) (token astrov1.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(deploymentID, tokens)
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
			return astrov1.ApiToken{}, ErrDeploymentTokenNotFound
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
			token, err = selectTokens(deploymentID, matchedTokens)
			if err != nil {
				return astrov1.ApiToken{}, err
			}
		}
	}
	if token.Id == "" {
		return astrov1.ApiToken{}, ErrDeploymentTokenNotFound
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

func GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID string, tokenTypes *[]DeploymentTokenType, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	if id == "" {
		tokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getDeploymentToken(id, name, deploymentID, "\nPlease select the Deployment API token:", tokens)
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

// RemoveOrgTokenDeploymentRole removes the deployment-scope role from an Organization token.
func RemoveOrgTokenDeploymentRole(id, name, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organizationID := ctx.Organization
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeORGANIZATION}
	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client)
	if err != nil {
		return err
	}
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, "")
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Deployment\n", token.Name)
	return nil
}

// RemoveWorkspaceTokenDeploymentRole removes the deployment-scope role from a Workspace token.
func RemoveWorkspaceTokenDeploymentRole(id, name, workspaceID, deploymentID string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organizationID := ctx.Organization
	wsTypes := []workspaceService.TokenType{workspaceService.TokenTypeWORKSPACE}
	token, err := workspaceService.GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &wsTypes, client)
	if err != nil {
		return err
	}
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, "")
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully removed from the Deployment\n", token.Name)
	return nil
}

// UpsertWorkspaceTokenDeploymentRole adds/updates a deployment-scope role on a Workspace token.
func UpsertWorkspaceTokenDeploymentRole(id, name, role, workspaceID, deploymentID, operation string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		wsTypes := []workspaceService.TokenType{workspaceService.TokenTypeWORKSPACE}
		token, err = workspaceService.GetTokenFromInputOrUser(id, name, workspaceID, ctx.Organization, &wsTypes, client)
	} else {
		depTypes := []DeploymentTokenType{DeploymentTokenTypeWORKSPACE}
		token, err = GetDeploymentTokenFromInputOrUser(id, name, deploymentID, ctx.Organization, &depTypes, client)
	}
	if err != nil {
		return err
	}
	// Short-circuit: already has this role on the deployment.
	if deploymentRoleOf(token, deploymentID) == role {
		return errWorkspaceTokenInDeployment
	}
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully added/updated to the Deployment\n", token.Name)
	return nil
}

// UpsertOrgTokenDeploymentRole adds/updates a deployment-scope role on an Organization token.
func UpsertOrgTokenDeploymentRole(id, name, role, deploymentID, operation string, out io.Writer, client astrov1.APIClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		token, err = organization.GetTokenFromInputOrUser(id, name, ctx.Organization, client)
	} else {
		depTypes := []DeploymentTokenType{DeploymentTokenTypeORGANIZATION}
		token, err = GetDeploymentTokenFromInputOrUser(id, name, deploymentID, ctx.Organization, &depTypes, client)
	}
	if err != nil {
		return err
	}
	// Short-circuit: already has this role on the deployment.
	if deploymentRoleOf(token, deploymentID) == role {
		return errOrgTokenInDeployment
	}
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully added/updated to the Deployment\n", token.Name)
	return nil
}
