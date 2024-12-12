package deployment

import (
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	workspaceService "github.com/astronomer/astro-cli/cloud/workspace-token"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"golang.org/x/exp/slices"
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

// List all deployment Tokens
func ListTokens(client astrocore.CoreClient, deploymentID string, tokenTypes *[]astrocore.ListDeploymentApiTokensParamsTokenTypes, out io.Writer) error {
	apiTokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
	if err != nil {
		return err
	}

	tab := newTokenTableOut()
	for i := range apiTokens {
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == deploymentID {
				role = apiTokens[i].Roles[j].Role
			}
		}
		created := TimeAgo(apiTokens[i].CreatedAt)
		var createdBy string
		switch {
		case apiTokens[i].CreatedBy.FullName != nil:
			createdBy = *apiTokens[i].CreatedBy.FullName
		case apiTokens[i].CreatedBy.ApiTokenName != nil:
			createdBy = *apiTokens[i].CreatedBy.ApiTokenName
		}
		tab.AddRow([]string{id, name, description, string(scope), role, created, createdBy}, false)
	}
	tab.Print(out)

	return nil
}

// create a deployment token
func CreateToken(name, description, role, deploymentID string, expiration int, cleanOutput bool, out io.Writer, client astrocore.CoreClient) error {
	if name == "" {
		return ErrInvalidTokenName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	CreateDeploymentAPITokenRequest := astrocore.CreateDeploymentApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
	}
	if expiration != 0 {
		CreateDeploymentAPITokenRequest.TokenExpiryPeriodInDays = &expiration
	}
	resp, err := client.CreateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deploymentID, CreateDeploymentAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	APIToken := resp.JSON200
	if cleanOutput {
		fmt.Println(*APIToken.Token)
	} else {
		fmt.Fprintf(out, "\nAstro Deployment API token %s was successfully created\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		fmt.Println("\n" + *APIToken.Token)
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// Update a deployment token
func UpdateToken(id, name, newName, description, role, deploymentID string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{
		"DEPLOYMENT",
	}
	organizationID := ctx.Organization
	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	roles := *token.Roles

	apiTokenID := token.Id

	UpdateDeploymentAPITokenRequest := astrocore.UpdateDeploymentApiTokenJSONRequestBody{}

	if newName == "" {
		UpdateDeploymentAPITokenRequest.Name = token.Name
	} else {
		UpdateDeploymentAPITokenRequest.Name = newName
	}

	if description == "" {
		UpdateDeploymentAPITokenRequest.Description = token.Description
	} else {
		UpdateDeploymentAPITokenRequest.Description = description
	}

	if role == "" {
		for i := range roles {
			if roles[i].EntityType == deploymentEntity && roles[i].EntityId == deploymentID {
				role = roles[i].Role
			}
		}
		UpdateDeploymentAPITokenRequest.Role = role
	} else {
		UpdateDeploymentAPITokenRequest.Role = role
	}

	resp, err := client.UpdateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deploymentID, apiTokenID, UpdateDeploymentAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Deployment API token %s was successfully updated\n", token.Name)
	return nil
}

// rotate a deployment API token
func RotateToken(id, name, deploymentID string, cleanOutput, force bool, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{
		"DEPLOYMENT",
	}
	organizationID := ctx.Organization
	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	apiTokenID := token.Id

	if !force {
		fmt.Println("WARNING: API Token rotation will invalidate the current token and cannot be undone.")
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to rotate the %s API token?", ansi.Bold(token.Name)))

		if !i {
			fmt.Println("Canceling token rotation")
			return nil
		}
	}
	resp, err := client.RotateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deploymentID, apiTokenID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	APIToken := resp.JSON200
	if cleanOutput {
		fmt.Println(*APIToken.Token)
	} else {
		fmt.Fprintf(out, "\nAstro Deployment API token %s was successfully rotated\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		fmt.Println("\n" + *APIToken.Token)
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// delete a deployments api token
func DeleteToken(id, name, deploymentID string, force bool, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	organizationID := ctx.Organization

	tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{
		"DEPLOYMENT",
	}

	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	apiTokenID := token.Id
	if !force {
		fmt.Println("WARNING: API token deletion cannot be undone.")
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name)))

		if !i {
			fmt.Println("Canceling API Token deletion")
			return nil
		}
	}

	resp, err := client.DeleteDeploymentApiTokenWithResponse(httpContext.Background(), organizationID, deploymentID, apiTokenID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Deployment API token %s was successfully deleted\n", token.Name)

	return nil
}

func selectTokens(deploymentID string, apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == deploymentID {
				role = apiTokens[i].Roles[j].Role
			}
		}
		created := TimeAgo(apiTokens[i].CreatedAt)
		var createdBy string
		switch {
		case apiTokens[i].CreatedBy.FullName != nil:
			createdBy = *apiTokens[i].CreatedBy.FullName
		case apiTokens[i].CreatedBy.ApiTokenName != nil:
			createdBy = *apiTokens[i].CreatedBy.ApiTokenName
		}

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			id,
			name,
			description,
			string(scope),
			role,
			created,
			createdBy,
		}, false)
		apiTokensMap[strconv.Itoa(index)] = apiTokens[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")

	selected, ok := apiTokensMap[choice]
	if !ok {
		return astrocore.ApiToken{}, errInvalidDeploymentTokenKey
	}
	return selected, nil
}

// get all deployment tokens
func getDeploymentTokens(deploymentID string, tokenTypes *[]astrocore.ListDeploymentApiTokensParamsTokenTypes, client astrocore.CoreClient) ([]astrocore.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.ApiToken{}, err
	}

	resp, err := client.ListDeploymentApiTokensWithResponse(httpContext.Background(), ctx.Organization, deploymentID, &astrocore.ListDeploymentApiTokensParams{TokenTypes: tokenTypes})
	if err != nil {
		return []astrocore.ApiToken{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrocore.ApiToken{}, err
	}

	APITokens := resp.JSON200.ApiTokens

	return APITokens, nil
}

func getDeploymentToken(id, name, deploymentID, message string, tokens []astrocore.ApiToken) (token astrocore.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(deploymentID, tokens)
		if err != nil {
			return astrocore.ApiToken{}, err
		}
	case name == "" && id != "":
		for i := range tokens {
			if tokens[i].Id == id {
				token = tokens[i]
			}
		}
		if token.Id == "" {
			return astrocore.ApiToken{}, ErrDeploymentTokenNotFound
		}
	case name != "" && id == "":
		var matchedTokens []astrocore.ApiToken
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
				return astrocore.ApiToken{}, err
			}
		}
	}
	if token.Id == "" {
		return astrocore.ApiToken{}, ErrDeploymentTokenNotFound
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

func getTokenByID(id, orgID string, client astroiamcore.CoreClient) (token astroiamcore.ApiToken, err error) {
	resp, err := client.GetApiTokenWithResponse(httpContext.Background(), orgID, id)
	if err != nil {
		return astroiamcore.ApiToken{}, err
	}
	err = astroiamcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroiamcore.ApiToken{}, err
	}
	return *resp.JSON200, nil
}

func GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID string, tokenTypes *[]astrocore.ListDeploymentApiTokensParamsTokenTypes, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) (token astroiamcore.ApiToken, err error) {
	if id == "" {
		tokens, err := getDeploymentTokens(deploymentID, tokenTypes, client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getDeploymentToken(id, name, deploymentID, "\nPlease select the Organization API token you would like to update:", tokens)
		if err != nil {
			return token, err
		}
		token, err = getTokenByID(tokenFromList.Id, organizationID, iamClient)
		if err != nil {
			return token, err
		}
	} else {
		token, err = getTokenByID(id, organizationID, iamClient)
		if err != nil {
			return token, err
		}
	}

	if tokenTypes != nil && len(*tokenTypes) > 0 { // verify the user has passed in an id that matches the operations expected token type
		stringTokenTypes := []string{}
		for _, tokenType := range *tokenTypes {
			stringTokenTypes = append(stringTokenTypes, string(tokenType))
		}
		if !slices.Contains(stringTokenTypes, string(token.Type)) {
			return token, errWrongTokenTypeSelected
		}
	}

	return token, err
}

func RemoveOrgTokenDeploymentRole(id, name, deploymentID string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organizationID := ctx.Organization
	tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{
		"ORGANIZATION",
	}
	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	roles := *token.Roles

	apiTokenID := token.Id
	var orgRole string
	apiTokenWorkspaceRoles := []astrocore.ApiTokenWorkspaceRoleRequest{}
	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{}
	for i := range roles {
		if roles[i].EntityId == deploymentID {
			continue // this removes the role in question
		}

		if roles[i].EntityType == organizationEntity {
			orgRole = roles[i].Role
		}

		if roles[i].EntityType == workspaceEntity {
			apiTokenWorkspaceRoles = append(apiTokenWorkspaceRoles, astrocore.ApiTokenWorkspaceRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}

		if roles[i].EntityType == deploymentEntity {
			apiTokenDeploymentRoles = append(apiTokenDeploymentRoles, astrocore.ApiTokenDeploymentRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}
	}

	updateOrganizationAPITokenRoles := astrocore.UpdateOrganizationApiTokenRolesRequest{
		Organization: orgRole,
		Deployment:   &apiTokenDeploymentRoles,
		Workspace:    &apiTokenWorkspaceRoles,
	}
	updateOrganizationAPITokenRequest := astrocore.UpdateOrganizationApiTokenRequest{
		Name:        token.Name,
		Description: token.Description,
		Roles:       updateOrganizationAPITokenRoles,
	}
	resp, err := client.UpdateOrganizationApiTokenWithResponse(httpContext.Background(), organizationID, apiTokenID, updateOrganizationAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Deployment\n", token.Name)
	return nil
}

func RemoveWorkspaceTokenDeploymentRole(id, name, workspaceID, deploymentID string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	organizationID := ctx.Organization

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{
		"WORKSPACE",
	}
	token, err := GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	roles := *token.Roles

	apiTokenID := token.Id
	var workspaceRole string
	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{}

	for i := range roles {
		if roles[i].EntityId == deploymentID {
			continue // this removes the role in question
		}

		if roles[i].EntityType == workspaceEntity {
			workspaceRole = roles[i].Role
		}

		if roles[i].EntityType == deploymentEntity {
			apiTokenDeploymentRoles = append(apiTokenDeploymentRoles, astrocore.ApiTokenDeploymentRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}
	}
	updateWorkspaceAPITokenRoles := astrocore.UpdateWorkspaceApiTokenRolesRequest{
		Deployment: &apiTokenDeploymentRoles,
		Workspace:  &workspaceRole,
	}
	updateWorkspaceAPITokenRequest := astrocore.UpdateWorkspaceApiTokenRequest{
		Name:        token.Name,
		Description: token.Description,
		Roles:       &updateWorkspaceAPITokenRoles,
	}

	resp, err := client.UpdateWorkspaceApiTokenWithResponse(httpContext.Background(), organizationID, workspaceID, apiTokenID, updateWorkspaceAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully removed from the Deployment\n", token.Name)
	return nil
}

func UpsertWorkspaceTokenDeploymentRole(id, name, role, workspaceID, deploymentID, operation string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	organizationID := ctx.Organization

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	var token astroiamcore.ApiToken
	if operation == "create" {
		tokenTypes := []astrocore.ListWorkspaceApiTokensParamsTokenTypes{"WORKSPACE"}
		token, err = workspaceService.GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client, iamClient)
		if err != nil {
			return err
		}
	} else {
		tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{"WORKSPACE"}
		token, err = GetDeploymentTokenFromInputOrUser(id, name, deploymentID, organizationID, &tokenTypes, client, iamClient)
		if err != nil {
			return err
		}
	}

	apiTokenID := token.Id
	var workspaceRole string
	apiTokenDeploymentRole := astrocore.ApiTokenDeploymentRoleRequest{
		EntityId: deploymentID,
		Role:     role,
	}
	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{apiTokenDeploymentRole}

	for i := range *token.Roles {
		roles := *token.Roles
		if roles[i].EntityId == deploymentID {
			if roles[i].Role == role {
				return errWorkspaceTokenInDeployment
			} else {
				continue
			}
		}

		if roles[i].EntityType == workspaceEntity {
			workspaceRole = roles[i].Role
		}

		if roles[i].EntityType == deploymentEntity {
			apiTokenDeploymentRoles = append(apiTokenDeploymentRoles, astrocore.ApiTokenDeploymentRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}
	}
	updateWorkspaceAPITokenRoles := astrocore.UpdateWorkspaceApiTokenRolesRequest{
		Deployment: &apiTokenDeploymentRoles,
		Workspace:  &workspaceRole,
	}
	updateWorkspaceAPITokenRequest := astrocore.UpdateWorkspaceApiTokenRequest{
		Name:        token.Name,
		Description: token.Description,
		Roles:       &updateWorkspaceAPITokenRoles,
	}

	resp, err := client.UpdateWorkspaceApiTokenWithResponse(httpContext.Background(), organizationID, workspaceID, apiTokenID, updateWorkspaceAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully added/updated to the Deployment\n", token.Name)
	return nil
}

func UpsertOrgTokenDeploymentRole(id, name, role, deploymentID, operation string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	var token astroiamcore.ApiToken
	if operation == "create" {
		token, err = organization.GetTokenFromInputOrUser(id, name, ctx.Organization, client, iamClient)
		if err != nil {
			return err
		}
	} else {
		tokenTypes := []astrocore.ListDeploymentApiTokensParamsTokenTypes{"ORGANIZATION"}
		token, err = GetDeploymentTokenFromInputOrUser(id, name, deploymentID, ctx.Organization, &tokenTypes, client, iamClient)
		if err != nil {
			return err
		}
	}

	apiTokenID := token.Id
	var orgRole string
	apiTokenWorkspaceRoles := []astrocore.ApiTokenWorkspaceRoleRequest{}
	apiTokenDeploymentRole := astrocore.ApiTokenDeploymentRoleRequest{
		EntityId: deploymentID,
		Role:     role,
	}
	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{apiTokenDeploymentRole}
	roles := *token.Roles
	for i := range roles {
		if roles[i].EntityId == deploymentID {
			if roles[i].Role == role {
				return errOrgTokenInDeployment
			} else {
				continue
			}
		}

		if roles[i].EntityType == organizationEntity {
			orgRole = roles[i].Role
		}

		if roles[i].EntityType == workspaceEntity {
			apiTokenWorkspaceRoles = append(apiTokenWorkspaceRoles, astrocore.ApiTokenWorkspaceRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}

		if roles[i].EntityType == deploymentEntity {
			apiTokenDeploymentRoles = append(apiTokenDeploymentRoles, astrocore.ApiTokenDeploymentRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}
	}

	updateOrganizationAPITokenRoles := astrocore.UpdateOrganizationApiTokenRolesRequest{
		Organization: orgRole,
		Deployment:   &apiTokenDeploymentRoles,
		Workspace:    &apiTokenWorkspaceRoles,
	}
	updateOrganizationAPITokenRequest := astrocore.UpdateOrganizationApiTokenRequest{
		Name:        token.Name,
		Description: token.Description,
		Roles:       updateOrganizationAPITokenRoles,
	}

	resp, err := client.UpdateOrganizationApiTokenWithResponse(httpContext.Background(), ctx.Organization, apiTokenID, updateOrganizationAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully added/updated to the Deployment\n", token.Name)
	return nil
}
