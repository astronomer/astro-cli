package env

import (
	httpcontext "context"
	"errors"
	"fmt"
	"net/http"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/emfetch"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	ErrScopeNotSpecified = errors.New("--workspace-id or --deployment-id must be specified")
	ErrScopeAmbiguous    = errors.New("--workspace-id and --deployment-id are mutually exclusive")
	ErrNotFound          = errors.New("environment object not found")
)

// getObjectListLimit is the page size for a single-row lookup by key.
// Multi-page reads take their page size and their bound from pkg/emfetch.
//
// ObjectKey filtering returns at most one row per (scope, objectType); two
// catches a server-side anomaly without forcing pagination.
const getObjectListLimit = 2

// Scope captures the target of an env-object operation. Exactly one of
// WorkspaceID / DeploymentID is set.
type Scope struct {
	WorkspaceID  string
	DeploymentID string
}

func (s Scope) Validate() error {
	switch {
	case s.WorkspaceID == "" && s.DeploymentID == "":
		return ErrScopeNotSpecified
	case s.WorkspaceID != "" && s.DeploymentID != "":
		return ErrScopeAmbiguous
	}
	return nil
}

// listObjects returns env-objects of the given type within the scope.
// resolveLinked includes inherited workspace objects when listing at deployment scope.
// includeSecrets requests secret values from the server (subject to org policy).
//
// The paging is pkg/emfetch's, so this and every other reader of the endpoint
// agree on how a window ends and what an exhausted bound means.
//
// The organization's refusal to resolve secrets is reported rather than retried
// without them, which is where this parts company with the other readers. These
// callers render a list and have nowhere to say the values in it were withheld,
// so a quietly structural listing would read as a complete one.
func listObjects(scope Scope, objectType astrov1.ListEnvironmentObjectsParamsObjectType, resolveLinked, includeSecrets bool, astroV1Client astrov1.APIClient) ([]astrov1.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	objs, err := emfetch.Paginate(httpcontext.Background(),
		func(ctx httpcontext.Context, offset, limit int) ([]astrov1.EnvironmentObject, int, error) {
			params := buildListParams(scope, objectType, nil, resolveLinked, includeSecrets, limit)
			params.Offset = &offset

			resp, err := astroV1Client.ListEnvironmentObjectsWithResponse(ctx, c.Organization, params)
			if err != nil {
				return nil, 0, err
			}
			if err := normalizeListErr(resp.HTTPResponse, resp.Body); err != nil {
				return nil, 0, err
			}
			if resp.JSON200 == nil {
				return nil, 0, fmt.Errorf("listing %s objects: the response carried no body", nounForListType(objectType))
			}
			return resp.JSON200.EnvironmentObjects, resp.JSON200.TotalCount, nil
		})
	if err != nil {
		return nil, fmt.Errorf("listing %s objects: %w", nounForListType(objectType), err)
	}
	return objs, nil
}

// getObject fetches a single env-object by ID or key.
//
// The platform has no GET-by-key endpoint; for keys, we filter the list endpoint
// server-side via ObjectKey and force resolveLinked=false so the returned ID is
// addressable in subsequent CRUD calls.
func getObject(idOrKey string, scope Scope, objectType astrov1.ListEnvironmentObjectsParamsObjectType, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if util.IsCUID(idOrKey) {
		resp, err := astroV1Client.GetEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, idOrKey)
		if err != nil {
			return nil, err
		}
		// A missing id has to report ErrNotFound like a missing key does.
		// Returning the raw server message meant errors.Is(…, ErrNotFound)
		// was false on this branch, so an upsert would not fall through to
		// create and --no-create would not explain itself — for exactly half
		// the inputs `set <id-or-key>` advertises.
		if resp.HTTPResponse != nil && resp.HTTPResponse.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, idOrKey)
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		return resp.JSON200, nil
	}

	if err := scope.Validate(); err != nil {
		return nil, err
	}
	params := buildListParams(scope, objectType, &idOrKey, false, includeSecrets, getObjectListLimit)

	resp, err := astroV1Client.ListEnvironmentObjectsWithResponse(httpcontext.Background(), c.Organization, params)
	if err != nil {
		return nil, err
	}
	if err := normalizeListErr(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	for i := range resp.JSON200.EnvironmentObjects {
		if resp.JSON200.EnvironmentObjects[i].ObjectKey == idOrKey {
			return &resp.JSON200.EnvironmentObjects[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, idOrKey)
}

// deleteObject deletes an env-object by ID or key.
func deleteObject(idOrKey string, scope Scope, objectType astrov1.ListEnvironmentObjectsParamsObjectType, astroV1Client astrov1.APIClient) error {
	id, err := resolveID(idOrKey, scope, objectType, astroV1Client)
	if err != nil {
		return err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	resp, err := astroV1Client.DeleteEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// resolveID returns idOrKey unchanged when it's already a CUID (no network call),
// otherwise looks up the ID by key. Used by Update / Delete paths so callers
// passing an ID don't pay for a redundant GET.
func resolveID(idOrKey string, scope Scope, objectType astrov1.ListEnvironmentObjectsParamsObjectType, astroV1Client astrov1.APIClient) (string, error) {
	if util.IsCUID(idOrKey) {
		return idOrKey, nil
	}
	existing, err := getObject(idOrKey, scope, objectType, false, astroV1Client)
	if err != nil {
		return "", err
	}
	return objectID(existing, idOrKey)
}

// objectID extracts the object's addressable ID, erroring when the platform
// returned an object without one.
func objectID(obj *astrov1.EnvironmentObject, idOrKey string) (string, error) {
	if obj.Id == nil || *obj.Id == "" {
		return "", fmt.Errorf("environment object %q has no id", idOrKey)
	}
	return *obj.Id, nil
}

// scopeRequest converts a Scope into the create-request scope enum + entity ID.
func scopeRequest(scope Scope) (scopeType astrov1.CreateEnvironmentObjectRequestScope, scopeEntityID string) {
	if scope.DeploymentID != "" {
		return astrov1.CreateEnvironmentObjectRequestScopeDEPLOYMENT, scope.DeploymentID
	}
	return astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE, scope.WorkspaceID
}

// ErrAutoLinkRequiresWorkspace is returned when a caller asks to auto-link an
// object to all deployments while creating it at deployment scope. Auto-link
// is a workspace-scope concept; deployment-scope objects are already pinned
// to a single deployment.
var ErrAutoLinkRequiresWorkspace = errors.New("--auto-link applies only to workspace-scoped objects")

// validateAutoLink rejects auto-link=true on a deployment-scoped object. nil
// (flag unset) and false are no-ops.
func validateAutoLink(scope Scope, autoLink *bool) error {
	if autoLink != nil && *autoLink && scope.DeploymentID != "" {
		return ErrAutoLinkRequiresWorkspace
	}
	return nil
}

func buildListParams(scope Scope, objectType astrov1.ListEnvironmentObjectsParamsObjectType, objectKey *string, resolveLinked, includeSecrets bool, limit int) *astrov1.ListEnvironmentObjectsParams {
	params := &astrov1.ListEnvironmentObjectsParams{
		ObjectType:    &objectType,
		ObjectKey:     objectKey,
		ShowSecrets:   &includeSecrets,
		ResolveLinked: &resolveLinked,
		Limit:         &limit,
	}
	if scope.WorkspaceID != "" {
		params.WorkspaceId = &scope.WorkspaceID
	} else if scope.DeploymentID != "" {
		params.DeploymentId = &scope.DeploymentID
	}
	return params
}

// normalizeListErr substitutes the friendlier org-level secrets-fetching
// guidance when applicable.
//
// The refusal is recognized from the response rather than from the error
// NormalizeAPIError built out of it, because that error carries only the
// message field of a JSON envelope: a refusal arriving as anything else loses
// the very words that identify it.
func normalizeListErr(httpResp *http.Response, body []byte) error {
	if httpResp == nil {
		return errors.New("the environment objects API returned no response")
	}
	if emfetch.IsOrgSecretsRefusal(httpResp.StatusCode, body) {
		return errors.New(SecretsFetchingNotAllowedErrMsg)
	}
	return astrov1.NormalizeAPIError(httpResp, body)
}
