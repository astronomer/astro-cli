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

func newTableOutToken() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"TOKEN", "NAME", "DESCRIPTION", "SCOPE", "WORKSPACE ROLE", "EXPIRES", "CREATED", "CREATED BY"},
	}
}

var (
	errInvalidWorkspaceTokenKey = errors.New("invalid workspace token selection")
	ErrWorkspaceTokenNotFound   = errors.New("no workspace token was found for the token name you provided")
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

	tab := newTableOutToken()
	for i := range apiTokens {
		token := apiTokens[i].ShortToken
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == workspace {
				role = apiTokens[i].Roles[j].Role
			}
		}
		expires := fmt.Sprint(apiTokens[i].ExpiryPeriodInDays)
		if expires == "0" {
			expires = "-"
		}
		created := TimeAgo(apiTokens[i].CreatedAt)
		createdBy := apiTokens[i].CreatedBy.FullName

		tab.AddRow([]string{token, name, *description, string(scope), role, expires, created, *createdBy}, false)

	}

	tab.Print(out)

	return nil
}

// create a workspace token
func CreateToken(name, description, role, workspace string, expiration int, out io.Writer, client astrocore.CoreClient) error {
	err := user.IsWorkspaceRoleValid(role)
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
	if workspace == "" {
		workspace = ctx.Workspace
	}
	CreateWorkspaceApiTokenRequest := astrocore.CreateWorkspaceApiTokenJSONRequestBody{
		Description: &description,
		Name:        name,
		Role:        role,
	}
	if expiration != 0 {
		CreateWorkspaceApiTokenRequest.TokenExpiryPeriodInDays = &expiration
	}
	resp, err := client.CreateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, CreateWorkspaceApiTokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace Token %s was successfully created\nCopy and Past this Token for your records.\n\n", name)
	ApiToken := resp.JSON200
	fmt.Println(ApiToken)
	fmt.Println(ApiToken.Token)
	fmt.Println("\nYou will not be shown this API Token value again.")

	return nil
}

// Update a workspace token
func UpdateToken(name, newName, description, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
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
	tokens, err := getWorkspaceTokens(workspace, client)
	if err != nil {
		return err
	}
	var token astrocore.ApiToken
	if name == "" {
		token, err = selectTokens(workspace, tokens)
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
			return ErrWorkspaceTokenNotFound
		}
	}
	apiTokenId := token.Id

	UpdateWorkspaceApiTokenRequest := astrocore.UpdateWorkspaceApiTokenJSONRequestBody{}

	if newName == "" {
		UpdateWorkspaceApiTokenRequest.Name = token.Name
	} else {
		UpdateWorkspaceApiTokenRequest.Name = newName
	}

	if description == "" {
		UpdateWorkspaceApiTokenRequest.Description = *token.Description
	} else {
		UpdateWorkspaceApiTokenRequest.Description = description
	}
	if role == "" {
		for i := range token.Roles {
			if token.Roles[i].EntityId == workspace {
				role = token.Roles[i].Role
			}
		}
		UpdateWorkspaceApiTokenRequest.Role = role
	} else {
		UpdateWorkspaceApiTokenRequest.Role = role
	}
	resp, err := client.UpdateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, apiTokenId, UpdateWorkspaceApiTokenRequest)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace Token %s was successfully updated\n", token.Name)
	return nil
}

// rotate a workspace API token
func RotateToken(name, workspace string, force bool, out io.Writer, client astrocore.CoreClient) error {
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
	var token astrocore.ApiToken
	if name == "" {
		token, err = selectTokens(workspace, tokens)
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
			return ErrWorkspaceTokenNotFound
		}
	}
	apiTokenId := token.Id

	if !force {
		fmt.Println("WARNING: API Token rotation will invalidate the current token and cannot be undone.")
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to rotate the %s token?", ansi.Bold(token.Name)))

		if !i {
			fmt.Println("Canceling token rotation")
			return nil
		}
	}

	resp, err := client.RotateWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, apiTokenId)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace Token %s was successfully rotated\n\n", token.Name)
	ApiToken := resp.JSON200
	fmt.Println(ApiToken.Token)
	fmt.Println("\nYou will not be shown this API Token value again.")
	return nil
}

// delete a workspaces api token
func DeleteToken(id, workspace string, force bool, out io.Writer, client astrocore.CoreClient) error {
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
	var token astrocore.ApiToken
	if id == "" {
		token, err = selectTokens(workspace, tokens)
		if err != nil {
			return err
		}
	} else {
		for i := range tokens {
			if tokens[i].Id == id {
				token = tokens[i]
			}
		}
		if token.Id == "" {
			return ErrWorkspaceTokenNotFound
		}
	}
	apiTokenId := token.Id

	if !force {
		fmt.Println("WARNING: API Token deletion cannot be undone.")
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to delete the %s token?", ansi.Bold(token.Name)))

		if !i {
			fmt.Println("Canceling token rotation")
			return nil
		}
	}

	resp, err := client.DeleteWorkspaceApiTokenWithResponse(httpContext.Background(), ctx.OrganizationShortName, workspace, apiTokenId)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Astro Workspace API token %s was successfully deleted\n", token.Name)
	return nil
}

func selectTokens(workspace string, apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {

	fmt.Println("\nPlease select the workspace token you would like to update:")

	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTableOutToken()
	for i := range apiTokens {

		token := apiTokens[i].ShortToken
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == workspace {
				role = apiTokens[i].Roles[j].Role
			}
		}
		expires := fmt.Sprint(apiTokens[i].ExpiryPeriodInDays)
		if expires == "0" {
			expires = "-"
		}
		created := TimeAgo(apiTokens[i].CreatedAt)
		createdBy := apiTokens[i].CreatedBy.FullName

		index := i + 1
		tab.AddRow([]string{
			strconv.Itoa(index),
			token,
			name,
			*description,
			string(scope),
			role,
			expires,
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

	ApiTokens := resp.JSON200.ApiTokens

	return ApiTokens, nil
}

func TimeAgo(date time.Time) string {
	duration := time.Since(date)
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes())

	if days > 0 {
		return fmt.Sprintf("%d days ago", days)
	} else if hours > 0 {
		return fmt.Sprintf("%d hours ago", hours)
	} else if minutes > 0 {
		return fmt.Sprintf("%d minutes ago", minutes)
	} else {
		return "Just now"
	}
}
