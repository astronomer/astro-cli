package registry

import (
	"fmt"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net/url"
	"os"
	"strings"
)

func newRegistryProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provider",
		Aliases: []string{"p"},
		Short:   "Interact with Airflow Providers from the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryAddProviderCmd(),
	)
	return cmd
}

func newRegistryAddProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "add [PROVIDER]",
		//Aliases: []string{"p"},
		Short: "Download a provider package from the Astronomer Registry",
		Long:  "Download a provider package as an Astro project dependency.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				providerName := args[0]
				addProviderByName(providerName)
			} else {
				providerName := input.Text("Enter the name of the provider package to download: ")
				addProviderByName(providerName)
			}
		},
	}
	cmd.Flags().StringVar(&providerVersion, "version", "latest", "Provider Version to add. Optional")
	return cmd
}

func getProviderSearchRoute(providerName string) string {
	getProviderUrl := url.URL{
		Scheme:   "https",
		Host:     registryHost,
		Path:     fmt.Sprintf("%s/%s", registryApi, providerSearchRoute),
		RawQuery: fmt.Sprintf(providerSearchQuery, url.QueryEscape(providerName)),
	}
	return getProviderUrl.String()
}

func getProviderRoute(providerId string, providerVersion string) string {
	getProviderUrl := url.URL{
		Scheme: "https",
		Host:   registryHost,
		Path:   fmt.Sprintf("%s/%s", registryApi, fmt.Sprintf(providerRoute, providerId, providerVersion)),
	}
	return getProviderUrl.String()
}

func addProviderByName(providerName string) {
	filledProviderRoute := getProviderSearchRoute(providerName)

	providersJson := httputil.RequestAndGetJsonBody(filledProviderRoute)
	providers, exists := providersJson["providers"].([]interface{})
	printutil.LogKeyNotExists(exists, "providers", providersJson)

	for _, provider := range providers {
		providerJson := provider.(map[string]interface{})

		providerId, childExists := providerJson["name"].(string) // displayName??
		printutil.LogKeyNotExists(childExists, "name", providerJson)

		thisProviderVersion, childExists := providerJson["version"].(string) // displayName??
		printutil.LogKeyNotExists(childExists, "name", providerJson)

		addProviderByIdAndVersion(providerId, thisProviderVersion)
	}
}

func addProviderByIdAndVersion(providerId string, providerVersion string) {
	filledProviderRoute := getProviderRoute(providerId, providerVersion)
	providerJson := httputil.RequestAndGetJsonBody(filledProviderRoute)

	name, exists := providerJson["name"].(string)
	printutil.LogKeyNotExists(exists, "name", providerJson)

	version, exists := providerJson["version"].(string)
	printutil.LogKeyNotExists(exists, "version", providerJson)

	addProviderToRequirementsTxt(name, version)
}

func addProviderToRequirementsTxt(name string, version string) {
	const filename = "requirements.txt"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0655)
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	b, err := os.ReadFile(filename)
	printutil.LogFatal(err)

	exists := strings.Contains(string(b), name)
	if exists {
		fmt.Printf("%s already exists in %s", name, filename)
	} else {
		log.Debugf("Couldn't find %s already in %s", name, string(b))
		providerString := fmt.Sprintf("%s==%s", name, version)
		_, err = f.WriteString(providerString + "\n")
		printutil.LogFatal(err)
		fmt.Printf("Wrote %s to %s", providerString, filename)
	}
}
