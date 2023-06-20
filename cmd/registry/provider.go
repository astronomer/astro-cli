package registry

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const writeAndReadPermissions = 0o655

func newRegistryProviderCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "provider",
		Aliases: []string{"p"},
		Short:   "Interact with Airflow Providers from the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryAddProviderCmd(out),
	)
	return cmd
}

func newRegistryAddProviderCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "add [PROVIDER]",
		// Aliases: []string{"p"},
		Short: "Download a provider package from the Astronomer Registry",
		Long:  "Download a provider package as an Astro project dependency.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				providerName := args[0]
			} else {
				providerName := input.Text("Enter the name of the provider package to download: ")
			}
			addProviderByName(providerName, out)
		},
	}
	cmd.Flags().StringVar(&providerVersion, "version", "latest", "Provider Version to add. Optional")
	return cmd
}

func getProviderSearchRoute(providerName string) string {
	getProviderURL := url.URL{
		Scheme:   "https",
		Host:     registryHost,
		Path:     fmt.Sprintf("%s/%s", registryAPI, providerSearchRoute),
		RawQuery: fmt.Sprintf(providerSearchQuery, url.QueryEscape(providerName)),
	}
	return getProviderURL.String()
}

func getProviderRoute(providerID, providerVersion string) string {
	getProviderURL := url.URL{
		Scheme: "https",
		Host:   registryHost,
		Path:   fmt.Sprintf("%s/%s", registryAPI, fmt.Sprintf(providerRoute, providerID, providerVersion)),
	}
	return getProviderURL.String()
}

func addProviderByName(providerName string, out io.Writer) {
	filledProviderRoute := getProviderSearchRoute(providerName)

	providersJSON := httputil.RequestAndGetJSONBody(filledProviderRoute)
	providers, exists := providersJSON["providers"].([]interface{})
	printutil.LogKeyNotExists(exists, "providers", providersJSON)

	for _, provider := range providers {
		providersJSON := provider.(map[string]interface{})

		providerID, childExists := providersJSON["name"].(string) // displayName??
		printutil.LogKeyNotExists(childExists, "name", providersJSON)

		thisProviderVersion, childExists := providersJSON["version"].(string) // displayName??
		printutil.LogKeyNotExists(childExists, "name", providersJSON)

		addProviderByIDAndVersion(providerID, thisProviderVersion, out)
	}
}

func addProviderByIDAndVersion(providerID, providerVersion string, out io.Writer) {
	filledProviderRoute := getProviderRoute(providerID, providerVersion)
	providersJSON := httputil.RequestAndGetJSONBody(filledProviderRoute)

	name, exists := providersJSON["name"].(string)
	printutil.LogKeyNotExists(exists, "name", providersJSON)

	version, exists := providersJSON["version"].(string)
	printutil.LogKeyNotExists(exists, "version", providersJSON)

	addProviderToRequirementsTxt(name, version, out)
}

func addProviderToRequirementsTxt(name, version string, out io.Writer) {
	const filename = "requirements.txt"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, writeAndReadPermissions)
	defer f.Close()
	if err != nil {
		log.Fatal(err)
	}

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
		fmt.Fprintf(out, "\nWrote %s to %s", providerString, filename)
	}
}
