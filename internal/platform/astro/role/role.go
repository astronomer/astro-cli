package role

import (
	httpContext "context"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

var rolePaginationLimit = 100

// RoleInfo is one role `astro organization role list` publishes. A default
// role is built in, and has no id.
type RoleInfo struct {
	Name        string `json:"name"`
	ID          string `json:"id,omitempty"`
	Description string `json:"description,omitempty"`
	ScopeType   string `json:"scope_type"`
	IsDefault   bool   `json:"is_default"`
}

// RoleList is what `astro organization role list` publishes: the default
// roles first, when they were asked for, then the Organization's own, in the
// order the table lists them.
type RoleList struct {
	Roles []RoleInfo `json:"roles"`
}

// Returns a list of all of an organizations roles
func GetOrgRoles(client astrov1.APIClient, shouldIncludeDefaultRoles bool) ([]astrov1.Role, []astrov1.DefaultRole, error) {
	offset := 0
	var roles []astrov1.Role

	ctx, err := context.GetCurrentContext()
	if err != nil {
		return nil, nil, err
	}
	var defaultRoles []astrov1.DefaultRole
	var includeDefaultRoles bool

	for {
		if len(defaultRoles) == 0 && shouldIncludeDefaultRoles {
			includeDefaultRoles = true
		} else {
			includeDefaultRoles = false
		}
		resp, err := client.ListRolesWithResponse(httpContext.Background(), ctx.Organization, &astrov1.ListRolesParams{
			IncludeDefaultRoles: &includeDefaultRoles,
			Offset:              &offset,
			Limit:               &rolePaginationLimit,
		})
		if err != nil {
			return nil, nil, err
		}
		err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, nil, err
		}
		if len(defaultRoles) == 0 && shouldIncludeDefaultRoles && resp.JSON200.DefaultRoles != nil {
			defaultRoles = *resp.JSON200.DefaultRoles
		}

		roles = append(roles, resp.JSON200.Roles...)

		if resp.JSON200.TotalCount <= offset {
			break
		}

		offset += rolePaginationLimit
	}

	return roles, defaultRoles, nil
}

// ListData returns the Organization's roles, with the default ones first
// when shouldIncludeDefaultRoles.
func ListData(client astrov1.APIClient, shouldIncludeDefaultRoles bool) (*RoleList, error) {
	roles, defaultRoles, err := GetOrgRoles(client, shouldIncludeDefaultRoles)
	if err != nil {
		return nil, err
	}
	list := &RoleList{Roles: make([]RoleInfo, 0, len(defaultRoles)+len(roles))}
	for i := range defaultRoles {
		list.Roles = append(list.Roles, RoleInfo{
			Name:        defaultRoles[i].Name,
			Description: deref(defaultRoles[i].Description),
			ScopeType:   string(defaultRoles[i].ScopeType),
			IsDefault:   true,
		})
	}
	for i := range roles {
		list.Roles = append(list.Roles, RoleInfo{
			Name:        roles[i].Name,
			ID:          roles[i].Id,
			Description: deref(roles[i].Description),
			ScopeType:   string(roles[i].ScopeType),
		})
	}
	return list, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
