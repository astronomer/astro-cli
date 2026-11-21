package apitoken

import (
	"context"
	"errors"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
)

// pageSize is how many tokens one request asks for.
const pageSize = 100

// Fetch pages through the Organization's API tokens that filters selects (the
// Workspace, Deployment or Organization-only filter it carries), in name order,
// the stable order the API offers (its token sorts have no id tiebreaker). It stops
// after limit tokens (every one when limit is below 1) and when ctx is
// canceled. Offset, Limit and Sorts of filters are set here.
//
// keep, when not nil, is a client-side filter: the API cannot filter by token
// scope, so limit then counts the tokens kept, and every page is read.
func Fetch(ctx context.Context, client astrov1.APIClient, organizationID string, filters astrov1.ListApiTokensParams, limit int, keep func(*astrov1.ApiToken) bool) ([]astrov1.ApiToken, error) {
	sorts := []astrov1.ListApiTokensParamsSorts{astrov1.ListApiTokensParamsSortsNameAsc}
	apiLimit := limit
	if keep != nil {
		apiLimit = 0
	}
	tokens, err := pagination.CollectUpTo(ctx, "API tokens", apiLimit, func(offset int) ([]astrov1.ApiToken, int, error) {
		size := pageSize
		if apiLimit > 0 {
			size = min(size, apiLimit-offset)
		}
		params := filters
		params.Offset, params.Limit, params.Sorts = &offset, &size, &sorts
		resp, err := client.ListApiTokensWithResponse(pagination.Background(ctx), organizationID, &params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		if resp.JSON200 == nil {
			return nil, 0, errors.New("list API tokens returned an empty response body")
		}
		return resp.JSON200.Tokens, resp.JSON200.TotalCount, nil
	})
	if err != nil || keep == nil {
		return tokens, err
	}
	kept := tokens[:0]
	for i := range tokens {
		if keep(&tokens[i]) {
			kept = append(kept, tokens[i])
		}
	}
	if limit > 0 && len(kept) > limit {
		kept = kept[:limit]
	}
	return kept, nil
}

// OfScopes is the keep filter for the tokens whose scope is one of scopes; nil
// (keep every token) when scopes is empty.
func OfScopes[S ~string](scopes []S) func(*astrov1.ApiToken) bool {
	if len(scopes) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		allowed[string(s)] = struct{}{}
	}
	return func(t *astrov1.ApiToken) bool {
		_, ok := allowed[string(t.Scope)]
		return ok
	}
}
