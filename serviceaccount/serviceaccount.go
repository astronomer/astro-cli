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
		Header:         []string{"NAME", "CATEGORY", "UUID", "APIKEY"},
	}
)

func Create(uuid, label, category, entityType string) error {
	req := houston.Request{
		Query: houston.ServiceAccountCreateRequest,
		Variables: map[string]interface{}{
			"entityUuid": uuid,
			"label":      label,
			"category":   category,
			"entityType": entityType,
		},
	}

	resp, err := req.Do()
	if err != nil {
		return err
	}

	sa := resp.Data.CreateServiceAccount

	tab.AddRow([]string{sa.Label, sa.Category, sa.Uuid, sa.ApiKey}, false)
	tab.SuccessMsg = "\n Service account successfully created."

	tab.Print()
	return nil
}

func Delete(uuid string) error {
	req := houston.Request{
		Query:     houston.ServiceAccountDeleteRequest,
		Variables: map[string]interface{}{"serviceAccountUuid": uuid},
	}

	resp, err := req.Do()
	if err != nil {
		return err
	}

	sa := resp.Data.DeleteServiceAccount

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", sa.Label, sa.Uuid)
	fmt.Println(msg)

	return nil
}

func Get(entityType, uuid string) error {
	req := houston.Request{
		Query:     houston.ServiceAccountsGetRequest,
		Variables: map[string]interface{}{"entityUuid": uuid, "entityType": entityType},
	}

	resp, err := req.Do()
	if err != nil {
		return err
	}

	sas := resp.Data.GetServiceAccounts

	for _, sa := range sas {
		tab.AddRow([]string{sa.Label, sa.Category, sa.Uuid, sa.ApiKey}, false)
	}

	tab.Print()
	return nil
}
