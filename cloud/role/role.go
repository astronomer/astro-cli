package role

import (
	httpContext "context"
	"io"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var rolePagnationLimit = 100

// Returns a list of all of an organizations roles
func GetOrgRoles(client astrocore.CoreClient, shouldIncludeDefaultRoles bool) ([]astrocore.Role, []astrocore.DefaultRole, error) {
	offset := 0
	var roles []astrocore.Role

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, nil, err
	}
	var defaultRoles []astrocore.DefaultRole
	var includeDefaultRoles bool

	for {
		if len(defaultRoles) == 0 && shouldIncludeDefaultRoles {
			includeDefaultRoles = true
		} else {
			includeDefaultRoles = false
		}
		resp, err := client.ListRolesWithResponse(httpContext.Background(), ctx.Organization, &astrocore.ListRolesParams{
			IncludeDefaultRoles: &includeDefaultRoles,
			Offset:              &offset,
			Limit:               &rolePagnationLimit,
		})
		if err != nil {
			return nil, nil, err
		}
		err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, nil, err
		}
		if len(defaultRoles) == 0 && shouldIncludeDefaultRoles {
			defaultRoles = *resp.JSON200.DefaultRoles
		}

		roles = append(roles, resp.JSON200.Roles...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += rolePagnationLimit
	}

	return roles, defaultRoles, nil
}

// Prints a list of all of an organizations roles
func ListOrgRoles(out io.Writer, client astrocore.CoreClient, shouldIncludeDefaultRoles bool) error {
	table := printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID", "DESCRIPTION"},
	}
	roles, defaultRoles, err := GetOrgRoles(client, shouldIncludeDefaultRoles)
	if err != nil {
		return err
	}
	for i := range defaultRoles {
		table.AddRow([]string{
			defaultRoles[i].Name,
			"",
			*defaultRoles[i].Description,
		}, false)
	}

	for i := range roles {
		var description string
		if roles[i].Description != nil {
			description = *roles[i].Description
		}
		table.AddRow([]string{
			roles[i].Name,
			roles[i].Id,
			description,
		}, false)
	}

	table.Print(out)
	return nil
}
