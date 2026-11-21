package role

import (
	httpContext "context"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
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

// GetOrgRoles returns the Organization's own roles, the first limit of them
// (all when limit is below 1) in name order, and the built-in default roles
// when shouldIncludeDefaultRoles; the default roles are not counted against
// limit. Paging stops when ctx is canceled.
func GetOrgRoles(ctx httpContext.Context, client astrov1.APIClient, shouldIncludeDefaultRoles bool, limit int) ([]astrov1.Role, []astrov1.DefaultRole, error) {
	cfg, err := context.GetCurrentContext()
	if err != nil {
		return nil, nil, err
	}
	var defaultRoles []astrov1.DefaultRole
	sorts := []astrov1.ListRolesParamsSorts{astrov1.ListRolesParamsSortsNameAsc}

	roles, err := pagination.CollectUpTo(ctx, "roles", limit, func(offset int) ([]astrov1.Role, int, error) {
		// The default roles ride on the first page only.
		includeDefaultRoles := offset == 0 && shouldIncludeDefaultRoles
		pageSize := rolePaginationLimit
		if limit > 0 {
			pageSize = min(pageSize, limit-offset)
		}
		resp, err := client.ListRolesWithResponse(pagination.Background(ctx), cfg.Organization, &astrov1.ListRolesParams{
			IncludeDefaultRoles: &includeDefaultRoles,
			Offset:              &offset,
			Limit:               &pageSize,
			Sorts:               &sorts,
		})
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		if includeDefaultRoles && resp.JSON200.DefaultRoles != nil {
			defaultRoles = *resp.JSON200.DefaultRoles
		}
		return resp.JSON200.Roles, resp.JSON200.TotalCount, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return roles, defaultRoles, nil
}

// ListData returns the Organization's roles, with the default ones first
// when shouldIncludeDefaultRoles.
func ListData(ctx httpContext.Context, client astrov1.APIClient, shouldIncludeDefaultRoles bool, limit int) (*RoleList, error) {
	roles, defaultRoles, err := GetOrgRoles(ctx, client, shouldIncludeDefaultRoles, limit)
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
