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
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"TOKEN", "NAME", "DESCRIPTION", "SCOPE", "WORKSPACE ROLE", "EXPIRES", "CREATED", "CREATED BY"},
	}
}

func newTokenSelectionTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "TOKEN", "NAME", "DESCRIPTION", "SCOPE", "WORKSPACE ROLE", "EXPIRES", "CREATED", "CREATED BY"},
	}
}

var (
	errInvalidWorkspaceTokenKey = errors.New("invalid workspace token selection")
	ErrWorkspaceTokenNotFound   = errors.New("no workspace token was found for the token name you provided")
)

const (
	workspaceConst = "WORKSPACE"
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
	fmt.Fprintf(out, "Astro Workspace Token %s was successfully created\nCopy and Past this Token for your records.\n\n", name)
	APIToken := resp.JSON200
	fmt.Println(*APIToken.Token)
	fmt.Println("\nYou will not be shown this API Token value again.")

	return nil
}

// Update a workspace token
func UpdateToken(name, newName, description, role, workspace string, out io.Writer, client astrocore.CoreClient) error {
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
		fmt.Println("\nPlease select the workspace token you would like to update:")
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
	apiTokenID := token.Id

	UpdateWorkspaceAPITokenRequest := astrocore.UpdateWorkspaceApiTokenJSONRequestBody{}

	if newName == "" {
		UpdateWorkspaceAPITokenRequest.Name = token.Name
	} else {
		UpdateWorkspaceAPITokenRequest.Name = newName
	}

	if description == "" {
		UpdateWorkspaceAPITokenRequest.Description = *token.Description
	} else {
		UpdateWorkspaceAPITokenRequest.Description = description
	}
	if role == "" {
		for i := range token.Roles {
			if token.Roles[i].EntityId == workspace {
				role = token.Roles[i].Role
			}
		}
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
		fmt.Println("\nPlease select the workspace token you would like to rotate:")
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
	apiTokenID := token.Id

	if !force {
		fmt.Println("WARNING: API Token rotation will invalidate the current token and cannot be undone.")
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to rotate the %s token?", ansi.Bold(token.Name)))

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
	fmt.Fprintf(out, "Astro Workspace Token %s was successfully rotated\n\n", token.Name)
	APIToken := resp.JSON200
	fmt.Println(*APIToken.Token)
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
		fmt.Println("\nPlease select the workspace token you would like to delete or remove:")
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
	apiTokenID := token.Id
	if string(token.Type) == workspaceConst {
		if !force {
			fmt.Println("WARNING: API Token deletion cannot be undone.")
			i, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to delete the %s token?", ansi.Bold(token.Name)))

			if !i {
				fmt.Println("Canceling token deletion")
				return nil
			}
		}
	} else {
		if !force {
			i, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to remove the %s token from the workspace?", ansi.Bold(token.Name)))

			if !i {
				fmt.Println("Canceling token removal")
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
	if string(token.Type) == workspaceConst {
		fmt.Fprintf(out, "Astro Workspace API token %s was successfully deleted\n", token.Name)
	} else {
		fmt.Fprintf(out, "Astro Organization API token %s was successfully removed from the workspace\n", token.Name)
	}
	return nil
}

func selectTokens(workspace string, apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
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

	APITokens := resp.JSON200.ApiTokens

	return APITokens, nil
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
