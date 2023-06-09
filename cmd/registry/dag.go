package registry

import (
	"fmt"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net/url"
)

const (
	registryHost        = "api.astronomer.io"
	registryApi         = "registryV2/v1alpha1/organizations/public"
	dagRoute            = "dags/%s/versions/%s"
	providerRoute       = "providers/%s/versions/%s"
	providerSearchRoute = "providers"
	providerSearchQuery = "limit=1&query=%s"
)

var (
	dagId           string
	dagVersion      string
	providerVersion string
	addProviders    bool
)

func newRegistryDagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dag",
		Aliases: []string{"d"},
		Short:   "Interact with DAGs from the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryDagAddCmd(),
	)
	return cmd
}

func newRegistryDagAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add [DAG ID]",
		Aliases: []string{"a"},
		Short:   "Download a DAG from the Astronomer Registry",
		Long:    "Download a DAG from the Astronomer Registry to your local Astro project.",
		Run: func(cmd *cobra.Command, args []string) {
			// if a dagId was provided in the args we use it
			if len(args) > 0 {
				dagId = args[0]
			} else {
				// no dagId was provided so ask the user for it
				dagId = input.Text("Enter the DAG ID to add: ")
			}
			downloadDag(dagId, dagVersion, addProviders)
		},
	}
	cmd.Flags().StringVar(&dagVersion, "version", "latest", "The DAG version to download. Optional.")
	cmd.Flags().BoolVar(&addProviders, "add-providers", false, "Attempt to add providers required for this DAG to requirements.txt.")
	return cmd
}

func downloadDag(dagId string, dagVersion string, addProviders bool) {
	// https://api.astronomer.io/registryV2/v1alpha1/organizations/public/dags?sorts=displayName%3Aasc&limit=1&query=foo
	filledDagRoute := getDagRoute(dagId, dagVersion)
	dagJson := httputil.RequestAndGetJsonBody(filledDagRoute)

	githubRawSourceUrl, exists := dagJson["githubRawSourceUrl"].(string)
	printutil.LogKeyNotExists(exists, "githubRawSourceUrl", dagJson)

	filePath, exists := dagJson["filePath"].(string)
	printutil.LogKeyNotExists(exists, "filePath", dagJson)

	httputil.DownloadResponseToFile(githubRawSourceUrl, filePath)
	fmt.Printf("Successfully added DAG %s:%s to %s", dagId, dagVersion, filePath)

	if addProviders {
		providers, exists := dagJson["providers"].([]interface{})
		printutil.LogKeyNotExists(exists, "providers", dagJson)
		for _, provider := range providers {
			providerJson := provider.(map[string]interface{})
			providerId, nexists := providerJson["name"].(string) // displayName??
			printutil.LogKeyNotExists(nexists, "name", providerJson)
			log.Infof("Adding provider required for DAG: %s", providerId)
			addProviderByName(providerId)
		}
	}
}

func getDagRoute(dagId string, dagVersion string) string {
	filledDagRoute := fmt.Sprintf(dagRoute, dagId, dagVersion)
	getDagUrl := url.URL{
		Scheme: "https",
		Host:   registryHost,
		Path:   fmt.Sprintf("%s/%s", registryApi, filledDagRoute),
	}
	return getDagUrl.String()
}
