package cmd

import (
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/software"
	"github.com/spf13/cobra"
)

var (
	environment string
	airgapped   bool

	softwareInitExample = `
# Generates a new helm values file
astro software init

# Generates a new helm values file for install on AWS (also accepts gcp and azure)
astro software init --environment aws

# Generates a new helm values file for install on AWS in and airgapped environment
astro software init --environment aws --airgapped
`
)

func newSoftwareRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "software",
		Aliases: []string{"sf"},
		Short:   "Manage Astronomer Software Install",
		Long:    "Utilities for installing Astronomer Software.",
	}
	cmd.AddCommand(
		newSoftwareInitCmd(),
	)

	return cmd
}

func newSoftwareInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Generate helm values for Software install",
		Long:    "Generate helm values for Software install. This generates a helm values file using given input for configuring the install.",
		Example: softwareInitExample,
		Args:    cobra.MaximumNArgs(1),
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: softwareInit,
	}
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Environment to install in (aws, azure, gcloud, other)")
	cmd.Flags().BoolVarP(&airgapped, "airgapped", "", false, "Whether environment is airgapped or not")

	return cmd
}

func softwareInit(cmd *cobra.Command, args []string) error {
	if environment == "" {
		environment = input.Text("Enter the environment you are installing Astronomer in: (aws, azure, gcloud, other) ")
	}

	if err := software.Init(environment, airgapped); err != nil {
		return err
	}

	return nil
}
