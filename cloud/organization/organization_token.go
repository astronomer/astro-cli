package organization

import (
	httpContext "context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	errInvalidOrganizationTokenKey = errors.New("invalid organization token selection")
	errOrganizationTokenNotFound   = errors.New("organization token specified was not found")
	errOrgTokenInWorkspace         = errors.New("this organization token has already been added to the workspace")
)

func newTokenSelectionTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "DESCRIPTION"},
	}
}

// Update a workspace token
func AddOrgTokenToWorkspace(name, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
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
	tokens, err := getOrganizationTokens(client)
	if err != nil {
		return err
	}
	var token astrocore.ApiToken
	if name == "" {
		token, err = selectTokenForWorkspace(tokens)
		if err != nil {
			return err
		}
	} else {
		for i := range tokens {
			if tokens[i].Name == name {
				token = tokens[i]
			}
		}
		if token.Id == "" {
			return errOrganizationTokenNotFound
		}
	}
	apiTokenID := token.Id

	var orgRole string
	for i := range token.Roles {
		if token.Roles[i].EntityId == workspace {
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
		Description: *token.Description,
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
	fmt.Fprintf(out, "Astro Organization Token %s was successfully added to the Workspace\n", token.Name)
	return nil
}

func selectTokenForWorkspace(apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	fmt.Println("\nPlease select the organization token you would like to add to the workspace:")
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		name := apiTokens[i].Name
		description := apiTokens[i].Description

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			name,
			*description,
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

// get all workspace tokens
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
