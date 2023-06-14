package organization

import (
	httpContext "context"
	"fmt"
	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"io"
	"os"
	"strconv"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/pkg/errors"
)

var (
	ErrInvalidName                 = errors.New("no name provided for the organization token. Retry with a valid name")
	errInvalidOrganizationTokenKey = errors.New("invalid Organization API token selection")
	errOrganizationTokenNotFound   = errors.New("organization token specified was not found")
	errOrgTokenInWorkspace         = errors.New("this Organization API token has already been added to the Workspace")
	errBothNameAndID               = errors.New("both an API token name and id were specified. Specify either the name or the id not both")
)

const (
	workspaceEntity    = "WORKSPACE"
	organizationEntity = "ORGANIZATION"
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

func AddOrgTokenToWorkspace(id, name, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}
	var token astrocore.ApiToken
	if id == "" {
		tokens, err := getOrganizationTokens(client)
		if err != nil {
			return err
		}
		token, err = getOrganizationToken(id, name, "\nPlease select the Organization API token you would like to add to the Workspace:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getOrganizationTokenById(id, ctx.OrganizationShortName, client)
		if err != nil {
			return err
		}
	}

	apiTokenID := token.Id

	var orgRole string
	for i := range token.Roles {
		if token.Roles[i].EntityId == workspaceEntity {
			return errOrgTokenInWorkspace
		}

		if token.Roles[i].EntityId == ctx.Organization {
			orgRole = token.Roles[i].Role
		}
	}

	apiTokenWorkspaceRole := astrocore.ApiTokenWorkspaceRole{
		EntityId: workspace,
		Role:     role,
	}
	apiTokenWorkspaceRoles := []astrocore.ApiTokenWorkspaceRole{apiTokenWorkspaceRole}

	updateOrganizationAPITokenRoles := astrocore.UpdateOrganizationApiTokenRoles{
		Organization: orgRole,
		Workspace:    &apiTokenWorkspaceRoles,
	}
	updateOrganizationAPITokenRequest := astrocore.UpdateOrganizationApiTokenRequest{
		Name:        token.Name,
		Description: token.Description,
		Roles:       updateOrganizationAPITokenRoles,
	}

	resp, err := client.UpdateOrganizationApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, apiTokenID, updateOrganizationAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully added to the Workspace\n", token.Name)
	return nil
}

func selectTokens(apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		var orgRole string

		for _, role := range apiTokens[i].Roles {
			if role.EntityType == organizationEntity {
				orgRole = role.Role
			}
		}
		expires := apiTokens[i].ExpiryPeriodInDays

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			name,
			description,
			orgRole,
			fmt.Sprint(expires),
		}, false)
		apiTokensMap[strconv.Itoa(index)] = apiTokens[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := apiTokensMap[choice]
	if !ok {
		return astrocore.ApiToken{}, errInvalidOrganizationTokenKey
	}
	return selected, nil
}

// get all organization tokens
func getOrganizationTokens(client astrocore.CoreClient) ([]astrocore.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.ApiToken{}, err
	}
	if ctx.OrganizationShortName == "" {
		return []astrocore.ApiToken{}, user.ErrNoShortName
	}

	resp, err := client.ListOrganizationApiTokensWithResponse(httpContext.Background(), ctx.OrganizationShortName, &astrocore.ListOrganizationApiTokensParams{})
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

func getOrganizationTokenById(id string, orgShortName string, client astrocore.CoreClient) (token astrocore.ApiToken, err error) {
	resp, err := client.GetOrganizationApiTokenWithResponse(httpContext.Background(), orgShortName, id)
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrocore.ApiToken{}, err
	}
	return *resp.JSON200, nil
}

func getOrganizationToken(id, name, message string, tokens []astrocore.ApiToken) (token astrocore.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(tokens)
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
			return astrocore.ApiToken{}, errOrganizationTokenNotFound
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
			token, err = selectTokens(matchedTokens)
			if err != nil {
				return astrocore.ApiToken{}, err
			}
		}
	case name != "" && id != "":
		return astrocore.ApiToken{}, errBothNameAndID
	}
	if token.Id == "" {
		return astrocore.ApiToken{}, errOrganizationTokenNotFound
	}
	return token, nil
}

// List all organization Tokens
func ListTokens(client astrocore.CoreClient, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	organization := ctx.Organization

	apiTokens, err := getOrganizationTokens(client)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	tab := newTokenTableOut()
	for i := range apiTokens {
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == organization {
				role = apiTokens[i].Roles[j].Role
			}
		}
		created := TimeAgo(apiTokens[i].CreatedAt)
		createdBy := apiTokens[i].CreatedBy.FullName
		tab.AddRow([]string{id, name, description, string(scope), role, created, *createdBy}, false)
	}
	tab.Print(out)

	return nil
}

// List all roles for a given organization Token
func ListTokenRoles(id string, client astrocore.CoreClient, out io.Writer) (err error) {
	var apiToken astrocore.ApiToken
	if id == "" {
		tokens, err := getOrganizationTokens(client)
		if err != nil {
			return err
		}
		apiToken, err = getOrganizationToken(id, "", "\nPlease select the Organization API token you would like to update:", tokens)
		if err != nil {
			return err
		}
	} else {
		ctx, err := context.GetCurrentContext()
		if err != nil {
			return err
		}
		apiToken, err = getOrganizationTokenById(id, ctx.OrganizationShortName, client)
		if err != nil {
			return err
		}
	}

	tab := newTokenRolesTableOut()
	for _, tokenRole := range apiToken.Roles {
		role := tokenRole.Role
		entityId := tokenRole.EntityId
		entityType := string(tokenRole.EntityType)
		tab.AddRow([]string{entityType, entityId, role}, false)
	}
	tab.Print(out)

	return nil
}

// create a organization token
func CreateToken(name, description, role, organization string, expiration int, cleanOutput bool, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsOrganizationRoleValid(role)
	if err != nil {
		return err
	}
	if name == "" {
		return ErrInvalidName
	}
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if organization == "" {
		organization = ctx.Organization
	}
	CreateOrganizationAPITokenRequest := astrocore.CreateOrganizationApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
	}
	if expiration != 0 {
		CreateOrganizationAPITokenRequest.TokenExpiryPeriodInDays = &expiration
	}
	resp, err := client.CreateOrganizationApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, CreateOrganizationAPITokenRequest)
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
		fmt.Fprintf(out, "\nAstro Organization API token %s was successfully created\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		fmt.Println("\n" + *APIToken.Token)
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// Update a organization token
func UpdateToken(id, name, newName, description, role, organization string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if organization == "" {
		organization = ctx.Organization
	}
	var token astrocore.ApiToken
	if id == "" {
		tokens, err := getOrganizationTokens(client)
		if err != nil {
			return err
		}
		token, err = getOrganizationToken(id, name, "\nPlease select the Organization API token you would like to update:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getOrganizationTokenById(id, ctx.OrganizationShortName, client)
		if err != nil {
			return err
		}
	}

	apiTokenID := token.Id

	UpdateOrganizationAPITokenRequest := astrocore.UpdateOrganizationApiTokenJSONRequestBody{}

	if newName == "" {
		UpdateOrganizationAPITokenRequest.Name = token.Name
	} else {
		UpdateOrganizationAPITokenRequest.Name = newName
	}

	if description == "" {
		UpdateOrganizationAPITokenRequest.Description = token.Description
	} else {
		UpdateOrganizationAPITokenRequest.Description = description
	}

	//if role == "" {
	//
	//	UpdateOrganizationAPITokenRequest.Roles = token.Roles
	//} else {
	//	err := user.IsOrganizationRoleValid(role)
	//	if err != nil {
	//		return err
	//	}
	//	UpdateOrganizationAPITokenRequest.Roles = role
	//}

	err = user.IsOrganizationRoleValid(role)
	if err != nil {
		return err
	}
	UpdateOrganizationAPITokenRequest.Roles = astrocore.UpdateOrganizationApiTokenRoles{Organization: role}

	resp, err := client.UpdateOrganizationApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, apiTokenID, UpdateOrganizationAPITokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Organization API token %s was successfully updated\n", token.Name)
	return nil
}

// rotate a organization API token
func RotateToken(id, name, organization string, cleanOutput, force bool, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if organization == "" {
		organization = ctx.Organization
	}
	var token astrocore.ApiToken

	if id == "" {
		tokens, err := getOrganizationTokens(client)
		if err != nil {
			return err
		}
		token, err = getOrganizationToken(id, name, "\nPlease select the Organization API token you would like to add to the Workspace:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getOrganizationTokenById(id, ctx.OrganizationShortName, client)
		if err != nil {
			return err
		}
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
	resp, err := client.RotateOrganizationApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, apiTokenID)
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
		fmt.Fprintf(out, "\nAstro Organization API token %s was successfully rotated\n", name)
		fmt.Println("Copy and paste this API token for your records.")
		fmt.Println("\n" + *APIToken.Token)
		fmt.Println("\nYou will not be shown this API token value again.")
	}
	return nil
}

// delete a organizations api token
func DeleteToken(id, name, organization string, force bool, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	if ctx.OrganizationShortName == "" {
		return user.ErrNoShortName
	}
	if organization == "" {
		organization = ctx.Organization
	}
	var token astrocore.ApiToken

	if id == "" {
		tokens, err := getOrganizationTokens(client)
		if err != nil {
			return err
		}
		token, err = getOrganizationToken(id, name, "\nPlease select the Organization API token you would like to add to the Workspace:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getOrganizationTokenById(id, ctx.OrganizationShortName, client)
		if err != nil {
			return err
		}
	}
	apiTokenID := token.Id
	if string(token.Type) == organizationEntity {
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
				fmt.Sprintf("\nAre you sure you want to remove the %s API token from the Organization?", ansi.Bold(token.Name)))

			if !i {
				fmt.Println("Canceling API Token removal")
				return nil
			}
		}
	}

	resp, err := client.DeleteOrganizationApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, apiTokenID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	if string(token.Type) == organizationEntity {
		fmt.Fprintf(out, "Astro Organization API token %s was successfully deleted\n", token.Name)
	} else {
		fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the Organization\n", token.Name)
	}
	return nil
}

func TimeAgo(date time.Time) string {
	duration := time.Since(date)
	days := int(duration.Hours() / 24) //nolint:gomnd
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
