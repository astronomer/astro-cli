package workspace

import (
	"fmt"

	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

// List all workspaces
func List() error {
	ws, err := api.GetWorkspaceAll()
	if err != nil {
		return err
	}

	fmt.Println(ws)
	for _, w := range ws {
		wsTitle := "Title     : %s\n"
		wsId := "Id        : %s\n"
		wsActiveFlag := "Act. Flag : %s\n"
		wsDesc := "Desc.     : %s\n"

		// rowTmp := "Title: %s\nId: %s\nActive Flag: %s\nDesc.: %s\n\n"
		rowTmp := wsTitle + wsId + wsActiveFlag + wsDesc
		fmt.Printf(rowTmp, w.Label, w.Uuid, w.Active, w.Description)
	}
	return nil
}
