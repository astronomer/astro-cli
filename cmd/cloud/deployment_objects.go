package cloud

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
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
)

const (
	requestString            = "metadata.webserver_url"
	warningConnectionCopyCMD = "WARNING! The password and extra field are not copied over. You will need to manually add these values"
	warningVariableCopyCMD   = "WARNING! Secret values are not copied over. You will need to manually add these values"
)

func newDeploymentConnectionRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connection",
		Aliases: []string{"con", "connections"},
		Short:   "Manage deployment connections",
		Long:    "Manage connections for an Astro Deployment.",
	}
	cmd.AddCommand(
		newDeploymentConnectionListCmd(out),
		newDeploymentConnectionCreateCmd(out),
		newDeploymentConnectionUpdateCmd(out),
		newDeploymentConnectionCopyCmd(out),
	)
	return cmd
}

func newDeploymentConnectionListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "list a Deployment's connections",
		Long:    "list connections for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create connections for a Deployment",
		Long:    "Create Airflow connections for an Astro Deployment",
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
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "The extra field configuration, defined as a stringified JSON object.")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update connections for a Deployment",
		Long:    "Update existing Airflow connections for an Astro Deployment",
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
	cmd.Flags().StringVarP(&extra, "extra", "e", "", "The extra field configuration, defined as a stringified JSON object.")

	return cmd
}

//nolint:dupl
func newDeploymentConnectionCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy connections from one Deployment to another",
		Long:    "Copy Airflow connections from one Astro Deployment to another. Passwords and extra configurations will not copy over. If a connection already exits with same connection ID in the target Deployment, that connection will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentConnectionCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy connections from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy connections from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied connections")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied connections")

	return cmd
}

func newDeploymentAirflowVariableRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow-variable",
		Aliases: []string{"var"},
		Short:   "Manage Airflow variables in an Astro Deployment",
		Long:    "Manage Airflow variables stored in an Astro Deployment's metadata database.",
	}
	cmd.AddCommand(
		newDeploymentAirflowVariableListCmd(out),
		newDeploymentAirflowVariableCreateCmd(out),
		newDeploymentAirflowVariableUpdateCmd(out),
		newDeploymentAirflowVariableCopyCmd(out),
	)
	return cmd
}

func newDeploymentAirflowVariableListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "list a Deployment's Airflow variables",
		Long:    "list Airflow variables stored in an Astro Deployment's metadata database",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl
func newDeploymentAirflowVariableCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create Airflow variables for a Deployment",
		Long:    "Create Airflow variables for an Astro Deployment",
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

//nolint:dupl
func newDeploymentAirflowVariableUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update Airflow variables for a Deployment",
		Long:    "Update Airflow variables for an Astro Deployment",
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

//nolint:dupl
func newDeploymentAirflowVariableCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy the Airflow variables from one Deployment to another",
		Long:    "Copy Airflow variables from one Astro Deployment to another Astro Deployment. If a variable already exits with same Key it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentAirflowVariableCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy Airflow variables from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy Airflow variables from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied Airflow variables")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied Airflow variables")

	return cmd
}

func newDeploymentPoolRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pool",
		Aliases: []string{"pl", "pools"},
		Short:   "Manage Deployment's Airflow pools",
		Long:    "Manage Airflow pools stored for an Astro Deployment",
	}
	cmd.AddCommand(
		newDeploymentPoolListCmd(out),
		newDeploymentPoolCreateCmd(out),
		newDeploymentPoolUpdateCmd(out),
		newDeploymentPoolCopyCmd(out),
	)
	return cmd
}

func newDeploymentPoolListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"li"},
		Short:   "list a Deployment's Airflow pools",
		Long:    "list Airflow pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolList(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")

	return cmd
}

//nolint:dupl
func newDeploymentPoolCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create Airflow pools for an Astro Deployment",
		Long:    "Create Airflow pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "The Airflow pool value. Required")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "The Airflow pool key. Required")
	cmd.Flags().StringVarP(&description, "description", "", "", "The Airflow pool description")

	return cmd
}

//nolint:dupl
func newDeploymentPoolUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"up"},
		Short:   "Update Airflow pools for an Astro Deployment",
		Long:    "Update Airflow pools for an Astro Deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolUpdate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&name, "name", "", "", "The pool value.  Required.")
	cmd.Flags().IntVarP(&slots, "slots", "s", 0, "The pool slots.")
	cmd.Flags().StringVarP(&description, "description", "", "", "The pool description.")

	return cmd
}

//nolint:dupl
func newDeploymentPoolCopyCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "copy",
		Aliases: []string{"cp"},
		Short:   "Copy Airflow pools from one Astro Deployment to another",
		Long:    "Copy Airflow pools from one Astro Deployment to another Astro Deployment. If a pool already exits with same name it will be updated",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPoolCopy(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&fromDeploymentID, "source-id", "s", "", "The ID of the Deployment to copy Airflow pools from.")
	cmd.Flags().StringVarP(&fromDeploymentName, "source-name", "n", "", "The name of the Deployment to copy Airflow pools from")
	cmd.Flags().StringVarP(&toDeploymentID, "target-id", "t", "", "The ID of the Deployment to receive the copied Airflow pools")
	cmd.Flags().StringVarP(&toDeploymentName, "target-name", "", "", "The name of the Deployment to receive the copied Airflow pools")

	return cmd
}

func deploymentConnectionList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the Deployment Airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	return deployment.ConnectionList(airlfowURL, airflowAPIClient, out)
}

func deploymentConnectionCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the Deployment Airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check connID and connType
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	if connType == "" {
		return errors.New("a connection type is needed to create a connection. Please use the '--conn-type' flag to specify a connection type")
	}

	return deployment.ConnectionCreate(airlfowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

func deploymentConnectionUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the Deployment Airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check connID and connType
	if connID == "" {
		return errors.New("a connection ID is needed to create a connection. Please use the '--conn-id' flag to specify a connection ID")
	}

	return deployment.ConnectionUpdate(airlfowURL, connID, connType, description, host, login, password, schema, extra, port, airflowAPIClient, out)
}

//nolint:dupl
func deploymentConnectionCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get To Airflow URls
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which Deployment should connections be copied from?")
	}
	fromValue, err := inspect.ReturnSpecifiedValue(ws, fromDeploymentName, fromDeploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the source Deployment Airflow webserver URL")
	}

	fromAirlfowURL := fmt.Sprintf("%v", fromValue)
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which Deployment should receive the connections?")
	}
	value, err := inspect.ReturnSpecifiedValue(ws, toDeploymentName, toDeploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the Deployment Airflow webserver URL")
	}

	toAirlfowURL := fmt.Sprintf("%v", value)
	fmt.Println(warningConnectionCopyCMD)
	return deployment.CopyConnection(fromAirlfowURL, toAirlfowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid Workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	return deployment.AirflowVariableList(airlfowURL, airflowAPIClient, out)
}

func deploymentAirflowVariableCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check key and varValue
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}

	if varValue == "" {
		return errors.New("a variable value is needed to create an airflow variable. Please use the '--value' flag to specify a value")
	}

	return deployment.VariableCreate(airlfowURL, varValue, key, description, airflowAPIClient, out)
}

func deploymentAirflowVariableUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check key
	if key == "" {
		return errors.New("a variable key is needed to create an airflow variable. Please use the '--key' flag to specify a key")
	}

	return deployment.VariableUpdate(airlfowURL, varValue, key, description, airflowAPIClient, out)
}

//nolint:dupl
func deploymentAirflowVariableCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get To Airflow URls
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be copied from?")
	}
	fromValue, err := inspect.ReturnSpecifiedValue(ws, fromDeploymentName, fromDeploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	fromAirlfowURL := fmt.Sprintf("%v", fromValue)
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should airflow variables be pasted to?")
	}
	value, err := inspect.ReturnSpecifiedValue(ws, toDeploymentName, toDeploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	toAirlfowURL := fmt.Sprintf("%v", value)

	fmt.Println(warningVariableCopyCMD)
	return deployment.CopyVariable(fromAirlfowURL, toAirlfowURL, airflowAPIClient, out)
}

func deploymentPoolList(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	return deployment.PoolList(airlfowURL, airflowAPIClient, out)
}

func deploymentPoolCreate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check name
	if name == "" {
		return errors.New("a pool name is needed to create a pool. Please use the '--name' flag to specify a name")
	}

	return deployment.PoolCreate(airlfowURL, name, description, slots, airflowAPIClient, out)
}

func deploymentPoolUpdate(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get Airflow URl
	value, err := inspect.ReturnSpecifiedValue(ws, deploymentName, deploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	airlfowURL := fmt.Sprintf("%v", value)

	// check key
	if name == "" {
		return errors.New("a pool name is needed to update a pool. Please use the '--name' flag to specify a name")
	}

	return deployment.PoolUpdate(airlfowURL, name, description, slots, airflowAPIClient, out)
}

//nolint:dupl
func deploymentPoolCopy(cmd *cobra.Command, out io.Writer) error {
	ws, err := coalesceWorkspace()
	if err != nil {
		return errors.Wrap(err, "failed to find a valid workspace")
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	requestedField = requestString
	// get To Airflow URls
	if fromDeploymentName == "" && fromDeploymentID == "" {
		fmt.Println("Which deployment should pools be copied from?")
	}
	fromValue, err := inspect.ReturnSpecifiedValue(ws, fromDeploymentName, fromDeploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find the source deployments airflow webserver URL")
	}

	fromAirlfowURL := fmt.Sprintf("%v", fromValue)
	if toDeploymentName == "" && toDeploymentID == "" {
		fmt.Println("Which deployment should pools be pasted to?")
	}
	value, err := inspect.ReturnSpecifiedValue(ws, toDeploymentName, toDeploymentID, platformCoreClient, astroCoreClient, requestedField)
	if err != nil {
		return errors.Wrap(err, "failed to find a deployments airflow webserver URL")
	}

	toAirlfowURL := fmt.Sprintf("%v", value)

	return deployment.CopyPool(fromAirlfowURL, toAirlfowURL, airflowAPIClient, out)
}
