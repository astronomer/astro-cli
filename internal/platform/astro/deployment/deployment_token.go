package deployment

// The `astro deployment token` family. Every function here returns what it did
// and leaves the rendering, and every question, to its caller: cmd/astro prints
// the text, publishes the json, and asks a person to pick a token or confirm a
// rotation or a deletion. Nothing in this file prints or reads stdin.

import (
	httpContext "context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/astronomer/astro-cli/context"
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
	errWrongTokenTypeSelected     = errors.New("the token selected is not of the type you are trying to modify")
)

const deploymentEntity = "DEPLOYMENT"

// TokenInfo is an API token as a token command reports it. Role is the role
// the token holds on the object the command is about, which for this family
// is the Deployment it named; the Workspace and Organization token families
// can report theirs in the same shape, reading Role for their own object.
type TokenInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	Role        string `json:"role,omitempty"`
	// CreatedBy is the creator's full name, or the name of the API token that
	// created it.
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy string     `json:"created_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Token is the secret, set only by a create or a rotate: the one time the
	// API returns it, so the one time a caller can keep it.
	Token string `json:"token,omitempty"`
}

// TokenAction is what a delete or a remove did to a token.
type TokenAction string

const (
	// TokenDeleted: the token no longer exists. Only a Deployment-scoped
	// token is deleted.
	TokenDeleted TokenAction = "deleted"
	// TokenRemoved: the token exists, without its role on the Deployment.
	TokenRemoved TokenAction = "removed"
)

// TokenRemoval names the token a delete or a remove acted on, and what it did.
type TokenRemoval struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Scope        string      `json:"scope"`
	DeploymentID string      `json:"deployment_id"`
	Action       TokenAction `json:"action"`
}

// TokenUpdate is the token an update left, and the name it had before, which
// is the name the text confirmation reports.
type TokenUpdate struct {
	Token        TokenInfo
	PreviousName string
}

// TokenPicker asks a person to choose one of tokens and returns its index.
// heading says why they are being asked. It is called only when the command
// was given neither a token ID nor a name, or the name given is shared.
type TokenPicker func(heading string, tokens []TokenInfo) (int, error)

const (
	pickHeading        = "\nPlease select the Deployment API token:"
	pickSharedNameHead = "\nThere are more than one API tokens with name %s. Please select an API token:"
)

// tokenRoles flattens a token's roles pointer to a usable slice.
func tokenRoles(t astrov1.ApiToken) []astrov1.ApiTokenRole { //nolint:gocritic // ApiToken is large; helper returns a slice and isn't hot
	if t.Roles == nil {
		return nil
	}
	return *t.Roles
}

// deploymentRoleOf returns the token's role on deploymentID, or "".
func deploymentRoleOf(t astrov1.ApiToken, deploymentID string) string { //nolint:gocritic // ApiToken is large; helper returns a short string
	const entityType = astrov1.ApiTokenRoleEntityTypeDEPLOYMENT
	for _, r := range tokenRoles(t) {
		if r.EntityType == entityType && r.EntityId == deploymentID {
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

// tokenInfo reports t with role as its role, and no secret.
func tokenInfo(t *astrov1.ApiToken, role string) TokenInfo {
	info := TokenInfo{
		ID:          t.Id,
		Name:        t.Name,
		Description: t.Description,
		Scope:       string(t.Scope),
		Role:        role,
		CreatedAt:   t.CreatedAt,
		ExpiresAt:   t.EndAt,
	}
	if t.CreatedBy != nil {
		switch {
		case t.CreatedBy.FullName != nil:
			info.CreatedBy = *t.CreatedBy.FullName
		case t.CreatedBy.ApiTokenName != nil:
			info.CreatedBy = *t.CreatedBy.ApiTokenName
		}
	}
	return info
}

// tokenInfos reports tokens with their roles on deploymentID, never nil.
func tokenInfos(tokens []astrov1.ApiToken, deploymentID string) []TokenInfo {
	infos := make([]TokenInfo, 0, len(tokens))
	for i := range tokens {
		infos = append(infos, tokenInfo(&tokens[i], deploymentRoleOf(tokens[i], deploymentID)))
	}
	return infos
}

// withSecret adds the secret the API returned with t, if it returned one.
func withSecret(info TokenInfo, t *astrov1.ApiToken) TokenInfo { //nolint:gocritic // a value in, a value out
	if t.Token != nil {
		info.Token = *t.Token
	}
	return info
}

// ListTokens lists tokens with a role in the given deployment. tokenTypes, when
// not empty, keeps only those scopes. The list is empty, never nil, when there
// are none.
func ListTokens(client astrov1.APIClient, deploymentID string, tokenTypes []DeploymentTokenType) ([]TokenInfo, error) {
	apiTokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
	if err != nil {
		return nil, err
	}
	return tokenInfos(apiTokens, deploymentID), nil
}

// CreateToken creates a Deployment-scoped API token and returns it with its
// secret.
func CreateToken(name, description, role, deploymentID string, expiration int, client astrov1.APIClient) (TokenInfo, error) {
	if name == "" {
		return TokenInfo{}, ErrInvalidTokenName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenInfo{}, err
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
		return TokenInfo{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return TokenInfo{}, err
	}
	created := resp.JSON200
	return withSecret(tokenInfo(created, role), created), nil
}

// UpdateToken updates a Deployment-scoped API token's name/description and optionally its deployment role.
func UpdateToken(id, name, newName, description, role, deploymentID string, pick TokenPicker, client astrov1.APIClient) (TokenUpdate, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenUpdate{}, err
	}
	organizationID := ctx.Organization
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeDEPLOYMENT}

	token, err := FindToken(id, name, deploymentID, tokenTypes, pick, client)
	if err != nil {
		return TokenUpdate{}, err
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
		return TokenUpdate{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return TokenUpdate{}, err
	}
	// The token as the update left it: the API's answer, or what was asked
	// for when it gave none.
	updated := token
	if resp.JSON200 != nil {
		updated = *resp.JSON200
	} else {
		updated.Name, updated.Description = updateReq.Name, *updateReq.Description
	}

	newRole := role
	if newRole == "" {
		newRole = deploymentRoleOf(token, deploymentID)
	}
	if newRole != "" {
		// Short-circuit: requested role is already set.
		if role != "" && deploymentRoleOf(token, deploymentID) == role {
			return TokenUpdate{}, errWorkspaceTokenInDeployment
		}
		if err := setDeploymentRole(token, deploymentID, newRole, organizationID, client); err != nil {
			return TokenUpdate{}, err
		}
	}
	return TokenUpdate{Token: tokenInfo(&updated, newRole), PreviousName: token.Name}, nil
}

// RotateToken rotates the secret of token, which FindToken found, and returns
// it with the new secret.
func RotateToken(token astrov1.ApiToken, deploymentID string, client astrov1.APIClient) (TokenInfo, error) { //nolint:gocritic // ApiToken is what FindToken returns
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenInfo{}, err
	}
	resp, err := client.RotateApiTokenWithResponse(httpContext.Background(), ctx.Organization, token.Id)
	if err != nil {
		return TokenInfo{}, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return TokenInfo{}, err
	}
	rotated := resp.JSON200
	role := deploymentRoleOf(*rotated, deploymentID)
	if role == "" {
		role = deploymentRoleOf(token, deploymentID)
	}
	return withSecret(tokenInfo(rotated, role), rotated), nil
}

// DeleteToken deletes token when it is Deployment-scoped, and otherwise
// removes its role on the Deployment, leaving the token itself.
func DeleteToken(token astrov1.ApiToken, deploymentID string, client astrov1.APIClient) (TokenRemoval, error) { //nolint:gocritic // ApiToken is what FindToken returns
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenRemoval{}, err
	}
	organizationID := ctx.Organization
	removal := TokenRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), DeploymentID: deploymentID}

	if string(token.Scope) == deploymentEntity {
		resp, err := client.DeleteApiTokenWithResponse(httpContext.Background(), organizationID, token.Id)
		if err != nil {
			return TokenRemoval{}, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return TokenRemoval{}, err
		}
		removal.Action = TokenDeleted
		return removal, nil
	}
	if err := setDeploymentRole(token, deploymentID, "", organizationID, client); err != nil {
		return TokenRemoval{}, err
	}
	removal.Action = TokenRemoved
	return removal, nil
}

// setDeploymentRole gives token role on deploymentID, or takes its role there
// away when role is "".
func setDeploymentRole(token astrov1.ApiToken, deploymentID, role, organizationID string, client astrov1.APIClient) error { //nolint:gocritic // ApiToken is large; called once per command
	newRoles := upsertDeploymentRole(tokenRoles(token), deploymentID, role)
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

// pickToken has pick choose among tokens.
func pickToken(pick TokenPicker, heading, deploymentID string, tokens []astrov1.ApiToken) (astrov1.ApiToken, error) {
	i, err := pick(heading, tokenInfos(tokens, deploymentID))
	if err != nil {
		return astrov1.ApiToken{}, err
	}
	if i < 0 || i >= len(tokens) {
		return astrov1.ApiToken{}, fmt.Errorf("token picker returned %d of %d tokens", i, len(tokens))
	}
	return tokens[i], nil
}

func getDeploymentToken(id, name, deploymentID string, tokens []astrov1.ApiToken, pick TokenPicker) (token astrov1.ApiToken, err error) {
	switch {
	case id == "" && name == "":
		token, err = pickToken(pick, pickHeading, deploymentID, tokens)
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
			token, err = pickToken(pick, fmt.Sprintf(pickSharedNameHead, name), deploymentID, matchedTokens)
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
	days := int(duration.Hours() / 24) //nolint:mnd // the value is clear from context
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

// FindToken finds the token a command means: the one with id, else the one
// named name among the tokens with a role on the Deployment, else the one a
// person picks through pick. tokenTypes, when not empty, are the scopes the
// command may act on.
func FindToken(id, name, deploymentID string, tokenTypes []DeploymentTokenType, pick TokenPicker, client astrov1.APIClient) (token astrov1.ApiToken, err error) {
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
func RemoveOrgTokenDeploymentRole(id, name, deploymentID string, pick TokenPicker, client astrov1.APIClient) (TokenRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenRemoval{}, err
	}
	tokenTypes := []DeploymentTokenType{DeploymentTokenTypeORGANIZATION}
	token, err := FindToken(id, name, deploymentID, tokenTypes, pick, client)
	if err != nil {
		return TokenRemoval{}, err
	}
	if err := setDeploymentRole(token, deploymentID, "", ctx.Organization, client); err != nil {
		return TokenRemoval{}, err
	}
	return TokenRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), DeploymentID: deploymentID, Action: TokenRemoved}, nil
}

// RemoveWorkspaceTokenDeploymentRole removes the deployment-scope role from a
// Workspace token. A token named by neither id nor name is picked from the
// Workspace's tokens, by the workspace-token package's own picker.
func RemoveWorkspaceTokenDeploymentRole(id, name, workspaceID, deploymentID string, client astrov1.APIClient) (TokenRemoval, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenRemoval{}, err
	}
	organizationID := ctx.Organization
	wsTypes := []workspaceService.TokenType{workspaceService.TokenTypeWORKSPACE}
	token, err := workspaceService.GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &wsTypes, client)
	if err != nil {
		return TokenRemoval{}, err
	}
	if err := setDeploymentRole(token, deploymentID, "", organizationID, client); err != nil {
		return TokenRemoval{}, err
	}
	return TokenRemoval{ID: token.Id, Name: token.Name, Scope: string(token.Scope), DeploymentID: deploymentID, Action: TokenRemoved}, nil
}

// UpsertWorkspaceTokenDeploymentRole adds/updates a deployment-scope role on a
// Workspace token, and returns the token with that role. An add ("create")
// finds the token among the Workspace's, through the workspace-token package's
// own picker; an update finds it among the Deployment's, through pick.
func UpsertWorkspaceTokenDeploymentRole(id, name, role, workspaceID, deploymentID, operation string, pick TokenPicker, client astrov1.APIClient) (TokenInfo, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenInfo{}, err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		wsTypes := []workspaceService.TokenType{workspaceService.TokenTypeWORKSPACE}
		token, err = workspaceService.GetTokenFromInputOrUser(id, name, workspaceID, ctx.Organization, &wsTypes, client)
	} else {
		token, err = FindToken(id, name, deploymentID, []DeploymentTokenType{DeploymentTokenTypeWORKSPACE}, pick, client)
	}
	if err != nil {
		return TokenInfo{}, err
	}
	// Short-circuit: already has this role on the deployment.
	if deploymentRoleOf(token, deploymentID) == role {
		return TokenInfo{}, errWorkspaceTokenInDeployment
	}
	if err := setDeploymentRole(token, deploymentID, role, ctx.Organization, client); err != nil {
		return TokenInfo{}, err
	}
	return tokenInfo(&token, role), nil
}

// UpsertOrgTokenDeploymentRole adds/updates a deployment-scope role on an
// Organization token, and returns the token with that role. An add ("create")
// finds the token among the Organization's, through the organization package's
// own picker; an update finds it among the Deployment's, through pick.
func UpsertOrgTokenDeploymentRole(id, name, role, deploymentID, operation string, pick TokenPicker, client astrov1.APIClient) (TokenInfo, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return TokenInfo{}, err
	}
	var token astrov1.ApiToken
	if operation == "create" {
		token, err = organization.GetTokenFromInputOrUser(id, name, ctx.Organization, client)
	} else {
		token, err = FindToken(id, name, deploymentID, []DeploymentTokenType{DeploymentTokenTypeORGANIZATION}, pick, client)
	}
	if err != nil {
		return TokenInfo{}, err
	}
	// Short-circuit: already has this role on the deployment.
	if deploymentRoleOf(token, deploymentID) == role {
		return TokenInfo{}, errOrgTokenInDeployment
	}
	if err := setDeploymentRole(token, deploymentID, role, ctx.Organization, client); err != nil {
		return TokenInfo{}, err
	}
	return tokenInfo(&token, role), nil
}
