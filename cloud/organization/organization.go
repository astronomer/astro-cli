package organization

import (
	http_context "context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	AstronomerConnectionErrMsg = "cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions. If you are using an API Key or Token make sure your context is correct.\n\nDetails"
)

var (
	errInvalidOrganizationKey   = errors.New("invalid organization selection")
	errInvalidOrganizationName  = errors.New("invalid organization name")
	Login                       = auth.Login
	CheckUserSession            = auth.CheckUserSession
	FetchDomainAuthConfig       = auth.FetchDomainAuthConfig
	switchedOrganizationMessage = "\nSuccessfully switched organization"
)

var organizationTableConfig = output.BuildTableConfig(
	[]output.Column[OrganizationInfo]{
		{Header: "NAME", Value: func(o OrganizationInfo) string { return o.Name }},
		{Header: "ID", Value: func(o OrganizationInfo) string { return o.ID }},
	},
	func(d any) []OrganizationInfo { return d.(*OrganizationList).Organizations },
	output.WithColorRow(func(o OrganizationInfo) bool { return o.IsCurrent }, [2]string{"\033[1;32m", "\033[0m"}),
	output.WithPadding([]int{44, 50}),
)

func ListOrganizations(platformCoreClient astroplatformcore.CoreClient) ([]astroplatformcore.Organization, error) {
	limit := 100
	organizationListParams := &astroplatformcore.ListOrganizationsParams{
		Limit: &limit,
	}
	resp, err := platformCoreClient.ListOrganizationsWithResponse(http_context.Background(), organizationListParams)
	if err != nil {
		return nil, err
	}
	err = astroplatformcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	orgsPaginated := *resp.JSON200
	orgs := orgsPaginated.Organizations
	return orgs, nil
}

func GetOrganization(orgID string, platformCoreClient astroplatformcore.CoreClient) (*astroplatformcore.Organization, error) {
	resp, err := platformCoreClient.GetOrganizationWithResponse(http_context.Background(), orgID, &astroplatformcore.GetOrganizationParams{})
	if err != nil {
		return nil, err
	}
	err = astroplatformcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// isCUID returns true if s is a valid CUID (25 chars: 'c' + 24 lowercase alphanumeric).
func isCUID(s string) bool {
	if len(s) != 25 || s[0] != 'c' {
		return false
	}
	for _, ch := range s[1:] {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

func findOrganizationByName(name string, platformCoreClient astroplatformcore.CoreClient) (*astroplatformcore.Organization, error) {
	pageSize := 100
	offset := 0
	for {
		params := &astroplatformcore.ListOrganizationsParams{Limit: &pageSize, Offset: &offset}
		resp, err := platformCoreClient.ListOrganizationsWithResponse(http_context.Background(), params)
		if err != nil {
			return nil, err
		}
		err = astroplatformcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
		if err != nil {
			return nil, err
		}
		paginated := *resp.JSON200
		for i := range paginated.Organizations {
			if paginated.Organizations[i].Name == name {
				return &paginated.Organizations[i], nil
			}
		}
		offset += pageSize
		if offset >= paginated.TotalCount {
			break
		}
	}
	return nil, nil
}

// ListData returns organization list data for structured output
func ListData(platformCoreClient astroplatformcore.CoreClient) (*OrganizationList, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	or, err := ListOrganizations(platformCoreClient)
	if err != nil {
		return nil, fmt.Errorf(AstronomerConnectionErrMsg+"%w", err)
	}

	result := &OrganizationList{
		Organizations: make([]OrganizationInfo, 0, len(or)),
	}

	for i := range or {
		isCurrent := c.Organization == or[i].Id
		result.Organizations = append(result.Organizations, OrganizationInfo{
			Name:      or[i].Name,
			ID:        or[i].Id,
			IsCurrent: isCurrent,
		})
	}

	return result, nil
}

// List all organizations
func List(out io.Writer, platformCoreClient astroplatformcore.CoreClient) error {
	return ListWithFormat(platformCoreClient, output.FormatTable, "", out)
}

// ListWithFormat lists organizations with the specified output format
func ListWithFormat(platformCoreClient astroplatformcore.CoreClient, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*OrganizationList, error) { return ListData(platformCoreClient) },
		organizationTableConfig, format, tmpl, out,
	)
}

func getOrganizationSelection(out io.Writer, platformCoreClient astroplatformcore.CoreClient) (*astroplatformcore.Organization, error) {
	tab := printutil.Table{
		Padding:        []int{5, 44, 50},
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}

	var c config.Context
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	or, err := ListOrganizations(platformCoreClient)
	if err != nil {
		return nil, err
	}

	deployMap := map[string]astroplatformcore.Organization{}
	for i := range or {
		index := i + 1

		color := c.Organization == or[i].Id
		tab.AddRow([]string{strconv.Itoa(index), or[i].Name, or[i].Id}, color)

		deployMap[strconv.Itoa(index)] = or[i]
	}
	tab.Print(out)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return nil, errInvalidOrganizationKey
	}

	return &selected, nil
}

func SwitchWithContext(domain string, targetOrg *astroplatformcore.Organization, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
	c, _ := context.GetCurrentContext()

	// reset org context
	orgProduct := "HYBRID"
	if targetOrg.Product != nil {
		orgProduct = fmt.Sprintf("%s", *targetOrg.Product) //nolint
	}
	_ = c.SetOrganizationContext(targetOrg.Id, orgProduct)
	// need to reset all relevant keys because of https://github.com/spf13/viper/issues/1106 :shrug
	_ = c.SetContextKey("token", c.Token)
	_ = c.SetContextKey("refreshtoken", c.RefreshToken)
	_ = c.SetContextKey("user_email", c.UserEmail)
	c, _ = context.GetCurrentContext()
	// call check user session which will trigger workspace switcher flow
	err := CheckUserSession(&c, coreClient, platformCoreClient, out)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, switchedOrganizationMessage)
	return nil
}

// Switch switches organizations
func Switch(orgNameOrID string, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
	// get current context
	c, err := context.GetCurrentContext()
	if err != nil {
		return err
	}

	// get target org
	var targetOrg *astroplatformcore.Organization
	switch {
	case orgNameOrID == "":
		targetOrg, err = getOrganizationSelection(out, platformCoreClient)
		if err != nil {
			return err
		}
	case isCUID(orgNameOrID):
		// Input looks like a CUID — fetch directly by ID
		targetOrg, err = GetOrganization(orgNameOrID, platformCoreClient)
		if err != nil {
			return err
		}
	default:
		// Input is a name — paginate through all orgs to find it
		targetOrg, err = findOrganizationByName(orgNameOrID, platformCoreClient)
		if err != nil {
			return err
		}
	}
	if targetOrg == nil {
		return errInvalidOrganizationName
	}

	if targetOrg.Id == c.Organization {
		fmt.Fprintln(out, "You selected the same organization as the current one. No switch was made")
		return nil
	}
	return SwitchWithContext(c.Domain, targetOrg, coreClient, platformCoreClient, out)
}

// Write the audit logs to the provided io.Writer.
func ExportAuditLogs(coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient, orgName, filePath string, earliest int) error {
	var orgID string
	or, err := ListOrganizations(platformCoreClient)
	if err != nil {
		return err
	}
	if orgName == "" {
		// get current context
		c, err := context.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
		for i := range or {
			if orgID == or[i].Id {
				orgName = or[i].Name
				break
			}
		}
	} else {
		for i := range or {
			if orgName == or[i].Name {
				orgID = or[i].Id
				break
			}
		}
		if orgID == "" {
			return errInvalidOrganizationName
		}
	}
	earliestString := fmt.Sprint(earliest)
	if filePath == "" {
		orgName = strings.ReplaceAll(strings.ToLower(orgName), " ", "")

		currentTime := time.Now()
		date := "-" + currentTime.Format("20060102")
		filePath = fmt.Sprintf("%s-logs-%d-day%s%s.ndjson.gz", orgName, earliest, pluralize(earliest), date)
	}

	organizationAuditLogsParams := &astrocore.GetOrganizationAuditLogsParams{
		Earliest: &earliestString,
	}
	resp, err := coreClient.GetOrganizationAuditLogsWithResponse(http_context.Background(), orgID, organizationAuditLogsParams)
	if err != nil {
		return err
	}
	err = astroplatformcore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	filePerms := 0o644
	err = os.WriteFile(filePath, resp.Body, os.FileMode(filePerms))
	if err != nil {
		return err
	}

	fmt.Println("Finished exporting logs to local GZIP file")
	return nil
}

func pluralize(count int) string {
	if count > 1 {
		return "s"
	}
	return ""
}

func IsOrgHosted() bool {
	c, _ := context.GetCurrentContext()
	return c.OrganizationProduct == "HOSTED"
}

func ListClusters(organizationID string, platformCoreClient astroplatformcore.CoreClient) ([]astroplatformcore.Cluster, error) {
	limit := 1000
	clusterListParams := &astroplatformcore.ListClustersParams{
		Limit: &limit,
	}
	resp, err := platformCoreClient.ListClustersWithResponse(http_context.Background(), organizationID, clusterListParams)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	csPaginated := *resp.JSON200
	cs := csPaginated.Clusters

	return cs, nil
}
