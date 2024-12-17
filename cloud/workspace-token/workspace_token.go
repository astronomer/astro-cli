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

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroiamcore "github.com/astronomer/astro-cli/astro-client-iam-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/user"
	workspaceService "github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
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

// List all workspace Tokens
func ListTokens(client astrocore.CoreClient, workspaceID string, tokenTypes *[]astrocore.ListWorkspaceApiTokensParamsTokenTypes, out io.Writer) error {
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
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == workspaceID {
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

// create a workspace token
func CreateToken(name, description, role, workspaceID string, expiration int, cleanOutput bool, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
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
	CreateWorkspaceAPITokenRequest := astrocore.CreateWorkspaceApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
	}
	if expiration != 0 {
		CreateWorkspaceAPITokenRequest.TokenExpiryPeriodInDays = &expiration
	}
	resp, err := client.CreateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.Organization, workspaceID, CreateWorkspaceAPITokenRequest)
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
		fmt.Fprintf(out, "\nAstro Workspace API token %s was successfully created\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		fmt.Println("\n" + *APIToken.Token)
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// Update a workspace token
func UpdateToken(id, name, newName, description, role, workspaceID string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	organizationID := ctx.Organization

	tokenTypes := []astrocore.ListWorkspaceApiTokensParamsTokenTypes{"WORKSPACE"}

	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{}
	roles := *token.Roles
	for i := range roles {
		if roles[i].EntityId == workspaceID {
			if roles[i].Role == role {
				return errWorkspaceTokenInDeployment
			} else {
				continue
			}
		}

		if roles[i].EntityType == deploymentEntity {
			apiTokenDeploymentRoles = append(apiTokenDeploymentRoles, astrocore.ApiTokenDeploymentRoleRequest{
				EntityId: roles[i].EntityId,
				Role:     roles[i].Role,
			})
		}
	}

	apiTokenID := token.Id

	UpdateWorkspaceAPITokenRequest := astrocore.UpdateWorkspaceApiTokenJSONRequestBody{
		Roles: &astrocore.UpdateWorkspaceApiTokenRolesRequest{
			Deployment: &apiTokenDeploymentRoles,
		},
	}

	if newName == "" {
		UpdateWorkspaceAPITokenRequest.Name = token.Name
	} else {
		UpdateWorkspaceAPITokenRequest.Name = newName
	}

	if description == "" {
		UpdateWorkspaceAPITokenRequest.Description = token.Description
	} else {
		UpdateWorkspaceAPITokenRequest.Description = description
	}

	if role == "" {
		for i := range roles {
			if roles[i].EntityType == workspaceEntity && roles[i].EntityId == workspaceID {
				role = roles[i].Role
			}
		}
		err := user.IsWorkspaceRoleValid(role)
		if err != nil {
			return err
		}
		UpdateWorkspaceAPITokenRequest.Roles.Workspace = &role
	} else {
		err := user.IsWorkspaceRoleValid(role)
		if err != nil {
			return err
		}
		UpdateWorkspaceAPITokenRequest.Roles.Workspace = &role
	}

	resp, err := client.UpdateWorkspaceApiTokenWithResponse(httpContext.Background(), organizationID, workspaceID, apiTokenID, UpdateWorkspaceAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully updated\n", token.Name)
	return nil
}

// rotate a workspace API token
func RotateToken(id, name, workspaceID string, cleanOutput, force bool, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	organizationID := ctx.Organization
	tokenTypes := []astrocore.ListWorkspaceApiTokensParamsTokenTypes{"WORKSPACE"}

	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client, iamClient)
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
	resp, err := client.RotateWorkspaceApiTokenWithResponse(httpContext.Background(), organizationID, workspaceID, apiTokenID)
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
		fmt.Fprintf(out, "\nAstro Workspace API token %s was successfully rotated\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		fmt.Println("\n" + *APIToken.Token)
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// delete a workspaces api token
func DeleteToken(id, name, workspaceID string, force bool, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	organizationID := ctx.Organization
	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, nil, client, iamClient)
	if err != nil {
		return err
	}
	apiTokenID := token.Id
	if string(token.Type) == workspaceEntity {
		if !force {
			fmt.Println("WARNING: API token deletion cannot be undone.")
			i, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to delete the %s API token?", ansi.Bold(token.Name)))

			if !i {
				fmt.Println("Canceling API Token deletion")
				return nil
			}
		}
	} else {
		if !force {
			i, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Workspace?", ansi.Bold(token.Name)))

			if !i {
				fmt.Println("Canceling API Token removal")
				return nil
			}
		}
	}

	resp, err := client.DeleteWorkspaceApiTokenWithResponse(httpContext.Background(), organizationID, workspaceID, apiTokenID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	if string(token.Type) == workspaceEntity {
		fmt.Fprintf(out, "Astro Workspace API token %s was successfully deleted\n", token.Name)
	} else {
		fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Workspace\n", token.Name)
	}
	return nil
}

func selectTokens(workspaceID string, apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == workspaceID {
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
		return astrocore.ApiToken{}, errInvalidWorkspaceTokenKey
	}
	return selected, nil
}

// get all workspace tokens
func getWorkspaceTokens(workspaceID string, tokenTypes *[]astrocore.ListWorkspaceApiTokensParamsTokenTypes, client astrocore.CoreClient) ([]astrocore.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.ApiToken{}, err
	}
	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	resp, err := client.ListWorkspaceApiTokensWithResponse(httpContext.Background(), ctx.Organization, workspaceID, &astrocore.ListWorkspaceApiTokensParams{TokenTypes: tokenTypes})
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

func getWorkspaceToken(id, name, workspaceID, message string, tokens []astrocore.ApiToken) (token astrocore.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(workspaceID, tokens)
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
			return astrocore.ApiToken{}, ErrWorkspaceTokenNotFound
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
			token, err = selectTokens(workspaceID, matchedTokens)
			if err != nil {
				return astrocore.ApiToken{}, err
			}
		}
	}
	if token.Id == "" {
		return astrocore.ApiToken{}, ErrWorkspaceTokenNotFound
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

func GetTokenFromInputOrUser(id, name, workspaceID, organizationID string, tokenTypes *[]astrocore.ListWorkspaceApiTokensParamsTokenTypes, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) (token astroiamcore.ApiToken, err error) {
	if id == "" {
		tokens, err := getWorkspaceTokens(workspaceID, tokenTypes, client)
		if err != nil {
			return token, err
		}
		tokenFromList, err := getWorkspaceToken(id, name, workspaceID, "\nPlease select the Workspace API token you would like to add to the Deployment:", tokens)
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

func RemoveOrgTokenWorkspaceRole(id, name, workspaceID string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}

	organizationID := ctx.Organization
	tokenTypes := []astrocore.ListWorkspaceApiTokensParamsTokenTypes{
		"ORGANIZATION",
	}
	token, err := GetTokenFromInputOrUser(id, name, workspaceID, organizationID, &tokenTypes, client, iamClient)
	if err != nil {
		return err
	}
	roles := *token.Roles

	apiTokenID := token.Id
	var orgRole string
	apiTokenWorkspaceRoles := []astrocore.ApiTokenWorkspaceRoleRequest{}
	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{}
	for i := range roles {
		if roles[i].EntityId == workspaceID {
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
	fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Workspace\n", token.Name)
	return nil
}

func UpsertOrgTokenWorkspaceRole(id, name, role, workspaceID, operation string, out io.Writer, client astrocore.CoreClient, iamClient astroiamcore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if workspaceID == "" {
		workspaceID = ctx.Workspace
	}
	var token astroiamcore.ApiToken
	if operation == "create" {
		token, err = organization.GetTokenFromInputOrUser(id, name, ctx.Organization, client, iamClient)
		if err != nil {
			return err
		}
	} else {
		tokenTypes := []astrocore.ListWorkspaceApiTokensParamsTokenTypes{"ORGANIZATION"}
		token, err = GetTokenFromInputOrUser(id, name, workspaceID, ctx.Organization, &tokenTypes, client, iamClient)
		if err != nil {
			return err
		}
	}

	apiTokenID := token.Id
	var orgRole string
	apiTokenWorkspaceRole := astrocore.ApiTokenWorkspaceRoleRequest{
		EntityId: workspaceID,
		Role:     role,
	}
	apiTokenWorkspaceRoles := []astrocore.ApiTokenWorkspaceRoleRequest{apiTokenWorkspaceRole}

	apiTokenDeploymentRoles := []astrocore.ApiTokenDeploymentRoleRequest{}
	roles := *token.Roles
	for i := range roles {
		if roles[i].EntityId == workspaceID {
			if roles[i].Role == role {
				return errOrgTokenInWorkspace
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
	fmt.Fprintf(out, "Astro Organization API token %s was successfully added/updated to the Workspace\n", token.Name)
	return nil
}
