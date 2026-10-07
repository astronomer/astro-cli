package organization

import (
	"bytes"
	"compress/gzip"
	http_context "context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/auth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/util"
)

const (
	AstronomerConnectionErrMsg = "cannot connect to Astronomer. Try to log in with astro login or check your internet connection and user permissions. If you are using an API Key or Token make sure your context is correct.\n\nDetails"
)

var (
	errInvalidOrganizationKey  = errors.New("invalid organization selection")
	errInvalidOrganizationName = errors.New("invalid organization name")
	Login                      = auth.Login
	CheckUserSession           = auth.CheckUserSession
	FetchDomainAuthConfig      = auth.FetchDomainAuthConfig
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

// organizationPager fetches one page of Organizations at a time for the
// pagination helpers.
func organizationPager(astroV1Client astrov1.APIClient) pagination.FetchPage[astrov1.Organization] {
	return func(offset int) ([]astrov1.Organization, int, error) {
		pageSize := 100
		params := &astrov1.ListOrganizationsParams{Limit: &pageSize, Offset: &offset}
		resp, err := astroV1Client.ListOrganizationsWithResponse(http_context.Background(), params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		return resp.JSON200.Organizations, resp.JSON200.TotalCount, nil
	}
}

// ListOrganizations returns every Organization the caller has access to, paging
// through the API as needed.
func ListOrganizations(astroV1Client astrov1.APIClient) ([]astrov1.Organization, error) {
	return pagination.Collect("organizations", organizationPager(astroV1Client))
}

func GetOrganization(orgID string, astroV1Client astrov1.APIClient) (*astrov1.Organization, error) {
	resp, err := astroV1Client.GetOrganizationWithResponse(http_context.Background(), orgID, &astrov1.GetOrganizationParams{})
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// findOrganizationByName pages through Organizations and returns as soon as it
// finds a name match, so a match on an early page avoids fetching later pages.
func findOrganizationByName(name string, astroV1Client astrov1.APIClient) (*astrov1.Organization, error) {
	return pagination.Find("organizations", organizationPager(astroV1Client), func(o astrov1.Organization) bool {
		return o.Name == name
	})
}

// ListData returns organization list data for structured output
func ListData(astroV1Client astrov1.APIClient) (*OrganizationList, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	or, err := ListOrganizations(astroV1Client)
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

// ListWithFormat lists organizations with the specified output format
func ListWithFormat(astroV1Client astrov1.APIClient, r output.Emitter) error {
	return output.PrintData(
		func() (*OrganizationList, error) { return ListData(astroV1Client) },
		organizationTableConfig, r,
	)
}

func getOrganizationSelection(out io.Writer, astroV1Client astrov1.APIClient) (*astrov1.Organization, error) {
	// Refused before anything is listed: a run that cannot ask has no use
	// for the list.
	list := picker.List{
		Header:  []string{"NAME", "ID"},
		Ask:     []input.Option{input.About("an organization"), input.AnsweredBy(NameOrIDAnswer)},
		Invalid: errInvalidOrganizationKey,
	}
	if err := input.MayAsk("\n> ", list.Ask...); err != nil {
		return nil, err
	}

	var c config.Context
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	or, err := ListOrganizations(astroV1Client)
	if err != nil {
		return nil, err
	}

	for i := range or {
		list.AddRow(c.Organization == or[i].Id, or[i].Name, or[i].Id)
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return nil, err
	}
	return &or[i], nil
}

func SwitchWithContext(domain string, targetOrg *astrov1.Organization, astroV1Client astrov1.APIClient, out io.Writer) error {
	c, _ := context.GetCurrentContext() //nolint:errcheck // falls back to the zero context in this shell code

	// reset org context
	orgProduct := "HYBRID"
	if targetOrg.Product != nil {
		orgProduct = fmt.Sprintf("%s", *targetOrg.Product) //nolint:staticcheck // renders the typed Product enum as a plain string
	}
	if err := c.SetOrganizationContext(targetOrg.Id, orgProduct); err != nil {
		return err
	}
	// need to reset all relevant keys because of https://github.com/spf13/viper/issues/1106
	if err := c.SetContextKey("token", c.Token); err != nil {
		return err
	}
	if err := c.SetContextKey("refreshtoken", c.RefreshToken); err != nil {
		return err
	}
	if err := c.SetContextKey("user_email", c.UserEmail); err != nil {
		return err
	}
	c, _ = context.GetCurrentContext() //nolint:errcheck // falls back to the zero context in this shell code
	// call check user session which will trigger workspace switcher flow
	return CheckUserSession(&c, astroV1Client, out)
}

// NameOrIDAnswer is what answers the question a switch naming no
// Organization asks.
const NameOrIDAnswer = "the organization name or ID as an argument"

// Switch makes the Organization named, by name or id, current, or, with no
// name, the one picked from a menu drawn on out. The login check that follows
// a switch picks the Organization's Workspace, and may ask for one on out too.
func Switch(orgNameOrID string, astroV1Client astrov1.APIClient, out io.Writer, shouldDisplayLoginLink bool) (*Switched, error) {
	// get current context
	c, err := context.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	// get target org
	var targetOrg *astrov1.Organization
	switch {
	case orgNameOrID == "":
		targetOrg, err = getOrganizationSelection(out, astroV1Client)
		if err != nil {
			return nil, err
		}
	case util.IsCUID(orgNameOrID):
		// Input looks like a CUID — fetch directly by ID
		targetOrg, err = GetOrganization(orgNameOrID, astroV1Client)
		if err != nil {
			return nil, err
		}
	default:
		// Input is a name — paginate through all orgs to find it
		targetOrg, err = findOrganizationByName(orgNameOrID, astroV1Client)
		if err != nil {
			return nil, err
		}
	}
	if targetOrg == nil {
		return nil, errInvalidOrganizationName
	}

	now := &Switched{Organization: OrganizationInfo{Name: targetOrg.Name, ID: targetOrg.Id, IsCurrent: true}}
	if targetOrg.Id == c.Organization {
		return now, nil
	}
	if err := SwitchWithContext(c.Domain, targetOrg, astroV1Client, out); err != nil {
		return nil, err
	}
	now.Changed = true
	return now, nil
}

// Write the audit logs to the provided io.Writer.
func ExportAuditLogs(astroV1Client astrov1.APIClient, orgName, filePath string, earliest int) error {
	var orgID string
	or, err := ListOrganizations(astroV1Client)
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
	if filePath == "" {
		orgName = strings.ReplaceAll(strings.ToLower(orgName), " ", "")

		currentTime := time.Now()
		date := "-" + currentTime.Format("20060102")
		filePath = fmt.Sprintf("%s-logs-%d-day%s%s.ndjson.gz", orgName, earliest, pluralize(earliest), date)
	}

	startDate := time.Now().AddDate(0, 0, -earliest)
	organizationAuditLogsParams := &astrov1.GetOrganizationAuditLogsParams{
		StartDate: &startDate,
	}
	resp, err := astroV1Client.GetOrganizationAuditLogsWithResponse(http_context.Background(), orgID, organizationAuditLogsParams)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	// The v1 audit-logs endpoint responds with Content-Encoding: gzip, so Go's
	// HTTP transport transparently decompresses the body and resp.Body is raw
	// NDJSON. Re-compress it here so the exported .gz file is a valid gzip, as
	// it was before the v1 migration. Guard against an already-gzipped body in
	// case the transport does not decompress it (e.g. a proxy strips the header).
	body, err := ensureGzip(resp.Body)
	if err != nil {
		return err
	}

	filePerms := 0o644
	err = os.WriteFile(filePath, body, os.FileMode(filePerms))
	if err != nil {
		return err
	}

	fmt.Println("Finished exporting logs to local GZIP file")
	return nil
}

// ensureGzip returns a gzip-compressed copy of body, unless body is already
// gzip-compressed (identified by the 0x1f 0x8b magic bytes), in which case it
// is returned unchanged.
func ensureGzip(body []byte) ([]byte, error) {
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		return body, nil
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pluralize(count int) string {
	if count > 1 {
		return "s"
	}
	return ""
}

func IsOrgHosted() bool {
	c, _ := context.GetCurrentContext() //nolint:errcheck // falls back to the zero context in this shell code
	return c.OrganizationProduct == "HOSTED"
}
