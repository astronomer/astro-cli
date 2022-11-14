package organization

import (
	"context"
	"io"
	"strconv"

	"github.com/pkg/errors"

	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/config"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	errInvalidOrganizationKey  = errors.New("invalid organization selection")
	errInvalidOrganizationName = errors.New("invalid organization name")
	AuthLogin                  = auth.Login
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

func listOrganizations(coreClient astrocore.CoreClient) ([]astrocore.Organization, error) {
	resp, err := coreClient.ListOrganizationsWithResponse(context.Background())
	err = astrocore.NormalizeApiError(resp.HTTPResponse, resp.Body, err)
	if err != nil {
		return nil, err
	}
	orgs := *resp.JSON200
	return orgs, nil
}

// List all Organizations
func List(out io.Writer, coreClient astrocore.CoreClient) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	or, err := listOrganizations(coreClient)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
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

func getOrganizationSelection(out io.Writer, coreClient astrocore.CoreClient) (string, error) {
	tab := printutil.Table{
		Padding:        []int{5, 44, 50},
		DynamicPadding: true,
		Header:         []string{"#", "NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}

	var c config.Context
	c, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}

	or, err := listOrganizations(coreClient)
	if err != nil {
		return "", err
	}

	deployMap := map[string]astrocore.Organization{}
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
		return "", errInvalidOrganizationKey
	}

	return selected.AuthServiceId, nil
}

// Switch switches organizations
func Switch(orgNameOrID string, astroClient astro.Client, coreClient astrocore.CoreClient, out io.Writer, shouldDisplayLoginLink bool) error {
	// get current context
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	// get auth id
	var id string
	if orgNameOrID == "" {
		id, err = getOrganizationSelection(out, coreClient)
		if err != nil {
			return err
		}
	} else {
		or, err := listOrganizations(coreClient)
		if err != nil {
			return err
		}
		for i := range or {
			if or[i].Name == orgNameOrID {
				id = or[i].AuthServiceId
			}
			if or[i].Id == orgNameOrID {
				id = or[i].AuthServiceId
			}
		}
		if id == "" {
			return errInvalidOrganizationName
		}
	}

	// log user into new organization
	err = AuthLogin(c.Domain, id, "", astroClient, coreClient, out, shouldDisplayLoginLink)
	if err != nil {
		return err
	}
	return nil
}

// Write the audit logs to the provided io.Writer.
func ExportAuditLogs(client astro.Client, out io.Writer, orgName string, earliest int) error {
	logStreamBuffer, err := client.GetOrganizationAuditLogs(orgName, earliest)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, logStreamBuffer)
	if err != nil {
		logStreamBuffer.Close()
		return err
	}
	logStreamBuffer.Close()
	return nil
}
