package cmd

import (
	sa "github.com/astronomer/astro-cli/serviceaccount"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	deploymentId string
	userId       string
	systemSA       bool
	category       string
	label          string

	saRootCmd = &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage astronomer service accounts",
		Long:    "Service-accounts represent a revokable token with access to the Astronomer platform",
	}

	saCreateCmd = &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a service-account in the astronomer platform",
		Long:    "Create a service-account in the astronomer platform",
		RunE:    saCreate,
	}

	saDeleteCmd = &cobra.Command{
		Use:     "delete [SA-ID]",
		Aliases: []string{"de"},
		Short:   "Delete a service-account in the astronomer platform",
		Long:    "Delete a service-account in the astronomer platform",
		RunE:    saDelete,
		Args:    cobra.ExactArgs(1),
	}

	saGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get a service-account by entity type and entity id",
		Long:  "Get a service-account by entity type and entity id",
		RunE:  saGet,
	}
)

func init() {
	// User root
	rootCmd.AddCommand(saRootCmd)

	// Service-account create
	saRootCmd.AddCommand(saCreateCmd)
	saCreateCmd.Flags().StringVarP(&workspaceId, "workspace-id", "w", "", "[ID]")
	saCreateCmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	saCreateCmd.Flags().StringVarP(&userId, "user-id", "u", "", "[ID]")
	saCreateCmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	saCreateCmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	saCreateCmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")

	saRootCmd.AddCommand(saGetCmd)
	saGetCmd.Flags().StringVarP(&workspaceId, "workspace-id", "w", "", "[ID]")
	saGetCmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	saGetCmd.Flags().StringVarP(&userId, "user-id", "u", "", "[ID]")
	saGetCmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")

	saRootCmd.AddCommand(saDeleteCmd)
}

func getValidEntity() (string, string, error) {
	var id string
	var entityType string
	singleArgVerify := 0

	if systemSA {
		entityType = "SYSTEM"
		singleArgVerify++
	}

	if len(workspaceId) > 0 {
		entityType = "WORKSPACE"
		id = workspaceId
		singleArgVerify++
	}

	if len(deploymentId) > 0 {
		entityType = "DEPLOYMENT"
		id = deploymentId
		singleArgVerify++
	}

	if len(userId) > 0 {
		entityType = "USER"
		id = userId
		singleArgVerify++
	}

	if singleArgVerify != 1 {
		return "", "", errors.New("must specify exactly one service-account type (system, workspace deployment, user")
	}

	return entityType, id, nil
}

func saCreate(cmd *cobra.Command, args []string) error {
	// Validation
	entityType, id, err := getValidEntity()
	if err != nil {
		return err
	}

	if len(label) == 0 {
		return errors.New("must provide a service-account label with the --label (-l) flag")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Create(id, label, category, entityType)
}

func saDelete(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Delete(args[0])
}

func saGet(cmd *cobra.Command, args []string) error {
	entityType, id, err := getValidEntity()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Get(entityType, id)
}
