package deployment

import (
	httpContext "context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/internal/platform/astro/apitoken"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/internal/usageerr"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
)

// The bundle endpoints live only on the v1alpha1 public API, so bundle operations
// use the v1alpha1 client while the deployment lookup (which resolves the org and
// deployment ids) stays on v1. Collapse onto a single client once the bundle API
// reaches v1.

const (
	bundleListLimit        = 1000
	bundleVersionListLimit = 1000
)

var (
	errCreateBundleTarget   = usageerr.Usage(errors.New("specify exactly one of --name (Dag bundle) or --mount-path (non-Dag bundle)"))
	errDagBundleNonDagFlags = usageerr.Usage(errors.New("--bundle-type and --dag-bundle-ids are only valid for non-Dag bundles (--mount-path)"))
	errUpdateBundleNoOp     = usageerr.Usage(errors.New("specify at least one of --description or --dag-bundle-ids"))
	errBundleSelector       = usageerr.Usage(errors.New("specify exactly one bundle identifier: the BUNDLE-ID argument, --name (Dag bundle), or --mount-path (non-Dag bundle)"))
	errFileBundleNonDag     = usageerr.Usage(errors.New("a file bundle is a Dag bundle: create it with --name, not --mount-path"))
	errBundleVersionLimit   = usageerr.Usage(errors.New("--limit must be at least 1"))
	errNoBundleInResponse   = errors.New("the API returned no bundle")
	errNoBundleList         = errors.New("the API returned no bundle list")
	errNoVersionList        = errors.New("the API returned no bundle version list")
)

// BundleList is the wire shape for `bundle list` output.
type BundleList struct {
	Bundles []BundleInfo `json:"bundles"`
}

// BundleInfo is one bundle in `bundle list` output: the API's
// DeploymentBundle field for field, under the CLI's snake_case keys rather
// than the API's camelCase ones. A field the API left out stays out, as does
// one a create or update could not read back (BundleResult): the API always
// gives a listed bundle its id, type and times.
type BundleInfo struct {
	ID                string     `json:"id,omitempty"`
	Type              string     `json:"type,omitempty"`
	IsDagBundle       *bool      `json:"is_dag_bundle,omitempty"`
	Name              *string    `json:"name,omitempty"`
	NonDagBundleType  *string    `json:"non_dag_bundle_type,omitempty"`
	NonDagMountPath   *string    `json:"non_dag_mount_path,omitempty"`
	DagBundleIDs      *[]string  `json:"dag_bundle_ids,omitempty"`
	CurrentVersion    *string    `json:"current_version,omitempty"`
	DesiredVersion    *string    `json:"desired_version,omitempty"`
	DeletionIsPending *bool      `json:"deletion_is_pending,omitempty"`
	CreatedAt         *time.Time `json:"created_at,omitempty"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
}

// BundleVersionList is the wire shape for `bundle versions list` output.
type BundleVersionList struct {
	Versions []BundleVersionInfo `json:"versions"`
}

// BundleVersionInfo is one version of a bundle in `bundle versions list`
// output: the API's BundleVersion field for field, under the CLI's snake_case
// keys. A field the API left out stays out. The file count and total size are
// a file bundle's, and the deploy id and description a deploy bundle's.
type BundleVersionInfo struct {
	ID              string    `json:"id"`
	Version         string    `json:"version"`
	PreviousVersion *string   `json:"previous_version,omitempty"`
	Action          string    `json:"action"`
	ActionPath      *string   `json:"action_path,omitempty"`
	DeployID        *string   `json:"deploy_id,omitempty"`
	Description     *string   `json:"description,omitempty"`
	FileCount       *int      `json:"file_count,omitempty"`
	TotalSize       *int      `json:"total_size,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	CreatedByID     *string   `json:"created_by_id,omitempty"`
}

// BundleRollback is what `astro bundle rollback` did, as it publishes it
// under --output json: the bundle's id, the --name or --mount-path it was
// picked by (absent when it was picked by id), the Deployment it is on, the
// version it was returned to, and the action.
type BundleRollback struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	NonDagMountPath string `json:"non_dag_mount_path,omitempty"`
	DeploymentID    string `json:"deployment_id"`
	Version         string `json:"version"`
	Action          string `json:"action"`
}

// BundleResult is what `astro deployment bundle create` and `update` publish
// under --output json: the bundle as the change left it, in the shape bundle
// list gives it, on the Deployment it is on.
//
// The API can answer a change that succeeded without the bundle (a 204, or a
// body that is not one), and the command then reads it back. When that fails
// too, the change still happened, so the command succeeds, with what it knows
// (the name or mount path, and the id when the response had one) and a
// warning saying the rest could not be read.
type BundleResult struct {
	BundleInfo
	DeploymentID string   `json:"deployment_id"`
	Warnings     []string `json:"warnings,omitempty"`
}

// BundleRemoval is what `astro deployment bundle delete` did, as it publishes it
// under --output json, under the keys bundle list gives a bundle: its id, the
// --name or --mount-path it was picked by (absent when it was picked by id),
// the Deployment it is on, and the action.
//
// The API removes a bundle in the background (bundle list shows it with
// deletion_is_pending until it is gone), so the action of a delete is
// deletion_requested, not the deleted of a removal that is done when it
// returns. A question answered no publishes nothing: it is input.ErrAborted.
type BundleRemoval struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	NonDagMountPath string `json:"non_dag_mount_path,omitempty"`
	DeploymentID    string `json:"deployment_id"`
	Action          string `json:"action"`
}

// The actions a BundleRemoval publishes.
const (
	bundleDeletionRequested = "deletion_requested"
	bundleRolledBack        = "rolled_back"
)

func bundleToInfo(b *astrov1alpha1.DeploymentBundle) BundleInfo {
	created, updated := b.CreatedAt, b.UpdatedAt
	return BundleInfo{
		ID:                b.Id,
		Type:              string(b.Type),
		IsDagBundle:       b.IsDagBundle,
		Name:              b.Name,
		NonDagBundleType:  b.NonDagBundleType,
		NonDagMountPath:   b.NonDagMountPath,
		DagBundleIDs:      b.DagBundleIds,
		CurrentVersion:    b.CurrentVersion,
		DesiredVersion:    b.DesiredVersion,
		DeletionIsPending: b.DeletionIsPending,
		CreatedAt:         &created,
		UpdatedAt:         &updated,
	}
}

func bundleTableConfig() *output.TableConfig {
	columns := []output.Column[BundleInfo]{
		{Header: "BUNDLE ID", Value: func(b BundleInfo) string { return b.ID }},
		{Header: "IS DAG BUNDLE", Value: func(b BundleInfo) string {
			return fmt.Sprintf("%t", b.IsDagBundle != nil && *b.IsDagBundle)
		}},
		{Header: "NAME", Value: func(b BundleInfo) string {
			if b.Name == nil {
				return notApplicable
			}
			return orNA(*b.Name)
		}},
		{Header: "MOUNT PATH", Value: func(b BundleInfo) string {
			if b.NonDagMountPath == nil {
				return notApplicable
			}
			return orNA(*b.NonDagMountPath)
		}},
		{Header: "VERSION", Value: func(b BundleInfo) string { return bundleVersionCell(&b) }},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []BundleInfo { return d.(*BundleList).Bundles },
		output.WithNoResultsMsg("No bundles found on this Deployment"),
	)
}

// bundleVersionCell shows the version a bundle is set to, or the running version
// and the one rolling out when both are known and differ. A missing running
// version is not treated as a rollout, since it is often never reported.
func bundleVersionCell(b *BundleInfo) string {
	if b.DeletionIsPending != nil && *b.DeletionIsPending {
		return "Deletion pending"
	}
	if b.DesiredVersion == nil || *b.DesiredVersion == "" {
		return notApplicable
	}
	if b.CurrentVersion != nil && *b.CurrentVersion != "" && *b.CurrentVersion != *b.DesiredVersion {
		return fmt.Sprintf("%s → %s (rolling out)", *b.CurrentVersion, *b.DesiredVersion)
	}
	return *b.DesiredVersion
}

func bundleVersionTableConfig() *output.TableConfig {
	columns := []output.Column[BundleVersionInfo]{
		{Header: "VERSION", Value: func(v BundleVersionInfo) string { return v.Version }},
		{Header: "ACTION", Value: func(v BundleVersionInfo) string { return v.Action }},
		{Header: "PATH", Value: func(v BundleVersionInfo) string {
			if v.ActionPath == nil {
				return notApplicable
			}
			return orNA(*v.ActionPath)
		}},
		{Header: "DEPLOY ID", Value: func(v BundleVersionInfo) string {
			if v.DeployID == nil {
				return notApplicable
			}
			return orNA(*v.DeployID)
		}},
		{Header: "CREATED", Value: func(v BundleVersionInfo) string { return apitoken.TimeAgo(v.CreatedAt) }},
		{Header: "CREATED BY", Value: func(v BundleVersionInfo) string {
			if v.CreatedByID == nil {
				return notApplicable
			}
			return orNA(*v.CreatedByID)
		}},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []BundleVersionInfo { return d.(*BundleVersionList).Versions },
		output.WithNoResultsMsg("No versions found for this bundle"),
	)
}

// CreateBundle registers a bundle on a deployment. A DAG bundle is created with a
// name; a non-DAG bundle is created with a mount path (and optional bundle type
// plus the DAG bundles it is served alongside). It publishes the bundle it
// created, in the shape bundle list gives each one. contentType is "deploy" for
// a bundle written by deploys, or "file" for one whose files are written with
// astro bundle files.
func CreateBundle(name, mountPath, contentType, bundleType, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, r output.Emitter, errOut io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	requestType := astrov1alpha1.CreateBundleRequestType(strings.ToUpper(contentType))
	if !requestType.Valid() {
		return usageerr.Usage(fmt.Errorf("invalid bundle --type %q: must be deploy or file", contentType))
	}
	if (name == "") == (mountPath == "") {
		return errCreateBundleTarget
	}
	if name != "" && (bundleType != "" || len(dagBundleIDs) > 0) {
		return errDagBundleNonDagFlags
	}
	if requestType == astrov1alpha1.CreateBundleRequestTypeFILE && mountPath != "" {
		return errFileBundleNonDag
	}

	dep, err := GetDeployment(wsID, deploymentID, "", ToChange, nil, astroV1Client)
	if err != nil {
		return err
	}

	isDagBundle := name != ""
	request := astrov1alpha1.CreateBundleRequest{
		Type:        requestType,
		IsDagBundle: &isDagBundle,
	}
	if name != "" {
		request.Name = &name
	}
	if mountPath != "" {
		request.NonDagMountPath = &mountPath
	}
	if bundleType != "" {
		request.NonDagBundleType = &bundleType
	}
	if bundleDescription != "" {
		request.Description = &bundleDescription
	}
	if len(dagBundleIDs) > 0 {
		request.DagBundleIds = &dagBundleIDs
	}

	resp, err := astroV1Alpha1Client.CreateBundleWithResponse(httpContext.Background(), dep.OrganizationId, dep.Id, request)
	if err != nil {
		return err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	// A success with no bundle in the body (a 204, or a body that is not
	// one) still created it: its name or mount path finds it.
	result := BundleResult{DeploymentID: dep.Id}
	bundle := resp.JSON200
	if bundle == nil {
		bundle, err = findBundle(httpContext.Background(), dep.OrganizationId, dep.Id, name, mountPath, astroV1Alpha1Client)
	}
	if bundle != nil {
		result.BundleInfo = bundleToInfo(bundle)
	} else {
		result.BundleInfo = BundleInfo{Name: nonEmpty(name), NonDagMountPath: nonEmpty(mountPath)}
		result.Warnings = []string{fmt.Sprintf("the bundle was created, but it could not be read back: %v", err)}
	}
	return emitBundleResult(r, errOut, &result, "Created")
}

// UpdateBundle changes a bundle's description and, for non-DAG bundles, the set of
// DAG bundles it is served alongside. The bundle is identified by id, DAG bundle
// name, or non-DAG mount path. It publishes the bundle as the update left it.
func UpdateBundle(bundleID, bundleName, bundleMountPath, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, r output.Emitter, errOut io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return err
	}
	if bundleDescription == "" && len(dagBundleIDs) == 0 {
		return errUpdateBundleNoOp
	}

	dep, err := GetDeployment(wsID, deploymentID, "", ToChange, nil, astroV1Client)
	if err != nil {
		return err
	}

	bundleID, err = resolveBundleID(httpContext.Background(), dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return err
	}

	request := astrov1alpha1.UpdateBundleRequest{}
	if bundleDescription != "" {
		request.Description = &bundleDescription
	}
	if len(dagBundleIDs) > 0 {
		request.DagBundleIds = &dagBundleIDs
	}

	resp, err := astroV1Alpha1Client.UpdateBundleWithResponse(httpContext.Background(), dep.OrganizationId, dep.Id, bundleID, request)
	if err != nil {
		return err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	// A success with no bundle in the body still updated it; read it back.
	result := BundleResult{DeploymentID: dep.Id}
	bundle := resp.JSON200
	if bundle == nil {
		bundle, err = getBundle(dep.OrganizationId, dep.Id, bundleID, astroV1Alpha1Client)
	}
	if bundle != nil {
		result.BundleInfo = bundleToInfo(bundle)
	} else {
		result.BundleInfo = BundleInfo{ID: bundleID, Name: nonEmpty(bundleName), NonDagMountPath: nonEmpty(bundleMountPath)}
		result.Warnings = []string{fmt.Sprintf("the bundle was updated, but it could not be read back: %v", err)}
	}
	return emitBundleResult(r, errOut, &result, "Updated")
}

// emitBundleResult publishes what a create or update did. In text, the line
// it always printed, naming the bundle by its id, or by its name or mount
// path when the id could not be read back, and then each warning.
func emitBundleResult(r output.Emitter, errOut io.Writer, result *BundleResult, verb string) error {
	return r.Emit(result, func(w io.Writer) error {
		bundle := result.ID
		switch {
		case bundle != "":
		case result.Name != nil:
			bundle = *result.Name
		case result.NonDagMountPath != nil:
			bundle = *result.NonDagMountPath
		}
		if _, err := fmt.Fprintf(w, "%s bundle %s on Deployment %s\n", verb, bundle, result.DeploymentID); err != nil {
			return err
		}
		// Warnings go to the command's error writer, not the result's text:
		// colored there on a terminal, and out of piped output. json carries
		// them as "warnings".
		for _, warning := range result.Warnings {
			ansi.Warn(errOut, warning)
		}
		return nil
	})
}

// nonEmpty is s's address, or nil for "", for a field left out when empty.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// listBundlesData fetches every bundle configured on a deployment, paging through
// all results.
func listBundlesData(ctx httpContext.Context, wsID, deploymentID string, limit int, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*BundleList, error) {
	dep, err := GetDeploymentContext(ctx, wsID, deploymentID, "", ToRead, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	bundles, err := listBundlesUpTo(ctx, dep.OrganizationId, dep.Id, limit, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}

	infos := make([]BundleInfo, 0, len(bundles))
	for i := range bundles {
		infos = append(infos, bundleToInfo(&bundles[i]))
	}
	return &BundleList{Bundles: infos}, nil
}

// listAllBundles pages through every bundle on a deployment.
func listAllBundles(ctx httpContext.Context, orgID, deploymentID string, astroV1Alpha1Client astrov1alpha1.APIClient) ([]astrov1alpha1.DeploymentBundle, error) {
	return listBundlesUpTo(ctx, orgID, deploymentID, 0, astroV1Alpha1Client)
}

// listBundlesUpTo pages through the bundles on a deployment, in the order the
// API returns them (it takes no sort), and stops after limit of them (every one
// when limit is below 1) or when ctx is canceled.
func listBundlesUpTo(ctx httpContext.Context, orgID, deploymentID string, limit int, astroV1Alpha1Client astrov1alpha1.APIClient) ([]astrov1alpha1.DeploymentBundle, error) {
	return pagination.CollectUpTo(ctx, "bundles", limit, func(offset int) ([]astrov1alpha1.DeploymentBundle, int, error) {
		pageSize := bundleListLimit
		if limit > 0 {
			pageSize = min(pageSize, limit-offset)
		}
		params := &astrov1alpha1.ListBundlesParams{Limit: &pageSize, Offset: &offset}
		resp, err := astroV1Alpha1Client.ListBundlesWithResponse(pagination.Background(ctx), orgID, deploymentID, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}

		// A success with no list in the body (a 204, or a body that is not
		// one) says nothing about the bundles, so it is not taken for none.
		if resp.JSON200 == nil {
			return nil, 0, errNoBundleList
		}
		return resp.JSON200.Bundles, resp.JSON200.TotalCount, nil
	})
}

// validateBundleSelector requires exactly one of the bundle id, DAG bundle name,
// or non-DAG mount path to identify a bundle.
func validateBundleSelector(bundleID, bundleName, bundleMountPath string) error {
	selectors := 0
	for _, s := range []string{bundleID, bundleName, bundleMountPath} {
		if s != "" {
			selectors++
		}
	}
	if selectors != 1 {
		return errBundleSelector
	}
	return nil
}

// resolveBundleID turns a user-supplied bundle identifier into a bundle id. An
// explicit id is returned as-is; a DAG bundle name or a non-DAG mount path is
// resolved to its id by listing the deployment's bundles, both of which are
// unique within a deployment.
func resolveBundleID(ctx httpContext.Context, orgID, deploymentID, bundleID, bundleName, bundleMountPath string, astroV1Alpha1Client astrov1alpha1.APIClient) (string, error) {
	if bundleID != "" {
		return bundleID, nil
	}
	bundle, err := findBundle(ctx, orgID, deploymentID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return "", err
	}
	return bundle.Id, nil
}

// findBundle finds the Dag bundle named bundleName, or the non-Dag bundle
// mounted at bundleMountPath, among a deployment's bundles.
//
// A bundle whose deletion is pending is passed over: it is on its way out,
// and a bundle deleted and then created again under the same name lists
// both until the first is gone. The API keeps names and mount paths unique
// among the rest, so one is left; were there more, the newest is taken, as
// the one a create just made.
func findBundle(ctx httpContext.Context, orgID, deploymentID, bundleName, bundleMountPath string, astroV1Alpha1Client astrov1alpha1.APIClient) (*astrov1alpha1.DeploymentBundle, error) {
	bundles, err := listAllBundles(ctx, orgID, deploymentID, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}

	var found *astrov1alpha1.DeploymentBundle
	for i := range bundles {
		bundle := &bundles[i]
		if bundle.DeletionIsPending != nil && *bundle.DeletionIsPending {
			continue
		}
		isDagBundle := bundle.IsDagBundle != nil && *bundle.IsDagBundle
		matches := (bundleName != "" && isDagBundle && bundle.Name != nil && *bundle.Name == bundleName) ||
			(bundleMountPath != "" && !isDagBundle && bundle.NonDagMountPath != nil && *bundle.NonDagMountPath == bundleMountPath)
		if matches && (found == nil || bundle.CreatedAt.After(found.CreatedAt)) {
			found = bundle
		}
	}
	if found != nil {
		return found, nil
	}

	if bundleName != "" {
		return nil, fmt.Errorf("no Dag bundle named %q on deployment %s", bundleName, deploymentID)
	}
	return nil, fmt.Errorf("no non-Dag bundle mounted at %q on deployment %s", bundleMountPath, deploymentID)
}

// getBundle reads one bundle by id.
func getBundle(orgID, deploymentID, bundleID string, astroV1Alpha1Client astrov1alpha1.APIClient) (*astrov1alpha1.DeploymentBundle, error) {
	resp, err := astroV1Alpha1Client.GetBundleWithResponse(httpContext.Background(), orgID, deploymentID, bundleID)
	if err != nil {
		return nil, err
	}
	if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errNoBundleInResponse
	}
	return resp.JSON200, nil
}

// ListBundlesWithFormat prints every bundle on a deployment in the requested format.
func ListBundlesWithFormat(ctx httpContext.Context, wsID, deploymentID string, limit int, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*BundleList, error) {
			return listBundlesData(ctx, wsID, deploymentID, limit, astroV1Client, astroV1Alpha1Client)
		},
		bundleTableConfig(), r,
	)
}

// DeleteBundle removes a bundle from a deployment, after asking unless force.
// The bundle is identified by id, DAG bundle name, or non-DAG mount path. It
// publishes the deletion it requested (BundleRemoval); a declined question is
// input.ErrAborted.
func DeleteBundle(bundleID, bundleName, bundleMountPath, wsID, deploymentID string, force bool, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return err
	}

	dep, err := GetDeployment(wsID, deploymentID, "", ToChange, nil, astroV1Client)
	if err != nil {
		return err
	}

	bundleID, err = resolveBundleID(httpContext.Background(), dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	removal := BundleRemoval{ID: bundleID, Name: bundleName, NonDagMountPath: bundleMountPath, DeploymentID: dep.Id}

	if !force {
		if err := input.ConfirmOrAbort(fmt.Sprintf("Delete bundle %s from Deployment %s?", bundleID, dep.Id), input.AnsweredBy("--yes")); err != nil {
			return err
		}
	}

	resp, err := astroV1Alpha1Client.DeleteBundleWithResponse(httpContext.Background(), dep.OrganizationId, dep.Id, bundleID)
	if err != nil {
		return err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	removal.Action = bundleDeletionRequested
	return r.Emit(removal, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "Requested deletion of bundle %s from Deployment %s\n", bundleID, dep.Id)
		return err
	})
}

// listBundleVersionsData fetches up to limit of a bundle's versions, newest
// first. The bundle is identified by id, DAG bundle name, or non-DAG mount path.
func listBundleVersionsData(ctx httpContext.Context, bundleID, bundleName, bundleMountPath string, limit int, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*BundleVersionList, error) {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, errBundleVersionLimit
	}

	dep, err := GetDeploymentContext(ctx, wsID, deploymentID, "", ToRead, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	bundleID, err = resolveBundleID(ctx, dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}

	versions, err := pagination.CollectUpTo(ctx, "bundle versions", limit, func(offset int) ([]astrov1alpha1.BundleVersion, int, error) {
		pageSize := min(limit-offset, bundleVersionListLimit)
		params := &astrov1alpha1.ListBundleVersionsParams{Limit: &pageSize, Offset: &offset}
		resp, err := astroV1Alpha1Client.ListBundleVersionsWithResponse(pagination.Background(ctx), dep.OrganizationId, dep.Id, bundleID, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		if resp.JSON200 == nil {
			return nil, 0, errNoVersionList
		}
		return resp.JSON200.Versions, resp.JSON200.TotalCount, nil
	})
	if err != nil {
		return nil, err
	}

	infos := make([]BundleVersionInfo, 0, len(versions))
	for i := range versions {
		v := &versions[i]
		infos = append(infos, BundleVersionInfo{
			ID:              v.Id,
			Version:         v.Version,
			PreviousVersion: v.PreviousVersion,
			Action:          string(v.Action),
			ActionPath:      v.ActionPath,
			DeployID:        v.DeployId,
			Description:     v.Description,
			FileCount:       v.FileCount,
			TotalSize:       v.TotalSize,
			CreatedAt:       v.CreatedAt,
			CreatedByID:     v.CreatedById,
		})
	}
	return &BundleVersionList{Versions: infos}, nil
}

// ListBundleVersionsWithFormat publishes up to limit of a bundle's versions,
// newest first, in the shape the command's format asks for.
func ListBundleVersionsWithFormat(ctx httpContext.Context, bundleID, bundleName, bundleMountPath string, limit int, wsID, deploymentID string, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*BundleVersionList, error) {
			return listBundleVersionsData(ctx, bundleID, bundleName, bundleMountPath, limit, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		},
		bundleVersionTableConfig(), r,
	)
}

// RollbackBundle moves a bundle back to an earlier version, after asking
// unless force. The bundle is identified by id, DAG bundle name, or non-DAG
// mount path. It publishes the rollback (BundleRollback); a declined question
// is input.ErrAborted.
func RollbackBundle(ctx httpContext.Context, bundleID, bundleName, bundleMountPath, version, bundleDescription, wsID, deploymentID string, force bool, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return err
	}

	dep, err := GetDeploymentContext(ctx, wsID, deploymentID, "", ToChange, nil, astroV1Client)
	if err != nil {
		return err
	}

	bundleID, err = resolveBundleID(ctx, dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return err
	}
	rollback := BundleRollback{ID: bundleID, Name: bundleName, NonDagMountPath: bundleMountPath, DeploymentID: dep.Id, Version: version}

	if !force {
		if err := input.ConfirmOrAbort(fmt.Sprintf("Roll back bundle %s on Deployment %s to version %s?", bundleID, dep.Id, version), input.AnsweredBy("--yes")); err != nil {
			return err
		}
	}

	request := astrov1alpha1.RollbackBundleRequest{Version: version}
	if bundleDescription != "" {
		request.Description = &bundleDescription
	}
	resp, err := astroV1Alpha1Client.RollbackBundleWithResponse(pagination.Background(ctx), dep.OrganizationId, dep.Id, bundleID, request)
	if err != nil {
		return err
	}
	if err := astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return err
	}

	rollback.Action = bundleRolledBack
	return r.Emit(rollback, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "Rolled back bundle %s on Deployment %s to version %s\n", bundleID, dep.Id, version)
		return err
	})
}
