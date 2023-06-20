package registry

import (
	"fmt"
	"io"
	"net/url"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	registryHost        = "api.astronomer.io"
	registryAPI         = "registryV2/v1alpha1/organizations/public"
	dagRoute            = "dags/%s/versions/%s"
	providerRoute       = "providers/%s/versions/%s"
	providerSearchRoute = "providers"
	providerSearchQuery = "limit=1&query=%s"
)

var (
	dagID           string
	dagVersion      string
	providerVersion string
	addProviders    bool
)

func newRegistryDagCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dag",
		Aliases: []string{"d"},
		Short:   "Interact with DAGs from the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryDagAddCmd(out),
	)
	return cmd
}

func newRegistryDagAddCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add [DAG ID]",
		Aliases: []string{"a"},
		Short:   "Download a DAG from the Astronomer Registry",
		Long:    "Download a DAG from the Astronomer Registry to your local Astro project.",
		Run: func(cmd *cobra.Command, args []string) {
			// if a dagID was provided in the args we use it
			if len(args) > 0 {
				dagID = args[0]
			} else {
				// no dagID was provided so ask the user for it
				dagID = input.Text("Enter the DAG ID to add: ")
			}
			downloadDag(dagID, dagVersion, addProviders, out)
		},
	}
	cmd.Flags().StringVar(&dagVersion, "version", "latest", "The DAG version to download. Optional.")
	cmd.Flags().BoolVar(&addProviders, "add-providers", false, "Attempt to add providers required for this DAG to requirements.txt.")
	return cmd
}

func downloadDag(dagID, dagVersion string, addProviders bool, out io.Writer) {
	// https://api.astronomer.io/registryV2/v1alpha1/organizations/public/dags?sorts=displayName%3Aasc&limit=1&query=foo
	filledDagRoute := getDagRoute(dagID, dagVersion)
	dagJSON := httputil.RequestAndGetJSONBody(filledDagRoute)

	githubRawSourceURL, exists := dagJSON["githubRawSourceUrl"].(string)
	if !exists {
	        log.Fatalf("Couldn't find key githubRawSourceUrl in Response! %v", dagJSON)
	}

	filePath, exists := dagJSON["filePath"].(string)
	printutil.LogKeyNotExists(exists, "filePath", dagJSON)

	httputil.DownloadResponseToFile(githubRawSourceURL, filePath)
	fmt.Fprintf(out, "Successfully added DAG %s:%s to %s ", dagID, dagVersion, filePath)

	if addProviders {
		providers, exists := dagJSON["providers"].([]interface{})
		printutil.LogKeyNotExists(exists, "providers", dagJSON)
		for _, provider := range providers {
			providerJSON := provider.(map[string]interface{})
			providerID, nexists := providerJSON["name"].(string) // displayName??
			printutil.LogKeyNotExists(nexists, "name", providerJSON)
			log.Infof("Adding provider required for DAG: %s", providerID)
			addProviderByName(providerID, out)
		}
	}
}

func getDagRoute(dagID, dagVersion string) string {
	filledDagRoute := fmt.Sprintf(dagRoute, dagID, dagVersion)
	getDagURL := url.URL{
		Scheme: "https",
		Host:   registryHost,
		Path:   fmt.Sprintf("%s/%s", registryAPI, filledDagRoute),
	}
	return getDagURL.String()
}
