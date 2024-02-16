package apitoken

import (
	httpContext "context"
	"errors"
	"fmt"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"io"
	"os"
	"strconv"
	"time"
)

var (
	ErrInvalidApiTokenKey  = errors.New("invalid ApiToken selected")
	apiTokenPagnationLimit = 100
)

// Returns a list of all of an organizations apiTokens
func GetOrgApiTokens(client astrocore.CoreClient) ([]astrocore.ApiToken, error) {
	offset := 0
	var apiTokens []astrocore.ApiToken

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	for {
		resp, err := client.ListOrganizationApiTokensWithResponse(httpContext.Background(), ctx.Organization, &astrocore.ListOrganizationApiTokensParams{
			Offset: &offset,
			Limit:  &apiTokenPagnationLimit,
		})
		if err != nil {
			return nil, err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		apiTokens = append(apiTokens, resp.JSON200.ApiTokens...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += apiTokenPagnationLimit
	}

	return apiTokens, nil
}

func CreateDeploymentApiToken(name, role, description, deployment string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	mutateApiTokenInput := astrocore.CreateDeploymentApiTokenRequest{
		Role:        role,
		Name:        name,
		Description: &description,
	}
	resp, err := client.CreateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, mutateApiTokenInput)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The apiToken %s was successfully added to the deployment with the role %s\n", name, role)
	return nil
}

func UpdateDeploymentApiTokenRole(apiTokenID, role, deployment string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	var apiToken *astrocore.ApiToken

	if apiTokenID == "" {
		// Get all dep apiTokens. Setting limit to 1000 for now
		apiTokens, err := GetDeploymentApiTokens(client, deployment, 1000)
		if err != nil {
			return err
		}
		apiToken, err = getApiToken(apiTokens)
		if err != nil {
			return err
		}
	} else {
		resp, err := client.GetDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, apiTokenID)
		if err != nil {
			fmt.Println("error in GetDeploymentApiTokenWithResponse")
			return err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			fmt.Println("error in NormalizeAPIError")
			return err
		}
		apiToken = resp.JSON200
	}

	mutateApiTokenInput := astrocore.UpdateDeploymentApiTokenRequest{
		Role: role,
		Name: apiToken.Name,
	}
	fmt.Println("deployment: " + deployment)
	resp, err := client.UpdateDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, apiToken.Id, mutateApiTokenInput)
	if err != nil {
		fmt.Println("error in MutateDeploymentApiTokenRoleWithResponse")
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		fmt.Println("error in NormalizeAPIError")
		return err
	}
	fmt.Fprintf(out, "The deployment apiToken %s role was successfully updated to %s\n", apiTokenID, role)
	return nil
}

func getApiToken(apitokens []astrocore.ApiToken) (*astrocore.ApiToken, error) {
	apiToken, err := SelectDeploymentApiToken(apitokens)
	if err != nil {
		return nil, err
	}

	return &apiToken, nil
}

func SelectDeploymentApiToken(apiTokens []astrocore.ApiToken) (astrocore.ApiToken, error) {
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "DEPLOYMENT_ROLE", "DESCRIPTION", "ID", "CREATE DATE", "UPDATE DATE"},
	}

	fmt.Println("\nPlease select the api token:")

	apiTokenMap := map[string]astrocore.ApiToken{}
	for i := range apiTokens {
		index := i + 1
		var role string
		for _, tokenRole := range apiTokens[i].Roles {
			if tokenRole.EntityType == "DEPLOYMENT" {
				role = tokenRole.Role
			}
		}

		table.AddRow([]string{
			strconv.Itoa(index),
			apiTokens[i].Name,
			role,
			apiTokens[i].Description,
			apiTokens[i].Id,
			apiTokens[i].CreatedAt.Format(time.RFC3339),
			apiTokens[i].UpdatedAt.Format(time.RFC3339),
		}, false)

		apiTokenMap[strconv.Itoa(index)] = apiTokens[i]
	}

	table.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := apiTokenMap[choice]
	if !ok {
		return astrocore.ApiToken{}, ErrInvalidApiTokenKey
	}
	return selected, nil
}

func RemoveDeploymentApiToken(apiTokenID, deployment string, out io.Writer, client astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	if apiTokenID == "" {
		// Get all org apiTokens. Setting limit to 1000 for now
		apiTokens, err := GetDeploymentApiTokens(client, deployment, apiTokenPagnationLimit)
		if err != nil {
			return err
		}
		apiToken, err := getApiToken(apiTokens)
		if err != nil {
			return err
		}
		apiTokenID = apiToken.Id
	}

	resp, err := client.DeleteDeploymentApiTokenWithResponse(httpContext.Background(), ctx.Organization, deployment, apiTokenID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The apiToken %s was successfully removed from the deployment\n", apiTokenID)
	return nil
}

// Returns a list of all of a deployments apiTokens
func GetDeploymentApiTokens(client astrocore.CoreClient, deployment string, limit int) ([]astrocore.ApiToken, error) {
	offset := 0
	var apiTokens []astrocore.ApiToken

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	for {
		resp, err := client.ListDeploymentApiTokensWithResponse(httpContext.Background(), ctx.Organization, deployment, &astrocore.ListDeploymentApiTokensParams{
			Offset: &offset,
			Limit:  &limit,
		})
		if err != nil {
			return nil, err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		apiTokens = append(apiTokens, resp.JSON200.ApiTokens...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += limit
	}

	return apiTokens, nil
}

// Prints a list of all of an deployments apiTokens
//
//nolint:dupl
func ListDeploymentApiTokens(out io.Writer, client astrocore.CoreClient, deployment string) error {
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DESCRIPTION", "ID", "DEPLOYMENT ROLE", "CREATE DATE", "UPDATE DATE"},
	}
	apiTokens, err := GetDeploymentApiTokens(client, deployment, apiTokenPagnationLimit)
	if err != nil {
		return err
	}

	for i := range apiTokens {
		var deploymentRole string
		for _, role := range apiTokens[i].Roles {
			if role.EntityId == deployment {
				deploymentRole = role.Role
			}
		}
		table.AddRow([]string{
			apiTokens[i].Name,
			apiTokens[i].Description,
			apiTokens[i].Id,
			deploymentRole,
			apiTokens[i].CreatedAt.Format(time.RFC3339),
			apiTokens[i].UpdatedAt.Format(time.RFC3339),
		}, false)
	}

	table.Print(out)
	return nil
}
