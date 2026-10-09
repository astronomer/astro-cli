package deployment

import (
	httpContext "context"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
)

// The bundle endpoints live only on the v1alpha1 public API, so bundle operations
// use the v1alpha1 client while the deployment lookup (which resolves the org and
// deployment ids) stays on v1. Collapse onto a single client once the bundle API
// reaches v1.

const bundleListLimit = 1000

var (
	errCreateBundleTarget   = errors.New("specify exactly one of --name (Dag bundle) or --mount-path (non-Dag bundle)")
	errDagBundleNonDagFlags = errors.New("--bundle-type and --dag-bundle-ids are only valid for non-Dag bundles (--mount-path)")
	errUpdateBundleNoOp     = errors.New("specify at least one of --description or --dag-bundle-ids")
	errBundleSelector       = errors.New("specify exactly one bundle identifier: the BUNDLE-ID argument, --name (Dag bundle), or --mount-path (non-Dag bundle)")
)

// BundleList is the wire shape for `bundle list` output.
type BundleList struct {
	Bundles []BundleInfo `json:"bundles"`
}

// BundleInfo is one bundle in `bundle list` output: the API's
// DeploymentBundle field for field, under the CLI's snake_case keys rather
// than the API's camelCase ones. A field the API left out stays out.
type BundleInfo struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	IsDagBundle       *bool     `json:"is_dag_bundle,omitempty"`
	Name              *string   `json:"name,omitempty"`
	NonDagBundleType  *string   `json:"non_dag_bundle_type,omitempty"`
	NonDagMountPath   *string   `json:"non_dag_mount_path,omitempty"`
	DagBundleIDs      *[]string `json:"dag_bundle_ids,omitempty"`
	CurrentVersion    *string   `json:"current_version,omitempty"`
	DesiredVersion    *string   `json:"desired_version,omitempty"`
	DeletionIsPending *bool     `json:"deletion_is_pending,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// BundleRemoval is what `astro deployment bundle delete` did, as it publishes it
// under --output json: the bundle's id, under the key bundle list gives it,
// the Deployment it was on, and the action `astro deployment delete` and the
// token removals publish.
type BundleRemoval struct {
	ID           string `json:"id"`
	DeploymentID string `json:"deployment_id"`
	Action       string `json:"action"`
}

func bundleToInfo(b *astrov1alpha1.DeploymentBundle) BundleInfo {
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
		CreatedAt:         b.CreatedAt,
		UpdatedAt:         b.UpdatedAt,
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
		{Header: "CURRENT VERSION", Value: func(b BundleInfo) string {
			if b.CurrentVersion == nil {
				return notApplicable
			}
			return orNA(*b.CurrentVersion)
		}},
		{Header: "DESIRED VERSION", Value: func(b BundleInfo) string {
			if b.DesiredVersion == nil {
				return notApplicable
			}
			return orNA(*b.DesiredVersion)
		}},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []BundleInfo { return d.(*BundleList).Bundles },
		output.WithNoResultsMsg("No bundles found on this deployment"),
	)
}

// CreateBundle registers a bundle on a deployment. A DAG bundle is created with a
// name; a non-DAG bundle is created with a mount path (and optional bundle type
// plus the DAG bundles it is served alongside). It publishes the bundle it
// created, in the shape bundle list gives each one.
func CreateBundle(name, mountPath, bundleType, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if (name == "") == (mountPath == "") {
		return errCreateBundleTarget
	}
	if name != "" && (bundleType != "" || len(dagBundleIDs) > 0) {
		return errDagBundleNonDagFlags
	}

	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
	if err != nil {
		return err
	}

	isDagBundle := name != ""
	request := astrov1alpha1.CreateBundleRequest{
		Type:        astrov1alpha1.CreateBundleRequestTypeDEPLOY,
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

	return r.Emit(bundleToInfo(resp.JSON200), func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "Created bundle %s on deployment %s\n", resp.JSON200.Id, dep.Id)
		return err
	})
}

// UpdateBundle changes a bundle's description and, for non-DAG bundles, the set of
// DAG bundles it is served alongside. The bundle is identified by id, DAG bundle
// name, or non-DAG mount path. It publishes the bundle as the update left it.
func UpdateBundle(bundleID, bundleName, bundleMountPath, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return err
	}
	if bundleDescription == "" && len(dagBundleIDs) == 0 {
		return errUpdateBundleNoOp
	}

	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
	if err != nil {
		return err
	}

	bundleID, err = resolveBundleID(dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
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

	return r.Emit(bundleToInfo(resp.JSON200), func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "Updated bundle %s on deployment %s\n", bundleID, dep.Id)
		return err
	})
}

// listBundlesData fetches every bundle configured on a deployment, paging through
// all results.
func listBundlesData(wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*BundleList, error) {
	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	bundles, err := listAllBundles(dep.OrganizationId, dep.Id, astroV1Alpha1Client)
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
func listAllBundles(orgID, deploymentID string, astroV1Alpha1Client astrov1alpha1.APIClient) ([]astrov1alpha1.DeploymentBundle, error) {
	var bundles []astrov1alpha1.DeploymentBundle
	limit := bundleListLimit
	for {
		offset := len(bundles)
		params := &astrov1alpha1.ListBundlesParams{Limit: &limit, Offset: &offset}
		resp, err := astroV1Alpha1Client.ListBundlesWithResponse(httpContext.Background(), orgID, deploymentID, params)
		if err != nil {
			return nil, err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}

		bundles = append(bundles, resp.JSON200.Bundles...)
		if len(resp.JSON200.Bundles) == 0 || len(bundles) >= resp.JSON200.TotalCount {
			break
		}
	}

	return bundles, nil
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
func resolveBundleID(orgID, deploymentID, bundleID, bundleName, bundleMountPath string, astroV1Alpha1Client astrov1alpha1.APIClient) (string, error) {
	if bundleID != "" {
		return bundleID, nil
	}

	bundles, err := listAllBundles(orgID, deploymentID, astroV1Alpha1Client)
	if err != nil {
		return "", err
	}

	for i := range bundles {
		bundle := &bundles[i]
		isDagBundle := bundle.IsDagBundle != nil && *bundle.IsDagBundle
		if bundleName != "" && isDagBundle && bundle.Name != nil && *bundle.Name == bundleName {
			return bundle.Id, nil
		}
		if bundleMountPath != "" && !isDagBundle && bundle.NonDagMountPath != nil && *bundle.NonDagMountPath == bundleMountPath {
			return bundle.Id, nil
		}
	}

	if bundleName != "" {
		return "", fmt.Errorf("no Dag bundle named %q on deployment %s", bundleName, deploymentID)
	}
	return "", fmt.Errorf("no non-Dag bundle mounted at %q on deployment %s", bundleMountPath, deploymentID)
}

// ListBundlesWithFormat prints every bundle on a deployment in the requested format.
func ListBundlesWithFormat(wsID, deploymentID string, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*BundleList, error) {
			return listBundlesData(wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		},
		bundleTableConfig(), r,
	)
}

// DeleteBundle removes a bundle from a deployment, after asking unless force.
// The bundle is identified by id, DAG bundle name, or non-DAG mount path. It
// publishes what it deleted through r, and says on out when the question was
// declined, which under --output json it never is: the question is refused
// there, so a run passes --yes.
func DeleteBundle(bundleID, bundleName, bundleMountPath, wsID, deploymentID string, force bool, out io.Writer, r output.Emitter, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return err
	}

	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
	if err != nil {
		return err
	}

	bundleID, err = resolveBundleID(dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return err
	}

	if !force {
		confirmed, err := input.Confirm(fmt.Sprintf("Are you sure you want to delete bundle %s from deployment %s?", bundleID, dep.Id), input.AnsweredBy("--yes"))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "Canceling bundle deletion")
			return nil
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

	removal := BundleRemoval{ID: bundleID, DeploymentID: dep.Id, Action: actionDeleted}
	return r.Emit(removal, func(w io.Writer) error {
		_, err := fmt.Fprintf(w, "Deleted bundle %s from deployment %s\n", bundleID, dep.Id)
		return err
	})
}
