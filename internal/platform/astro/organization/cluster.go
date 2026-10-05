package organization

import (
	http_context "context"
	"errors"
	"io"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/pkg/output"
)

// The endpoint excludes SHARED clusters, which is what Standard Deployments run on.
const noClustersMsg = "No clusters found in this Organization. Only Dedicated and Hybrid clusters are listed."

var clusterTableConfig = output.BuildTableConfig(
	[]output.Column[ClusterInfo]{
		{Header: "NAME", Value: func(c ClusterInfo) string { return c.Name }},
		{Header: "ID", Value: func(c ClusterInfo) string { return c.ID }},
		{Header: "CLOUD PROVIDER", Value: func(c ClusterInfo) string { return c.CloudProvider }},
		{Header: "REGION", Value: func(c ClusterInfo) string { return c.Region }},
		{Header: "TYPE", Value: func(c ClusterInfo) string { return c.Type }},
		{Header: "STATUS", Value: func(c ClusterInfo) string { return c.Status }},
	},
	func(d any) []ClusterInfo { return d.(*ClusterList).Clusters },
	output.WithNoResultsMsg(noClustersMsg),
)

// ListClusters returns every cluster in the Organization, paging through the API
// as needed. Pages are sorted by name so an offset cannot skip or repeat a
// cluster where two pages meet.
func ListClusters(organizationID string, astroV1Client astrov1.APIClient) ([]astrov1.Cluster, error) {
	sorts := []astrov1.ListClustersParamsSorts{astrov1.ListClustersParamsSortsNameAsc}
	return pagination.Collect("clusters", func(offset int) ([]astrov1.Cluster, int, error) {
		pageSize := 1000
		params := &astrov1.ListClustersParams{Limit: &pageSize, Offset: &offset, Sorts: &sorts}
		resp, err := astroV1Client.ListClustersWithResponse(http_context.Background(), organizationID, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		if resp.JSON200 == nil {
			return nil, 0, errors.New("list clusters returned an empty response body")
		}
		return resp.JSON200.Clusters, resp.JSON200.TotalCount, nil
	})
}

// ListClustersData returns cluster list data for structured output
func ListClustersData(astroV1Client astrov1.APIClient) (*ClusterList, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	cs, err := ListClusters(c.Organization, astroV1Client)
	if err != nil {
		return nil, err
	}

	result := &ClusterList{
		Clusters: make([]ClusterInfo, 0, len(cs)),
	}
	for i := range cs {
		result.Clusters = append(result.Clusters, ClusterInfo{
			Name:          cs[i].Name,
			ID:            cs[i].Id,
			CloudProvider: string(cs[i].CloudProvider),
			Region:        cs[i].Region,
			Type:          string(cs[i].Type),
			Status:        string(cs[i].Status),
		})
	}

	return result, nil
}

// ListClustersWithFormat lists the Organization's clusters with the specified output format
func ListClustersWithFormat(astroV1Client astrov1.APIClient, format output.Format, out io.Writer) error {
	return output.PrintData(
		func() (*ClusterList, error) { return ListClustersData(astroV1Client) },
		clusterTableConfig, format, out,
	)
}
