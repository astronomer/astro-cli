package cmd

import (
	"fmt"

	sa "github.com/astronomerio/astro-cli/serviceaccount"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	workspaceUuid  string
	deploymentUuid string
	userUuid       string
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
		Use:     "create ENTITY-UUID",
		Aliases: []string{"cr"},
		Short:   "Create a service-account in the astronomer platform",
		Long:    "Create a service-account in the astronomer platform",
		RunE:    saCreate,
	}

	// saDeleteCmd = &cobra.Command{
	// 	Use:     "delete UUID",
	// 	Aliases: []string{"de"},
	// 	Short:   "Delete a service-account in the astronomer platform",
	// 	Long:    "Delete a service-account in the astronomer platform",
	// 	RunE:    saDelete,
	// 	Args:    cobra.ExactArgs(1),
	// }

	saGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get a service-account by entity type and entity uuid",
		Long:  "Get a service-account by entity type and entity uuid",
		RunE:  saGet,
	}
)

func init() {
	// User root
	RootCmd.AddCommand(saRootCmd)

	// Service-account create
	saRootCmd.AddCommand(saCreateCmd)
	saCreateCmd.Flags().StringVarP(&workspaceUuid, "workspace-uuid", "w", "", "[UUID]")
	saCreateCmd.Flags().StringVarP(&deploymentUuid, "deployment-uuid", "d", "", "[UUID]")
	saCreateCmd.Flags().StringVarP(&userUuid, "user-uuid", "u", "", "[UUID]")
	saCreateCmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	saCreateCmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	saCreateCmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")

	saRootCmd.AddCommand(saGetCmd)
	saGetCmd.Flags().StringVarP(&workspaceUuid, "workspace-uuid", "w", "", "[UUID]")
	saGetCmd.Flags().StringVarP(&deploymentUuid, "deployment-uuid", "d", "", "[UUID]")
	saGetCmd.Flags().StringVarP(&userUuid, "user-uuid", "u", "", "[UUID]")
	saGetCmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
}

func getValidEntity() (string, string, error) {
	var uuid string
	var entityType string
	singleArgVerify := 0

	if systemSA {
		entityType = "SYSTEM"
		singleArgVerify++
	}

	if len(workspaceUuid) > 0 {
		entityType = "WORKSPACE"
		uuid = workspaceUuid
		singleArgVerify++
	}

	if len(deploymentUuid) > 0 {
		entityType = "DEPLOYMENT"
		uuid = deploymentUuid
		singleArgVerify++
	}

	if len(userUuid) > 0 {
		entityType = "USER"
		uuid = userUuid
		singleArgVerify++
	}

	if singleArgVerify != 1 {
		return "", "", errors.New("must specify exactly one service-account type (system, workspace deployment, user")
	}

	return entityType, uuid, nil
}

func saCreate(cmd *cobra.Command, args []string) error {
	// Validation
	entityType, uuid, err := getValidEntity()
	if err != nil {
		return err
	}

	if len(label) == 0 {
		label = fmt.Sprintf("%s %s Service Account", entityType, uuid)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Create(uuid, label, category, entityType)
}

// func saDelete(cmd *cobra.Command, args []string) error {
// 	// Silence Usage as we have now validated command input
// 	cmd.SilenceUsage = true

// 	return sa.Delete()
// }

func saGet(cmd *cobra.Command, args []string) error {
	entityType, uuid, err := getValidEntity()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Get(entityType, uuid)
}
