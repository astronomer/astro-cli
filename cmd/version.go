package cmd

import (
	"fmt"

	"github.com/astronomerio/astro-cli/pkg/github"
	"github.com/spf13/cobra"
)

var (
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
	fmt.Printf("Astro CLI Version: v%s\n", version)
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

	repoLatestTag, err := github.RepoLatestRequest("astronomerio", "astro-cli")
	if err != nil {
		fmt.Println(err)
	}

	currentTag, err := github.RepoTagRequest("astronomerio", "astro-cli", string("v")+version)
	if err != nil {
		fmt.Println(err)
	}

	currentPub := currentTag.PublishedAt.Format("2006.01.02")
	latestPub := repoLatestTag.PublishedAt.Format("2006.01.02")

	if repoLatestTag.TagName > version {
		fmt.Printf("Astro CLI Version: v%s (%s)\n", version, currentPub)
		fmt.Printf("Astro CLI Latest: %s (%s)\n", repoLatestTag.TagName, latestPub)
		fmt.Println(`There is a more recent version of the Astronomer CLI available.
You can install the latest tagged release with the following command

	$ curl -sL https://install.astronomer.io | sudo bash`)
		return nil
	}

	return nil
}
