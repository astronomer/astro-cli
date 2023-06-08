package registry

import (
	"fmt"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/logrusorgru/aurora"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	dagVersion      string
	providerVersion string
)

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

	providersJson := requestAndGetJsonBody(filledProviderRoute)
	providers, exists := providersJson["providers"].([]interface{})
	logKeyNotExists(exists, "providers", providersJson)

	for _, provider := range providers {
		providerJson := provider.(map[string]interface{})

		providerId, childExists := providerJson["name"].(string) // displayName??
		logKeyNotExists(childExists, "name", providerJson)

		providerVersion, childExists := providerJson["version"].(string) // displayName??
		logKeyNotExists(childExists, "name", providerJson)

		addProviderByIdAndVersion(providerId, providerVersion)
	}
}

func addProviderByIdAndVersion(providerId string, providerVersion string) {
	filledProviderRoute := getProviderRoute(providerId, providerVersion)
	providerJson := requestAndGetJsonBody(filledProviderRoute)

	name, exists := providerJson["name"].(string)
	logKeyNotExists(exists, "name", providerJson)

	version, exists := providerJson["version"].(string)
	logKeyNotExists(exists, "version", providerJson)

	addProviderToRequirementsTxt(name, version)
	// TODO
	//addProviderExampleDag(name)
}

// TODO
//func addProviderExampleDag(name string) {
//	downloadDag()
//}

func addProviderToRequirementsTxt(name string, version string) {
	const filename = "requirements.txt"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0655)

	b, err := os.ReadFile(filename)
	logFatal(err)

	exists := strings.Contains(string(b), name)
	if exists {
		log.Infof("%s already exists in %s", name, filename)
	} else {
		log.Debugf("Couldn't find %s already in %s", name, string(b))
		providerString := fmt.Sprintf("%s==%s", name, version)
		log.Infof("Writing %s to %s", providerString, filename)
		_, err = f.WriteString(providerString + "\n")
		logFatal(err)
		defer f.Close()
	}
}

func downloadDag(dagName string, dagVersion string) {
	// https://api.astronomer.io/registryV2/v1alpha1/organizations/public/dags?sorts=displayName%3Aasc&limit=1&query=foo
	filledDagRoute := getDagRoute(dagName, dagVersion)
	dagJson := requestAndGetJsonBody(filledDagRoute)

	githubRawSourceUrl, exists := dagJson["githubRawSourceUrl"].(string)
	logKeyNotExists(exists, "githubRawSourceUrl", dagJson)

	filePath, exists := dagJson["filePath"].(string)
	logKeyNotExists(exists, "filePath", dagJson)

	downloadDagToFile(githubRawSourceUrl, filePath)

	providers, exists := dagJson["providers"].([]interface{})
	logKeyNotExists(exists, "providers", dagJson)
	for _, provider := range providers {
		providerJson := provider.(map[string]interface{})
		providerId, nexists := providerJson["name"].(string) // displayName??
		logKeyNotExists(nexists, "name", providerJson)
		log.Infof("Adding provider required for DAG: %s", providerId)
		addProviderByName(providerId)
	}
}

func getDagRoute(dagName string, dagVersion string) string {
	filledDagRoute := fmt.Sprintf(dagRoute, dagName, dagVersion)
	getDagUrl := url.URL{
		Scheme: "https",
		Host:   registryHost,
		Path:   fmt.Sprintf("%s/%s", registryApi, filledDagRoute),
	}
	return getDagUrl.String()
}

func downloadDagToFile(sourceUrl string, path string) {
	file, err := create(path)
	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	resp, err := client.Get(sourceUrl)
	logFatal(err)
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close()
	_, err = io.Copy(file, resp.Body)
	//goland:noinspection GoUnhandledErrorResult
	defer file.Close()
	log.Infof("Downloaded file %s from %s", path, sourceUrl)
}

func newRegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"r"},
		Short:   "Interact with the Astronomer Registry",
	}
	cmd.AddCommand(
		newRegistryAddCmd(),
		newRegistryInitCmd(),
	)
	return cmd
}

func newRegistryInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "init",
		//Aliases: []string{"r"},
		Short: "Utilize the Astronomer Registry",
		Long:  "Utilize the Astronomer Registry to initialize a project from a template.",
	}
	//cmd.AddCommand(
	//	newRegistryAddDagCmd(),
	//	newRegistryAddProviderCmd(),
	//)
	return cmd
}

func newRegistryAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Aliases: []string{"a"},
	}
	cmd.AddCommand(
		newRegistryAddDagCmd(),
		newRegistryAddProviderCmd(),
	)
	return cmd
}

func newRegistryAddDagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dag [DAG ID]",
		Aliases: []string{"d"},
		Short:   "Utilize the Astronomer Registry to add a DAG",
		Long:    "Utilize the Astronomer Registry to download a DAG to your local project.",
		Run: func(cmd *cobra.Command, args []string) {
			var dagName string

			// if a dagName was provided in the args we use it
			if len(args) > 0 {
				dagName = args[0]
			} else {
				// no dagName was provided so ask the user for it
				dagName = input.Text("Enter DAG ID to download: ")
			}
			downloadDag(dagName, dagVersion)
		},
	}
	cmd.Flags().StringVar(&dagVersion, "version", "latest", "Optional DAG Version to Download.")
	return cmd
}

func newRegistryAddProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "provider [PROVIDER]",
		//Aliases: []string{"p"},
		Short: "Utilize the Astronomer Registry to add a Provider",
		Long:  "Utilize the Astronomer Registry to download a Provider like Snowflake or Great Expectations to your project's dependencies.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				providerName := args[0]
				addProviderByName(providerName)
			} else {
				providerName := input.Text("Enter Provider ID to download: ")
				addProviderByName(providerName)
			}
		},
	}
	cmd.Flags().StringVar(&providerVersion, "version", "latest", "Optional Provider Version to add.")
	return cmd
}

// AddCmds adds all the command initialized in this package for the cmd package to import
func AddCmds() []*cobra.Command {
	return []*cobra.Command{
		newRegistryCmd(),
	}
}

func main() {
	// DEMO
	_ = os.Remove("requirements.txt")
	_ = os.Remove("dags/sagemaker-batch-inference.py")

	log.Info(aurora.Bold(aurora.Cyan("DEMO: ADDING SPECIFIC PROVIDER BY NAME AND ID")))
	const providerId = "apache-airflow-providers-airbyte"
	const providerVersion = "3.2.0"
	addProviderByIdAndVersion(providerId, providerVersion)

	log.Info(aurora.Bold(aurora.Cyan("DEMO: ADDING PROVIDER BY NAME")))
	const providerName = "amazon"
	addProviderByName(providerName)

	log.Info(aurora.Bold(aurora.Cyan("DEMO: ADDING DAG BY NAME AND VERSION")))
	const dagName = "sagemaker-batch-inference"
	const dagVersion = "1.0.1"
	downloadDag(dagName, dagVersion)
}
