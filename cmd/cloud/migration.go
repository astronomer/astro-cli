package cloud

import (
	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/context"
)

var ErrorShortName = errors.New("cannot find organization shortname")

// migrate config
func migrateCloudConfig(coreClient astrocore.CoreClient) error {
	ctx, err := context.GetCurrentContext()
	if err != nil {
		return err
	}
	// backfill OrganizationShortName
	if (ctx.OrganizationShortName == "") && (ctx.Organization != "") {
		orgs, err := organization.ListOrganizations(coreClient)
		if err != nil {
			return err
		}
		shortName := ""
		for i := range orgs {
			if orgs[i].Id == ctx.Organization {
				shortName = orgs[i].ShortName
				break
			}
		}
		if shortName == "" {
			return ErrorShortName
		}
		err = ctx.SetContextKey("organization_short_name", shortName)
		if err != nil {
			return err
		}
	}
	return nil
}
