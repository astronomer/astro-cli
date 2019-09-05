package deployment

import (
	"fmt"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	tab = printutil.Table{
		Padding:        []int{40, 40, 50, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "CATEGORY", "ID", "APIKEY"},
	}
)

func Create(id, label, category, entityType, role string) error {
	req := houston.Request{
		Query: houston.ServiceAccountCreateRequest,
		Variables: map[string]interface{}{
			"entityId":   id,
			"label":      label,
			"category":   category,
			"entityType": entityType,
			"role":       role,
		},
	}

	resp, err := req.Do()
	if err != nil {
		return err
	}

	sa := resp.Data.CreateServiceAccount

	tab.AddRow([]string{sa.Label, sa.Category, sa.Id, sa.ApiKey}, false)
	tab.SuccessMsg = "\n Service account successfully created."

	tab.Print()
	return nil
}

func Delete(id string) error {
	req := houston.Request{
		Query:     houston.ServiceAccountDeleteRequest,
		Variables: map[string]interface{}{"serviceAccountId": id},
	}

	resp, err := req.Do()
	if err != nil {
		return err
	}

	sa := resp.Data.DeleteServiceAccount

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", sa.Label, sa.Id)
	fmt.Println(msg)

	return nil
}

func Get(entityType, id string) error {
	req := houston.Request{
		Query:     houston.ServiceAccountsGetRequest,
		Variables: map[string]interface{}{"entityId": id, "entityType": entityType},
	}

	resp, err := req.Do()
	if err != nil {
		return err
	}

	sas := resp.Data.GetServiceAccounts

	for _, sa := range sas {
		tab.AddRow([]string{sa.Label, sa.Category, sa.Id, sa.ApiKey}, false)
	}

	tab.Print()
	return nil
}
