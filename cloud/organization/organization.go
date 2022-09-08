package organization

import (
	"io"
	"encoding/json"
	"strconv"
	// "net/url"
	"fmt"
	http_context "context"

	"github.com/pkg/errors"

	astro "github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/auth"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
)

var (
	httpClient = httputil.NewHTTPClient()

	errInvalidOrganizationKey = errors.New("invalid organization selection")
	authLogin = auth.Login
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
}

func ListOrganizations(c config.Context) ([]astro.Organization, error) {
	orgDomain := "https://api.astronomer-dev.io/v1alpha1/organizations"
	authToken := c.Token
	// data := url.Values{
	// 	"authorization": {authToken},
	// }
	ctx := http_context.Background()
	doOptions := &httputil.DoOptions{
		Context: ctx,
		Headers: map[string]string{"authorization": authToken},
	}
	res, err := httpClient.Do("POST", orgDomain, doOptions)
	if err != nil {
		return []astro.Organization{}, fmt.Errorf("could not retrieve token: %w", err)
	}
	defer res.Body.Close()
	var orgRes []astro.Organization
	fmt.Println(res)
	fmt.Println(res.Body)
	err = json.NewDecoder(res.Body).Decode(&orgRes)
	if err != nil {
		return []astro.Organization{}, fmt.Errorf("cannot decode response: %w", err)
	}

	return orgRes, err
}

// List all Organizations
func List(client astro.Client, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	or, err := ListOrganizations(c)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	tab := newTableOut()
	for i := range or {
		name := or[i].Name
		OrganizationID := or[i].ID

		var color bool

		if c.Organization == or[i].ID {
			color = true
		} else {
			color = false
		}
		tab.AddRow([]string{name, OrganizationID}, color)
	}

	tab.Print(out)

	return nil
}

func getOrganizationSelection(client astro.Client, out io.Writer) (string, error) {
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

	or, err := ListOrganizations(c)
	if err != nil {
		return "", err
	}

	deployMap := map[string]astro.Organization{}
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

	return selected.ID, nil
}

// Switch switches organizations
func Switch(id string, client astro.Client, out io.Writer) error {
	if id == "" {
		_id, err := getOrganizationSelection(client, out)
		if err != nil {
			return err
		}

		id = _id
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	// log user into new organization
	err = authLogin(c.Domain, id, client, out, false, false)
	if err != nil {
		return err
	}

	// // validate workspace
	// _, err = client.ListOrganizations()
	// if err != nil {
	// 	return errors.Wrap(err, "organization id is not valid")
	// }

	// // set organization id in context
	// err = c.SetContextKey("organization", id)
	// if err != nil {
	// 	return err
	// }

	// // update workspace
	// workspaces, err := client.ListWorkspaces(id)
	// if err != nil {
	// 	return errors.Wrap(err, "Invalid authentication token. Try to log in again with a new token or check your internet connection.\n\nDetails")
	// }

	return nil
}