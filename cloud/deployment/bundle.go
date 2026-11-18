package deployment

import (
	httpContext "context"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
)

// The bundle endpoints live only on the v1alpha1 public API, so bundle operations
// use the v1alpha1 client while the deployment lookup (which resolves the org and
// deployment ids) stays on v1. Collapse onto a single client once the bundle API
// reaches v1.

const (
	bundleListLimit          = 1000
	bundleVersionListLimit   = 1000
	shortBundleVersionLength = 12
)

var (
	errCreateBundleTarget   = errors.New("specify exactly one of --name (Dag bundle) or --mount-path (non-Dag bundle)")
	errDagBundleNonDagFlags = errors.New("--bundle-type and --dag-bundle-ids are only valid for non-Dag bundles (--mount-path)")
	errUpdateBundleNoOp     = errors.New("specify at least one of --description or --dag-bundle-ids")
	errBundleSelector       = errors.New("specify exactly one bundle identifier: the BUNDLE-ID argument, --name (Dag bundle), or --mount-path (non-Dag bundle)")
	errFileBundleNonDag     = errors.New("a file bundle is a Dag bundle: create it with --name, not --mount-path")
	errBundleVersionLimit   = errors.New("--limit must be at least 1")
)

// BundleList is the wire shape for `bundle list` output.
type BundleList struct {
	Bundles []astrov1alpha1.DeploymentBundle `json:"bundles"`
}

// BundleVersionList is the wire shape for `bundle versions list` output.
type BundleVersionList struct {
	Versions []astrov1alpha1.BundleVersion `json:"versions"`
}

func bundleTableConfig() *output.TableConfig {
	columns := []output.Column[astrov1alpha1.DeploymentBundle]{
		{Header: "BUNDLE ID", Value: func(b astrov1alpha1.DeploymentBundle) string { return b.Id }},
		{Header: "IS DAG BUNDLE", Value: func(b astrov1alpha1.DeploymentBundle) string {
			return fmt.Sprintf("%t", b.IsDagBundle != nil && *b.IsDagBundle)
		}},
		{Header: "NAME", Value: func(b astrov1alpha1.DeploymentBundle) string {
			if b.Name == nil {
				return notApplicable
			}
			return orNA(*b.Name)
		}},
		{Header: "MOUNT PATH", Value: func(b astrov1alpha1.DeploymentBundle) string {
			if b.NonDagMountPath == nil {
				return notApplicable
			}
			return orNA(*b.NonDagMountPath)
		}},
		{Header: "VERSION", Value: func(b astrov1alpha1.DeploymentBundle) string { return bundleVersionCell(&b) }},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []astrov1alpha1.DeploymentBundle { return d.(*BundleList).Bundles },
		output.WithNoResultsMsg("No bundles found on this deployment"),
	)
}

// bundleVersionCell shows the version a bundle is set to, or the running version
// and the one rolling out when both are known and differ. A missing running
// version is not treated as a rollout, since it is often never reported.
func bundleVersionCell(b *astrov1alpha1.DeploymentBundle) string {
	if b.DeletionIsPending != nil && *b.DeletionIsPending {
		return "Deletion pending"
	}
	if b.DesiredVersion == nil || *b.DesiredVersion == "" {
		return notApplicable
	}
	if b.CurrentVersion != nil && *b.CurrentVersion != "" && *b.CurrentVersion != *b.DesiredVersion {
		return fmt.Sprintf("%s → %s (rolling out)", shortBundleVersion(b.Type, *b.CurrentVersion), shortBundleVersion(b.Type, *b.DesiredVersion))
	}
	return *b.DesiredVersion
}

func bundleVersionTableConfig() *output.TableConfig {
	columns := []output.Column[astrov1alpha1.BundleVersion]{
		{Header: "VERSION", Value: func(v astrov1alpha1.BundleVersion) string { return v.Version }},
		{Header: "ACTION", Value: func(v astrov1alpha1.BundleVersion) string { return string(v.Action) }},
		{Header: "PATH", Value: func(v astrov1alpha1.BundleVersion) string {
			if v.ActionPath == nil {
				return notApplicable
			}
			return orNA(*v.ActionPath)
		}},
		{Header: "DEPLOY ID", Value: func(v astrov1alpha1.BundleVersion) string {
			if v.DeployId == nil {
				return notApplicable
			}
			return orNA(*v.DeployId)
		}},
		{Header: "CREATED", Value: func(v astrov1alpha1.BundleVersion) string { return TimeAgo(v.CreatedAt) }},
		{Header: "CREATED BY", Value: func(v astrov1alpha1.BundleVersion) string {
			if v.CreatedById == nil {
				return notApplicable
			}
			return orNA(*v.CreatedById)
		}},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []astrov1alpha1.BundleVersion { return d.(*BundleVersionList).Versions },
		output.WithNoResultsMsg("No versions found for this bundle"),
	)
}

// shortBundleVersion abbreviates a file bundle's version, which is a sha256 digest.
func shortBundleVersion(bundleType astrov1alpha1.DeploymentBundleType, version string) string {
	if bundleType == astrov1alpha1.DeploymentBundleTypeFILE && len(version) > shortBundleVersionLength {
		return version[:shortBundleVersionLength]
	}
	return version
}

// CreateBundle registers a bundle on a deployment. A DAG bundle is created with a
// name; a non-DAG bundle is created with a mount path (and optional bundle type
// plus the DAG bundles it is served alongside). contentType is "deploy" for a
// bundle written by deploys, or "file" for one whose files are written with
// `astro bundle files`.
func CreateBundle(name, mountPath, contentType, bundleType, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	requestType := astrov1alpha1.CreateBundleRequestType(strings.ToUpper(contentType))
	if !requestType.Valid() {
		return fmt.Errorf("invalid bundle --type %q: must be deploy or file", contentType)
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

	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
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

	fmt.Fprintf(out, "Created bundle %s on deployment %s\n", resp.JSON200.Id, dep.Id)
	return nil
}

// UpdateBundle changes a bundle's description and, for non-DAG bundles, the set of
// DAG bundles it is served alongside. The bundle is identified by id, DAG bundle
// name, or non-DAG mount path.
func UpdateBundle(bundleID, bundleName, bundleMountPath, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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

	fmt.Fprintf(out, "Updated bundle %s on deployment %s\n", bundleID, dep.Id)
	return nil
}

// ListBundlesData fetches every bundle configured on a deployment, paging through
// all results.
func ListBundlesData(wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*BundleList, error) {
	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	bundles, err := listAllBundles(dep.OrganizationId, dep.Id, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}

	return &BundleList{Bundles: bundles}, nil
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
func ListBundlesWithFormat(wsID, deploymentID string, format output.Format, tmpl string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*BundleList, error) {
			return ListBundlesData(wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		},
		bundleTableConfig(), format, tmpl, out,
	)
}

// DeleteBundle removes a bundle from a deployment. The bundle is identified by id,
// DAG bundle name, or non-DAG mount path.
func DeleteBundle(bundleID, bundleName, bundleMountPath, wsID, deploymentID string, force bool, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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
		confirmed, _ := input.Confirm(fmt.Sprintf("Are you sure you want to delete bundle %s from deployment %s?", bundleID, dep.Id))
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

	fmt.Fprintf(out, "Deleted bundle %s from deployment %s\n", bundleID, dep.Id)
	return nil
}

// ListBundleVersionsData fetches up to limit of a bundle's versions, newest first.
// The bundle is identified by id, DAG bundle name, or non-DAG mount path.
func ListBundleVersionsData(bundleID, bundleName, bundleMountPath string, limit int, wsID, deploymentID string, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) (*BundleVersionList, error) {
	if err := validateBundleSelector(bundleID, bundleName, bundleMountPath); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, errBundleVersionLimit
	}

	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	bundleID, err = resolveBundleID(dep.OrganizationId, dep.Id, bundleID, bundleName, bundleMountPath, astroV1Alpha1Client)
	if err != nil {
		return nil, err
	}

	var versions []astrov1alpha1.BundleVersion
	for len(versions) < limit {
		offset := len(versions)
		pageLimit := min(limit-offset, bundleVersionListLimit)
		params := &astrov1alpha1.ListBundleVersionsParams{Limit: &pageLimit, Offset: &offset}
		resp, err := astroV1Alpha1Client.ListBundleVersionsWithResponse(httpContext.Background(), dep.OrganizationId, dep.Id, bundleID, params)
		if err != nil {
			return nil, err
		}
		err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}

		versions = append(versions, resp.JSON200.Versions...)
		if len(resp.JSON200.Versions) == 0 || len(versions) >= resp.JSON200.TotalCount {
			break
		}
	}

	return &BundleVersionList{Versions: versions}, nil
}

// ListBundleVersionsWithFormat prints up to limit of a bundle's versions in the requested format.
func ListBundleVersionsWithFormat(bundleID, bundleName, bundleMountPath string, limit int, wsID, deploymentID string, format output.Format, tmpl string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	return output.PrintData(
		func() (*BundleVersionList, error) {
			return ListBundleVersionsData(bundleID, bundleName, bundleMountPath, limit, wsID, deploymentID, astroV1Client, astroV1Alpha1Client)
		},
		bundleVersionTableConfig(), format, tmpl, out,
	)
}

// RollbackBundle moves a bundle back to an earlier version. The bundle is
// identified by id, DAG bundle name, or non-DAG mount path.
func RollbackBundle(bundleID, bundleName, bundleMountPath, version, bundleDescription, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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

	request := astrov1alpha1.RollbackBundleRequest{Version: version}
	if bundleDescription != "" {
		request.Description = &bundleDescription
	}

	resp, err := astroV1Alpha1Client.RollbackBundleWithResponse(httpContext.Background(), dep.OrganizationId, dep.Id, bundleID, request)
	if err != nil {
		return err
	}
	err = astrov1alpha1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Rolled back bundle %s on deployment %s to version %s\n", bundleID, dep.Id, version)
	return nil
}
