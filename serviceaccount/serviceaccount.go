package deployment

import (
	"fmt"

	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

func Create(uuid, label, category, entityType string) error {
	sa, err := api.CreateServiceAccount(uuid, label, category, entityType)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("%s %s token: %s", sa.EntityType, sa.Uuid, sa.ApiKey)
	fmt.Println(msg)

	return nil
}

func Delete(uuid string) error {
	resp, err := api.DeleteServiceAccount(uuid)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("Service Account %s (%s) successfully deleted", resp.Label, resp.Uuid)
	fmt.Println(msg)

	return nil
}

func Get(entityType, uuid string) error {
	r := "  %-30s %-50s %-30s"

	resp, err := api.GetServiceAccounts(entityType, uuid)
	if err != nil {
		return err
	}

	h := fmt.Sprintf(r, "NAME", "UUID", "CATEGORY")
	fmt.Println(h)

	for _, sa := range resp {
		fullStr := fmt.Sprintf(r, sa.Label, sa.Uuid, sa.Category)
		fmt.Println(fullStr)
	}
	return nil
}
