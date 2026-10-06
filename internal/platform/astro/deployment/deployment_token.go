package deployment

// The `astro deployment token` family. Every function here returns what it did
// and leaves the rendering, and every question, to its caller: cmd/astro prints
// the text, publishes the json, and asks a person to pick a token or confirm a
// rotation or a deletion. Nothing in this file prints or reads stdin. What it
// returns is the shape all three token families share, in apitoken.

import (
	httpContext "context"
	"errors"
	"fmt"
	"slices"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	workspaceService "github.com/astronomer/astro-cli/internal/platform/astro/workspace-token"
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

var (
	ErrDeploymentTokenNotFound    = errors.New("no Deployment API token was found for the API token name you provided")
	errWorkspaceTokenInDeployment = errors.New("this Workspace API token has already been added to the Deployment with that role")
	errOrgTokenInDeployment       = errors.New("this Organization API token has already been added to the Deployment with that role")
	errDeploymentTokenRoleSet     = errors.New("this Deployment API token already has that role on the Deployment")
	errWrongTokenTypeSelected     = errors.New("the token selected is not of the type you are trying to modify")
)

const deploymentEntity = "DEPLOYMENT"

const (
	// The plain choice has no heading here: the picker asks it in words
	// that suit the command, which this package does not know.
	pickSharedNameHead = "\nThere are more than one API tokens with name %s. Please select an API token:"
)

// deploymentRoleOf returns the token's role on deploymentID, or "".
func deploymentRoleOf(t *astrov1.ApiToken, deploymentID string) string {
	return apitoken.RoleOn(t, astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, deploymentID)
}

// deploymentRoleReader reads each token's role on deploymentID.
func deploymentRoleReader(deploymentID string) func(*astrov1.ApiToken) string {
	return func(t *astrov1.ApiToken) string { return deploymentRoleOf(t, deploymentID) }
}

// ListTokens lists tokens with a role in the given deployment. tokenTypes, when
// not empty, keeps only those scopes. The list is empty, never nil, when there
// are none.
func ListTokens(client astrov1.APIClient, deploymentID string, tokenTypes []DeploymentTokenType) ([]apitoken.Token, error) {
	apiTokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
	if err != nil {
		return nil, err
	}
	return apitoken.AllFromAPI(apiTokens, deploymentRoleReader(deploymentID)), nil
}

// CreateToken creates a Deployment-scoped API token and returns it with its
// secret.
func CreateToken(name, description, role, deploymentID string, expiration int, client astrov1.APIClient) (apitoken.Token, error) {
	if name == "" {
		return apitoken.Token{}, ErrInvalidTokenName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Token{}, err
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
		return apitoken.Token{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Token{}, err
	}
	created := resp.JSON200
	return apitoken.WithSecret(apitoken.FromAPI(created, role), created), nil
}

// UpdateToken updates a Deployment-scoped API token's name and description,
// and its role on the Deployment when role is not "". An empty newName or
// description keeps the token's own; an empty role leaves the role alone, and
// no role change is sent.
//
// A role the token already holds on the Deployment is refused before anything
// is sent, as the workspace-token and organization-token adds and updates
// refuse theirs. The role is changed before the name and description, so a
// role the API refuses leaves the token as it was.
func UpdateToken(id, name, newName, description, role, deploymentID string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Update, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Update{}, err
	}
	organizationID := ctx.Organization
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeDEPLOYMENT}

	token, err := FindToken(id, name, deploymentID, tokenTypes, pick, client)
	if err != nil {
		return apitoken.Update{}, err
	}

	currentRole := deploymentRoleOf(&token, deploymentID)
	if role != "" && role == currentRole {
		return apitoken.Update{}, errDeploymentTokenRoleSet
	}
	if role != "" {
		if err := setDeploymentRole(&token, deploymentID, role, organizationID, client); err != nil {
			return apitoken.Update{}, err
		}
	}

	updated, err := updateNameAndDescription(&token, newName, description, organizationID, client)
	if err != nil {
		return apitoken.Update{}, err
	}

	newRole := currentRole
	if role != "" {
		newRole = role
	}
	return apitoken.Update{Token: apitoken.FromAPI(&updated, newRole), PreviousName: token.Name}, nil
}

// updateNameAndDescription sends token's new name and description, an empty
// one keeping its own, and returns the token as the update left it: the API's
// answer, or what was asked for when it gave none.
func updateNameAndDescription(token *astrov1.ApiToken, newName, description, organizationID string, client astrov1.APIClient) (astrov1.ApiToken, error) {
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
		return astrov1.ApiToken{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return astrov1.ApiToken{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	updated := *token
	updated.Name, updated.Description = updateReq.Name, *updateReq.Description
	return updated, nil
}

// RotateToken rotates the secret of token, which FindToken found, and returns
// it with the new secret.
func RotateToken(token astrov1.ApiToken, deploymentID string, client astrov1.APIClient) (apitoken.Token, error) { //nolint:gocritic // ApiToken is what FindToken returns
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Token{}, err
	}
	resp, err := client.RotateApiTokenWithResponse(httpContext.Background(), ctx.Organization, token.Id)
	if err != nil {
		return apitoken.Token{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Token{}, err
	}
	rotated := resp.JSON200
	role := deploymentRoleOf(rotated, deploymentID)
	if role == "" {
		role = deploymentRoleOf(&token, deploymentID)
	}
	return apitoken.WithSecret(apitoken.FromAPI(rotated, role), rotated), nil
}

// DeleteToken deletes token when it is Deployment-scoped, and otherwise
// removes its role on the Deployment, leaving the token itself.
func DeleteToken(token astrov1.ApiToken, deploymentID string, client astrov1.APIClient) (apitoken.DeploymentRemoval, error) { //nolint:gocritic // ApiToken is what FindToken returns
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	organizationID := ctx.Organization
	removal := apitoken.DeploymentRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), DeploymentID: deploymentID}

	if string(token.Scope) == deploymentEntity {
		resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
		if err != nil {
			return apitoken.DeploymentRemoval{}, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return apitoken.DeploymentRemoval{}, err
		}
		removal.Action = apitoken.Deleted
		return removal, nil
	}
	if err := setDeploymentRole(&token, deploymentID, "", organizationID, client); err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	removal.Action = apitoken.Removed
	return removal, nil
}

// setDeploymentRole gives token role on deploymentID, or takes its role there
// away when role is "".
func setDeploymentRole(token *astrov1.ApiToken, deploymentID, role, organizationID string, client astrov1.APIClient) error {
	newRoles := apitoken.WithRole(apitoken.Roles(token), astrov1.ApiTokenRoleEntityTypeDEPLOYMENT, deploymentID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// getDeploymentTokens lists tokens with a role in the given deployment, filtered client-side by scope.
func getDeploymentTokens(deploymentID string, tokenTypes []DeploymentTokenType, client astrov1.APIClient) ([]astrov1.ApiToken, error) {
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
	if len(tokenTypes) == 0 {
		return tokens, nil
	}
	allowed := map[string]struct{}{}
	for _, t := range tokenTypes {
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

func getDeploymentToken(id, name, deploymentID string, tokens []astrov1.ApiToken, pick apitoken.Picker) (token astrov1.ApiToken, err error) {
	roleOf := deploymentRoleReader(deploymentID)
	switch {
	case id == "" && name == "":
		token, err = apitoken.Pick(pick, "", tokens, roleOf)
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
			token, err = apitoken.Pick(pick, fmt.Sprintf(pickSharedNameHead, name), matchedTokens, roleOf)
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

// FindToken finds the token a command means: the one with id, else the one
// named name among the tokens with a role on the Deployment, else the one a
// person picks through pick. tokenTypes, when not empty, are the scopes the
// command may act on.
func FindToken(id, name, deploymentID string, tokenTypes []DeploymentTokenType, pick apitoken.Picker, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return token, err
	}
	organizationID := ctx.Organization
	if id == "" {
		tokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getDeploymentToken(id, name, deploymentID, tokens, pick)
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
	if len(tokenTypes) > 0 {
		stringTokenTypes := []string{}
		for _, tokenType := range tokenTypes {
			stringTokenTypes = append(stringTokenTypes, string(tokenType))
		}
		if !slices.Contains(stringTokenTypes, string(token.Scope)) {
			return token, errWrongTokenTypeSelected
		}
	}
	return token, err
}

// RemoveOrgTokenDeploymentRole removes the deployment-scope role from an Organization token.
func RemoveOrgTokenDeploymentRole(id, name, deploymentID string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.DeploymentRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeORGANIZATION}
	token, err := FindToken(id, name, deploymentID, tokenTypes, pick, client)
	if err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	if err := setDeploymentRole(&token, deploymentID, "", ctx.Organization, client); err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	return apitoken.DeploymentRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), DeploymentID: deploymentID, Action: apitoken.Removed}, nil
}

// RemoveWorkspaceTokenDeploymentRole removes the deployment-scope role from a
// Workspace token. A token named by neither id nor name is picked, through
// pick, from the Workspace's tokens.
func RemoveWorkspaceTokenDeploymentRole(id, name, workspaceID, deploymentID string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.DeploymentRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	organizationID := ctx.Organization
	wsTypes := []workspaceService.TokenType{workspaceService.TokenTypeWORKSPACE}
	token, err := workspaceService.FindToken(id, name, workspaceID, organizationID, wsTypes, pick, client)
	if err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	if err := setDeploymentRole(&token, deploymentID, "", organizationID, client); err != nil {
		return apitoken.DeploymentRemoval{}, err
	}
	return apitoken.DeploymentRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), DeploymentID: deploymentID, Action: apitoken.Removed}, nil
}

// UpsertWorkspaceTokenDeploymentRole adds/updates a deployment-scope role on a
// Workspace token, and returns the token with that role. An add ("create")
// finds the token among the Workspace's, an update among the Deployment's;
// either asks through pick, which the caller chooses to suit.
func UpsertWorkspaceTokenDeploymentRole(id, name, role, workspaceID, deploymentID, operation string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Token, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Token{}, err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		wsTypes := []workspaceService.TokenType{workspaceService.TokenTypeWORKSPACE}
		token, err = workspaceService.FindToken(id, name, workspaceID, ctx.Organization, wsTypes, pick, client)
	} else {
		token, err = FindToken(id, name, deploymentID, []DeploymentTokenType{DeploymentTokenTypeWORKSPACE}, pick, client)
	}
	if err != nil {
		return apitoken.Token{}, err
	}
	// Short-circuit: already has this role on the deployment.
	if deploymentRoleOf(&token, deploymentID) == role {
		return apitoken.Token{}, errWorkspaceTokenInDeployment
	}
	if err := setDeploymentRole(&token, deploymentID, role, ctx.Organization, client); err != nil {
		return apitoken.Token{}, err
	}
	return apitoken.FromAPI(&token, role), nil
}

// UpsertOrgTokenDeploymentRole adds/updates a deployment-scope role on an
// Organization token, and returns the token with that role. An add ("create")
// finds the token among the Organization's, an update among the Deployment's;
// either asks through pick, which the caller chooses to suit.
func UpsertOrgTokenDeploymentRole(id, name, role, deploymentID, operation string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Token, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Token{}, err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		token, err = organization.FindToken(id, name, ctx.Organization, pick, client)
	} else {
		token, err = FindToken(id, name, deploymentID, []DeploymentTokenType{DeploymentTokenTypeORGANIZATION}, pick, client)
	}
	if err != nil {
		return apitoken.Token{}, err
	}
	// Short-circuit: already has this role on the deployment.
	if deploymentRoleOf(&token, deploymentID) == role {
		return apitoken.Token{}, errOrgTokenInDeployment
	}
	if err := setDeploymentRole(&token, deploymentID, role, ctx.Organization, client); err != nil {
		return apitoken.Token{}, err
	}
	return apitoken.FromAPI(&token, role), nil
}
