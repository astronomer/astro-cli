package organization

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/pkg/errors"

	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	httpClient = httputil.NewHTTPClient()

	errInvalidOrganizationKey  = errors.New("invalid organization selection")
	errInvalidOrganizationName = errors.New("invalid organization name")
	AuthLogin                  = auth.Login
)

type OrgRes struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	AuthServiceID string   `json:"authServiceId"`
	Domains       []string `json:"domains"`
	ShortName     string   `json:"shortName"`
	CreatedAt     string   `json:"createdAt"`
}

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

func listOrganizations(c *config.Context) ([]OrgRes, error) {
	var orgDomain string
	if c.Domain == "localhost" {
		orgDomain = config.CFG.LocalCore.GetString() + "/organizations"
	} else {
		orgDomain = "https://api." + c.Domain + "/v1alpha1/organizations"
	}
	authToken := c.Token
	ctx := context.Background()
	doOptions := &httputil.DoOptions{
		Context: ctx,
		Headers: map[string]string{"authorization": authToken},
	}
	res, err := httpClient.Do("GET", orgDomain, doOptions)
	if err != nil {
		return []OrgRes{}, fmt.Errorf("could not retrieve organization list: %w", err)
	}
	var orgResponse []OrgRes
	err = json.NewDecoder(res.Body).Decode(&orgResponse)
	if err != nil {
		return []OrgRes{}, fmt.Errorf("cannot decode organization list response: %w", err)
	}

	return orgResponse, err
}

// List all Organizations
func List(out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	or, err := listOrganizations(&c)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	tab := newTableOut()
	for i := range or {
		name := or[i].Name
		organizationID := or[i].ID

		var color bool

		if c.Organization == or[i].ID {
			color = true
		}
		tab.AddRow([]string{name, organizationID}, color)
	}

	tab.Print(out)

	return nil
}

func getOrganizationSelection(out io.Writer) (string, error) {
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

	or, err := listOrganizations(&c)
	if err != nil {
		return "", err
	}

	deployMap := map[string]OrgRes{}
	for i := range or {
		index := i + 1

		color := c.Organization == or[i].ID
		tab.AddRow([]string{strconv.Itoa(index), or[i].Name, or[i].ID}, color)

		deployMap[strconv.Itoa(index)] = or[i]
	}
	tab.Print(out)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return "", errInvalidOrganizationKey
	}

	return selected.AuthServiceID, nil
}

// Switch switches organizations
func Switch(orgName string, client astro.Client, out io.Writer) error {
	// get current context
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	// get auth id
	var id string
	if orgName == "" {
		id, err = getOrganizationSelection(out)
		if err != nil {
			return err
		}
	} else {
		or, err := listOrganizations(&c)
		if err != nil {
			return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
		}
		for i := range or {
			if or[i].Name == orgName {
				id = or[i].AuthServiceID
			}
		}
		if id == "" {
			return errInvalidOrganizationName
		}
	}

	// log user into new organization
	err = AuthLogin(c.Domain, id, client, out, false, false)
	if err != nil {
		return err
	}
	return nil
}
