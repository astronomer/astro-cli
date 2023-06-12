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
	errBothNameAndID               = errors.New("both a token name and id were specified. Specify either the name or the id not both")
)

const (
	workspaceEntity = "WORKSPACE"
)

func newTokenSelectionTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "DESCRIPTION", "EXPIRES"},
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
	tokens, err := getOrganizationTokens(client)
	if err != nil {
		return err
	}
	token, err := getOrganizationToken(id, name, "\nPlease select the organization token you would like to add to the workspace:", tokens)
	if err != nil {
		return err
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
	fmt.Fprintf(out, "Astro Organization Token %s was successfully added to the Workspace\n", token.Name)
	return nil
}

func selectTokens(apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		expires := apiTokens[i].ExpiryPeriodInDays

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			name,
			description,
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
			fmt.Printf("\nThere are more than one tokens with name %s. Please select a token:\n", name)
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
