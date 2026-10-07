package workspacetoken

// The `astro workspace token` family. Every function here returns what it did
// and leaves the rendering, and every question, to its caller: cmd/astro prints
// the text, publishes the json, and asks a person to pick a token or confirm a
// rotation or a deletion. Nothing in this file prints. What it returns is the
// shape all three token families share, in apitoken.

import (
	httpContext "context"
	"errors"
	"fmt"
	"slices"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/internal/platform/astro/user"
	workspaceService "github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/input"
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

var (
	errWorkspaceTokenRoleSet  = errors.New("this Workspace API token already has that role on the Workspace")
	ErrWorkspaceTokenNotFound = errors.New("no Workspace API token was found for the API token name you provided")
	errOrgTokenInWorkspace    = errors.New("this Organization API token has already been added to the Workspace with that role")
	errWrongTokenTypeSelected = errors.New("the token selected is not of the type you are trying to modify")
)

const (
	workspaceEntity = "WORKSPACE"

	// The plain choice has no heading here: the picker asks it in words
	// that suit the command, which this package does not know.
	pickSharedNameHead = "\nThere are more than one API tokens with name %s. Please select an API token:"
)

// workspaceRoleOf returns the token's role on workspaceID, or "".
func workspaceRoleOf(t *astrov1.ApiToken, workspaceID string) string {
	return apitoken.RoleOn(t, astrov1.ApiTokenRoleEntityTypeWORKSPACE, workspaceID)
}

// workspaceRoleReader reads each token's role on workspaceID.
func workspaceRoleReader(workspaceID string) func(*astrov1.ApiToken) string {
	return func(t *astrov1.ApiToken) string { return workspaceRoleOf(t, workspaceID) }
}

// Target resolves the Workspace a command means, "" being the current one, and
// the Organization it is in.
func Target(workspaceID string) (wsID, organizationID string, err error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return "", "", err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	return workspaceID, ctx.Organization, nil
}

// ListTokens lists tokens with a role in the Workspace ("" is the current
// one), each with its role there. tokenTypes, when not empty, keeps only those
// scopes. The list is empty, never nil, when there are none.
func ListTokens(client astrov1.APIClient, workspaceID string, tokenTypes []TokenType) ([]apitoken.Token, error) {
	workspaceID, _, err := Target(workspaceID)
	if err != nil {
		return nil, err
	}
	apiTokens, err := getWorkspaceTokens(workspaceID, tokenTypes, client)
	if err != nil {
		return nil, err
	}
	return apitoken.AllFromAPI(apiTokens, workspaceRoleReader(workspaceID)), nil
}

// CreateToken creates a Workspace-scoped API token and returns it with its
// secret.
func CreateToken(name, description, role, workspaceID string, expiration int, client astrov1.APIClient) (apitoken.Token, error) {
	if err := user.IsWorkspaceRoleValid(role); err != nil {
		return apitoken.Token{}, err
	}
	if name == "" {
		return apitoken.Token{}, workspaceService.ErrInvalidTokenName
	}
	workspaceID, organizationID, err := Target(workspaceID)
	if err != nil {
		return apitoken.Token{}, err
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
	resp, err := client.CreateApiTokenWithResponse(httpContext.Background(), organizationID, req)
	if err != nil {
		return apitoken.Token{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Token{}, err
	}
	created := resp.JSON200
	return apitoken.WithSecret(apitoken.FromAPI(created, role), created), nil
}

// UpdateToken updates a Workspace-scoped API token's name and description,
// and its role on the Workspace when role is not "". An empty newName or
// description keeps the token's own; an empty role leaves the role alone, and
// no role change is sent.
//
// A role that is not a Workspace role is refused before anything is asked or
// sent, and a role the token already holds on the Workspace before anything
// is sent. The role is changed before the name and description, so a role
// the API refuses leaves the token as it was; the name and description are
// sent only when one is given.
func UpdateToken(id, name, newName, description, role, workspaceID string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Update, error) {
	if role != "" {
		if err := user.IsWorkspaceRoleValid(role); err != nil {
			return apitoken.Update{}, err
		}
	}
	workspaceID, organizationID, err := Target(workspaceID)
	if err != nil {
		return apitoken.Update{}, err
	}
	tokenTypes := []TokenType{TokenTypeWORKSPACE}

	token, err := FindToken(id, name, workspaceID, organizationID, tokenTypes, pick, client)
	if err != nil {
		return apitoken.Update{}, err
	}

	currentRole := workspaceRoleOf(&token, workspaceID)
	if role != "" && role == currentRole {
		return apitoken.Update{}, errWorkspaceTokenRoleSet
	}
	if role != "" {
		if err := setWorkspaceRole(&token, workspaceID, role, organizationID, client); err != nil {
			return apitoken.Update{}, err
		}
	}

	newRole := currentRole
	if role != "" {
		newRole = role
	}
	// The name and description are sent only when one was given, so a
	// role-only update is one call. A failure after the role went through
	// says that it did, with the token as it now is.
	updated := token
	if newName != "" || description != "" {
		updated, err = updateNameAndDescription(&token, newName, description, organizationID, client)
		if err != nil {
			if role != "" {
				return apitoken.Update{Token: apitoken.FromAPI(&token, newRole), PreviousName: token.Name}, apitoken.RenameFailedAfterRole(role, err)
			}
			return apitoken.Update{}, err
		}
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
// it with the new secret and its role on workspaceID.
func RotateToken(token astrov1.ApiToken, workspaceID string, client astrov1.APIClient) (apitoken.Token, error) { //nolint:gocritic // ApiToken is what FindToken returns
	workspaceID, organizationID, err := Target(workspaceID)
	if err != nil {
		return apitoken.Token{}, err
	}
	resp, err := client.RotateApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
	if err != nil {
		return apitoken.Token{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return apitoken.Token{}, err
	}
	rotated := resp.JSON200
	role := workspaceRoleOf(rotated, workspaceID)
	if role == "" {
		role = workspaceRoleOf(&token, workspaceID)
	}
	return apitoken.WithSecret(apitoken.FromAPI(rotated, role), rotated), nil
}

// DeleteToken deletes token when it is Workspace-scoped, and otherwise (an
// Organization token with a role on the Workspace) removes its Workspace role,
// leaving the token itself.
func DeleteToken(token astrov1.ApiToken, workspaceID string, client astrov1.APIClient) (apitoken.WorkspaceRemoval, error) { //nolint:gocritic // ApiToken is what FindToken returns
	workspaceID, organizationID, err := Target(workspaceID)
	if err != nil {
		return apitoken.WorkspaceRemoval{}, err
	}
	removal := apitoken.WorkspaceRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), WorkspaceID: workspaceID}
	if string(token.Scope) == workspaceEntity {
		resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
		if err != nil {
			return apitoken.WorkspaceRemoval{}, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return apitoken.WorkspaceRemoval{}, err
		}
		removal.Action = apitoken.Deleted
		return removal, nil
	}
	// Detach by removing the workspace role.
	if err := setWorkspaceRole(&token, workspaceID, "", organizationID, client); err != nil {
		return apitoken.WorkspaceRemoval{}, err
	}
	removal.Action = apitoken.Removed
	return removal, nil
}

// setWorkspaceRole gives token role on workspaceID, or takes its role there
// away when role is "".
func setWorkspaceRole(token *astrov1.ApiToken, workspaceID, role, organizationID string, client astrov1.APIClient) error {
	newRoles := apitoken.WithRole(apitoken.Roles(token), astrov1.ApiTokenRoleEntityTypeWORKSPACE, workspaceID, role)
	resp, err := client.UpdateApiTokenRolesWithResponse(httpContext.Background(), organizationID, token.Id, astrov1.UpdateApiTokenRolesRequest{Roles: newRoles})
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// getWorkspaceTokens lists tokens with a role in the given workspace, then filters client-side
// by token scope if tokenTypes is not empty.
func getWorkspaceTokens(workspaceID string, tokenTypes []TokenType, client astrov1.APIClient) ([]astrov1.ApiToken, error) {
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

// getWorkspaceToken picks the token id or name means from tokens, offering
// each with its role on workspaceID when a person has to choose.
func getWorkspaceToken(id, name, workspaceID string, tokens []astrov1.ApiToken, pick apitoken.Picker) (token astrov1.ApiToken, err error) {
	roleOf := workspaceRoleReader(workspaceID)
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
			// Refused before the picker, so a run that may not ask (-o json)
			// says what would answer it: the name answers nothing here.
			if err := input.MayAsk(fmt.Sprintf("Several API tokens are named %s; which one?", name),
				input.About("an API token"), input.AnsweredBy("the token's ID instead of its name")); err != nil {
				return astrov1.ApiToken{}, err
			}
			token, err = apitoken.Pick(pick, fmt.Sprintf(pickSharedNameHead, name), matchedTokens, roleOf)
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
// named name among the tokens with a role on the Workspace, else the one a
// person picks through pick. tokenTypes, when not empty, are the scopes the
// command may act on. The Workspace is the current one when workspaceID is
// "", and the tokens offered are shown with their role on it.
func FindToken(id, name, workspaceID, organizationID string, tokenTypes []TokenType, pick apitoken.Picker, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
	if id == "" {
		// Resolved here, not only in the list, so the roles offered are the
		// ones on the Workspace listed.
		workspaceID, _, err = Target(workspaceID)
		if err != nil {
			return token, err
		}
		tokens, err := getWorkspaceTokens(workspaceID, tokenTypes, client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getWorkspaceToken(id, name, workspaceID, tokens, pick)
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

// RemoveOrgTokenWorkspaceRole removes the workspace-scope role from an
// Organization token, found among the Workspace's through pick.
func RemoveOrgTokenWorkspaceRole(id, name, workspaceID string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.WorkspaceRemoval, error) {
	workspaceID, organizationID, err := Target(workspaceID)
	if err != nil {
		return apitoken.WorkspaceRemoval{}, err
	}
	tokenTypes := []TokenType{TokenTypeORGANIZATION}
	token, err := FindToken(id, name, workspaceID, organizationID, tokenTypes, pick, client)
	if err != nil {
		return apitoken.WorkspaceRemoval{}, err
	}
	if err := setWorkspaceRole(&token, workspaceID, "", organizationID, client); err != nil {
		return apitoken.WorkspaceRemoval{}, err
	}
	return apitoken.WorkspaceRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), WorkspaceID: workspaceID, Action: apitoken.Removed}, nil
}

// UpsertOrgTokenWorkspaceRole adds or updates a workspace-scope role on an
// Organization token, and returns the token with that role. An add ("create")
// finds the token among the Organization's, an update among the Workspace's
// Organization tokens; either asks through pick, which the caller chooses to
// suit.
func UpsertOrgTokenWorkspaceRole(id, name, role, workspaceID, operation string, pick apitoken.Picker, client astrov1.APIClient) (apitoken.Token, error) {
	workspaceID, organizationID, err := Target(workspaceID)
	if err != nil {
		return apitoken.Token{}, err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		token, err = organization.FindToken(id, name, organizationID, pick, client)
	} else {
		tokenTypes := []TokenType{TokenTypeORGANIZATION}
		token, err = FindToken(id, name, workspaceID, organizationID, tokenTypes, pick, client)
	}
	if err != nil {
		return apitoken.Token{}, err
	}

	// Short-circuit: already has this role on the workspace.
	if workspaceRoleOf(&token, workspaceID) == role {
		return apitoken.Token{}, errOrgTokenInWorkspace
	}
	if err := setWorkspaceRole(&token, workspaceID, role, organizationID, client); err != nil {
		return apitoken.Token{}, err
	}
	return apitoken.FromAPI(&token, role), nil
}
