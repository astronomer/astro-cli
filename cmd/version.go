package cmd

import (
	"fmt"
	"os"

	"github.com/astronomerio/astro-cli/pkg/github"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/spf13/cobra"
)

var (
	http       = httputil.NewHTTPClient()
	version    string
	gitCommit  string
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Astronomer CLI version",
		Long:  "Astronomer CLI version",
		RunE:  printVersion,
	}

	upgradeCmd = &cobra.Command{
		Use:   "upgrade",
		Short: "Check for newer version of Astronomer CLI",
		Long:  "Check for newer version of Astronomer CLI",
		RunE:  upgradeCheck,
	}
)

func init() {
	RootCmd.AddCommand(versionCmd)
	RootCmd.AddCommand(upgradeCmd)
}

func printVersion(cmd *cobra.Command, args []string) error {
	fmt.Printf("Astro CLI Version: %s\n", version)
	fmt.Printf("Git Commit: %s\n", gitCommit)
	return nil
}

func upgradeCheck(cmd *cobra.Command, args []string) error {
	if len(version) == 0 {
		fmt.Println(`Your current Astronomer CLI is not bound to a git commit.
This is likely the result of building from source. You can install the latest tagged release with the following command

	$ curl -sL https://install.astronomer.io | sudo bash`)
		return nil
	}

	api := github.NewGithubClient(http)
	repoLatest, err := api.RepoLatestRequest("astronomerio", "astro-cli")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println(repoLatest.TagName)
	return nil
}
