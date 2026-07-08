package deployment

import (
	httpContext "context"
	"fmt"
	"io"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
)

// The bundle endpoints live only on the v1alpha1 public API, so bundle operations
// use the v1alpha1 client while the deployment lookup (which resolves the org and
// deployment ids) stays on v1. Collapse onto a single client once the bundle API
// reaches v1.

const bundleListLimit = 1000

var (
	errCreateBundleTarget   = errors.New("specify exactly one of --name (DAG bundle) or --mount-path (non-DAG bundle)")
	errDagBundleNonDagFlags = errors.New("--bundle-type and --dag-bundle-ids are only valid for non-DAG bundles (--mount-path)")
	errUpdateBundleNoOp     = errors.New("specify at least one of --description or --dag-bundle-ids")
)

// BundleList is the wire shape for `bundle list` output.
type BundleList struct {
	Bundles []astrov1alpha1.DeploymentBundle `json:"bundles"`
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
		{Header: "CURRENT VERSION", Value: func(b astrov1alpha1.DeploymentBundle) string {
			if b.CurrentVersion == nil {
				return notApplicable
			}
			return orNA(*b.CurrentVersion)
		}},
		{Header: "DESIRED VERSION", Value: func(b astrov1alpha1.DeploymentBundle) string {
			if b.DesiredVersion == nil {
				return notApplicable
			}
			return orNA(*b.DesiredVersion)
		}},
	}
	return output.BuildTableConfig(
		columns,
		func(d any) []astrov1alpha1.DeploymentBundle { return d.(*BundleList).Bundles },
		output.WithNoResultsMsg("No bundles found on this deployment"),
	)
}

// CreateBundle registers a bundle on a deployment. A DAG bundle is created with a
// name; a non-DAG bundle is created with a mount path (and optional bundle type
// plus the DAG bundles it is served alongside).
func CreateBundle(name, mountPath, bundleType, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
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

	fmt.Fprintf(out, "Created bundle %s on deployment %s\n", resp.JSON200.Id, dep.Id)
	return nil
}

// UpdateBundle changes a bundle's description and, for non-DAG bundles, the set of
// DAG bundles it is served alongside.
func UpdateBundle(bundleID, bundleDescription string, dagBundleIDs []string, wsID, deploymentID string, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	if bundleDescription == "" && len(dagBundleIDs) == 0 {
		return errUpdateBundleNoOp
	}

	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
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

	var bundles []astrov1alpha1.DeploymentBundle
	limit := bundleListLimit
	for {
		offset := len(bundles)
		params := &astrov1alpha1.ListBundlesParams{Limit: &limit, Offset: &offset}
		resp, err := astroV1Alpha1Client.ListBundlesWithResponse(httpContext.Background(), dep.OrganizationId, dep.Id, params)
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

	return &BundleList{Bundles: bundles}, nil
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

// DeleteBundle removes a bundle from a deployment.
func DeleteBundle(bundleID, wsID, deploymentID string, force bool, out io.Writer, astroV1Client astrov1.APIClient, astroV1Alpha1Client astrov1alpha1.APIClient) error {
	dep, err := GetDeployment(wsID, deploymentID, "", false, nil, astroV1Client)
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
