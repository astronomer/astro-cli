package cmd

import (
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	sa "github.com/astronomer/astro-cli/serviceaccount"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	deploymentId string
	userId       string
	systemSA     bool
	category     string
	label        string
)

func newSaRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service-account",
		Aliases: []string{"sa"},
		Short:   "Manage astronomer service accounts",
		Long:    "Service-accounts represent a revokable token with access to the Astronomer platform",
	}
	cmd.AddCommand(
		newSaCreateCmd(client, out),
		newSaDeleteCmd(client, out),
		newSaGetCmd(client, out),
	)
	return cmd
}

func newSaCreateCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a service-account in the astronomer platform",
		Long:    "Create a service-account in the astronomer platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return saCreate(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceId, "workspace-id", "w", "", "[ID]")
	cmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	cmd.Flags().StringVarP(&userId, "user-id", "u", "", "[ID]")
	cmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	cmd.Flags().StringVarP(&category, "category", "c", "default", "CATEGORY")
	cmd.Flags().StringVarP(&label, "label", "l", "", "LABEL")
	cmd.Flags().StringVarP(&role, "role", "r", "viewer", "ROLE")
	return cmd
}

func newSaDeleteCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete [SA-ID]",
		Aliases: []string{"de"},
		Short:   "Delete a service-account in the astronomer platform",
		Long:    "Delete a service-account in the astronomer platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return saDelete(cmd, args, client, out)
		},
		Args:    cobra.ExactArgs(1),
	}
	return cmd
}

func newSaGetCmd(client *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a service-account by entity type and entity id",
		Long:  "Get a service-account by entity type and entity id",
		RunE: func(cmd *cobra.Command, args []string) error {
			return saGet(cmd, args, client, out)
		},
	}
	cmd.Flags().StringVarP(&workspaceId, "workspace-id", "w", "", "[ID]")
	cmd.Flags().StringVarP(&deploymentId, "deployment-id", "d", "", "[ID]")
	cmd.Flags().StringVarP(&userId, "user-id", "u", "", "[ID]")
	cmd.Flags().BoolVarP(&systemSA, "system-sa", "s", false, "")
	return cmd
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

func saCreate(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Validation
	entityType, id, err := getValidEntity()
	if err != nil {
		return err
	}

	if len(label) == 0 {
		return errors.New("must provide a service-account label with the --label (-l) flag")
	}

	if err := validateRole(role); err != nil {
		return errors.Wrap(err, "failed to find a valid role")
	}
	fullRole := strings.Join([]string{entityType, strings.ToUpper(role)}, "_")
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	return sa.Create(id, label, category, entityType, fullRole, client, out)
}

func saDelete(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Delete(args[0], client, out)
}

func saGet(cmd *cobra.Command, args []string, client *houston.Client, out io.Writer) error {
	entityType, id, err := getValidEntity()
	if err != nil {
		return err
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	return sa.Get(entityType, id, client, out)
}
