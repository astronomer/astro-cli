package workspace

import (
	httpContext "context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/user"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/pkg/errors"
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
	errInvalidWorkspaceTokenKey = errors.New("invalid Workspace API token selection")
	ErrWorkspaceTokenNotFound   = errors.New("no Workspace API token was found for the API token name you provided")
	errBothNameAndID            = errors.New("both an API token name and id were specified. Specify either the name or the id not both")
)

const (
	workspaceEntity = "WORKSPACE"
)

// List all workspace Tokens
func ListTokens(client astrocore.CoreClient, workspace string, out io.Writer) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if workspace == "" {
		workspace = ctx.Workspace
	}

	apiTokens, err := getWorkspaceTokens(workspace, client)
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
			if apiTokens[i].Roles[j].EntityId == workspace {
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

// create a workspace token
func CreateToken(name, description, role, workspace string, expiration int, cleanOutput bool, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsWorkspaceRoleValid(role)
	if err != nil {
		return err
	}
	if name == "" {
		return ErrInvalidTokenName
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
	CreateWorkspaceAPITokenRequest := astrocore.CreateWorkspaceApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
	}
	if expiration != 0 {
		CreateWorkspaceAPITokenRequest.TokenExpiryPeriodInDays = &expiration
	}
	resp, err := client.CreateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, CreateWorkspaceAPITokenRequest)
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
func UpdateToken(id, name, newName, description, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
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
	tokens, err := getWorkspaceTokens(workspace, client)
	if err != nil {
		return err
	}
	token, err := getWorkspaceToken(id, name, workspace, "\nPlease select the Workspace API token you would like to update:", tokens)
	if err != nil {
		return err
	}
	apiTokenID := token.Id

	UpdateWorkspaceAPITokenRequest := astrocore.UpdateWorkspaceApiTokenJSONRequestBody{}

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
		// no role was provided so ask the user for it
		role = input.Text("enter a user Organization role(WORKSPACE_MEMBER, WORKSPACE_OPERATOR, WORKSPACE_OWNER) to update user: ")
		err := user.IsWorkspaceRoleValid(role)
		if err != nil {
			return err
		}
		UpdateWorkspaceAPITokenRequest.Role = role
	} else {
		err := user.IsWorkspaceRoleValid(role)
		if err != nil {
			return err
		}
		UpdateWorkspaceAPITokenRequest.Role = role
	}
	resp, err := client.UpdateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, apiTokenID, UpdateWorkspaceAPITokenRequest)
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
func RotateToken(id, name, workspace string, cleanOutput, force bool, out io.Writer, client astrocore.CoreClient) error {
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
	tokens, err := getWorkspaceTokens(workspace, client)
	if err != nil {
		return err
	}
	token, err := getWorkspaceToken(id, name, workspace, "\nPlease select the Workspace API token you would like to rotate:", tokens)
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
	resp, err := client.RotateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, apiTokenID)
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
func DeleteToken(id, name, workspace string, force bool, out io.Writer, client astrocore.CoreClient) error {
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
	tokens, err := getWorkspaceTokens(workspace, client)
	if err != nil {
		return err
	}
	token, err := getWorkspaceToken(id, name, workspace, "\nPlease select the Workspace APItoken you would like to delete or remove:", tokens)
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

	resp, err := client.DeleteWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, apiTokenID)
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

func selectTokens(workspace string, apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == workspace {
				role = apiTokens[i].Roles[j].Role
			}
		}
		created := TimeAgo(apiTokens[i].CreatedAt)
		createdBy := apiTokens[i].CreatedBy.FullName

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			id,
			name,
			description,
			string(scope),
			role,
			created,
			*createdBy,
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
func getWorkspaceTokens(workspace string, client astrocore.CoreClient) ([]astrocore.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.ApiToken{}, err
	}
	if ctx.OrganizationShortName == "" {
		return []astrocore.ApiToken{}, user.ErrNoShortName
	}
	if workspace == "" {
		workspace = ctx.Workspace
	}

	resp, err := client.ListWorkspaceApiTokensWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, &astrocore.ListWorkspaceApiTokensParams{})
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

func getWorkspaceToken(id, name, workspace, message string, tokens []astrocore.ApiToken) (token astrocore.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(workspace, tokens)
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
			token, err = selectTokens(workspace, matchedTokens)
			if err != nil {
				return astrocore.ApiToken{}, err
			}
		}
	case name != "" && id != "":
		return astrocore.ApiToken{}, errBothNameAndID
	}
	if token.Id == "" {
		return astrocore.ApiToken{}, ErrWorkspaceTokenNotFound
	}
	return token, nil
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
