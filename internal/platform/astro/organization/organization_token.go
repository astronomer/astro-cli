package organization

// The `astro organization token` family. Every function here returns what it
// did and leaves the rendering, and every question, to its caller: cmd/astro
// prints the text, publishes the json, and asks a person to pick a token or
// confirm a rotation or a deletion. Nothing in this file prints. What it
// returns is the shape all three token families share, in apitoken.

import (
	httpContext "context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	"github.com/astronomer/astro-cli/pkg/input"
)

var (
	ErrInvalidName               = errors.New("no name provided for the organization token. Retry with a valid name")
	errOrganizationTokenNotFound = errors.New("organization token specified was not found")
	errOrgTokenInWorkspace       = errors.New("this Organization API token has already been added to the Workspace with that role")
	errWrongTokenTypeSelected    = errors.New("the token selected is not of the type you are trying to modify")
)

const (
	organizationEntity = "ORGANIZATION"
	tokenPaginationLim = 100

	pickHeading        = "\nPlease select the Organization API token you would like to update:"
	pickSharedNameHead = "\nThere are more than one API tokens with name %s. Please select an API token:"
)

// orgRoleOf returns the token's Organization role, if present.
func orgRoleOf(token *astrov1.ApiToken) string {
	for _, r := range apitoken.Roles(token) {
		if r.EntityType == astrov1.ApiTokenRoleEntityTypeORGANIZATION {
			return r.Role
		}
	}
	return ""
}

// AddOrgTokenToWorkspace gives an Organization token role on the Workspace
// ("" is the current one), and returns the token with that role. A token
// named by neither id nor name is picked through pick.
func AddOrgTokenToWorkspace(id, name, role, workspaceID string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Token, error) {
	if err := user.IsWorkspaceRoleValid(role); err != nil {
		return apitoken.Token{}, err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Token{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	token, err := FindToken(id, name, ctx.Organization, pick, client)
	if err != nil {
		return apitoken.Token{}, err
	}

	// Short-circuit: already has this role on the workspace.
	if apitoken.RoleOn(&token, astrov1.ApiTokenRoleEntityTypeWORKSPACE, workspaceID) == role {
		return apitoken.Token{}, errOrgTokenInWorkspace
	}

	newRoles := apitoken.WithRole(apitoken.Roles(&token), astrov1.ApiTokenRoleEntityTypeWORKSPACE, workspaceID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{
		Roles: newRoles,
	})
	if err != nil {
		return apitoken.Token{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Token{}, err
	}
	return apitoken.FromAPI(&token, role), nil
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

// getOrganizationToken picks the token id or name means from tokens, offering
// each with its Organization role when a person has to choose.
func getOrganizationToken(id, name string, tokens []astrov1.ApiToken, pick apitoken.Picker) (token astrov1.ApiToken, err error) {
	switch {
	case id == "" && name == "":
		token, err = apitoken.Pick(pick, pickHeading, tokens, orgRoleOf)
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
			// Refused before the picker, so a run that may not ask (-o json)
			// says what would answer it: the name answers nothing here.
			if err := input.MayAsk(fmt.Sprintf("Several API tokens are named %s; which one?", name),
				input.About("an API token"), input.AnsweredBy("the token's ID instead of its name")); err != nil {
				return astrov1.ApiToken{}, err
			}
			token, err = apitoken.Pick(pick, fmt.Sprintf(pickSharedNameHead, name), matchedTokens, orgRoleOf)
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

// ListTokens lists the Organization-scoped API tokens, each with its
// Organization role. The list is empty, never nil, when there are none.
func ListTokens(client astrov1.APIClient) ([]apitoken.Token, error) {
	apiTokens, err := listOrgScopedAPITokens(client)
	if err != nil {
		return nil, err
	}
	return apitoken.AllFromAPI(apiTokens, orgRoleOf), nil
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

// FindToken finds the Organization token a command means: the one with id,
// else the one named name, else the one a person picks through pick. A token
// of another scope is refused.
func FindToken(id, name, organization string, pick apitoken.Picker, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	if id == "" {
		tokens, err := listOrgScopedAPITokens(client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getOrganizationToken(id, name, tokens, pick)
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

// FindCurrentToken is FindToken in the current Organization.
func FindCurrentToken(id, name string, pick apitoken.Picker, client astrov1.APIClient) (astrov1.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return astrov1.ApiToken{}, err
	}
	return FindToken(id, name, ctx.Organization, pick, client)
}

// ListTokenRoles lists every role an Organization token holds, empty and
// never nil when it holds none.
func ListTokenRoles(id string, pick apitoken.Picker, client astrov1.APIClient) ([]apitoken.Role, error) {
	apiToken, err := FindCurrentToken(id, "", pick, client)
	if err != nil {
		return nil, err
	}
	roles := []apitoken.Role{}
	for _, r := range apitoken.Roles(&apiToken) {
		roles = append(roles, apitoken.Role{EntityType: string(r.EntityType), EntityID: r.EntityId, Role: r.Role})
	}
	return roles, nil
}

// CreateToken creates an Organization-scoped API token and returns it with
// its secret.
func CreateToken(name, description, role string, expiration int, client astrov1.APIClient) (apitoken.Token, error) {
	if err := user.IsOrganizationRoleValid(role); err != nil {
		return apitoken.Token{}, err
	}
	if name == "" {
		return apitoken.Token{}, ErrInvalidName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Token{}, err
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
		return apitoken.Token{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Token{}, err
	}
	created := resp.JSON200
	return apitoken.WithSecret(apitoken.FromAPI(created, role), created), nil
}

// UpdateToken updates an Organization-scoped API token. Name/description are updated via
// UpdateApiToken; a role change is written via UpdateApiTokenRoles (full role-set replacement).
// The name and description are sent first, so a role that is not valid is
// refused after them.
func UpdateToken(id, name, newName, description, role string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Update, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.Update{}, err
	}
	token, err := FindToken(id, name, ctx.Organization, pick, client)
	if err != nil {
		return apitoken.Update{}, err
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
		return apitoken.Update{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Update{}, err
	}
	// The token as the update left it: the API's answer, or what was asked
	// for when it gave none.
	updated := token
	if resp.JSON200 != nil {
		updated = *resp.JSON200
	} else {
		updated.Name, updated.Description = updateReq.Name, *updateReq.Description
	}

	newRole := orgRoleOf(&token)
	if role != "" {
		if err := user.IsOrganizationRoleValid(role); err != nil {
			return apitoken.Update{}, err
		}
		newRoles := apitoken.WithRole(apitoken.Roles(&token), astrov1.ApiTokenRoleEntityTypeORGANIZATION, ctx.Organization, role)
		rolesResp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), ctx.Organization, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
		if err != nil {
			return apitoken.Update{}, err
		}
		if err := astrov1.NormalizeAPIError(rolesResp.HTTPResponse, rolesResp.Body); err != nil {
			return apitoken.Update{}, err
		}
		newRole = role
	}
	return apitoken.Update{Token: apitoken.FromAPI(&updated, newRole), PreviousName: token.Name}, nil
}

// RotateToken rotates the secret of token, which FindToken found, and returns
// it with the new secret.
func RotateToken(token astrov1.ApiToken, client astrov1.APIClient) (apitoken.Token, error) { //nolint:gocritic // ApiToken is what FindToken returns
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
	role := orgRoleOf(rotated)
	if role == "" {
		role = orgRoleOf(&token)
	}
	return apitoken.WithSecret(apitoken.FromAPI(rotated, role), rotated), nil
}

// DeleteToken deletes token, an Organization token FindToken found.
func DeleteToken(token astrov1.ApiToken, client astrov1.APIClient) (apitoken.OrganizationRemoval, error) { //nolint:gocritic // ApiToken is what FindToken returns
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return apitoken.OrganizationRemoval{}, err
	}
	resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), ctx.Organization, token.Id)
	if err != nil {
		return apitoken.OrganizationRemoval{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.OrganizationRemoval{}, err
	}
	return apitoken.OrganizationRemoval{
		ID: token.Id, Name: token.Name, Scope: string(token.Scope),
		OrganizationID: ctx.Organization, Action: apitoken.Deleted,
	}, nil
}
