package astro

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	airflowclient "github.com/astronomer/astro-cli/internal/platform/astro/clients/airflowclient"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
)

var (
	connID             string
	connType           string
	host               string
	login              string
	password           string
	schema             string
	extra              string
	fromDeploymentID   string
	fromDeploymentName string
	toDeploymentID     string
	toDeploymentName   string
	port               int
	varValue           string
	key                string
	slots              int
	includeDeferred    string
)

const (
	webserverURLField        = "metadata.airflow_api_url"
	warningConnectionCopyCMD = "WARNING! The password and extra field are not copied over. You will need to manually add these values"
	warningVariableCopyCMD   = "WARNING! Secret values are not copied over. You will need to manually add these values"
)

func newDeploymentConnectionRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connection",
		Aliases: []string{"con", "connections"},
		Short:   "Manage Airflow connections in an Astro Deployment",
		Long:    "Manage Airflow connections stored in an Astro Deployment's metadata database.",
	}
	cmd.AddCommand(
		newDeploymentConnectionListCmd(out),
		newDeploymentConnectionCreateCmd(out),
		newDeploymentConnectionUpdateCmd(out),
		newDeploymentConnectionCopyCmd(out),
		newDeploymentConnectionDeleteCmd(out),
	)
	return cmd
}

func newDeploymentConnectionListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "List a Deployment's connections",
		Long:    "List Airflow connections in a Deployment's metadata database. Passwords and sensitive extras are not included in the output.",
		Example: `
  $ astro deployment connection list --deployment-id <deployment-id>
  $ astro deployment connection list --deployment-name my-deployment
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentConnectionCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create a connection in a Deployment",
		Long:    "Create an Airflow connection in a Deployment's metadata database. If a connection with the same ID already exists, it is updated.",
		Example: `
  $ astro deployment connection create --deployment-id <deployment-id> --conn-id my-conn --conn-type postgres --host localhost --port 5432
  $ astro deployment connection create --deployment-id <deployment-id> --conn-id my-conn --conn-type http --host https://api.example.com
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection ID. Required.")
	cmd.Flags().StringVarP(&connType, "conn-type", "t", "", "The connection type. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The connection description.")
	cmd.Flags().StringVarP(&host, "host", "", "", "The connection host.")
	cmd.Flags().StringVarP(&login, "login", "l", "", "The connection login or username.")
	cmd.Flags().StringVarP(&password, "password", "p", "", "The connection password.")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "The connection schema.")
	cmd.Flags().IntVarP(&port, "port", "o", 0, "The connection port.")
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "Extra connection fields, as a JSON string.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentConnectionUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update a connection in a Deployment",
		Long:    "Update an existing Airflow connection in a Deployment's metadata database.",
		Example: `
  $ astro deployment connection update --deployment-id <deployment-id> --conn-id my-conn --conn-type postgres --host new-host
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection ID. Required.")
	cmd.Flags().StringVarP(&connType, "conn-type", "t", "", "The connection type. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The connection description.")
	cmd.Flags().StringVarP(&host, "host", "", "", "The connection host.")
	cmd.Flags().StringVarP(&login, "login", "l", "", "The connection login or username.")
	cmd.Flags().StringVarP(&password, "password", "p", "", "The connection password.")
	cmd.Flags().StringVarP(&schema, "schema", "s", "", "The connection schema.")
	cmd.Flags().IntVarP(&port, "port", "o", 0, "The connection port.")
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "Extra connection fields, as a JSON string.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentConnectionCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy connections from one Deployment to another",
		Long:    "Copy Airflow connections from one Astro Deployment to another. Passwords and extra configurations will not copy over. If a connection with the same ID already exists in the target Deployment, that connection is updated.",
		Example: `
  $ astro deployment connection copy --source-id <source-deployment-id> --target-id <target-deployment-id>
  $ astro deployment connection copy --source-name my-source-deployment --target-name my-target-deployment
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy connections from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy connections from.")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied connections.")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied connections.")

	return cmd
}

func newDeploymentConnectionDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete a connection from a Deployment",
		Long:    "Delete an Airflow connection from a Deployment's metadata database.",
		Example: `
  $ astro deployment connection delete --deployment-id <deployment-id> --conn-id my-conn
  $ astro deployment connection delete --deployment-name my-deployment --conn-id my-conn --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionDelete(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&connID, "conn-id", "i", "", "The connection ID. Required.")
	cmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Delete the connection without asking for confirmation.")

	return cmd
}

func newDeploymentAirflowVariableRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "airflow-variable",
		// `var` belongs to the sibling `variable` command, which registers it
		// first; claiming it here left no short spelling reaching this one at
		// all. These are the spellings `astro env airflow-variable` already uses.
		Aliases: []string{"airflow-var", "airflow-vars", "airflow-variables"},
		Short:   "Manage Airflow variables in an Astro Deployment",
		Long:    "Manage Airflow variables stored in an Astro Deployment's metadata database.",
	}
	cmd.AddCommand(
		newDeploymentAirflowVariableListCmd(out),
		newDeploymentAirflowVariableCreateCmd(out),
		newDeploymentAirflowVariableUpdateCmd(out),
		newDeploymentAirflowVariableCopyCmd(out),
		newDeploymentAirflowVariableDeleteCmd(out),
	)
	return cmd
}

func newDeploymentAirflowVariableListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "List a Deployment's Airflow variables",
		Long:    "List Airflow variables stored in a Deployment's metadata database.",
		Example: `
  $ astro deployment airflow-variable list --deployment-id <deployment-id>
  $ astro deployment airflow-variable list --deployment-name my-deployment
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentAirflowVariableCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Airflow variable in a Deployment",
		Long:    "Create an Airflow variable in a Deployment's metadata database. If a variable with the same key already exists, it is updated.",
		Example: `
  $ astro deployment airflow-variable create --deployment-id <deployment-id> --key my_var --value my_value
  $ astro deployment airflow-variable create --deployment-id <deployment-id> --key my_var --value my_value --description "A useful variable"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&varValue, "value", "v", "", "The Airflow variable value. Required.")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The Airflow variable key. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The Airflow variable description.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentAirflowVariableUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Airflow variable in a Deployment",
		Long:    "Update an existing Airflow variable in a Deployment's metadata database.",
		Example: `
  $ astro deployment airflow-variable update --deployment-id <deployment-id> --key my_var --value new_value
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&varValue, "value", "v", "", "The Airflow variable value. Required.")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The Airflow variable key. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The Airflow variable description.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentAirflowVariableCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy Airflow variables from one Deployment to another",
		Long:    "Copy Airflow variables from one Astro Deployment to another. If a variable with the same key already exists in the target Deployment, that variable is updated.",
		Example: `
  $ astro deployment airflow-variable copy --source-id <source-deployment-id> --target-id <target-deployment-id>
  $ astro deployment airflow-variable copy --source-name my-source-deployment --target-name my-target-deployment
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy Airflow variables from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy Airflow variables from.")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied Airflow variables.")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied Airflow variables.")

	return cmd
}

func newDeploymentAirflowVariableDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete an Airflow variable from a Deployment",
		Long:    "Delete an Airflow variable from a Deployment's metadata database.",
		Example: `
  $ astro deployment airflow-variable delete --deployment-id <deployment-id> --key my_var
  $ astro deployment airflow-variable delete --deployment-name my-deployment --key my_var --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableDelete(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&key, "key", "k", "", "The Airflow variable key. Required.")
	cmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Delete the Airflow variable without asking for confirmation.")

	return cmd
}

func newDeploymentPoolRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pool",
		Aliases: []string{"pl", "pools"},
		Short:   "Manage Airflow pools in an Astro Deployment",
		Long:    "Manage Airflow pools stored in an Astro Deployment's metadata database.",
	}
	cmd.AddCommand(
		newDeploymentPoolListCmd(out),
		newDeploymentPoolCreateCmd(out),
		newDeploymentPoolUpdateCmd(out),
		newDeploymentPoolCopyCmd(out),
		newDeploymentPoolDeleteCmd(out),
	)
	return cmd
}

func newDeploymentPoolListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "List a Deployment's Airflow pools",
		Long:    "List Airflow pools in a Deployment. Pools limit how many task instances can run concurrently for tasks assigned to the pool.",
		Example: `
  $ astro deployment pool list --deployment-id <deployment-id>
  $ astro deployment pool list --deployment-name my-deployment
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentPoolCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Airflow pool in an Astro Deployment",
		Long:    "Create an Airflow pool in a Deployment. A pool's slot count limits how many tasks assigned to it can run at once. If a pool with the same name already exists, it is updated.",
		Example: `
  $ astro deployment pool create --deployment-id <deployment-id> --name my-pool --slots 5
  $ astro deployment pool create --deployment-id <deployment-id> --name my-pool --slots 10 --description "Pool for ML tasks"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "Name of the pool. Required.")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "Number of slots in the pool. Required.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The pool description.")
	cmd.Flags().StringVarP(&includeDeferred, "include-deferred", "", "", "If set to 'enable', deferred tasks are considered when calculating open pool slots. Default is 'disable'. Possible values are disable/enable.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentPoolUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update an Airflow pool in an Astro Deployment",
		Long:    "Update an Airflow pool's slot count, description or deferred-task setting in a Deployment.",
		Example: `
  $ astro deployment pool update --deployment-id <deployment-id> --name my-pool --slots 10
  $ astro deployment pool update --deployment-id <deployment-id> --name my-pool --slots 10 --description "Updated pool"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "Name of the pool. Required.")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "Number of slots in the pool.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The pool description.")
	cmd.Flags().StringVarP(&includeDeferred, "include-deferred", "", "", "If set to 'enable', deferred tasks are considered when calculating open pool slots. Required for Airflow 3+ Deployments. Possible values are disable/enable.")

	return cmd
}

//nolint:dupl // the duplication is acceptable here
func newDeploymentPoolCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy Airflow pools from one Astro Deployment to another",
		Long:    "Copy Airflow pools from one Astro Deployment to another. If a pool with the same name already exists in the target Deployment, that pool is updated.",
		Example: `
  $ astro deployment pool copy --source-id <source-deployment-id> --target-id <target-deployment-id>
  $ astro deployment pool copy --source-name my-source-deployment --target-name my-target-deployment
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy Airflow pools from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy Airflow pools from.")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied Airflow pools.")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied Airflow pools.")

	return cmd
}

func newDeploymentPoolDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Delete an Airflow pool from an Astro Deployment",
		Long:    "Delete an Airflow pool from a Deployment. Airflow's default_pool cannot be deleted.",
		Example: `
  $ astro deployment pool delete --deployment-id <deployment-id> --name my-pool
  $ astro deployment pool delete --deployment-name my-deployment --name my-pool --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolDelete(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "Name of the pool. Required.")
	cmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Delete the pool without asking for confirmation.")

	return cmd
}

func deploymentConnectionList(cmd *cobra.Command, out io.Writer) error {
	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.ConnectionList(airflowURL, airflowAPIClient, out)
}

func deploymentConnectionCreate(cmd *cobra.Command, out io.Writer) error {
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}
	if connType == "" {
		return errors.New("a connection type is needed to create a connection. Please use the '--conn-type' flag to specify a connection type")
	}

	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.ConnectionCreate(airflowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

func deploymentConnectionUpdate(cmd *cobra.Command, out io.Writer) error {
	if connID == "" {
		return errors.New("a connection ID is needed to update a connection. Please use the '--conn-id' flag to specify a connection ID")
	}
	if connType == "" {
		return errors.New("a connection type is needed to update a connection. Please use the '--conn-type' flag to specify a connection type")
	}

	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.ConnectionUpdate(airflowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

//nolint:dupl // the duplication is acceptable here
func deploymentConnectionCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get source deployment
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which Deployment should connections be copied from?")
	}
	fromDeployment, err := deployment.GetDeployment(ws, fromDeploymentID, fromDeploymentName, false, nil, astroV1Client)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment")
	}

	fromAirflowURL, err := getAirflowURL(&fromDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment Airflow webserver URL")
	}

	// get target deployment
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which Deployment should receive the connections?")
	}
	toDeployment, err := deployment.GetDeployment(ws, toDeploymentID, toDeploymentName, false, nil, astroV1Client)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment")
	}

	toAirflowURL, err := getAirflowURL(&toDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment Airflow webserver URL")
	}

	fmt.Println(warningConnectionCopyCMD)
	return deployment.CopyConnection(fromAirflowURL, toAirflowURL, airflowAPIClient, out)
}

func deploymentConnectionDelete(cmd *cobra.Command, out io.Writer) error {
	if connID == "" {
		return errors.New("a connection ID is needed to delete a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	requestedDeployment, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.ConnectionDelete(airflowURL, requestedDeployment.Name, connID, forceDelete, airflowAPIClient, out)
}

func deploymentAirflowVariableList(cmd *cobra.Command, out io.Writer) error {
	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.AirflowVariableList(airflowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableCreate(cmd *cobra.Command, out io.Writer) error {
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}
	if varValue == "" {
		return errors.New("a variable value is needed to create an airflow variable. Please use the '--value' flag to specify a value")
	}

	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.VariableCreate(airflowURL, varValue, key, description, airflowAPIClient, out)
}

func deploymentAirflowVariableUpdate(cmd *cobra.Command, out io.Writer) error {
	if key == "" {
		return errors.New("a variable key is needed to update an airflow variable. Please use the '--key' flag to specify a key")
	}
	if varValue == "" {
		return errors.New("a variable value is needed to update an airflow variable. Please use the '--value' flag to specify a value")
	}

	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.VariableUpdate(airflowURL, varValue, key, description, airflowAPIClient, out)
}

//nolint:dupl // the duplication is acceptable here
func deploymentAirflowVariableCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get source deployment
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be copied from?")
	}
	fromDeployment, err := deployment.GetDeployment(ws, fromDeploymentID, fromDeploymentName, false, nil, astroV1Client)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment")
	}

	fromAirflowURL, err := getAirflowURL(&fromDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	// get target deployment
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be pasted to?")
	}
	toDeployment, err := deployment.GetDeployment(ws, toDeploymentID, toDeploymentName, false, nil, astroV1Client)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment")
	}

	toAirflowURL, err := getAirflowURL(&toDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the target deployments airflow webserver URL")
	}

	fmt.Println(warningVariableCopyCMD)
	return deployment.CopyVariable(fromAirflowURL, toAirflowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableDelete(cmd *cobra.Command, out io.Writer) error {
	if key == "" {
		return errors.New("a variable key is needed to delete an airflow variable. Please use the '--key' flag to specify a key")
	}

	requestedDeployment, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.VariableDelete(airflowURL, requestedDeployment.Name, key, forceDelete, airflowAPIClient, out)
}

func deploymentPoolList(cmd *cobra.Command, out io.Writer) error {
	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.PoolList(airflowURL, airflowAPIClient, out)
}

func deploymentPoolCreate(cmd *cobra.Command, out io.Writer) error {
	if name == "" {
		return errors.New("a pool name is needed to create a pool. Please use the '--name' flag to specify a name")
	}
	if !cmd.Flags().Changed("slots") {
		return errors.New("a slot count is needed to create a pool. Please use the '--slots' flag to specify a slot count")
	}

	var includeDeferredValue bool
	switch includeDeferred {
	case enable:
		includeDeferredValue = true
	case disable, "":
		includeDeferredValue = false
	default:
		return errors.New("Invalid --include-deferred value")
	}

	_, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.PoolCreate(airflowURL, name, description, slots, includeDeferredValue, airflowAPIClient, out)
}

func deploymentPoolUpdate(cmd *cobra.Command, out io.Writer) error {
	if name == "" {
		return errors.New("a pool name is needed to update a pool. Please use the '--name' flag to specify a name")
	}

	var includeDeferredValue bool
	switch includeDeferred {
	case enable:
		includeDeferredValue = true
	case disable, "":
		includeDeferredValue = false
	default:
		return errors.New("Invalid --include-deferred value")
	}

	requestedDeployment, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	if airflowversions.AirflowMajorVersionForRuntimeVersion(requestedDeployment.RuntimeVersion) >= "3" && includeDeferred == "" {
		return errors.New("an include deferred value is needed to update a pool. Please use the '--include-deferred' flag to specify a value")
	}

	return deployment.PoolUpdate(airflowURL, name, description, slots, includeDeferredValue, airflowAPIClient, out)
}

func deploymentPoolCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	// get source deployment
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should pools be copied from?")
	}
	fromDeployment, err := deployment.GetDeployment(ws, fromDeploymentID, fromDeploymentName, false, nil, astroV1Client)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment")
	}

	fromAirflowURL, err := getAirflowURL(&fromDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	// get target deployment
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should pools be pasted to?")
	}
	toDeployment, err := deployment.GetDeployment(ws, toDeploymentID, toDeploymentName, false, nil, astroV1Client)
	if err != nil {
		return errors.Wrap(err, "failed to find the target Deployment")
	}

	toAirflowURL, err := getAirflowURL(&toDeployment)
	if err != nil {
		return errors.Wrap(err, "failed to find the target deployments airflow webserver URL")
	}

	return deployment.CopyPool(fromAirflowURL, toAirflowURL, airflowAPIClient, out)
}

func deploymentPoolDelete(cmd *cobra.Command, out io.Writer) error {
	if name == "" {
		return errors.New("a pool name is needed to delete a pool. Please use the '--name' flag to specify a name")
	}
	if name == airflowclient.DefaultPoolName {
		return errors.New("the default_pool cannot be deleted. Use 'astro deployment pool update' to change its slots instead")
	}

	requestedDeployment, airflowURL, err := resolveDeploymentAirflowURL(cmd)
	if err != nil {
		return err
	}

	return deployment.PoolDelete(airflowURL, requestedDeployment.Name, name, forceDelete, airflowAPIClient, out)
}

func resolveDeploymentAirflowURL(cmd *cobra.Command) (astrov1.Deployment, string, error) {
	ws, err := coalesceWorkspace()
	if err != nil {
		return astrov1.Deployment{}, "", errors.Wrap(err, "failed to find a valid workspace")
	}

	cmd.SilenceUsage = true

	requestedDeployment, err := deployment.GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, "", err
	}

	airflowURL, err := getAirflowURL(&requestedDeployment)
	if err != nil {
		return astrov1.Deployment{}, "", err
	}
	return requestedDeployment, airflowURL, nil
}

func getAirflowURL(depl *astrov1.Deployment) (string, error) {
	value, err := inspect.ReturnSpecifiedValue(depl, webserverURLField, astroV1Client)
	if err != nil {
		return "", errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airflowURL := fmt.Sprintf("%v", value)
	splitAirflowURL := strings.Split(airflowURL, "?")[0]

	return splitAirflowURL, nil
}
