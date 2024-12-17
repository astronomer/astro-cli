package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/logger"
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
		Use:   "add [PROVIDER]",
		Short: "Download a provider package from the Astronomer Registry",
		Long:  "Download a provider package as an Astro project dependency.",
		Run: func(cmd *cobra.Command, args []string) {
			var providerName string
			if len(args) > 0 {
				providerName = args[0]
			} else {
				providerName = input.Text("Enter the name of the provider package to download: ")
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
	if !exists {
		jsonString, _ := json.Marshal(providersJSON)
		logger.Fatalf("Couldn't find key 'providers' in Response! %s", jsonString)
	}

	for _, provider := range providers {
		childProvidersJSON := provider.(map[string]interface{})

		providerID, childExists := childProvidersJSON["name"].(string) // displayName??
		if !childExists {
			jsonString, _ := json.Marshal(childProvidersJSON)
			logger.Fatalf("Couldn't find key 'name' in Response! %s", jsonString)
		}
		thisProviderVersion, childExists := childProvidersJSON["version"].(string) // displayName??
		if !childExists {
			jsonString, _ := json.Marshal(childProvidersJSON)
			logger.Fatalf("Couldn't find key 'version' in Response! %s", jsonString)
		}
		addProviderByIDAndVersion(providerID, thisProviderVersion, out)
	}
}

func addProviderByIDAndVersion(providerID, providerVersion string, out io.Writer) {
	filledProviderRoute := getProviderRoute(providerID, providerVersion)
	providersJSON := httputil.RequestAndGetJSONBody(filledProviderRoute)

	name, exists := providersJSON["name"].(string)
	if !exists {
		jsonString, _ := json.Marshal(providersJSON)
		logger.Fatalf("Couldn't find key 'name' in Response! %s", jsonString)
	}

	version, exists := providersJSON["version"].(string)
	if !exists {
		jsonString, _ := json.Marshal(providersJSON)
		logger.Fatalf("Couldn't find key 'version' in Response! %s", jsonString)
	}
	addProviderToRequirementsTxt(name, version, out)
}

func addProviderToRequirementsTxt(name, version string, out io.Writer) {
	const filename = "requirements.txt"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, writeAndReadPermissions)
	if err != nil {
		logger.Fatal(err)
	}

	b, err := os.ReadFile(filename)
	if err != nil {
		logger.Fatal(err)
	}

	exists := strings.Contains(string(b), name)
	if exists {
		fmt.Printf("%s already exists in %s", name, filename)
	} else {
		logger.Debugf("Couldn't find %s already in %s", name, string(b))
		providerString := fmt.Sprintf("%s==%s", name, version)
		_, err = f.WriteString(providerString + "\n")
		if err != nil {
			logger.Fatal(err)
		}
		_, _ = fmt.Fprintf(out, "\nWrote %s to %s", providerString, filename)
	}
	_ = f.Close()
}
