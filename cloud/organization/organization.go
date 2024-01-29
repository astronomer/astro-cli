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

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

func ListOrganizations(platformCoreClient astroplatformcore.CoreClient) ([]astroplatformcore.Organization, error) {
	organizationListParams := &astroplatformcore.ListOrganizationsParams{}
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

// List all Organizations
func List(out io.Writer, platformCoreClient astroplatformcore.CoreClient) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	or, err := ListOrganizations(platformCoreClient)
	if err != nil {
		return fmt.Errorf(AstronomerConnectionErrMsg+"%w", err)
	}
	tab := newTableOut()
	for i := range or {
		name := or[i].Name
		organizationID := or[i].Id

		var color bool

		if c.Organization == or[i].Id {
			color = true
		}
		tab.AddRow([]string{name, organizationID}, color)
	}

	tab.Print(out)

	return nil
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
	_ = c.SetOrganizationContext(targetOrg.Id, targetOrg.Name, orgProduct)
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
	if orgNameOrID == "" {
		targetOrg, err = getOrganizationSelection(out, platformCoreClient)
		if err != nil {
			return err
		}
	} else {
		or, err := ListOrganizations(platformCoreClient)
		if err != nil {
			return err
		}
		for i := range or {
			if or[i].Name == orgNameOrID {
				targetOrg = &or[i]
			}
			if or[i].Id == orgNameOrID {
				targetOrg = &or[i]
			}
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
	if orgName == "" {
		// get current context
		c, err := context.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
		orgName = c.OrganizationShortName
	} else {
		or, err := ListOrganizations(platformCoreClient)
		if err != nil {
			return err
		}
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
		orgShortName := strings.ReplaceAll(strings.ToLower(orgName), " ", "")

		currentTime := time.Now()
		date := "-" + currentTime.Format("20060102")
		filePath = fmt.Sprintf("%s-logs-%d-day%s%s.ndjson.gz", orgShortName, earliest, pluralize(earliest), date)
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

	err = os.WriteFile(filePath, resp.Body, os.ModePerm)
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
