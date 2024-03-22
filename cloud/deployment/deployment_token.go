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
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
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
	errInvalidDeploymentTokenKey = errors.New("invalid Deployment API token selection")
	ErrDeploymentTokenNotFound   = errors.New("no Deployment API token was found for the API token name you provided")
)

const (
	deploymentEntity = "DEPLOYMENT"
)

// List all deployment Tokens
func ListTokens(client astrocore.CoreClient, deployment string, out io.Writer) error {
	apiTokens, err := getDeploymentTokens(deployment, client)
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
			if apiTokens[i].Roles[j].EntityId == deployment {
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
func CreateToken(name, description, role, deployment string, expiration int, cleanOutput bool, out io.Writer, client astrocore.CoreClient) error {
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
	resp, err := client.CreateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, CreateDeploymentAPITokenRequest)
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
func UpdateToken(id, name, newName, description, role, deployment string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var token astrocore.ApiToken
	if id == "" {
		tokens, err := getDeploymentTokens(deployment, client)
		if err != nil {
			return err
		}
		token, err = getDeploymentToken(id, name, deployment, "\nPlease select the Deployment API token you would like to update:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getDeploymentTokenByID(id, deployment, ctx.Organization, client)
		if err != nil {
			return err
		}
	}

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
		for i := range token.Roles {
			if token.Roles[i].EntityType == deploymentEntity && token.Roles[i].EntityId == deployment {
				role = token.Roles[i].Role
			}
		}
		UpdateDeploymentAPITokenRequest.Role = role
	} else {
		UpdateDeploymentAPITokenRequest.Role = role
	}

	resp, err := client.UpdateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, apiTokenID, UpdateDeploymentAPITokenRequest)
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
func RotateToken(id, name, deployment string, cleanOutput, force bool, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var token astrocore.ApiToken
	if id == "" {
		tokens, err := getDeploymentTokens(deployment, client)
		if err != nil {
			return err
		}
		token, err = getDeploymentToken(id, name, deployment, "\nPlease select the Deployment API token you would like to update:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getDeploymentTokenByID(id, deployment, ctx.Organization, client)
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
	resp, err := client.RotateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, apiTokenID)
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
func DeleteToken(id, name, deployment string, force bool, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	var token astrocore.ApiToken
	if id == "" {
		tokens, err := getDeploymentTokens(deployment, client)
		if err != nil {
			return err
		}
		token, err = getDeploymentToken(id, name, deployment, "\nPlease select the Deployment API token you would like to update:", tokens)
		if err != nil {
			return err
		}
	} else {
		token, err = getDeploymentTokenByID(id, deployment, ctx.Organization, client)
		if err != nil {
			return err
		}
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

	resp, err := client.DeleteDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, apiTokenID)
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

func selectTokens(deployment string, apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	apiTokensMap := map[string]astrocore.ApiToken{}
	tab := newTokenSelectionTableOut()
	for i := range apiTokens {
		id := apiTokens[i].Id
		name := apiTokens[i].Name
		description := apiTokens[i].Description
		scope := apiTokens[i].Type
		var role string
		for j := range apiTokens[i].Roles {
			if apiTokens[i].Roles[j].EntityId == deployment {
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
func getDeploymentTokens(deployment string, client astrocore.CoreClient) ([]astrocore.ApiToken, error) {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return []astrocore.ApiToken{}, err
	}

	resp, err := client.ListDeploymentApiTokensWithResponse(httpContext.Background(), ctx.Organization, deployment, &astrocore.ListDeploymentApiTokensParams{})
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

func getDeploymentToken(id, name, deployment, message string, tokens []astrocore.ApiToken) (token astrocore.ApiToken, err error) { //nolint:gocognit
	switch {
	case id == "" && name == "":
		fmt.Println(message)
		token, err = selectTokens(deployment, tokens)
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
			token, err = selectTokens(deployment, matchedTokens)
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

func getDeploymentTokenByID(id, deploymentID, orgID string, client astrocore.CoreClient) (token astrocore.ApiToken, err error) {
	resp, err := client.GetDeploymentApiTokenWithResponse(httpContext.Background(), orgID, deploymentID, id)
	if err != nil {
		return astrocore.ApiToken{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrocore.ApiToken{}, err
	}
	return *resp.JSON200, nil
}
